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

// Reconcile detects executions missed while the daemon was down (PRD §7.1,
// SPEC-09 §3): for each enabled recurring schedule, count expected
// occurrences in [last_run_at|created_at, now], subtract the schedule runs
// actually recorded, and apply missed_policy once.
//
// Called at daemon start, on the daemon's periodic watchdog cadence, and on
// demand from the CLI / GUI. reconcileMu serializes concurrent callers so an
// overlapping startup + periodic + manual reconcile can never double-fire a
// window; a paused scheduler is left untouched so the gap stays open until
// the scheduler resumes.
func (s *Scheduler) Reconcile() error {
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

		switch sch.MissedPolicy {
		case "run_now":
			row, err := s.db.Tasks().Get(sch.TaskSlug)
			if err != nil {
				fmt.Fprintf(os.Stderr, "heka: reconcile %s: task missing: %v\n", sch.Slug, err)
				break
			}
			var t task.Task
			if err := json.Unmarshal([]byte(row.ParsedJSON), &t); err != nil {
				fmt.Fprintf(os.Stderr, "heka: reconcile %s: %v\n", sch.Slug, err)
				break
			}
			_, err = s.run.Start(s.context(), &t, executor.Options{
				Trigger:    "schedule",
				ScheduleID: sch.ID,
				BaseDir:    filepath.Dir(row.YAMLPath),
			})
			switch {
			case errors.Is(err, executor.ErrAlreadyRunning):
				// Overlap (D11): record the skip so the gap is visible in history.
				s.recordHalt(sch, "skipped")
			case err != nil:
				fmt.Fprintf(os.Stderr, "heka: reconcile run %s: %v\n", sch.Slug, err)
			}
		default: // "skip"
			// One collapsed 'missed' row; the real gap is visible in history.
			s.recordHalt(sch, "missed")
		}
		// Whatever the policy, the window is now closed.
		sch.LastRunAt = db.Now()
		_ = s.db.Schedules().Save(sch)
	}
	return nil
}

// countOccurrences walks the cron spec from start to end (newest window
// first) and counts activations. Bounded defensively.
func countOccurrences(spec string, start, end time.Time) int {
	if end.Before(start) {
		return 0
	}
	sched, err := specParser.Parse(spec)
	if err != nil {
		return 0
	}
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
