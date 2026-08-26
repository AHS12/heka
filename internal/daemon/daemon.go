// Package daemon is the persistent background runtime (SPEC-06). The daemon
// owns the DB, the executor, the scheduler, the tasks index, and health; the
// GUI and CLI are HTTP clients of it through the IPC layer (SPEC-07).
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"heka/internal/config"
	"heka/internal/core/executor"
	"heka/internal/core/scheduler"
	"heka/internal/core/task"
	"heka/internal/db"
	"heka/internal/ipc"
	"heka/internal/notify"
	"heka/internal/osapp"
)

const (
	heartbeatInterval = 5 * time.Second
	syncInterval      = 2 * time.Second
	shutdownQuietSecs = 10 // max wait for in-flight groups at shutdown
)

// Daemon wires the daemon's subsystems together.
type Daemon struct {
	cfg       config.Config
	db        *db.DB
	exec      *executor.Executor
	scheduler *scheduler.Scheduler
	version   string

	execCtx    context.Context
	cancelExec context.CancelFunc

	startedAt time.Time
	shutdown  chan struct{}
	once      sync.Once
}

// Pause halts the scheduler; recurring and one-time ticks are silently
// skipped. The flag is persisted in KV so it survives daemon restarts.
func (d *Daemon) Pause() {
	d.scheduler.Pause()
	_ = d.db.KV().Set("scheduler_paused", "true")
}

// Resume unfreezes the scheduler.
func (d *Daemon) Resume() {
	d.scheduler.Resume()
	_ = d.db.KV().Delete("scheduler_paused")
}

// SchedulerPaused reports whether the scheduler is currently paused.
func (d *Daemon) SchedulerPaused() bool {
	return d.scheduler.IsPaused()
}

func newDaemon(cfg config.Config, version string, database *db.DB) *Daemon {
	execCtx, cancel := context.WithCancel(context.Background())
	return &Daemon{
		cfg:        cfg,
		db:         database,
		version:    version,
		execCtx:    execCtx,
		cancelExec: cancel,
		startedAt:  time.Now(),
		shutdown:   make(chan struct{}),
	}
}

// Run is the daemon's main: open DB → bind endpoint (singleton) → start
// executor/scheduler/heartbeat/tasks-sync → serve the IPC API until shutdown →
// graceful exit. It returns nil after a clean shutdown.
func Run(cfg config.Config, version string) error {
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	ln, err := ipc.Listen(cfg)
	if err != nil {
		return fmt.Errorf("bind IPC endpoint: %w", err)
	}
	defer ln.Close()

	d, handler, err := startCore(cfg, version, database)
	if err != nil {
		return err
	}

	server := &http.Server{Handler: handler}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(ln) }()

	select {
	case <-d.shutdown:
	case err := <-serveErr:
		if err != nil {
			return err
		}
	}

	_ = server.Close()
	d.shutdownAll()
	return nil
}

// RunTray starts the daemon with a system tray (SPEC-15 §1). The core runs
// in a goroutine; the main thread is handed to systray.Run which blocks until
// Quit is clicked.
func RunTray(cfg config.Config, version string) error {
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	d, handler, err := startCore(cfg, version, database)
	if err != nil {
		return err
	}

	// IPC server in background (tray is the main-thread owner).
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(mustListen(cfg)) }()

	// Tray blocks the main thread; core is already running in goroutines.
	osapp.RunTray(osapp.TrayDeps{
		Cfg:        cfg,
		DB:         database,
		Pause:      d.Pause,
		Resume:     d.Resume,
		IsPaused:   d.SchedulerPaused,
		Version:    version,
		OnShutdown: func() { d.once.Do(func() { close(d.shutdown) }) },
	})

	// Tray exited (Quit clicked): graceful shutdown.
	_ = server.Close()
	d.shutdownAll()
	return nil
}

// startCore initialises the executor, scheduler, heartbeat, tasks-sync, and
// wires the IPC handler. Returns the daemon (for Pause/Resume/health) and the
// HTTP handler. Both Run and RunTray delegate here.
func startCore(cfg config.Config, version string, database *db.DB) (*Daemon, http.Handler, error) {
	// Daemon process env first, then the secret store (SPEC-11 §4).
	resolver := func(name string) (string, bool) {
		if v, ok := os.LookupEnv(name); ok {
			return v, true
		}
		v, ok, _ := database.Secrets().Get(name)
		return v, ok
	}
	d := newDaemon(cfg, version, database)
	d.exec = executor.New(database, cfg.MaxOutputBytes, 5*time.Second, resolver, cfg.RunArtifactsDir)

	// Notifications (SPEC-11).
	notifier := notify.New(
		notify.WithResolver(resolver),
		notify.WithLogger(func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}),
	)
	d.exec.OnGroupFinished(func(r executor.GroupResult) {
		row, err := database.Tasks().Get(r.TaskSlug)
		if err != nil {
			return
		}
		var t task.Task
		if err := json.Unmarshal([]byte(row.ParsedJSON), &t); err != nil {
			return
		}
		notifier.NotifyTaskResult(&t, r.FinalStatus)
	})

	// Scheduler (SPEC-09).
	d.scheduler = scheduler.New(database, d.exec)
	if err := d.scheduler.Sync(); err != nil {
		return nil, nil, fmt.Errorf("load schedules: %w", err)
	}
	// Restore persisted pause state (SPEC-15 §2).
	if v, _, _ := database.KV().Get("scheduler_paused"); v == "true" {
		d.scheduler.Pause()
	}
	d.scheduler.Start(d.execCtx)
	go func() {
		if err := d.scheduler.Reconcile(); err != nil {
			fmt.Fprintf(os.Stderr, "heka: reconcile missed runs: %v\n", err)
		}
	}()

	d.noteStartup()
	go d.heartbeatLoop()
	go d.syncLoop()
	go d.retentionLoop()

	handler := ipc.NewServer(ipc.Deps{
		Health:        d.health,
		Tasks:         database.Tasks(),
		Runs:          database.Runs(),
		Schedules:     database.Schedules(),
		SyncSchedules: d.scheduler.Sync,
		Secrets:       database.Secrets(),
		TaskFiles:     taskFS{dir: cfg.TasksDir},
		SyncTasks:     func() error { d.syncTasks(); return nil },
		Runner:        d.exec,
		Shutdown:      func() { d.once.Do(func() { close(d.shutdown) }) },
		Pause:         d.Pause,
		Resume:        d.Resume,
		IsPaused:      d.SchedulerPaused,
		GetSettings:   d.getSettings,
		UpdateSettings: d.updateSettings,
	}).Handler()

	return d, handler, nil
}

// noteStartup records daemon identity in kv (SPEC-06 §3) and reconciles OS
// registration (watchdog + startup) with the currently running binary so an
// upgrade never leaves entries pointing at a deleted install path.
func (d *Daemon) noteStartup() {
	p := strconv.Itoa(os.Getpid())
	_ = d.db.KV().Set("daemon_pid", p)
	_ = d.db.KV().Set("daemon_version", d.version)
	_ = d.db.KV().Set("heartbeat", db.Now())
	osapp.RepairEntries()
}

// heartbeatLoop keeps the heartbeat fresh while running.
func (d *Daemon) heartbeatLoop() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = d.db.KV().Set("heartbeat", db.Now())
		case <-d.shutdown:
			return
		}
	}
}

// syncLoop polls the tasks directory into the index (SPEC-06 §5).
func (d *Daemon) syncLoop() {
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.syncTasks()
		case <-d.shutdown:
			return
		}
	}
}

// retentionLoop prunes old runs on a nightly schedule (SPEC-16 §3). It also
// runs once at startup to catch anything missed while the daemon was down.
func (d *Daemon) retentionLoop() {
	// Prune once at startup.
	d.prune()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.prune()
		case <-d.shutdown:
			return
		}
	}
}

func (d *Daemon) prune() {
	cutoff := time.Now().UTC().AddDate(0, 0, -d.cfg.LogRetentionDays)
	if err := d.db.Runs().Prune(cutoff); err != nil {
		fmt.Fprintf(os.Stderr, "heka: prune runs: %v\n", err)
	}
}

func (d *Daemon) getSettings() ipc.SettingsDTO {
	days := d.cfg.LogRetentionDays
	if v, ok, _ := d.db.KV().Get("log_retention_days"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	return ipc.SettingsDTO{LogRetentionDays: days}
}

func (d *Daemon) updateSettings(s ipc.SettingsDTO) error {
	if s.LogRetentionDays <= 0 {
		return fmt.Errorf("log_retention_days must be > 0")
	}
	if err := d.db.KV().Set("log_retention_days", strconv.Itoa(s.LogRetentionDays)); err != nil {
		return err
	}
	d.cfg.LogRetentionDays = s.LogRetentionDays
	return nil
}

// health assembles the Health snapshot the IPC layer serves.
func (d *Daemon) health() ipc.Health {
	hb, _, _ := d.db.KV().Get("heartbeat")
	parsed, _ := time.Parse(time.RFC3339, hb)
	h := ipc.Health{
		Version:       d.version,
		UptimeSeconds: int64(time.Since(d.startedAt).Seconds()),
		Core:          "healthy",
		Scheduler:     "starting",
		LastHeartbeat: parsed,
	}
	if d.scheduler != nil {
		if d.scheduler.IsPaused() {
			h.Scheduler = "paused"
		} else {
			h.Scheduler = "running"
		}
		if next, taskSlug := d.scheduler.NextRun(); !next.IsZero() {
			h.NextRunAt = next
			h.NextTaskSlug = taskSlug
		}
	}
	return h
}

// shutdownAll is the graceful sequence (SPEC-06 §2): cancel the executor (and
// with it the scheduler), wait for in-flight groups; the deferred closes
// finish the rest.
func (d *Daemon) shutdownAll() {
	d.cancelExec()
	deadline := time.Now().Add(shutdownQuietSecs * time.Second)
	for d.exec.Active() > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
}

// mustListen binds the IPC endpoint or panics. Used by RunTray where the
// listener is started in a goroutine and a fatal error should crash fast.
func mustListen(cfg config.Config) net.Listener {
	ln, err := ipc.Listen(cfg)
	if err != nil {
		panic(fmt.Sprintf("bind IPC endpoint: %v", err))
	}
	return ln
}
