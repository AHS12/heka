// Package daemon is the persistent background runtime (SPEC-06). The daemon
// owns the DB, the executor, the scheduler, the tasks index, and health; the
// GUI and CLI are HTTP clients of it through the IPC layer (SPEC-07).
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Daemon process env first, then the secret store (SPEC-11 §4), for every
	// ${VAR} resolution the executor and notifier perform.
	resolver := func(name string) (string, bool) {
		if v, ok := os.LookupEnv(name); ok {
			return v, true
		}
		v, ok, _ := database.Secrets().Get(name)
		return v, ok
	}
	d := newDaemon(cfg, version, database)
	d.exec = executor.New(database, cfg.MaxOutputBytes, 5*time.Second, resolver, cfg.RunArtifactsDir)

	// Notifications (SPEC-11): fire on group completion, per-task notify_on.
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

	// Scheduler (SPEC-09): load schedules, start ticking, reconcile missed
	// runs while the daemon was down.
	d.scheduler = scheduler.New(database, d.exec)
	if err := d.scheduler.Sync(); err != nil {
		return fmt.Errorf("load schedules: %w", err)
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
	}).Handler()

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

	// Graceful: stop accepting, drain in-flight groups, then close.
	_ = server.Close()
	d.shutdownAll()
	return nil
}

// noteStartup records daemon identity in kv (SPEC-06 §3).
func (d *Daemon) noteStartup() {
	p := strconv.Itoa(os.Getpid())
	_ = d.db.KV().Set("daemon_pid", p)
	_ = d.db.KV().Set("daemon_version", d.version)
	_ = d.db.KV().Set("heartbeat", db.Now())
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
		h.Scheduler = "running"
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
