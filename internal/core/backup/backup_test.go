package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"heka/internal/db"
)

// seedDataDir builds a realistic data dir: a migrated SQLite database with
// tasks/schedules/secrets/runs/kv rows, the vault key, task YAML files, and
// an optional config.yaml. The DB handle is returned still open so callers
// can test snapshotting against a live (WAL) database.
func seedDataDir(t *testing.T) (*db.DB, string) {
	t.Helper()
	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Tasks().Save(db.Task{
		ID: "01", Slug: "nightly-build", Name: "Nightly build",
		YAMLPath: filepath.Join(dataDir, "tasks", "nightly-build.yaml"),
		Enabled:  true, CreatedAt: db.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Schedules().Save(db.Schedule{
		ID: "s1", Slug: "nightly", TaskSlug: "nightly-build", Kind: "recurring",
		Cron: "0 3 * * *", Timezone: "local", Enabled: true,
		MissedPolicy: "skip", NextRunAt: db.Now(), CreatedAt: db.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Secrets().Set("API_TOKEN", "super-secret-value"); err != nil {
		t.Fatal(err)
	}
	if err := database.Secrets().Set("BACKUP_PASSPHRASE", "pw-123"); err != nil {
		t.Fatal(err)
	}
	if err := database.Runs().Create(db.Run{
		RunID: "r1", GroupID: "g1", Attempt: 1, TaskSlug: "nightly-build",
		Trigger: "manual", Status: "success", CreatedAt: db.Now(), Stdout: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{
		{"daemon_pid", "1234"}, {"heartbeat", db.Now()}, {"scheduler_paused", "true"},
	} {
		if err := database.KV().Set(kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}

	tasksDir := filepath.Join(dataDir, "tasks")
	if err := os.MkdirAll(tasksDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "nightly-build.yaml"),
		[]byte("version: 1\nname: Nightly build\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.yaml"),
		[]byte("version: 1\nlog_retention_days: 30\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return database, dataDir
}

func createOpts(dataDir, pass string, includes Includes) CreateOptions {
	return CreateOptions{
		DataDir:      dataDir,
		TasksDir:     filepath.Join(dataDir, "tasks"),
		ArtifactsDir: filepath.Join(dataDir, "runs"),
		OutPath:      filepath.Join(dataDir, "backups", ArchiveName(time.Now())),
		Passphrase:   pass,
		AppVersion:   "test",
		Includes:     includes,
	}
}

func TestRoundTrip(t *testing.T) {
	_, src := seedDataDir(t)
	pass := "correct horse battery staple"
	opts := createOpts(src, pass, Includes{RunHistory: true})

	manifest, err := Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Counts.Tasks != 1 || manifest.Counts.Schedules != 1 ||
		manifest.Counts.Secrets != 2 || manifest.Counts.Runs != 1 {
		t.Fatalf("unexpected counts: %+v", manifest.Counts)
	}
	if manifest.SchemaVersion != db.MaxMigrationVersion() {
		t.Fatalf("schema version = %d, want %d", manifest.SchemaVersion, db.MaxMigrationVersion())
	}
	if !manifest.HasConfig {
		t.Fatal("config.yaml should be detected")
	}

	// Restore into an existing install (so a safety backup is warranted).
	dst := t.TempDir()
	existing, err := db.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	existing.Close()
	res, err := Restore(RestoreOptions{
		ZipPath:       opts.OutPath,
		Passphrase:    pass,
		DataDir:       dst,
		TasksDir:      filepath.Join(dst, "tasks"),
		ArtifactsDir:  filepath.Join(dst, "runs"),
		IncludeConfig: true,
		CurrentSchema: db.MaxMigrationVersion(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.SafetyBackupPath == "" {
		t.Fatal("expected a safety backup (source had data)")
	}
	if _, err := os.Stat(res.SafetyBackupPath); err != nil {
		t.Fatalf("safety backup missing: %v", err)
	}

	restored, err := db.Open(dst)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer restored.Close()

	task, err := restored.Tasks().Get("nightly-build")
	if err != nil {
		t.Fatalf("restored task missing: %v", err)
	}
	if !task.Enabled {
		t.Fatal("restored task should keep its enabled flag")
	}
	if _, err := restored.Schedules().Get("s1"); err != nil {
		t.Fatalf("restored schedule missing: %v", err)
	}
	if v, ok, _ := restored.Secrets().Get("API_TOKEN"); !ok || v != "super-secret-value" {
		t.Fatalf("restored secret = %q ok=%v (vault key must travel with the db)", v, ok)
	}
	if v, ok, _ := restored.KV().Get("scheduler_paused"); !ok || v != "true" {
		t.Fatalf("restored kv scheduler_paused = %q ok=%v", v, ok)
	}
	// Volatile keys were stripped from the snapshot.
	if _, ok, _ := restored.KV().Get("daemon_pid"); ok {
		t.Fatal("daemon_pid should be stripped from archives")
	}
	if _, ok, _ := restored.KV().Get("heartbeat"); ok {
		t.Fatal("heartbeat should be stripped from archives")
	}
	// Task YAML restored.
	data, err := os.ReadFile(filepath.Join(dst, "tasks", "nightly-build.yaml"))
	if err != nil || !strings.Contains(string(data), "Nightly build") {
		t.Fatalf("restored task yaml missing: %v", err)
	}
	// Config restored per IncludeConfig.
	cfg, err := os.ReadFile(filepath.Join(dst, "config.yaml"))
	if err != nil || !strings.Contains(string(cfg), "log_retention_days: 30") {
		t.Fatalf("restored config missing: %v", err)
	}
}

func TestRoundTripWithoutRunHistory(t *testing.T) {
	_, src := seedDataDir(t)
	pass := "pw"
	opts := createOpts(src, pass, Includes{RunHistory: false})

	manifest, err := Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Counts.Runs != 0 {
		t.Fatalf("runs should be stripped, got %d", manifest.Counts.Runs)
	}

	dst := t.TempDir()
	if _, err := Restore(RestoreOptions{
		ZipPath:       opts.OutPath,
		Passphrase:    pass,
		DataDir:       dst,
		TasksDir:      filepath.Join(dst, "tasks"),
		CurrentSchema: db.MaxMigrationVersion(),
	}); err != nil {
		t.Fatal(err)
	}
	restored, err := db.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var runs int
	rows, err := restored.Runs().ListRuns(db.RunsFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	runs = len(rows.Runs)
	if runs != 0 {
		t.Fatalf("restored db should have no runs, got %d", runs)
	}
}

func TestInspect(t *testing.T) {
	_, src := seedDataDir(t)
	pass := "pw"
	opts := createOpts(src, pass, Includes{RunHistory: true})
	if _, err := Create(opts); err != nil {
		t.Fatal(err)
	}

	m, err := Inspect(opts.OutPath, pass)
	if err != nil {
		t.Fatal(err)
	}
	if m.Counts.Tasks != 1 || !m.HasConfig {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if _, err := Inspect(opts.OutPath, "wrong"); err == nil {
		t.Fatal("wrong passphrase should fail")
	}
}

func TestRestoreRefusesRunningDaemon(t *testing.T) {
	_, src := seedDataDir(t)
	pass := "pw"
	opts := createOpts(src, pass, Includes{RunHistory: true})
	if _, err := Create(opts); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	marker := filepath.Join(dst, "heka.db")
	if err := os.WriteFile(marker, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Restore(RestoreOptions{
		ZipPath:       opts.OutPath,
		Passphrase:    pass,
		DataDir:       dst,
		TasksDir:      filepath.Join(dst, "tasks"),
		CurrentSchema: db.MaxMigrationVersion(),
		DaemonRunning: func() bool { return true },
	})
	if err == nil || !strings.Contains(err.Error(), ErrDaemonRunning.Error()) {
		t.Fatalf("want ErrDaemonRunning, got %v", err)
	}
	data, _ := os.ReadFile(marker)
	if string(data) != "original" {
		t.Fatal("restore must not touch anything while the daemon runs")
	}
}

func TestRestoreRefusesNewerSchema(t *testing.T) {
	_, src := seedDataDir(t)
	pass := "pw"
	opts := createOpts(src, pass, Includes{RunHistory: true})
	if _, err := Create(opts); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	_, err := Restore(RestoreOptions{
		ZipPath:       opts.OutPath,
		Passphrase:    pass,
		DataDir:       dst,
		TasksDir:      filepath.Join(dst, "tasks"),
		CurrentSchema: 0, // nothing supported → any archive is "too new"
	})
	if err == nil || !strings.Contains(err.Error(), ErrBackupTooNew.Error()) {
		t.Fatalf("want ErrBackupTooNew, got %v", err)
	}
}

func TestSnapshotWhileDBOpen(t *testing.T) {
	// The DB handle stays open (WAL active) across Create — this mirrors a
	// running daemon and must still produce a complete, restorable snapshot.
	database, src := seedDataDir(t)
	pass := "pw"
	opts := createOpts(src, pass, Includes{RunHistory: true})
	if _, err := Create(opts); err != nil {
		t.Fatal(err)
	}
	_ = database // still open on purpose

	dst := t.TempDir()
	if _, err := Restore(RestoreOptions{
		ZipPath:       opts.OutPath,
		Passphrase:    pass,
		DataDir:       dst,
		TasksDir:      filepath.Join(dst, "tasks"),
		CurrentSchema: db.MaxMigrationVersion(),
	}); err != nil {
		t.Fatal(err)
	}
	restored, err := db.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if v, ok, _ := restored.Secrets().Get("BACKUP_PASSPHRASE"); !ok || v != "pw-123" {
		t.Fatalf("secret lost across live snapshot: %q ok=%v", v, ok)
	}
}

func TestRestoreSafetyBackupOnEmptyTarget(t *testing.T) {
	_, src := seedDataDir(t)
	pass := "pw"
	opts := createOpts(src, pass, Includes{RunHistory: true})
	if _, err := Create(opts); err != nil {
		t.Fatal(err)
	}
	// Fresh install: no db, no vault key → no safety backup, restore proceeds.
	dst := t.TempDir()
	res, err := Restore(RestoreOptions{
		ZipPath:       opts.OutPath,
		Passphrase:    pass,
		DataDir:       dst,
		TasksDir:      filepath.Join(dst, "tasks"),
		CurrentSchema: db.MaxMigrationVersion(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.SafetyBackupPath != "" {
		t.Fatal("fresh target should not produce a safety backup")
	}
	restored, err := db.Open(dst)
	if err != nil {
		t.Fatalf("restored data dir unusable: %v", err)
	}
	restored.Close()
}

func TestConfigValidateAndNextRun(t *testing.T) {
	// Valid interval config.
	c := Config{Schedule: ScheduleSpec{Kind: ScheduleInterval, EveryHours: 6}, KeepLastLocal: 3}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	// Interval bounds: 1 hour up to one year.
	c.Schedule.EveryHours = 0
	if err := c.Validate(); err == nil {
		t.Fatal("every_hours=0 must be rejected")
	}
	c.Schedule.EveryHours = 8761
	if err := c.Validate(); err == nil {
		t.Fatal("every_hours beyond a year must be rejected")
	}
	c.Schedule.EveryHours = 8760
	if err := c.Validate(); err != nil {
		t.Fatalf("every_hours=8760 (yearly) rejected: %v", err)
	}
	// Daily format.
	c.Schedule = ScheduleSpec{Kind: ScheduleDaily, AtTime: "25:00"}
	if err := c.Validate(); err == nil {
		t.Fatal("25:00 must be rejected")
	}
	c.Schedule.AtTime = "03:45"
	if err := c.Validate(); err != nil {
		t.Fatalf("03:45 rejected: %v", err)
	}
	// Weekly: weekday bounds + time format.
	c.Schedule = ScheduleSpec{Kind: ScheduleWeekly, Weekday: 7, AtTime: "03:00"}
	if err := c.Validate(); err == nil {
		t.Fatal("weekday=7 must be rejected")
	}
	c.Schedule = ScheduleSpec{Kind: ScheduleWeekly, Weekday: 3, AtTime: "bad"}
	if err := c.Validate(); err == nil {
		t.Fatal("weekly with bad time must be rejected")
	}
	c.Schedule = ScheduleSpec{Kind: ScheduleWeekly, Weekday: 3, AtTime: "09:30"}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid weekly rejected: %v", err)
	}
	// Monthly: day bounds + time format.
	c.Schedule = ScheduleSpec{Kind: ScheduleMonthly, DayOfMonth: 29, AtTime: "03:00"}
	if err := c.Validate(); err == nil {
		t.Fatal("day_of_month=29 must be rejected")
	}
	c.Schedule = ScheduleSpec{Kind: ScheduleMonthly, DayOfMonth: 15, AtTime: "04:00"}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid monthly rejected: %v", err)
	}
	// S3 needs endpoint + bucket together.
	c = Config{Schedule: ScheduleSpec{Kind: ScheduleOff}, S3: S3Config{Bucket: "b"}}
	if err := c.Validate(); err == nil {
		t.Fatal("bucket without endpoint must be rejected")
	}
	c.S3.Endpoint = "account.r2.cloudflarestorage.com"
	if err := c.Validate(); err != nil {
		t.Fatalf("valid s3 config rejected: %v", err)
	}

	after := time.Date(2026, 9, 4, 10, 0, 0, 0, time.Local) // a Friday

	// NextRun: interval is strictly after the given time.
	next := (ScheduleSpec{Kind: ScheduleInterval, EveryHours: 6}).NextRun(after)
	if !next.Equal(after.Add(6 * time.Hour)) {
		t.Fatalf("interval next = %v", next)
	}
	// Daily: same-day 03:45 already passed → tomorrow 03:45.
	next = (ScheduleSpec{Kind: ScheduleDaily, AtTime: "03:45"}).NextRun(after)
	want := time.Date(2026, 9, 5, 3, 45, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("daily next = %v, want %v", next, want)
	}
	// Daily: time still ahead today → today.
	next = (ScheduleSpec{Kind: ScheduleDaily, AtTime: "23:00"}).NextRun(after)
	want = time.Date(2026, 9, 4, 23, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("daily next = %v, want %v", next, want)
	}
	// Weekly: Friday 10:00 after, Monday 09:00 → next Monday.
	next = (ScheduleSpec{Kind: ScheduleWeekly, Weekday: 1, AtTime: "09:00"}).NextRun(after)
	want = time.Date(2026, 9, 7, 9, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("weekly next = %v, want %v", next, want)
	}
	// Weekly: same day, time ahead → today.
	next = (ScheduleSpec{Kind: ScheduleWeekly, Weekday: 5, AtTime: "18:00"}).NextRun(after)
	want = time.Date(2026, 9, 4, 18, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("weekly same-day next = %v, want %v", next, want)
	}
	// Weekly: same day, time passed → next week.
	next = (ScheduleSpec{Kind: ScheduleWeekly, Weekday: 5, AtTime: "08:00"}).NextRun(after)
	want = time.Date(2026, 9, 11, 8, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("weekly next-week next = %v, want %v", next, want)
	}
	// Monthly: the 15th at 04:00 → Sep 15 (after is Sep 4).
	next = (ScheduleSpec{Kind: ScheduleMonthly, DayOfMonth: 15, AtTime: "04:00"}).NextRun(after)
	want = time.Date(2026, 9, 15, 4, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("monthly next = %v, want %v", next, want)
	}
	// Monthly: the 1st at 03:00, already passed → Oct 1 (crosses nothing).
	next = (ScheduleSpec{Kind: ScheduleMonthly, DayOfMonth: 1, AtTime: "03:00"}).NextRun(after)
	want = time.Date(2026, 10, 1, 3, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("monthly next-month next = %v, want %v", next, want)
	}
	// Monthly: Dec 15 past → Jan 15 next year (year rollover).
	decAfter := time.Date(2026, 12, 20, 10, 0, 0, 0, time.Local)
	next = (ScheduleSpec{Kind: ScheduleMonthly, DayOfMonth: 15, AtTime: "03:00"}).NextRun(decAfter)
	want = time.Date(2027, 1, 15, 3, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("monthly year-rollover next = %v, want %v", next, want)
	}
	// Off → zero.
	if !(ScheduleSpec{Kind: ScheduleOff}).NextRun(after).IsZero() {
		t.Fatal("off schedule must produce zero time")
	}
}

func TestParseConfigDefaultsAndEncode(t *testing.T) {
	// Empty string → defaults (schedule off).
	c, err := ParseConfig("", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c.Schedule.Kind != ScheduleOff || c.KeepLastLocal != 5 || !c.Includes.RunHistory {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	// Round-trip through Encode.
	c.Schedule = ScheduleSpec{Kind: ScheduleInterval, EveryHours: 12}
	enc, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseConfig(enc, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Schedule.Kind != ScheduleInterval || got.Schedule.EveryHours != 12 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	// Corrupt JSON rejected.
	if _, err := ParseConfig("{not json", t.TempDir()); err == nil {
		t.Fatal("corrupt config must be rejected")
	}
	// Invalid values rejected.
	if _, err := ParseConfig(`{"schedule":{"kind":"interval","every_hours":99999}}`, t.TempDir()); err == nil {
		t.Fatal("invalid interval must be rejected")
	}
}

func TestGeneratePassphrase(t *testing.T) {
	a, err := GeneratePassphrase()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GeneratePassphrase()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) < 32 || a == b {
		t.Fatalf("passphrase generation weak: %q vs %q", a, b)
	}
}

func TestFriendlyS3Error(t *testing.T) {
	redirect := errors.New("The requested URL returned error: 301 Moved Permanently <html>301 Moved Permanently</html>")

	err := friendlyS3Error(redirect, false)
	if err == nil || !strings.Contains(err.Error(), "Use HTTPS") {
		t.Fatalf("plain-HTTP redirect should advise enabling HTTPS, got: %v", err)
	}

	err = friendlyS3Error(redirect, true)
	if err == nil || !strings.Contains(err.Error(), "exactly the storage endpoint") {
		t.Fatalf("HTTPS redirect should advise checking the endpoint host, got: %v", err)
	}

	other := errors.New("connection refused")
	if got := friendlyS3Error(other, false); got != other {
		t.Fatalf("non-redirect errors must pass through unchanged, got: %v", got)
	}
	if got := friendlyS3Error(nil, false); got != nil {
		t.Fatalf("nil must stay nil, got: %v", got)
	}
}
