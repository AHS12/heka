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

	"github.com/gen2brain/beeep"

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
	heartbeatInterval    = 5 * time.Second
	syncInterval         = 2 * time.Second
	defaultReconcileMins = 2 // missed-run watchdog cadence (fast catch-up)
	minReconcileMins     = 2
	maxReconcileMins     = 10
	minWatchdogMins      = 1 // OS watchdog task cadence bounds
	maxWatchdogMins      = 60
	shutdownQuietSecs    = 10 // max wait for in-flight groups at shutdown
)

// Daemon wires the daemon's subsystems together.
type Daemon struct {
	cfg       config.Config
	db        *db.DB
	exec      *executor.Executor
	scheduler *scheduler.Scheduler
	backup    *BackupManager
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

// Resume unfreezes the scheduler and immediately catches up anything missed
// while paused, instead of waiting for the next periodic reconcile tick.
func (d *Daemon) Resume() {
	d.scheduler.Resume()
	_ = d.db.KV().Delete("scheduler_paused")
	go d.reconcile("resume")
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

	// Bind IPC before starting the core (same order as Run): a losing daemon
	// at login (startup entry racing the first watchdog tick) then exits
	// without ever running its scheduler. The successful bind doubles as the
	// singleton lock (SPEC-06 §1).
	ln, err := ipc.Listen(cfg)
	if err != nil {
		return fmt.Errorf("bind IPC endpoint: %w", err)
	}
	defer ln.Close()

	d, handler, err := startCore(cfg, version, database)
	if err != nil {
		return err
	}

	// IPC server in background (tray is the main-thread owner).
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(ln) }()

	// The tray owns the main thread and nothing else watches d.shutdown —
	// without this, `heka daemon stop` (IPC shutdown) is silently ignored by
	// tray daemons. Relay the signal into a tray quit; RunTray then returns
	// and the graceful shutdown below completes.
	go func() {
		<-d.shutdown
		osapp.QuitTray()
	}()

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
	beeep.AppName = "Heka"
	notifier := notify.New(
		notify.WithDesktop(func(title, message string) error {
			return beeep.Notify(title, message, "")
		}),
		notify.WithResolver(resolver),
		notify.WithLogger(func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}),
		notify.WithSoundResolver(func(eventType string) string {
			key := "sound_" + eventType
			if v, ok, _ := database.KV().Get(key); ok && v != "" {
				return v
			}
			return "system"
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
		notifier.NotifyTaskResult(notify.TaskResult{
			Task:     &t,
			Status:   r.FinalStatus,
			Trigger:  r.Trigger,
			Duration: r.Duration,
			ExitCode: r.ExitCode,
		})
	})

	// Backup manager (Settings → Backup): archive jobs + schedule loop.
	d.backup = newBackupManager(cfg, database, version, resolver)

	// Scheduler (SPEC-09).
	d.scheduler = scheduler.New(database, d.exec)
	if err := d.scheduler.Sync(); err != nil {
		return nil, nil, fmt.Errorf("load schedules: %w", err)
	}
	// Restore persisted pause state (SPEC-15 §2).
	if v, _, _ := database.KV().Get("scheduler_paused"); v == "true" {
		d.scheduler.Pause()
		d.logf("info", "scheduler", "scheduler restored as paused; resuming reconciles the backlog")
	}
	d.scheduler.Start(d.execCtx)
	go d.reconcile("startup")

	d.noteStartup()
	go d.heartbeatLoop()
	go d.syncLoop()
	go d.retentionLoop()
	go d.reconcileLoop()
	go d.backup.Loop(d.shutdown)

	handler := ipc.NewServer(ipc.Deps{
		Health:        d.health,
		Tasks:         database.Tasks(),
		Runs:          database.Runs(),
		Schedules:     database.Schedules(),
		Logs:          database.Logs(),
		SyncSchedules: d.scheduler.Sync,
		Reconcile: func() error {
			err := d.scheduler.ReconcileWithReason("manual")
			d.backup.Reconcile("manual")
			return err
		},
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
		PreviewSound:   func(preset string) error { return notify.PlaySound(preset, preset) },

		GetBackupConfig:        d.backup.getConfig,
		UpdateBackupConfig:     d.backup.updateConfig,
		RunBackup:              d.backup.runNow,
		BackupStatus:           d.backup.status,
		BackupHistory:          d.backup.history,
		TestBackupDestinations: d.backup.test,
		SecretsUsage:           func() (map[string][]string, error) { return secretsUsage(database) },
	}).Handler()

	return d, handler, nil
}

// noteStartup records daemon identity in kv (SPEC-06 §3) and reconciles OS
// registration (watchdog + startup) with the currently running binary so an
// upgrade never leaves entries pointing at a deleted install path. The
// watchdog task's cadence is also reconciled with the configured interval.
func (d *Daemon) noteStartup() {
	p := strconv.Itoa(os.Getpid())
	_ = d.db.KV().Set("daemon_pid", p)
	_ = d.db.KV().Set("daemon_version", d.version)
	_ = d.db.KV().Set("heartbeat", db.Now())
	d.logf("info", "daemon", "daemon started (version %s, pid %s)", d.version, p)
	osapp.RepairEntries()
	if err := d.applyWatchdogTask(); err != nil {
		fmt.Fprintf(os.Stderr, "heka: reconcile watchdog interval: %v\n", err)
	}
}

// logf writes a daemon event to the daemon log table (surfaced in the GUI
// Logs → System view) and stderr. Best-effort: logging must never break the
// operation it is reporting.
func (d *Daemon) logf(level, event, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "heka: %s\n", msg)
	_ = d.db.Logs().Add(level, event, msg)
}

// reconcile runs one missed-work pass with the caller's reason recorded in
// the daemon log ("startup", "wake", "resume", "manual", "periodic"): both
// task schedules and the backup schedule catch up through the same pipeline.
// Fire-and-forget by design — failures land in the log.
func (d *Daemon) reconcile(reason string) {
	if err := d.scheduler.ReconcileWithReason(reason); err != nil {
		d.logf("warn", "reconcile", "reconcile (%s) failed: %v", reason, err)
	}
	d.backup.Reconcile(reason)
}

// heartbeatLoop keeps the heartbeat fresh while running. A gap between ticks
// that greatly exceeds the tick interval is the signature of host sleep or
// hibernate — cron ticks die with the process's timers, so on wake we
// reconcile immediately instead of waiting for the next periodic pass.
func (d *Daemon) heartbeatLoop() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	last := time.Now()
	for {
		select {
		case now := <-ticker.C:
			if overslept(last, now) {
				d.logf("info", "daemon", "wake from sleep detected — reconciling missed runs")
				d.reconcile("wake")
			}
			last = now
			_ = d.db.KV().Set("heartbeat", db.Now())
		case <-d.shutdown:
			return
		}
	}
}

// overslept reports whether the gap between consecutive heartbeat ticks
// greatly exceeds the interval (host slept between ticks).
func overslept(last, now time.Time) bool {
	return now.Sub(last) > 2*heartbeatInterval
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

// reconcileLoop periodically catches missed schedule activations while the
// daemon was up but idle (PC asleep, app backgrounded, clock drift). The cron
// engine ticks to wall-clock seconds; if the host was off (laptop sleep, sleep
// + hibernate), nothing fires until the next cron time after wake. This loop
// runs Reconcile on a cadence read from KV each tick so the user can tune it
// from the Reliability section without a daemon restart. Window accounting in
// Reconcile keeps the watchdog from double-firing: once a window closes, the
// next loop iteration finds nothing to do until the daemon itself was down.
func (d *Daemon) reconcileLoop() {
	current := d.reconcileInterval()
	ticker := time.NewTicker(current)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if d.scheduler.IsPaused() {
				continue
			}
			d.reconcile("periodic")
			// Re-read interval: a KV change shrinks/extends the next tick.
			if next := d.reconcileInterval(); next != current {
				current = next
				ticker.Reset(current)
			}
		case <-d.shutdown:
			return
		}
	}
}

// reconcileInterval reads the user-configured missed-run watchdog cadence.
// Clamped to [minReconcileMins, maxReconcileMins] minutes so a bad KV value
// can't make the daemon hot-loop or freeze changes.
func (d *Daemon) reconcileInterval() time.Duration {
	mins := defaultReconcileMins
	if v, ok, _ := d.db.KV().Get("reconcile_interval_min"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			mins = n
		}
	}
	return time.Duration(clampInt(mins, minReconcileMins, maxReconcileMins)) * time.Minute
}

// watchdogInterval reads the user-configured OS watchdog task cadence
// (minutes), clamped to [minWatchdogMins, maxWatchdogMins]. The default is
// osapp.DefaultWatchdogInterval.
func (d *Daemon) watchdogInterval() int {
	mins := int(osapp.DefaultWatchdogInterval / time.Minute)
	if v, ok, _ := d.db.KV().Get("watchdog_interval_min"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			mins = n
		}
	}
	return clampInt(mins, minWatchdogMins, maxWatchdogMins)
}

// applyWatchdogTask reconciles the OS watchdog scheduled task with the
// configured interval — used after a settings change and at daemon startup.
// No-op when the watchdog isn't installed (the installer already reports
// permission problems with actionable text).
func (d *Daemon) applyWatchdogTask() error {
	installer := osapp.NewInstaller()
	installed, interval, err := installer.Status()
	if err != nil || !installed {
		return nil
	}
	if int(interval.Minutes()) == d.watchdogInterval() {
		return nil
	}
	exe, err := osapp.GUIExecutable()
	if err != nil {
		return nil
	}
	if err := installer.Install(time.Duration(d.watchdogInterval())*time.Minute, exe); err != nil {
		return fmt.Errorf("watchdog task not updated: %w", err)
	}
	return nil
}

// clampInt confines v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (d *Daemon) prune() {
	cutoff := time.Now().UTC().AddDate(0, 0, -d.cfg.LogRetentionDays)
	if err := d.db.Runs().Prune(cutoff); err != nil {
		fmt.Fprintf(os.Stderr, "heka: prune runs: %v\n", err)
	}
	if err := d.db.Logs().Prune(cutoff); err != nil {
		fmt.Fprintf(os.Stderr, "heka: prune daemon log: %v\n", err)
	}
}

func (d *Daemon) getSettings() ipc.SettingsDTO {
	days := d.cfg.LogRetentionDays
	if v, ok, _ := d.db.KV().Get("log_retention_days"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	soundSuccess, _, _ := d.db.KV().Get("sound_success")
	soundFailure, _, _ := d.db.KV().Get("sound_failure")
	soundTimeout, _, _ := d.db.KV().Get("sound_timeout")
	if soundSuccess == "" {
		soundSuccess = "system"
	}
	if soundFailure == "" {
		soundFailure = "system"
	}
	if soundTimeout == "" {
		soundTimeout = "system"
	}
	return ipc.SettingsDTO{
		LogRetentionDays:     days,
		SoundSuccess:         soundSuccess,
		SoundFailure:         soundFailure,
		SoundTimeout:         soundTimeout,
		ReconcileIntervalMin: int(d.reconcileInterval() / time.Minute),
		WatchdogIntervalMin:  d.watchdogInterval(),
	}
}

func (d *Daemon) updateSettings(s ipc.SettingsDTO) error {
	if s.LogRetentionDays <= 0 {
		return fmt.Errorf("log_retention_days must be > 0")
	}
	if s.ReconcileIntervalMin < minReconcileMins || s.ReconcileIntervalMin > maxReconcileMins {
		return fmt.Errorf(
			"reconcile_interval_min must be between %d and %d",
			minReconcileMins, maxReconcileMins,
		)
	}
	// WatchdogIntervalMin 0 = "not provided" (older clients) — keep current.
	if s.WatchdogIntervalMin != 0 &&
		(s.WatchdogIntervalMin < minWatchdogMins || s.WatchdogIntervalMin > maxWatchdogMins) {
		return fmt.Errorf(
			"watchdog_interval_min must be between %d and %d",
			minWatchdogMins, maxWatchdogMins,
		)
	}
	if err := notify.ValidatePreset(s.SoundSuccess); err != nil {
		return fmt.Errorf("sound_success: %w", err)
	}
	if err := notify.ValidatePreset(s.SoundFailure); err != nil {
		return fmt.Errorf("sound_failure: %w", err)
	}
	if err := notify.ValidatePreset(s.SoundTimeout); err != nil {
		return fmt.Errorf("sound_timeout: %w", err)
	}
	if err := d.db.KV().Set("log_retention_days", strconv.Itoa(s.LogRetentionDays)); err != nil {
		return err
	}
	if err := d.db.KV().Set("sound_success", s.SoundSuccess); err != nil {
		return err
	}
	if err := d.db.KV().Set("sound_failure", s.SoundFailure); err != nil {
		return err
	}
	if err := d.db.KV().Set("sound_timeout", s.SoundTimeout); err != nil {
		return err
	}
	if err := d.db.KV().Set("reconcile_interval_min", strconv.Itoa(s.ReconcileIntervalMin)); err != nil {
		return err
	}
	if s.WatchdogIntervalMin != 0 {
		if err := d.db.KV().Set("watchdog_interval_min", strconv.Itoa(s.WatchdogIntervalMin)); err != nil {
			return err
		}
	}
	d.cfg.LogRetentionDays = s.LogRetentionDays
	// Persisted first; a failed task update surfaces as the settings error,
	// and the next daemon start retries the reconciliation.
	return d.applyWatchdogTask()
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

