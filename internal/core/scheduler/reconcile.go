package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"heka/internal/core/executor"
	"heka/internal/core/task"
	"heka/internal/db"
)

// Reconcile detects executions missed while the daemon was down or asleep
// (PRD §7.1, SPEC-09 §3): for each enabled recurring schedule, count expected
// occurrences in [last_run_at|created_at, now], subtract the schedule runs
// actually recorded, and apply missed_policy once.
//
// Called at daemon start, on the daemon's periodic watchdog cadence, on wake
// from sleep, after a scheduler resume, and on demand from the CLI / GUI.
// reconcileMu serializes concurrent callers so an overlapping startup +
// periodic + manual reconcile can never double-fire a window; a paused
// scheduler is left untouched so the gap stays open until it resumes.
func (s *Scheduler) Reconcile() error {
	return s.ReconcileWithReason("periodic")
}

// ReconcileWithReason is Reconcile with the caller's trigger recorded in the
// daemon log: "startup", "wake", "resume", "manual", or "periodic". Every
// pass logs its outcome — including the quiet periodic heartbeat — so the
// Logs → System view proves the mechanism is alive (PRD §7.1 visibility).
func (s *Scheduler) ReconcileWithReason(reason string) error {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()

	s.mu.Lock()
	paused := s.paused
	s.mu.Unlock()
	if paused {
		return nil
	}

	now := time.Now()
	scheds, err := s.db.Schedules().List()
	if err != nil {
		return err
	}
	handled := 0
	for _, sch := range scheds {
		if sch.Kind != "recurring" || !sch.Enabled {
			continue
		}
		start := parseTime(sch.LastRunAt)
		if start.IsZero() {
			start = parseTime(sch.CreatedAt)
		}
		expected := countOccurrences(sch.Cron, start, now)
		if expected <= 0 {
			continue
		}

		fired := s.countFired(sch.ID, start)
		missed := expected - fired
		if missed <= 0 {
			continue
		}

		if s.catchUp(sch, missed, start) {
			// Policy applied (or the miss is permanent): close the window so
			// the next pass starts fresh. Transient failures leave it open.
			handled++
			sch.LastRunAt = db.Now()
			// The catch-up fired outside the cron engine, so nothing else
			// advances next_run_at — re-derive it from the spec.
			if next := nextRunOf(sch.Cron, now); next != "" {
				sch.NextRunAt = next
			}
			_ = s.db.Schedules().Save(sch)
		}
	}
	s.logf("info", "reconcile", "reconcile (%s): checked %d schedule(s), %d caught up",
		reason, len(scheds), handled)
	return nil
}

// catchUp applies a schedule's missed_policy for one detected gap and reports
// whether the window may be closed (false = transient failure, retry next
// pass). Every outcome is logged to the daemon log and stderr.
func (s *Scheduler) catchUp(sch db.Schedule, missed int, since time.Time) bool {
	window := since.Format(time.RFC3339)
	switch sch.MissedPolicy {
	case "run_now":
		row, err := s.db.Tasks().Get(sch.TaskSlug)
		if err != nil {
			s.logf("warn", "reconcile",
				"schedule %q: missed %d activation(s) since %s but task %q not found — closing window",
				sch.Slug, missed, window, sch.TaskSlug)
			return true
		}
		var t task.Task
		if err := json.Unmarshal([]byte(row.ParsedJSON), &t); err != nil {
			s.logf("warn", "reconcile",
				"schedule %q: missed %d activation(s) since %s but task %q is corrupt (%v) — closing window",
				sch.Slug, missed, window, sch.TaskSlug, err)
			return true
		}
		handle, err := s.run.Start(s.context(), &t, executor.Options{
			Trigger:    "schedule",
			ScheduleID: sch.ID,
			BaseDir:    filepath.Dir(row.YAMLPath),
		})
		switch {
		case errors.Is(err, executor.ErrAlreadyRunning):
			// Overlap (D11): record the skip so the gap is visible in history.
			s.recordHalt(sch, "skipped")
			s.logf("info", "reconcile",
				"schedule %q: missed %d activation(s) since %s — task already running, recorded skip",
				sch.Slug, missed, window)
		case err != nil:
			s.logf("warn", "reconcile",
				"schedule %q: missed %d activation(s) since %s — start failed (%v), will retry next pass",
				sch.Slug, missed, window, err)
			return false
		default:
			s.logf("info", "reconcile",
				"schedule %q: missed %d activation(s) since %s — started catch-up run (group %s)",
				sch.Slug, missed, window, handle.GroupID)
		}
	default: // "skip"
		// One collapsed 'missed' row; the real gap is visible in history.
		s.recordHalt(sch, "missed")
		s.logf("info", "reconcile",
			"schedule %q: missed %d activation(s) since %s — missed_policy=skip, recorded as missed",
			sch.Slug, missed, window)
	}
	return true
}

// logf writes a daemon event to the daemon log table and stderr. Best-effort:
// logging must never break the operation it is reporting.
func (s *Scheduler) logf(level, event, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "heka: %s\n", msg)
	_ = s.db.Logs().Add(level, event, msg)
}

// countOccurrences walks the cron spec from start to end (newest window
// first) and counts activations. Bounded defensively.
//
// Both bounds are converted to the daemon's local zone before evaluation.
// Stored timestamps are UTC (db.Now), and robfig evaluates a time.Local
// schedule in the zone of the passed-in time — leaving the UTC strings
// would count field-based specs ("00 09 * * *") against UTC ticks while
// the running engine fires local ticks, so a boot between the local tick
// and its UTC twin silently dropped the missed run (v0.8.0 field report).
func countOccurrences(spec string, start, end time.Time) int {
	if end.Before(start) {
		return 0
	}
	sched, err := specParser.Parse(spec)
	if err != nil {
		return 0
	}
	start, end = start.Local(), end.Local()
	count := 0
	for next := sched.Next(start); next.Before(end) && count < 1000; next = sched.Next(next) {
		count++
	}
	return count
}

// countFired is the number of schedule-triggered runs recorded in the window.
func (s *Scheduler) countFired(scheduleID string, since time.Time) int {
	n, err := s.db.Runs().CountBySchedule(scheduleID, since)
	if err != nil {
		return 0
	}
	return n
}
