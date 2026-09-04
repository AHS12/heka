// Package scheduler drives recurring and one-time executions (SPEC-09).
//
// Schedules live only in SQLite (D3); the engine keeps an in-memory registry
// rebuilt by Sync() after every mutation and at daemon start. The cron specs
// accept both 5-field expressions and robfig descriptors (@every/@daily/...),
// which covers every PRD §12 recurrence option.
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/robfig/cron/v3"

	"heka/internal/core/executor"
	"heka/internal/core/task"
	"heka/internal/db"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// specParser is the grammar shared by validation, counting, and the running
// engine: optional seconds, 5 fields, and @-descriptors (@every/@daily/...).
var specParser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// Runner is the execution seam (producer: *executor.Executor).
type Runner interface {
	Start(ctx context.Context, t *task.Task, opt executor.Options) (*executor.Handle, error)
}

// record is what the registry holds for one schedule.
type record struct {
	entryID cron.EntryID // recurring
	timer   *time.Timer  // onetime
	oneShot bool
}

// Scheduler owns recurrence + one-time dispatch.
type Scheduler struct {
	db   *db.DB
	run  Runner
	cron *cron.Cron

	mu      sync.Mutex
	recs    map[string]record
	ctx     context.Context
	paused  bool

	reconcileMu sync.Mutex // serializes Reconcile callers (startup/loop/IPC)
}

// New builds an empty scheduler. Call Sync to load schedules, Start to begin
// ticking.
func New(database *db.DB, run Runner) *Scheduler {
	return &Scheduler{
		db:   database,
		run:  run,
		cron: cron.New(cron.WithParser(specParser)),
		recs: map[string]record{},
		ctx:  context.Background(),
	}
}

// Start begins the cron loop and stops everything when ctx cancels (daemon
// shutdown).
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	s.cron.Start()
	go func() {
		<-ctx.Done()
		s.Stop()
	}()
}

// Pause freezes the scheduler: recurring ticks and one-time jobs are silently
// skipped without writing skipped run rows. next_run_at stops advancing.
// Persisted via the caller (daemon KV) so the flag survives restarts.
func (s *Scheduler) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = true
}

// Resume unfreezes the scheduler; the next cron tick will fire normally.
func (s *Scheduler) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = false
}

// IsPaused reports whether the scheduler is currently paused.
func (s *Scheduler) IsPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// Stop halts the cron loop and all pending one-time timers. Idempotent.
func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.recs {
		if r.timer != nil {
			r.timer.Stop()
		}
	}
}

// Sync rebuilds the registry from the schedules table (SPEC-09 §2): every
// mutation (CRUD/enable/disable) and daemon start call it, so the running
// engine and the DB never disagree for more than one call.
func (s *Scheduler) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.recs {
		s.stopRecordLocked(id)
	}
	s.recs = map[string]record{}

	scheds, err := s.db.Schedules().List()
	if err != nil {
		return err
	}
	for _, sch := range scheds {
		if !sch.Enabled {
			continue
		}
		s.registerLocked(sch)
	}
	return nil
}

func (s *Scheduler) registerLocked(sch db.Schedule) {
	switch sch.Kind {
	case "recurring":
		id, err := s.cron.AddFunc(sch.Cron, func() { s.fire(sch.ID) })
		if err != nil {
			s.logf("warn", "scheduler", "schedule %q: invalid cron: %v", sch.Slug, err)
			return
		}
		s.recs[sch.ID] = record{entryID: id}
		// Derive from the parsed spec: cron entries carry a zero Next until
		// cron.Start initializes them, so Entry(id).Next is unusable here and
		// a startup before Start would leave next_run_at stale forever.
		s.setNextRunTime(sch.ID, nextRunOf(sch.Cron, time.Now()))
	case "onetime":
		delay := time.Until(parseTime(sch.RunAt))
		if delay < 0 {
			delay = 0 // overdue: fire immediately; the engine marks it done
		}
		timer := time.AfterFunc(delay, func() { s.fireOnetime(sch.ID) })
		s.recs[sch.ID] = record{timer: timer, oneShot: true}
		// Persist the planned run time so health shows it even pre-fire.
		s.setNextRunTime(sch.ID, sch.RunAt)
	}
}

func (s *Scheduler) stopRecordLocked(id string) {
	if r, ok := s.recs[id]; ok && r.timer != nil {
		r.timer.Stop()
	}
}

// fire handles a recurring tick (SPEC-09 §2). When the scheduler is paused
// the tick is silently dropped — no skipped run row, next_run_at frozen.
func (s *Scheduler) fire(id string) {
	s.mu.Lock()
	rec, ok := s.recs[id]
	paused := s.paused
	s.mu.Unlock()
	if !ok {
		return
	}
	if paused {
		return
	}
	sch, err := s.db.Schedules().Get(id)
	if err != nil || !sch.Enabled {
		return
	}
	s.dispatch(sch)
	// Advance next_run_at from the cron entry (the fire may lag by ms).
	entry := s.cron.Entry(rec.entryID)
	if !entry.Next.IsZero() {
		s.setNextRunTime(id, runAtOf(entry.Next))
	}
}

// fireOnetime dispatches once and marks the job done (SPEC-09 §2).
// Skipped when the scheduler is paused.
func (s *Scheduler) fireOnetime(id string) {
	s.mu.Lock()
	paused := s.paused
	s.mu.Unlock()
	if paused {
		return
	}
	sch, err := s.db.Schedules().Get(id)
	if err != nil || !sch.Enabled {
		return
	}
	s.dispatch(sch)
	sch.Enabled = false
	sch.LastRunAt = db.Now()
	_ = s.db.Schedules().Save(sch)
	s.mu.Lock()
	s.stopRecordLocked(id)
	delete(s.recs, id)
	s.mu.Unlock()
}

// dispatch executes the schedule's task through the runner (SPEC-09 §1).
func (s *Scheduler) dispatch(sch db.Schedule) {
	row, err := s.db.Tasks().Get(sch.TaskSlug)
	if err != nil {
		s.logf("warn", "scheduler", "schedule %q: task %q not found, cannot fire", sch.Slug, sch.TaskSlug)
		s.setLast(sch.ID, "error", "task not found")
		return
	}
	var t task.Task
	if err := json.Unmarshal([]byte(row.ParsedJSON), &t); err != nil {
		s.logf("warn", "scheduler", "schedule %q: task %q definition corrupt: %v", sch.Slug, sch.TaskSlug, err)
		s.setLast(sch.ID, "error", "task definition corrupt")
		return
	}
	handle, err := s.run.Start(s.context(), &t, executor.Options{
		Trigger:    "schedule",
		ScheduleID: sch.ID,
		BaseDir:    filepath.Dir(row.YAMLPath),
	})
	if errors.Is(err, executor.ErrAlreadyRunning) {
		// Overlap (D11): record the skip, never start a duplicate.
		s.recordHalt(sch, "skipped")
		s.setLast(sch.ID, "skipped", db.Now())
		return
	}
	if err != nil {
		s.logf("warn", "scheduler", "schedule %q: start failed: %v", sch.Slug, err)
		s.setLast(sch.ID, "error", err.Error())
		return
	}
	s.setLast(sch.ID, "running", db.Now())
	go func() {
		<-handle.Done
		s.finishStatus(sch.ID)
	}()
}

// finishStatus copies the group's final status onto the schedule row.
func (s *Scheduler) finishStatus(id string) {
	rows, err := s.db.Runs().ListBySchedule(id, 1)
	if err != nil || len(rows) == 0 {
		return
	}
	at := ""
	if rows[0].FinishedAt != nil {
		at = *rows[0].FinishedAt
	}
	s.setLast(id, rows[0].Status, at)
}

// recordHalt writes a 'skipped'/'missed' row so history explains the gap.
func (s *Scheduler) recordHalt(sch db.Schedule, status string) {
	now := db.Now()
	row := db.Run{
		RunID: ulid.Make().String(), GroupID: ulid.Make().String(),
		Attempt: 0, TaskSlug: sch.TaskSlug, ScheduleID: sch.ID,
		Trigger: "schedule", Status: status,
		StartedAt: &now, FinishedAt: &now, CreatedAt: now,
	}
	if err := s.db.Runs().Create(row); err != nil {
		s.logf("warn", "scheduler", "record %s for %s: %v", status, sch.Slug, err)
	}
}

func (s *Scheduler) setLast(id, status, at string) {
	sch, err := s.db.Schedules().Get(id)
	if err != nil {
		return
	}
	if at != "" {
		sch.LastRunAt = at
	}
	sch.LastStatus = status
	_ = s.db.Schedules().Save(sch)
}

func (s *Scheduler) setNextRunTime(id, next string) {
	sch, err := s.db.Schedules().Get(id)
	if err != nil {
		return
	}
	if next != "" {
		sch.NextRunAt = next
	}
	_ = s.db.Schedules().Save(sch)
}

// NextRun returns the nearest upcoming activation and its task slug (health).
func (s *Scheduler) NextRun() (time.Time, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best time.Time
	var slug string
	for id, r := range s.recs {
		var next time.Time
		if r.oneShot {
			if sch, err := s.db.Schedules().Get(id); err == nil {
				next = parseTime(sch.RunAt)
			}
		} else {
			next = s.cron.Entry(r.entryID).Next
		}
		if next.Before(time.Now()) {
			continue
		}
		if best.IsZero() || next.Before(best) {
			best = next
			if sch, err := s.db.Schedules().Get(id); err == nil {
				slug = sch.TaskSlug
			}
		}
	}
	return best, slug
}

func (s *Scheduler) context() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctx
}

// ValidateSchedule is the SPEC-09 §4 validation used by the IPC create/update
// handlers. It does not touch the DB.
func ValidateSchedule(sch db.Schedule) error {
	if !slugRe.MatchString(sch.Slug) {
		return fmt.Errorf("slug must match lowercase slug format")
	}
	if sch.TaskSlug == "" {
		return fmt.Errorf("task_slug required")
	}
	switch sch.Kind {
	case "recurring":
		if sch.Cron == "" {
			return fmt.Errorf("cron required for recurring schedules")
		}
		if _, err := specParser.Parse(sch.Cron); err != nil {
			return fmt.Errorf("invalid cron %q: %w", sch.Cron, err)
		}
	case "onetime":
		if sch.RunAt == "" {
			return fmt.Errorf("run_at required for one-time jobs")
		}
		if t := parseTime(sch.RunAt); t.Before(time.Now()) {
			return fmt.Errorf("run_at must be in the future")
		}
	default:
		return fmt.Errorf("kind must be \"recurring\" or \"onetime\"")
	}
	if sch.MissedPolicy != "" && sch.MissedPolicy != "skip" && sch.MissedPolicy != "run_now" {
		return fmt.Errorf("missed_policy must be \"skip\" or \"run_now\"")
	}
	return nil
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func runAtOf(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// nextRunOf computes the next activation of a cron spec independently of the
// running engine (usable before cron.Start and after reconcile catch-ups).
// Evaluated in the daemon's local zone to match the live engine (see
// countOccurrences) — empty string for an invalid spec.
func nextRunOf(spec string, from time.Time) string {
	sched, err := specParser.Parse(spec)
	if err != nil {
		return ""
	}
	return runAtOf(sched.Next(from.Local()))
}
