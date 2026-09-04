// Package executor runs Heka tasks as direct processes (SPEC-05).
//
// Execution model: one run group per logical run = sequential attempts (retry
// loop), one process spawn + one runs row per attempt. Different tasks run
// concurrently; the same task never overlaps itself (D11).
//
// The executor never uses a shell (PRD §22) and knows nothing about
// notifications, scheduling, or the UI — it only spawns, captures, and
// records.
package executor

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"heka/internal/core/task"
	"heka/internal/db"
)

// ErrAlreadyRunning is returned by Start when the task's slug lock is held
// (D11). Callers map it to a 409 conflict (SPEC-07) or a skipped tick
// (SPEC-09).
var ErrAlreadyRunning = errors.New("executor: task is already running")

// ErrNotRunning is returned by Cancel when the slug has no active group.
var ErrNotRunning = errors.New("executor: no active run")

// EnvResolver looks up a variable for ${VAR} resolution. The production
// implementation checks the daemon process env first, then the secret store
// (SPEC-11).
type EnvResolver func(name string) (string, bool)

// Options configure one logical run.
type Options struct {
	// Trigger records why the run started: manual|schedule|cli|system.
	Trigger string
	// ScheduleID is the owning schedule row, if this was schedule-triggered.
	ScheduleID string
	// BaseDir is the task file's directory; relative script/command/working
	// paths resolve against it (SPEC-04 §3, master spec §26).
	BaseDir string
}

// Handle is the client view of a running group.
type Handle struct {
	GroupID string
	Done    <-chan struct{} // closes when the group finishes

	cancel     context.CancelFunc
	cancelOnce sync.Once
	done       chan struct{}

	mu  sync.Mutex
	err error
}

// Cancel requests graceful termination, escalating to force-kill after the
// grace period; retries are aborted. Safe to call repeatedly.
func (h *Handle) Cancel() {
	h.cancelOnce.Do(h.cancel)
}

// Err returns the group-level error (DB failures etc.), valid after Done.
func (h *Handle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

// GroupResult is the group-completion summary (SPEC-11 §3): daemon-side
// consumers (notifications) read trigger, final status, duration, and exit code.
type GroupResult struct {
	GroupID     string
	TaskSlug    string
	Trigger     string // manual|schedule|cli|system
	FinalStatus string // success|failed|timed_out|cancelled
	Duration    time.Duration
	ExitCode    int
}

// Executor owns the process execution engine.
type Executor struct {
	db        *db.DB
	maxOutput int64
	grace     time.Duration
	env       EnvResolver

	// artifactsDir is the per-run capture root (config run_artifacts_dir):
	// each group writes <dir>/<group>/stdout.log, stderr.log and run.json.
	// Empty disables file capture — output still lands in the runs DB.
	artifactsDir string

	mu              sync.Mutex
	running         map[string]*Handle // slug → active group
	onGroupFinished func(GroupResult)
}

// OnGroupFinished registers a callback fired once per group after the final
// attempt's row is written and the slug lock released (SPEC-11 §3). Multiple
// registrations replace the previous one.
func (e *Executor) OnGroupFinished(fn func(GroupResult)) {
	e.mu.Lock()
	e.onGroupFinished = fn
	e.mu.Unlock()
}

// New builds an executor. env resolves ${VAR} refs; pass nil for plain
// pass-through of unresolved values. artifactsDir enables per-run output
// files (<artifactsDir>/<group>/stdout.log | stderr.log | run.json); leave
// empty to capture to the runs DB only.
func New(database *db.DB, maxOutputBytes int64, grace time.Duration, env EnvResolver, artifactsDir string) *Executor {
	if env == nil {
		env = func(string) (string, bool) { return "", false }
	}
	return &Executor{
		db:           database,
		maxOutput:    maxOutputBytes,
		grace:        grace,
		env:          env,
		artifactsDir: artifactsDir,
		running:      map[string]*Handle{},
	}
}

// Start launches a run group for the task and returns immediately after the
// group's attempt 0 begins. ErrAlreadyRunning is returned when the slug's
// lock is held (SPEC-05 §1).
func (e *Executor) Start(ctx context.Context, t *task.Task, opt Options) (*Handle, error) {
	e.mu.Lock()
	if _, busy := e.running[t.Slug]; busy {
		e.mu.Unlock()
		return nil, ErrAlreadyRunning
	}
	groupCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	h := &Handle{
		GroupID: ulid.Make().String(),
		Done:    done,
		cancel:  cancel,
		done:    done,
	}
	e.running[t.Slug] = h
	e.mu.Unlock()

	trigger := opt.Trigger
	if trigger == "" {
		trigger = "manual"
	}

	go e.runGroup(groupCtx, t, opt, trigger, h)
	return h, nil
}

// runGroup drives the retry loop: attempts until success, terminal status, or
// context cancellation; then releases the slug lock, fires the completion
// callback, writes the run manifest, and closes the handle.
func (e *Executor) runGroup(ctx context.Context, t *task.Task, opt Options, trigger string, h *Handle) {
	// Resolve the per-run output directory: the task's output_dir overrides
	// the global artifacts root (config run_artifacts_dir, default
	// <data_dir>/runs); an empty root disables file capture.
	resolved := t.Resolve(opt.BaseDir)
	outDir := e.artifactsDir
	if t.OutputDir != "" {
		outDir = t.OutputDir
		if !filepath.IsAbs(outDir) {
			outDir = filepath.Join(resolved.WorkingDir, outDir)
		}
	}
	ga := e.openGroupArtifactsAt(outDir, h.GroupID)
	if ga != nil {
		defer func() {
			ga.stdout.Close()
			ga.stderr.Close()
		}()
	}
	manifest := runManifest{
		GroupID:   h.GroupID,
		TaskSlug:  t.Slug,
		Trigger:   trigger,
		StartedAt: db.Now(),
	}
	last := attemptResult{status: "cancelled"}
	defer func() {
		e.mu.Lock()
		delete(e.running, t.Slug)
		cb := e.onGroupFinished
		e.mu.Unlock()
		manifest.FinishedAt = db.Now()
		if ga != nil {
			// Best-effort: a manifest write failure never fails the group.
			_ = writeManifest(manifest, outDir, h.GroupID)
		}
		if cb != nil {
			cb(GroupResult{
				GroupID:     h.GroupID,
				TaskSlug:    t.Slug,
				Trigger:     trigger,
				FinalStatus: last.status,
				Duration:    last.duration,
				ExitCode:    last.exitCode,
			})
		}
		close(h.done)
	}()

	maxAttempts := t.Retry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		last = e.runAttempt(ctx, t, opt, trigger, h.GroupID, attempt, ga)
		manifest.Attempts = append(manifest.Attempts, attemptMeta{
			Attempt:    attempt,
			Status:     last.status,
			DurationMs: last.duration.Milliseconds(),
			ExitCode:   last.exitCode,
		})
		if last.err != nil {
			h.setErr(last.err)
			return
		}
		switch last.status {
		case "success", "timed_out", "cancelled":
			return
		}
		// failed: retry if attempts remain, honoring the delay (cancel aborts).
		if attempt+1 >= maxAttempts {
			return
		}
		delay := time.Duration(t.Retry.DelaySeconds) * time.Second
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
	}
}

// Active counts currently running groups. Shutdown sequencing uses it to
// wait for in-flight work to drain (SPEC-06).
func (e *Executor) Active() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.running)
}

// Cancel requests cancellation of the slug's active group (SPEC-07). Returns
// ErrNotRunning when nothing is active.
func (e *Executor) Cancel(slug string) error {
	e.mu.Lock()
	h, ok := e.running[slug]
	e.mu.Unlock()
	if !ok {
		return ErrNotRunning
	}
	h.Cancel()
	return nil
}

func (h *Handle) setErr(err error) {
	h.mu.Lock()
	h.err = err
	h.mu.Unlock()
}
