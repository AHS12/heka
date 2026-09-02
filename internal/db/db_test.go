package db

import (
	"errors"
	"testing"
	"time"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestMigrateFreshAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Fresh DB: all migrations recorded.
	var count int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Fatalf("schema_migrations count = %d, want >= 1", count)
	}
	// Re-open: idempotent, no error, no extra rows.
	d.Close()
	d2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if err := d2.sql.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Fatalf("schema_migrations count after reopen = %d", count)
	}
	// No application tables are missing.
	for _, table := range []string{"tasks", "schedules", "runs", "secrets", "kv", "daemon_log"} {
		var n int
		if err := d2.sql.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("table %s missing after migration", table)
		}
	}
}

func TestWALEnabled(t *testing.T) {
	d := openTest(t)
	var mode string
	if err := d.sql.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestForeignKeysOn(t *testing.T) {
	d := openTest(t)
	var fk int
	if err := d.sql.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}
}

func TestTaskStoreRoundTrip(t *testing.T) {
	d := openTest(t)
	store := d.Tasks()
	task := Task{
		ID: "t1", Slug: "daily-research", Name: "Daily Research",
		YAMLPath: "/tasks/daily-research.yaml", ParsedJSON: `{"name":"Daily Research"}`,
		Enabled: true, CreatedAt: Now(),
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("daily-research")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Daily Research" || !got.Enabled || got.YAMLPath != task.YAMLPath {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	// Upsert by slug preserves id but updates fields.
	task.ID = "t1b"
	task.Name = "Renamed"
	task.Enabled = false
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get("daily-research")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Renamed" || got.Enabled {
		t.Fatalf("upsert mismatch: %+v", got)
	}
	// Missing → ErrNotFound.
	if _, err := store.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing: err = %v, want ErrNotFound", err)
	}
	// SetEnabled + List + Delete.
	if err := store.SetEnabled("daily-research", true); err != nil {
		t.Fatal(err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}
	if err := store.Delete("daily-research"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("daily-research"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: err = %v, want ErrNotFound", err)
	}
}

func TestScheduleForeignKeys(t *testing.T) {
	d := openTest(t)
	// A schedule referencing a missing task must fail (SPEC-03 §4).
	_, err := d.sql.Exec(`
INSERT INTO schedules (id, slug, task_slug, kind, created_at)
VALUES ('s1', 'sched-1', 'missing-task', 'recurring', ?)`, Now())
	if err == nil {
		t.Fatal("expected FK violation, got nil")
	}
}

func TestScheduleStoreRoundTrip(t *testing.T) {
	d := openTest(t)
	task := Task{ID: "t1", Slug: "backup", Name: "Backup", YAMLPath: "/x.yaml", ParsedJSON: "{}", CreatedAt: Now()}
	if err := d.Tasks().Save(task); err != nil {
		t.Fatal(err)
	}
	store := d.Schedules()
	sch := Schedule{
		ID: "s1", Slug: "nightly-backup", TaskSlug: "backup", Kind: "recurring",
		Cron: "@daily", Timezone: "local", Enabled: true, MissedPolicy: "skip",
		NextRunAt: "2026-08-25T00:00:00Z", CreatedAt: Now(),
	}
	if err := store.Save(sch); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "recurring" || got.NextRunAt != sch.NextRunAt {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if err := store.SetEnabled("s1", false); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get("s1")
	if got.Enabled {
		t.Fatal("SetEnabled(false) did not stick")
	}
	byTask, err := store.ListByTask("backup")
	if err != nil {
		t.Fatal(err)
	}
	if len(byTask) != 1 || byTask[0].ID != "s1" {
		t.Fatalf("ListByTask = %+v", byTask)
	}
	if err := store.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("s1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: err = %v, want ErrNotFound", err)
	}
}

func TestScheduleListWithLatestRunExcludesManualRuns(t *testing.T) {
	d := openTest(t)
	if err := d.Tasks().Save(Task{
		ID: "t1", Slug: "backup", Name: "Backup", YAMLPath: "/x.yaml", ParsedJSON: "{}", CreatedAt: Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.Schedules().Save(Schedule{
		ID: "s1", Slug: "nightly-backup", TaskSlug: "backup", Kind: "recurring",
		Cron: "@daily", Enabled: true, MissedPolicy: "skip", CreatedAt: Now(),
	}); err != nil {
		t.Fatal(err)
	}
	started := "2026-08-24T08:00:01Z"
	finished := "2026-08-24T08:00:02Z"
	manualStarted := "2026-08-24T09:00:01Z"
	for _, run := range []Run{
		{RunID: "run-scheduled", GroupID: "group-scheduled", TaskSlug: "backup", ScheduleID: "s1", Trigger: "schedule", Status: "success", StartedAt: &started, FinishedAt: &finished, CreatedAt: Now()},
		{RunID: "run-skipped", GroupID: "group-skipped", TaskSlug: "backup", ScheduleID: "s1", Trigger: "schedule", Status: "skipped", StartedAt: &finished, FinishedAt: &finished, CreatedAt: Now()},
		{RunID: "run-missed", GroupID: "group-missed", TaskSlug: "backup", ScheduleID: "s1", Trigger: "schedule", Status: "missed", StartedAt: &started, FinishedAt: &started, CreatedAt: Now()},
		{RunID: "run-manual", GroupID: "group-manual", TaskSlug: "backup", Trigger: "manual", Status: "success", StartedAt: &manualStarted, FinishedAt: &manualStarted, CreatedAt: Now()},
	} {
		if err := d.Runs().Create(run); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := d.Schedules().ListWithLatestRun()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.LastRunID != "run-skipped" || row.LastRunStatus != "skipped" {
		t.Fatalf("latest scheduled run = %q/%q", row.LastRunID, row.LastRunStatus)
	}
	if row.SkippedCount != 1 || row.MissedCount != 1 {
		t.Fatalf("counts = skipped %d, missed %d", row.SkippedCount, row.MissedCount)
	}
}

func TestRunStoreGroupAndAttempt(t *testing.T) {
	d := openTest(t)
	store := d.Runs()
	finished := "2026-08-24T08:00:17Z"
	started := "2026-08-24T08:00:01Z"
	exit := 3
	r := Run{
		RunID: "run-2", GroupID: "group-1", Attempt: 1, TaskSlug: "daily",
		Trigger: "schedule", Status: "failed",
		StartedAt: &started, FinishedAt: &finished, ExitCode: &exit,
		Stdout: "out", Stderr: "boom", CreatedAt: Now(),
	}
	if err := store.Create(r); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("run-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.GroupID != "group-1" || got.Attempt != 1 || got.ExitCode == nil || *got.ExitCode != 3 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if got.Stdout != "out" || got.Stderr != "boom" {
		t.Fatalf("captured output mismatch: %+v", got)
	}
	// Update writes the terminal state.
	got.Status = "success"
	got.ExitCode = intPtr(0)
	if err := store.Update(got); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := store.Get("run-2")
	if reloaded.Status != "success" || *reloaded.ExitCode != 0 {
		t.Fatalf("update mismatch: %+v", reloaded)
	}
	// ListByTask newest first.
	r2 := r
	r2.RunID = "run-1"
	r2.Attempt = 0
	r2.GroupID = "group-1"
	r2.StartedAt = &started
	r2.FinishedAt = nil
	r2.Status = "running"
	r2.ExitCode = nil
	if err := store.Create(r2); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListByTask("daily", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].RunID != "run-2" {
		t.Fatalf("ListByTask ordering = %+v", list)
	}
	// NULL schedule_id scans as empty string.
	if got.ScheduleID != "" {
		t.Fatalf("ScheduleID = %q, want \"\"", got.ScheduleID)
	}
}

func TestRunPrune(t *testing.T) {
	d := openTest(t)
	store := d.Runs()
	mk := func(id, status, finished string, exit *int) Run {
		var finishedAt *string
		if finished != "" {
			finishedAt = &finished
		}
		return Run{
			RunID: id, GroupID: "g", Attempt: 0, TaskSlug: "t", Trigger: "manual",
			Status: status, FinishedAt: finishedAt, ExitCode: exit, CreatedAt: Now(),
		}
	}
	old := "2026-01-01T00:00:00Z"
	recent := "2026-08-24T00:00:00Z"
	runs := []Run{
		mk("old-1", "success", old, intPtr(0)),
		mk("old-2", "failed", old, intPtr(1)),
		mk("recent-1", "success", recent, intPtr(0)),
		mk("active-1", "running", "", nil),
	}
	for _, r := range runs {
		if err := store.Create(r); err != nil {
			t.Fatal(err)
		}
	}
	cutoff := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Prune(cutoff); err != nil {
		t.Fatal(err)
	}
	rem, err := store.ListByTask("t", 10)
	if err != nil {
		t.Fatal(err)
	}
	// old-1, old-2 are gone; recent-1 survives; active "(running) never touched.
	got := map[string]bool{}
	for _, r := range rem {
		got[r.RunID] = true
	}
	for _, wantSurvivor := range []string{"recent-1", "active-1"} {
		if !got[wantSurvivor] {
			t.Errorf("%s was pruned, want survivor", wantSurvivor)
		}
	}
	for _, gone := range []string{"old-1", "old-2"} {
		if got[gone] {
			t.Errorf("%s survived, want pruned", gone)
		}
	}
	// Prune on an empty table is a no-op.
	t2 := openTest(t)
	if err := t2.Runs().Prune(cutoff); err != nil {
		t.Fatal(err)
	}
}

func TestSecretAndKVStore(t *testing.T) {
	d := openTest(t)
	if err := d.Secrets().Set("KEY", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := d.Secrets().Set("KEY", "v2"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := d.Secrets().Get("KEY")
	if err != nil || !ok || v != "v2" {
		t.Fatalf("Get = %q, %v, %v; want v2, true, nil", v, ok, err)
	}
	if _, ok, _ := d.Secrets().Get("MISSING"); ok {
		t.Fatal("Get missing key = ok")
	}
	keys, err := d.Secrets().Keys()
	if err != nil || len(keys) != 1 || keys[0] != "KEY" {
		t.Fatalf("Keys = %v, %v", keys, err)
	}
	if err := d.Secrets().Delete("KEY"); err != nil {
		t.Fatal(err)
	}

	if err := d.KV().Set("heartbeat", "2026-08-24T08:00:00Z"); err != nil {
		t.Fatal(err)
	}
	hb, ok, err := d.KV().Get("heartbeat")
	if err != nil || !ok || hb == "" {
		t.Fatalf("KV Get = %q, %v, %v", hb, ok, err)
	}
	if err := d.KV().Delete("heartbeat"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := d.KV().Get("heartbeat"); ok {
		t.Fatal("KV value survived delete")
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Secrets().Set("K", "persist-me"); err != nil {
		t.Fatal(err)
	}
	if err := d.KV().Set("state", "alive"); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	d2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if v, ok, _ := d2.Secrets().Get("K"); !ok || v != "persist-me" {
		t.Fatalf("secret did not survive reopen: %q, %v", v, ok)
	}
	if v, ok, _ := d2.KV().Get("state"); !ok || v != "alive" {
		t.Fatalf("kv did not survive reopen: %q, %v", v, ok)
	}
}

func TestLogStore(t *testing.T) {
	d := openTest(t)
	for i := 0; i < 3; i++ {
		if err := d.Logs().Add("info", "reconcile", "entry"); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Logs().Add("warn", "scheduler", "oops"); err != nil {
		t.Fatal(err)
	}
	rows, err := d.Logs().List(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("List(2) = %d rows, want 2 (newest first)", len(rows))
	}
	if rows[0].Level != "warn" || rows[0].Event != "scheduler" {
		t.Fatalf("newest row = %+v, want the warn/scheduler entry", rows[0])
	}
	if rows[0].TS == "" || rows[0].ID == 0 {
		t.Fatalf("row missing ts/id: %+v", rows[0])
	}

	// Retention prunes only old entries.
	future := time.Now().UTC().Add(time.Hour)
	if err := d.Logs().Prune(future); err != nil {
		t.Fatal(err)
	}
	rows, err = d.Logs().List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("Prune(left) left %d rows", len(rows))
	}
}

func intPtr(v int) *int { return &v }
