package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"heka/internal/core/executor"
	"heka/internal/core/task"
	"heka/internal/db"
)

// fakeRunner records dispatches; never spawns.
type fakeRunner struct {
	mu    sync.Mutex
	calls []executor.Options
	slug  string
	busy  bool
	err   error // returned when set and not busy
}

func (f *fakeRunner) Start(_ context.Context, t *task.Task, opt executor.Options) (*executor.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.slug = t.Slug
	if f.busy {
		return nil, executor.ErrAlreadyRunning
	}
	if f.err != nil {
		return nil, f.err
	}
	f.calls = append(f.calls, opt)
	done := make(chan struct{})
	return &executor.Handle{GroupID: "g", Done: done}, nil
}

func (f *fakeRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func setup(t *testing.T) (*db.DB, *Scheduler, *fakeRunner) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	// Seed a task so schedules have something to dispatch.
	tk := task.Task{Version: 1, Slug: "daily", Name: "Daily", Type: "script", Runtime: "custom", Script: "x.sh"}
	parsed, _ := json.Marshal(tk)
	if err := database.Tasks().Save(db.Task{
		ID: "id-daily", Slug: "daily", Name: "Daily",
		YAMLPath: "/tasks/daily.yaml", ParsedJSON: string(parsed),
		Enabled: true, CreatedAt: db.Now(), UpdatedAt: db.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{}
	sch := New(database, runner)
	return database, sch, runner
}

func saveSchedule(t *testing.T, d *db.DB, sch db.Schedule) {
	t.Helper()
	if err := d.Schedules().Save(sch); err != nil {
		t.Fatal(err)
	}
}

func TestRecurringFires(t *testing.T) {
	database, sch, runner := setup(t)
	now := db.Now()
	saveSchedule(t, database, db.Schedule{
		ID: "s1", Slug: "every-second", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1s", Enabled: true, MissedPolicy: "skip", CreatedAt: now,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	sch.Start(ctx)
	defer sch.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for runner.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if runner.count() == 0 {
		t.Fatal("recurring schedule never fired")
	}
	runner.mu.Lock()
	opt := runner.calls[0]
	runner.mu.Unlock()
	if opt.Trigger != "schedule" || opt.ScheduleID != "s1" {
		t.Fatalf("dispatch options = %+v", opt)
	}
	// Row reflects the fire.
	row, err := database.Schedules().Get("s1")
	if err != nil {
		t.Fatal(err)
	}
	if row.LastRunAt == "" || row.NextRunAt == "" {
		t.Fatalf("row = %+v", row)
	}
	// Health's next-run points forward.
	next, slug := sch.NextRun()
	if next.Before(time.Now()) || slug != "daily" {
		t.Fatalf("NextRun = %v %q", next, slug)
	}
}

func TestManualOverlapRecordsScheduledSkip(t *testing.T) {
	database, sch, runner := setup(t)
	runner.busy = true
	saveSchedule(t, database, db.Schedule{
		ID: "s2", Slug: "busy-task", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1s", Enabled: true, MissedPolicy: "skip", CreatedAt: db.Now(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	sch.Start(ctx)
	defer sch.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for {
		rows, _ := database.Runs().ListByTask("daily", 10)
		if len(rows) > 0 {
			if rows[0].Status != "skipped" || rows[0].Trigger != "schedule" || rows[0].ScheduleID != "s2" {
				t.Fatalf("scheduled overlap row = %+v, want schedule/s2 skipped", rows[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no skip row recorded")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestOnetimeFiresOnce(t *testing.T) {
	database, sch, runner := setup(t)
	saveSchedule(t, database, db.Schedule{
		ID: "s3", Slug: "once", TaskSlug: "daily", Kind: "onetime",
		RunAt: db.Now(), Enabled: true, MissedPolicy: "skip", CreatedAt: db.Now(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	sch.Start(ctx)
	defer sch.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for runner.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if runner.count() != 1 {
		t.Fatalf("fires = %d, want exactly 1", runner.count())
	}
	row, _ := database.Schedules().Get("s3")
	if row.Enabled {
		t.Fatal("one-time job still enabled after firing")
	}
	// A re-Sync must not re-register the done job.
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if runner.count() != 1 {
		t.Fatalf("fires after re-sync = %d, want 1", runner.count())
	}
}

func TestMissedSkip(t *testing.T) {
	database, sch, runner := setup(t)
	saveSchedule(t, database, db.Schedule{
		ID: "s4", Slug: "missed", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1m", Enabled: true, MissedPolicy: "skip",
		LastRunAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		CreatedAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	sch.Start(ctx)
	defer sch.Stop()

	if err := sch.Reconcile(); err != nil {
		t.Fatal(err)
	}
	rows, _ := database.Runs().ListByTask("daily", 10)
	if len(rows) != 1 || rows[0].Status != "missed" {
		t.Fatalf("runs = %+v, want one missed row", rows)
	}
	if runner.count() != 0 {
		t.Fatalf("skip policy must not run, got %d calls", runner.count())
	}
	// Window closed: re-reconcile finds nothing new.
	if err := sch.Reconcile(); err != nil {
		t.Fatal(err)
	}
	rows, _ = database.Runs().ListByTask("daily", 10)
	if len(rows) != 1 {
		t.Fatalf("reconcile created more rows: %+v", rows)
	}
}

func TestMissedRunNow(t *testing.T) {
	database, sch, runner := setup(t)
	saveSchedule(t, database, db.Schedule{
		ID: "s5", Slug: "catch-up", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1m", Enabled: true, MissedPolicy: "run_now",
		LastRunAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	sch.Start(ctx)
	defer sch.Stop()

	if err := sch.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if runner.count() != 1 {
		t.Fatalf("run_now policy: got %d calls, want 1", runner.count())
	}
}

func TestDisabledSchedulesNotReconciled(t *testing.T) {
	database, sch, runner := setup(t)
	saveSchedule(t, database, db.Schedule{
		ID: "s6", Slug: "off", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1m", Enabled: false, MissedPolicy: "run_now",
		LastRunAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
	})
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := sch.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if runner.count() != 0 {
		t.Fatalf("disabled schedules must not fire, got %d", runner.count())
	}
}

// Regression: a daily schedule whose last successful run is ~2 days back
// (two missed 9:00 activations) must fire exactly one catch-up run. Mirrors
// the real-world "PC off for two days, reconcile now did nothing" report.
func TestReconcileTwoMissedDays(t *testing.T) {
	database, sch, runner := setup(t)

	// Window spanning >= 2 daily 9:00 activations regardless of time-of-day.
	lastRun := time.Now().Add(-50 * time.Hour)
	finished := lastRun.Add(2 * time.Minute)
	saveSchedule(t, database, db.Schedule{
		ID: "s9", Slug: "daily-check", TaskSlug: "daily", Kind: "recurring",
		Cron: "00 09 * * *", Enabled: true, MissedPolicy: "run_now",
		LastRunAt:  finished.UTC().Format(time.RFC3339),
		LastStatus: "success",
		CreatedAt:  lastRun.Add(-24 * time.Hour).UTC().Format(time.RFC3339),
	})
	// The last successful schedule run inside that window.
	at := finished.UTC().Format(time.RFC3339)
	if err := database.Runs().Create(db.Run{
		RunID: ulid.Make().String(), GroupID: ulid.Make().String(),
		TaskSlug: "daily", ScheduleID: "s9", Trigger: "schedule",
		Status: "success", StartedAt: &at, FinishedAt: &at, CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}

	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := sch.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if runner.count() != 1 {
		t.Fatalf("two missed days: fires = %d, want 1", runner.count())
	}
}

// Regression (v0.7.5 field report, 2026-09-03): a schedule run that finishes
// in under a second stores started_at == finished_at == last_run_at (RFC3339
// has second precision). The inclusive window boundary then counted that
// already-accounted run as "fired" in the next window, so the missed 9:00
// activation after a daemon restart computed missed = 1 - 1 = 0 and was
// never caught up. The bound must be exclusive.
func TestReconcileSubSecondRunDoesNotMaskNextTick(t *testing.T) {
	database, sch, runner := setup(t)

	// Most recent 9:00 tick at or before now; the previous run paid for the
	// tick a day earlier and left last_run_at == its own started_at.
	now := time.Now()
	tick := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	for !tick.Before(now) {
		tick = tick.AddDate(0, 0, -1)
	}
	prev := tick.AddDate(0, 0, -1)
	at := prev.UTC().Format(time.RFC3339)

	saveSchedule(t, database, db.Schedule{
		ID: "s12", Slug: "daily-check", TaskSlug: "daily", Kind: "recurring",
		Cron: "00 09 * * *", Enabled: true, MissedPolicy: "run_now",
		LastRunAt:  at,
		LastStatus: "success",
		CreatedAt:  prev.AddDate(0, 0, -1).UTC().Format(time.RFC3339),
	})
	// The sub-second run that produced last_run_at.
	if err := database.Runs().Create(db.Run{
		RunID: ulid.Make().String(), GroupID: ulid.Make().String(),
		TaskSlug: "daily", ScheduleID: "s12", Trigger: "schedule",
		Status: "success", StartedAt: &at, FinishedAt: &at, CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}

	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := sch.ReconcileWithReason("startup"); err != nil {
		t.Fatal(err)
	}
	if runner.count() != 1 {
		t.Fatalf("missed tick masked by previous sub-second run: fires = %d, want 1", runner.count())
	}
	row, _ := database.Schedules().Get("s12")
	if got := parseTime(row.LastRunAt); got.Before(now.Add(-time.Minute)) {
		t.Fatalf("window did not close: last_run_at = %s", row.LastRunAt)
	}
}

func TestReconcileSkippedWhilePaused(t *testing.T) {
	database, sch, runner := setup(t)
	saveSchedule(t, database, db.Schedule{
		ID: "s8", Slug: "paused-missed", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1m", Enabled: true, MissedPolicy: "run_now",
		LastRunAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
	})
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	sch.Pause()
	if err := sch.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if runner.count() != 0 {
		t.Fatalf("paused schedule must not fire, got %d", runner.count())
	}
	// Resume: the same window is still open and now fires.
	sch.Resume()
	if err := sch.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if runner.count() != 1 {
		t.Fatalf("after resume fires = %d, want 1", runner.count())
	}
}

// Regression for v0.7 follow-up: every catch-up decision must land in the
// daemon log (Logs → System view), and a transient start failure must leave
// the window open so the next pass retries instead of dropping the run.
func TestReconcileLogsAndRetries(t *testing.T) {
	database, sch, runner := setup(t)
	saveSchedule(t, database, db.Schedule{
		ID: "s10", Slug: "retry", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1m", Enabled: true, MissedPolicy: "run_now",
		LastRunAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
	})
	boom := errors.New("spawn failed")
	runner.err = boom
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	sch.Start(ctx)
	defer sch.Stop()

	// Transient start failure: no run, window stays open, warn logged.
	if err := sch.ReconcileWithReason("startup"); err != nil {
		t.Fatal(err)
	}
	if runner.count() != 0 {
		t.Fatalf("failing runner fired %d times", runner.count())
	}
	row, _ := database.Schedules().Get("s10")
	if got := parseTime(row.LastRunAt); got.After(time.Now().Add(-5 * time.Minute)) {
		t.Fatalf("transient failure closed the window: last_run_at = %s", row.LastRunAt)
	}
	logs, err := database.Logs().List(10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.Event == "reconcile" && l.Level == "warn" && strings.Contains(l.Message, "retry") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no warn log for transient failure: %+v", logs)
	}

	// Runner recovers: next pass fires the catch-up and logs it.
	runner.err = nil
	if err := sch.ReconcileWithReason("startup"); err != nil {
		t.Fatal(err)
	}
	if runner.count() != 1 {
		t.Fatalf("run_now after recovery fired %d times, want 1", runner.count())
	}
	row, _ = database.Schedules().Get("s10")
	if got := parseTime(row.LastRunAt); got.Before(time.Now().Add(-5 * time.Minute)) {
		t.Fatalf("recovery did not close the window: last_run_at = %s", row.LastRunAt)
	}
	logs, _ = database.Logs().List(10)
	found = false
	for _, l := range logs {
		if l.Event == "reconcile" && l.Level == "info" && strings.Contains(l.Message, "catch-up") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no info log for catch-up run: %+v", logs)
	}
}

// Every reconcile pass — periodic heartbeat included — lands in the daemon
// log so the system log proves the loop is alive, and a quiet pass fires
// nothing.
func TestReconcileManualLogsWhenQuiet(t *testing.T) {
	database, sch, runner := setup(t)
	saveSchedule(t, database, db.Schedule{
		ID: "s11", Slug: "quiet", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1m", Enabled: true, MissedPolicy: "run_now", CreatedAt: db.Now(),
	})
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := sch.Reconcile(); err != nil {
		t.Fatal(err)
	}
	logs, _ := database.Logs().List(10)
	if len(logs) != 1 || !strings.Contains(logs[0].Message, "periodic") ||
		!strings.Contains(logs[0].Message, "0 caught up") {
		t.Fatalf("periodic heartbeat not logged: %+v", logs)
	}
	if err := sch.ReconcileWithReason("manual"); err != nil {
		t.Fatal(err)
	}
	logs, _ = database.Logs().List(10)
	if len(logs) != 2 || !strings.Contains(logs[0].Message, "manual") {
		t.Fatalf("manual quiet pass not logged: %+v", logs)
	}
	if runner.count() != 0 {
		t.Fatalf("quiet reconcile fired %d times", runner.count())
	}
}

func TestValidateSchedule(t *testing.T) {
	good := db.Schedule{Slug: "nightly", TaskSlug: "daily", Kind: "recurring", Cron: "@daily"}
	if err := ValidateSchedule(good); err != nil {
		t.Fatalf("valid schedule rejected: %v", err)
	}
	cases := map[string]db.Schedule{
		"bad slug": {Slug: "Bad Slug", TaskSlug: "daily", Kind: "recurring", Cron: "@daily"},
		"no task":  {Slug: "x", TaskSlug: "", Kind: "recurring", Cron: "@daily"},
		"bad cron": {Slug: "x", TaskSlug: "daily", Kind: "recurring", Cron: "not-a-cron"},
		"no cron":  {Slug: "x", TaskSlug: "daily", Kind: "recurring"},
		"past onetime": {Slug: "x", TaskSlug: "daily", Kind: "onetime",
			RunAt: time.Now().Add(-time.Hour).Format(time.RFC3339)},
		"no run_at":  {Slug: "x", TaskSlug: "daily", Kind: "onetime"},
		"bad policy": {Slug: "x", TaskSlug: "daily", Kind: "recurring", Cron: "@daily", MissedPolicy: "nope"},
	}
	for name, sch := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateSchedule(sch); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCountOccurrences(t *testing.T) {
	start := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	end := start.Add(10*time.Minute + time.Second)
	if n := countOccurrences("@every 1m", start, end); n != 10 {
		t.Fatalf("count = %d, want 10", n)
	}
	if n := countOccurrences("@every 1m", end, end); n != 0 {
		t.Fatalf("empty window count = %d", n)
	}
}

func TestPauseResume(t *testing.T) {
	database, sch, runner := setup(t)
	saveSchedule(t, database, db.Schedule{
		ID: "s7", Slug: "pause-test", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1s", Enabled: true, MissedPolicy: "skip", CreatedAt: db.Now(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	sch.Start(ctx)
	defer sch.Stop()

	// Wait for at least one fire.
	deadline := time.Now().Add(3 * time.Second)
	for runner.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if runner.count() == 0 {
		t.Fatal("schedule never fired before pause")
	}

	// Pause: ticks should stop, no skipped run rows written.
	sch.Pause()
	if !sch.IsPaused() {
		t.Fatal("IsPaused should be true after Pause")
	}
	countBeforePause := runner.count()
	time.Sleep(2500 * time.Millisecond) // wait for 2+ tick windows
	if runner.count() != countBeforePause {
		t.Fatalf("ticks fired while paused: %d → %d", countBeforePause, runner.count())
	}

	// Verify no skipped run rows were written for the pause period.
	rows, _ := database.Runs().ListByTask("daily", 10)
	for _, r := range rows {
		if r.Status == "skipped" {
			// Skipped rows from pause are NOT expected (unlike overlap skips).
			t.Fatal("pause must not write skipped run rows")
		}
	}

	// Resume: ticks should restart.
	sch.Resume()
	if sch.IsPaused() {
		t.Fatal("IsPaused should be false after Resume")
	}
	deadline = time.Now().Add(3 * time.Second)
	for runner.count() == countBeforePause && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if runner.count() == countBeforePause {
		t.Fatal("ticks did not resume after Resume")
	}
}

func TestHealthShowsPaused(t *testing.T) {
	database, sch, _ := setup(t)
	saveSchedule(t, database, db.Schedule{
		ID: "s8", Slug: "health-pause", TaskSlug: "daily", Kind: "recurring",
		Cron: "@every 1m", Enabled: true, MissedPolicy: "skip", CreatedAt: db.Now(),
	})
	if err := sch.Sync(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sch.Start(ctx)
	defer sch.Stop()

	// Before pause: NextRun returns a time.
	next, _ := sch.NextRun()
	if next.IsZero() {
		t.Fatal("NextRun should not be zero before pause")
	}

	sch.Pause()
	// After pause: NextRun still returns the same (frozen) time.
	nextAfter, _ := sch.NextRun()
	if !nextAfter.Equal(next) {
		t.Fatalf("NextRun changed while paused: %v → %v", next, nextAfter)
	}
}
