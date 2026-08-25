package scheduler

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

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
}

func (f *fakeRunner) Start(_ context.Context, t *task.Task, opt executor.Options) (*executor.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.slug = t.Slug
	if f.busy {
		return nil, executor.ErrAlreadyRunning
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

func TestOverlapRecordsSkipped(t *testing.T) {
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
			if rows[0].Status != "skipped" {
				t.Fatalf("status = %s, want skipped", rows[0].Status)
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
	// Fires strictly before end; a 1s tail gives the full 10 buckets. (The
	// engine treats this as heuristic anyway — anchors differ by ±1.)
	end := start.Add(10*time.Minute + time.Second)
	if n := countOccurrences("@every 1m", start, end); n != 10 {
		t.Fatalf("count = %d, want 10", n)
	}
	// Empty windows count nothing.
	if n := countOccurrences("@every 1m", end, end); n != 0 {
		t.Fatalf("empty window count = %d", n)
	}
}
