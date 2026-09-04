package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"heka/internal/config"
	"heka/internal/core/backup"
	"heka/internal/db"
	"heka/internal/ipc"
)

// newTestBackupManager builds a manager over a seeded data dir with a fixed
// clock and a silent toast sink. The passphrase is stored in the vault so
// jobs can actually run.
func newTestBackupManager(t *testing.T) (*BackupManager, *db.DB, string) {
	t.Helper()
	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Secrets().Set(backup.SecretPassphrase, "test-passphrase"); err != nil {
		t.Fatal(err)
	}
	if err := database.Secrets().Set(backup.SecretS3AccessKeyID, "k"); err != nil {
		t.Fatal(err)
	}
	if err := database.Secrets().Set(backup.SecretS3SecretAccessKey, "s"); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 9, 4, 12, 0, 0, 0, time.Local)
	m := newBackupManager(config.Config{
		DataDir: dataDir, TasksDir: filepath.Join(dataDir, "tasks"),
		RunArtifactsDir: filepath.Join(dataDir, "runs"),
	}, database, "test", func(name string) (string, bool) {
		v, ok, _ := database.Secrets().Get(name)
		return v, ok
	})
	m.now = func() time.Time { return fixed }
	toasts := 0
	m.notify = func(title, message string) { toasts++ }
	return m, database, dataDir
}

// waitSettled polls until the job reaches a terminal state.
func waitSettled(t *testing.T, m *BackupManager, jobID string) ipc.BackupJobDTO {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		hist, err := m.history(10)
		if err == nil {
			for _, j := range hist {
				if j.ID == jobID && j.Status != "running" {
					return j
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("backup job never settled")
	return ipc.BackupJobDTO{}
}

// waitIdle polls until the in-flight slot is released (the row can go
// terminal a moment before execute's deferred finish runs).
func waitIdle(t *testing.T, m *BackupManager) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if !m.status().Running {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("backup manager never went idle")
}

func TestBackupConfigRoundTrip(t *testing.T) {
	m, database, _ := newTestBackupManager(t)

	// Defaults surface with the vault passphrase detected.
	got := m.getConfig()
	if got.Schedule.Kind != backup.ScheduleOff || !got.PassphraseSet {
		t.Fatalf("default config = %+v", got)
	}

	// Valid config persists and sets the schedule cursor.
	dto := m.getConfig()
	dto.Schedule = backup.ScheduleSpec{Kind: backup.ScheduleInterval, EveryHours: 6}
	if err := m.updateConfig(dto); err != nil {
		t.Fatal(err)
	}
	raw, ok, _ := database.KV().Get(backup.KVConfig)
	if !ok || !strings.Contains(raw, `"every_hours":6`) {
		t.Fatalf("KV config = %q", raw)
	}
	cur, ok, _ := database.KV().Get(backup.KVNextRun)
	if !ok {
		t.Fatal("next-run cursor missing after config update")
	}
	want := m.now().Add(6 * time.Hour).UTC().Format(time.RFC3339)
	if cur != want {
		t.Fatalf("cursor = %q, want %q", cur, want)
	}

	// Invalid config is rejected without touching KV.
	dto.Schedule.EveryHours = 99999
	if err := m.updateConfig(dto); err == nil {
		t.Fatal("invalid schedule must be rejected")
	}
	dto.Schedule = backup.ScheduleSpec{Kind: backup.ScheduleOff}
	if err := m.updateConfig(dto); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := database.KV().Get(backup.KVNextRun); ok {
		t.Fatal("cursor must be cleared when the schedule is off")
	}
}

func TestBackupRunNowLifecycle(t *testing.T) {
	m, database, dataDir := newTestBackupManager(t)

	jobID, err := m.runNow()
	if err != nil {
		t.Fatal(err)
	}
	done := waitSettled(t, m, jobID)
	if done.Status != "success" {
		t.Fatalf("status = %s (%s)", done.Status, done.Err)
	}
	if done.Trigger != "manual" || done.FinishedAt == "" || done.SizeBytes <= 0 {
		t.Fatalf("job = %+v", done)
	}
	if _, err := os.Stat(done.LocalPath); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
	// The archive landed in the default dir and the manifest is discoverable.
	if filepath.Dir(done.LocalPath) != filepath.Join(dataDir, "backups") {
		t.Fatalf("archive dir = %s", filepath.Dir(done.LocalPath))
	}
	if _, err := backup.Inspect(done.LocalPath, "test-passphrase"); err != nil {
		t.Fatalf("inspect: %v", err)
	}

	// Status shows the last job and reports idle.
	st := m.status()
	if st.Running || st.Last == nil || st.Last.ID != jobID {
		t.Fatalf("status = %+v", st)
	}
	_ = database
}

func TestBackupRunNowFailsWithoutPassphrase(t *testing.T) {
	m, database, _ := newTestBackupManager(t)
	if err := database.Secrets().Delete(backup.SecretPassphrase); err != nil {
		t.Fatal(err)
	}
	jobID, err := m.runNow()
	if err != nil {
		t.Fatal(err)
	}
	done := waitSettled(t, m, jobID)
	if done.Status != "failed" || !strings.Contains(done.Err, "passphrase") {
		t.Fatalf("job = %+v", done)
	}
}

func TestBackupConcurrentRunsRejected(t *testing.T) {
	m, _, _ := newTestBackupManager(t)
	// A claimed slot blocks a second begin.
	job, err := m.begin("manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.begin("scheduled"); err == nil {
		t.Fatal("second begin must be rejected while running")
	}
	m.finish(job)
	if _, err := m.begin("scheduled"); err != nil {
		t.Fatalf("begin after finish must succeed: %v", err)
	}
}

func TestBackupLoopFiresDueSchedule(t *testing.T) {
	m, database, _ := newTestBackupManager(t)

	dto := m.getConfig()
	dto.Schedule = backup.ScheduleSpec{Kind: backup.ScheduleInterval, EveryHours: 1}
	if err := m.updateConfig(dto); err != nil {
		t.Fatal(err)
	}
	// Drag the cursor into the past: the next tick must fire the missed run.
	past := m.now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	if err := database.KV().Set(backup.KVNextRun, past); err != nil {
		t.Fatal(err)
	}

	m.tick() // synchronous — fires the job in a goroutine
	st := m.status()
	if !st.Running {
		t.Fatal("tick should have started a job")
	}
	// 2h late is far beyond the grace window: a catch-up, not the cadence.
	if st.Current.Trigger != "catch-up" {
		t.Fatalf("missed-window trigger = %q, want catch-up", st.Current.Trigger)
	}
	// Cursor advanced to fixed+1h immediately (double-fire guard).
	cur, _, _ := database.KV().Get(backup.KVNextRun)
	want := m.now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	if cur != want {
		t.Fatalf("cursor = %q, want %q", cur, want)
	}

	// The job completes and a second tick does not re-fire (cursor future).
	waitSettled(t, m, st.Current.ID)
	waitIdle(t, m)
	if st.NextRunAt != want {
		t.Fatalf("status next run = %q, want %q", st.NextRunAt, want)
	}
	m.tick()
	time.Sleep(50 * time.Millisecond)
	if st2 := m.status(); st2.Running {
		t.Fatal("future cursor must not fire")
	}
}

func TestBackupLoopOffScheduleNeverFires(t *testing.T) {
	m, _, _ := newTestBackupManager(t)
	m.tick()
	if m.status().Running {
		t.Fatal("off schedule must not fire")
	}
}

// A loop tick only slightly past the due time is the normal cadence fire —
// trigger stays "scheduled" and no reconcile line is logged.
func TestBackupLoopWithinGraceIsScheduled(t *testing.T) {
	m, _, _ := newTestBackupManager(t)
	dto := m.getConfig()
	dto.Schedule = backup.ScheduleSpec{Kind: backup.ScheduleInterval, EveryHours: 1}
	if err := m.updateConfig(dto); err != nil {
		t.Fatal(err)
	}
	past := m.now().Add(-30 * time.Second).UTC().Format(time.RFC3339)
	if err := m.db.KV().Set(backup.KVNextRun, past); err != nil {
		t.Fatal(err)
	}

	m.tickReason("loop")
	st := m.status()
	if !st.Running || st.Current.Trigger != "scheduled" {
		t.Fatalf("in-grace loop tick = %+v, want a scheduled fire", st.Current)
	}
	waitSettled(t, m, st.Current.ID)
}

// Reconcile (wake/periodic/manual/startup) must catch up a missed backup
// window exactly once, label it "catch-up", log the missed window, and
// advance the cursor so the loop does not re-fire.
func TestBackupReconcileCatchUp(t *testing.T) {
	m, database, _ := newTestBackupManager(t)
	dto := m.getConfig()
	dto.Schedule = backup.ScheduleSpec{Kind: backup.ScheduleInterval, EveryHours: 1}
	if err := m.updateConfig(dto); err != nil {
		t.Fatal(err)
	}
	past := m.now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	if err := database.KV().Set(backup.KVNextRun, past); err != nil {
		t.Fatal(err)
	}

	m.Reconcile("manual")
	st := m.status()
	if !st.Running || st.Current == nil || st.Current.Trigger != "catch-up" {
		t.Fatalf("reconcile did not start a catch-up job: %+v", st)
	}
	want := m.now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	if cur, _, _ := database.KV().Get(backup.KVNextRun); cur != want {
		t.Fatalf("cursor = %q, want %q", cur, want)
	}
	// The missed window is visible in the system log.
	logs, _ := database.Logs().List(20)
	found := false
	for _, l := range logs {
		if l.Event == "backup" && strings.Contains(l.Message, "was missed — starting catch-up run") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no reconcile log for the missed window: %+v", logs)
	}

	// The job settles successfully; a loop tick right after does not re-fire.
	done := waitSettled(t, m, st.Current.ID)
	if done.Status != "success" || done.Trigger != "catch-up" {
		t.Fatalf("job = %+v", done)
	}
	m.tickReason("loop")
	time.Sleep(50 * time.Millisecond)
	if st2 := m.status(); st2.Running {
		t.Fatal("future cursor must not re-fire after reconcile")
	}
}

func TestBackupFailInterrupted(t *testing.T) {
	m, database, _ := newTestBackupManager(t)
	ghost := db.BackupJob{ID: "ghost", Trigger: "scheduled", Status: "running", StartedAt: db.Now()}
	if err := database.Backups().Insert(ghost); err != nil {
		t.Fatal(err)
	}
	m.failInterrupted()
	got, err := database.Backups().Get("ghost")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || !strings.Contains(got.Error, "interrupted") {
		t.Fatalf("ghost job = %+v", got)
	}
}

func TestBackupPruneLocal(t *testing.T) {
	m, _, dataDir := newTestBackupManager(t)
	outDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := []string{
		"heka-backup-20260901-000000.zip", "heka-backup-20260902-000000.zip",
		"heka-backup-20260903-000000.zip", "pre-restore-20260903-100000.zip",
		"notes.txt",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(outDir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m.pruneLocal(outDir, 2)
	remaining, _ := os.ReadDir(outDir)
	var names []string
	for _, e := range remaining {
		names = append(names, e.Name())
	}
	// Only the newest 2 backups are kept; foreign files untouched
	// (2 kept + pre-restore + notes.txt = 4).
	if len(names) != 4 ||
		!contains(names, "heka-backup-20260903-000000.zip") ||
		!contains(names, "heka-backup-20260902-000000.zip") ||
		!contains(names, "pre-restore-20260903-100000.zip") {
		t.Fatalf("prune kept %v", names)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestBackupTestDestinations(t *testing.T) {
	m, _, dataDir := newTestBackupManager(t)
	res := m.test()
	if res.Local == nil || !res.Local.OK || res.Local.Path != filepath.Join(dataDir, "backups") {
		t.Fatalf("local test = %+v", res.Local)
	}
	if res.S3 != nil {
		t.Fatal("S3 test must be skipped when unconfigured")
	}
}

func TestSecretsUsage(t *testing.T) {
	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	seed := map[string]string{
		"uses-env":  `{"version":1,"name":"A","slug":"uses-env","type":"script","timeout":30,"environment":{"TOKEN":"${API_TOKEN}","PLAIN":"x"}}`,
		"uses-hook": `{"version":1,"name":"B","slug":"uses-hook","type":"script","timeout":30,"notify":{"webhooks":[{"format":"telegram","url":"https://t.me/bot${BOT_TOKEN}/send","chat_id":"${CHAT_ID}"}]}}`,
		"uses-none": `{"version":1,"name":"C","slug":"uses-none","type":"script","timeout":30}`,
		"broken":    `{not json`,
	}
	for slug, parsed := range seed {
		if err := database.Tasks().Save(db.Task{
			ID: slug, Slug: slug, Name: slug, ParsedJSON: parsed,
			Enabled: true, CreatedAt: db.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := secretsUsage(database)
	if err != nil {
		t.Fatal(err)
	}
	if got := usage["API_TOKEN"]; len(got) != 1 || got[0] != "uses-env" {
		t.Fatalf("API_TOKEN usage = %v", got)
	}
	if got := usage["BOT_TOKEN"]; len(got) != 1 || got[0] != "uses-hook" {
		t.Fatalf("BOT_TOKEN usage = %v", got)
	}
	if got := usage["CHAT_ID"]; len(got) != 1 || got[0] != "uses-hook" {
		t.Fatalf("CHAT_ID usage = %v", got)
	}
	if _, ok := usage["PLAIN"]; ok {
		t.Fatal("non-reference values must not appear in the usage map")
	}
	if _, ok := usage["uses-none"]; ok {
		t.Fatal("task slugs must not be keys of the usage map")
	}
}
