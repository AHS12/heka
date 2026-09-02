package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"heka/internal/config"
	"heka/internal/core/executor"
	"heka/internal/core/task"
	"heka/internal/db"
)

// TestMain isolates this package's tests onto their own named pipe so
// parallel package runs (ipc + daemon + CLI) never collide on the shared
// per-user endpoint.
func TestMain(m *testing.M) {
	_ = os.Setenv("HEKA_PIPE_NAME", fmt.Sprintf("heka-ipc-test-%d", os.Getpid()))
	os.Exit(m.Run())
}

// fakeRunner is the SPEC-07 §7 seam: records Start/Cancel, never spawns.
type fakeRunner struct {
	mu        sync.Mutex
	started   []string // slugs
	lastOpt   executor.Options
	lastCtx   context.Context
	busy      bool
	cancelErr error
}

func (f *fakeRunner) Start(ctx context.Context, t *task.Task, opt executor.Options) (*executor.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy {
		return nil, executor.ErrAlreadyRunning
	}
	f.started = append(f.started, t.Slug)
	f.lastOpt = opt
	f.lastCtx = ctx
	done := make(chan struct{})
	return &executor.Handle{GroupID: "group-fake", Done: done}, nil
}

func (f *fakeRunner) Cancel(slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancelErr != nil {
		return f.cancelErr
	}
	return nil
}

// startTestServer serves a real server over the real platform transport.
func startTestServer(t *testing.T, deps Deps) config.Config {
	t.Helper()
	cfg, err := config.Load(map[string]string{"LOCALAPPDATA": t.TempDir()}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ln, err := Listen(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	srv := &http.Server{Handler: NewServer(deps).Handler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	return cfg
}

func openDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func seedTask(t *testing.T, d *db.DB, slug, name string, enabled bool) {
	t.Helper()
	tk := task.Task{Version: 1, Slug: slug, Name: name, Type: "script", Runtime: "custom", Script: "x.sh"}
	parsed, _ := json.Marshal(tk)
	if err := d.Tasks().Save(db.Task{
		ID: "id-" + slug, Slug: slug, Name: name, YAMLPath: "/tasks/" + slug + ".yaml",
		ParsedJSON: string(parsed), Enabled: enabled, CreatedAt: db.Now(), UpdatedAt: db.Now(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHealthRoundTrip(t *testing.T) {
	database := openDB(t)
	runner := &fakeRunner{}
	cfg := startTestServer(t, Deps{
		Health: func() Health {
			return Health{Version: "t", UptimeSeconds: 3, Core: "healthy", Scheduler: "starting", LastHeartbeat: time.Now()}
		},
		Tasks: database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: runner,
	})
	h, err := NewClient(cfg).Health()
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != "t" || h.Core != "healthy" || h.UptimeSeconds < 0 {
		t.Fatalf("health = %+v", h)
	}
}

func TestTasksListAndGet(t *testing.T) {
	database := openDB(t)
	seedTask(t, database, "alpha", "Alpha", true)
	seedTask(t, database, "beta", "Beta", false)
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: &fakeRunner{},
	})
	client := NewClient(cfg)

	list, err := client.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Slug != "alpha" || !list[0].Enabled {
		t.Fatalf("list = %+v", list)
	}

	detail, err := client.GetTask("beta")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.Name != "Beta" || detail.Enabled {
		t.Fatalf("detail = %+v", detail)
	}

	_, err = client.GetTask("nope")
	var ipcErr *Error
	if !errors.As(err, &ipcErr) || ipcErr.Code != "not_found" {
		t.Fatalf("missing task err = %v, want not_found envelope", err)
	}
}

func TestSystemLogRoute(t *testing.T) {
	database := openDB(t)
	if err := database.Logs().Add("info", "reconcile", "reconcile (manual): checked 1 schedule(s), 0 caught up"); err != nil {
		t.Fatal(err)
	}
	if err := database.Logs().Add("warn", "scheduler", `schedule "x": start failed`); err != nil {
		t.Fatal(err)
	}
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(),
		Logs:   database.Logs(), Runner: &fakeRunner{},
	})
	client := NewClient(cfg)
	logs, err := client.SystemLog(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("logs = %+v, want 2 entries newest first", logs)
	}
	if logs[0].Level != "warn" || logs[0].Event != "scheduler" {
		t.Fatalf("newest entry = %+v", logs[0])
	}
}

func TestRunAndConflict(t *testing.T) {
	database := openDB(t)
	seedTask(t, database, "alpha", "Alpha", true)
	runner := &fakeRunner{}
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: runner,
	})
	client := NewClient(cfg)

	resp, err := client.RunTask("alpha", "cli")
	if err != nil {
		t.Fatal(err)
	}
	if resp.GroupID != "group-fake" || resp.Status != "running" {
		t.Fatalf("run resp = %+v", resp)
	}
	if len(runner.started) != 1 || runner.started[0] != "alpha" {
		t.Fatalf("runner saw %v", runner.started)
	}

	// Busy → 409 conflict envelope.
	runner.busy = true
	var ipcErr *Error
	_, err = client.RunTask("alpha", "")
	if !errors.As(err, &ipcErr) || ipcErr.Code != "conflict" {
		t.Fatalf("conflict err = %v", err)
	}
	runner.busy = false

	// Unknown slug → 404.
	ipcErr = nil
	_, err = client.RunTask("ghost", "")
	if !errors.As(err, &ipcErr) || ipcErr.Code != "not_found" {
		t.Fatalf("run ghost err = %v", err)
	}
}

func TestEnableDisableCancel(t *testing.T) {
	database := openDB(t)
	seedTask(t, database, "alpha", "Alpha", true)
	runner := &fakeRunner{}
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: runner,
	})
	client := NewClient(cfg)

	if err := client.Disable("alpha"); err != nil {
		t.Fatal(err)
	}
	row, _ := database.Tasks().Get("alpha")
	if row.Enabled {
		t.Fatal("disable did not stick")
	}
	if err := client.Enable("alpha"); err != nil {
		t.Fatal(err)
	}
	row, _ = database.Tasks().Get("alpha")
	if !row.Enabled {
		t.Fatal("enable did not stick")
	}

	if err := client.Cancel("alpha"); err != nil {
		t.Fatal(err)
	}
	runner.cancelErr = executor.ErrNotRunning
	err := client.Cancel("alpha")
	var ipcErr *Error
	if !errors.As(err, &ipcErr) || ipcErr.Code != "not_found" {
		t.Fatalf("cancel idle err = %v", err)
	}
}

func TestRunsListAndDetail(t *testing.T) {
	database := openDB(t)
	seedTask(t, database, "alpha", "Alpha", true)
	finished := "2026-08-25T08:00:17Z"
	exit := 0
	if err := database.Runs().Create(db.Run{
		RunID: "run-1", GroupID: "grp-1", Attempt: 0, TaskSlug: "alpha",
		Trigger: "manual", Status: "success", StartedAt: &finished, FinishedAt: &finished,
		ExitCode: &exit, Stdout: "out", CreatedAt: db.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: &fakeRunner{},
	})
	client := NewClient(cfg)

	runs, err := client.TaskRuns("alpha", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].RunID != "run-1" || runs[0].Status != "success" {
		t.Fatalf("runs = %+v", runs)
	}

	detail, err := client.Run("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.TaskSlug != "alpha" || detail.Stdout != "out" || detail.ExitCode != 0 {
		t.Fatalf("detail = %+v", detail)
	}

	_, err = client.Run("missing")
	var ipcErr *Error
	if !errors.As(err, &ipcErr) || ipcErr.Code != "not_found" {
		t.Fatalf("missing run err = %v", err)
	}
}

func TestMethodAndRouteErrors(t *testing.T) {
	database := openDB(t)
	var ipcErr *Error
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: &fakeRunner{},
	})
	client := NewClient(cfg)

	// Wrong method → 405 envelope.
	err := client.do("POST", "/v1/health", nil, nil)
	if errors.As(err, &ipcErr) && ipcErr.Code != "method_not_allowed" {
		t.Fatalf("method err = %v", err)
	}

	// Unknown route → 404 envelope.
	err = client.do("GET", "/v1/nope", nil, nil)
	if !errors.As(err, &ipcErr) || ipcErr.Code != "not_found" {
		t.Fatalf("route err = %v", err)
	}

	// Tasks are fully live now: POST with a non-YAML body → 422 invalid_task.
	err = client.do("POST", "/v1/tasks", map[string]string{}, nil)
	if !errors.As(err, &ipcErr) || ipcErr.Code != "invalid_task" {
		t.Fatalf("task write err = %v", err)
	}
	// Secrets are live: wrong method → 405.
	err = client.do("POST", "/v1/secrets", map[string]string{}, nil)
	if !errors.As(err, &ipcErr) || ipcErr.Code != "method_not_allowed" {
		t.Fatalf("secrets method err = %v", err)
	}
}

func TestPanicBecomes500Envelope(t *testing.T) {
	database := openDB(t)
	cfg := startTestServer(t, Deps{
		Health: func() Health { panic("boom") },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: &fakeRunner{},
	})
	_, err := NewClient(cfg).Health()
	var ipcErr *Error
	if !errors.As(err, &ipcErr) || ipcErr.Code != "internal" {
		t.Fatalf("panic err = %v", err)
	}
}

func TestClientDaemonNotRunning(t *testing.T) {
	cfg, err := config.Load(map[string]string{"LOCALAPPDATA": t.TempDir()}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewClient(cfg).Health()
	if !errors.Is(err, ErrDaemonNotRunning) {
		t.Fatalf("err = %v, want ErrDaemonNotRunning", err)
	}
}

func TestRunSurvivesRequestContext(t *testing.T) {
	// Regression: POST run must not pass the request context through — the
	// http server cancels it when the response ends, which used to kill the
	// background run immediately (caught by the live SPEC-08 smoke).
	database := openDB(t)
	seedTask(t, database, "alpha", "Alpha", true)
	runner := &fakeRunner{}
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: runner,
	})
	if _, err := NewClient(cfg).RunTask("alpha", "cli"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.lastCtx == nil {
		t.Fatal("runner received no context")
	}
	if err := runner.lastCtx.Err(); err != nil {
		t.Fatalf("run context was cancelled after the response: %v", err)
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	database := openDB(t)
	seedTask(t, database, "alpha", "Alpha", true)
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks:  database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(), Runner: &fakeRunner{},
	})
	client := NewClient(cfg)
	big := `{"trigger":"` + strings.Repeat("x", 2<<20) + `"}`
	err := client.do("POST", "/v1/tasks/alpha/run", strings.NewReader(big), nil)
	var ipcErr *Error
	if !errors.As(err, &ipcErr) || ipcErr.Code != "request_too_large" {
		t.Fatalf("oversize err = %v", err)
	}
}

func TestShutdownCallsDeps(t *testing.T) {
	database := openDB(t)
	var mu sync.Mutex
	shutdownCalled := false
	cfg := startTestServer(t, Deps{
		Health:   func() Health { return Health{Core: "healthy"} },
		Tasks:    database.Tasks(),
		Runs:     database.Runs(),
		Runner:   &fakeRunner{},
		Shutdown: func() { mu.Lock(); shutdownCalled = true; mu.Unlock() },
	})
	if err := NewClient(cfg).Shutdown(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	done := shutdownCalled
	mu.Unlock()
	if !done {
		t.Fatal("Shutdown dep was not invoked")
	}
}

func TestSchedulesCRUD(t *testing.T) {
	database := openDB(t)
	seedTask(t, database, "alpha", "Alpha", true)
	syncCalls := 0
	cfg := startTestServer(t, Deps{
		Health:        func() Health { return Health{Core: "healthy"} },
		Tasks:         database.Tasks(),
		Runs:          database.Runs(),
		Schedules:     database.Schedules(),
		SyncSchedules: func() error { syncCalls++; return nil },
		Runner:        &fakeRunner{},
	})
	client := NewClient(cfg)
	var ipcErr *Error

	// Invalid cron → 400.
	_, err := client.CreateSchedule(Schedule{Slug: "bad", TaskSlug: "alpha", Kind: "recurring", Cron: "nope"})
	if !errors.As(err, &ipcErr) || ipcErr.Code != "bad_request" {
		t.Fatalf("bad cron err = %v", err)
	}
	// Unknown task → 404.
	_, err = client.CreateSchedule(Schedule{Slug: "ok", TaskSlug: "ghost", Kind: "recurring", Cron: "@daily"})
	if !errors.As(err, &ipcErr) || ipcErr.Code != "not_found" {
		t.Fatalf("unknown task err = %v", err)
	}
	// Past one-time → 400.
	_, err = client.CreateSchedule(Schedule{
		Slug: "past", TaskSlug: "alpha", Kind: "onetime",
		RunAt: time.Now().Add(-time.Hour).Format(time.RFC3339),
	})
	if !errors.As(err, &ipcErr) || ipcErr.Code != "bad_request" {
		t.Fatalf("past onetime err = %v", err)
	}

	// Happy path.
	created, err := client.CreateSchedule(Schedule{
		Slug: "nightly", TaskSlug: "alpha", Kind: "recurring", Cron: "@daily", MissedPolicy: "run_now",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !created.Enabled || created.MissedPolicy != "run_now" {
		t.Fatalf("created = %+v", created)
	}
	if syncCalls == 0 {
		t.Fatal("create did not trigger registry sync")
	}

	list, err := client.ListSchedules()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v (%v)", list, err)
	}

	// Toggle.
	if err := client.DisableSchedule(created.ID); err != nil {
		t.Fatal(err)
	}
	row, _ := database.Schedules().Get(created.ID)
	if row.Enabled {
		t.Fatal("disable did not stick")
	}
	if err := client.EnableSchedule(created.ID); err != nil {
		t.Fatal(err)
	}

	// Delete clears the registry too (Sync is called again).
	if err := client.DeleteSchedule(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Schedules().Get(created.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("schedule still exists: %v", err)
	}
	if syncCalls < 2 {
		t.Fatalf("delete did not re-sync (calls=%d)", syncCalls)
	}

	// Delete of a missing schedule → 404.
	if err := client.DeleteSchedule("nope"); !errors.As(err, &ipcErr) || ipcErr.Code != "not_found" {
		t.Fatalf("delete missing err = %v", err)
	}
}

func TestReconcileSchedulesEndpoint(t *testing.T) {
	database := openDB(t)
	calls := 0
	cfg := startTestServer(t, Deps{
		Health:    func() Health { return Health{Core: "healthy"} },
		Tasks:     database.Tasks(),
		Runs:      database.Runs(),
		Schedules: database.Schedules(),
		Runner:    &fakeRunner{},
		Reconcile: func() error { calls++; return nil },
	})
	client := NewClient(cfg)

	if err := client.ReconcileSchedules(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("reconcile calls = %d, want 1", calls)
	}

	// GET must be rejected (POST only).
	var ipcErr *Error
	if _, err := client.RawGet("/v1/schedules/reconcile"); !errors.As(err, &ipcErr) || ipcErr.Code != "method_not_allowed" {
		t.Fatalf("GET method err = %v", err)
	}
}

func TestReconcileSchedulesRejectedWhilePaused(t *testing.T) {
	database := openDB(t)
	calls := 0
	paused := true
	cfg := startTestServer(t, Deps{
		Health:    func() Health { return Health{Core: "healthy"} },
		Tasks:     database.Tasks(),
		Runs:      database.Runs(),
		Schedules: database.Schedules(),
		Runner:    &fakeRunner{},
		Reconcile: func() error { calls++; return nil },
		IsPaused:  func() bool { return paused },
	})
	client := NewClient(cfg)
	var ipcErr *Error

	if err := client.ReconcileSchedules(); !errors.As(err, &ipcErr) || ipcErr.Code != "scheduler_paused" {
		t.Fatalf("paused reconcile err = %v, want scheduler_paused", err)
	}
	if calls != 0 {
		t.Fatalf("reconcile calls while paused = %d, want 0", calls)
	}

	// Resuming unblocks the endpoint.
	paused = false
	if err := client.ReconcileSchedules(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("reconcile calls after resume = %d, want 1", calls)
	}
}

func TestSecretsE2E(t *testing.T) {
	database := openDB(t)
	cfg := startTestServer(t, Deps{
		Health:  func() Health { return Health{Core: "healthy"} },
		Tasks:   database.Tasks(),
		Runs:    database.Runs(),
		Secrets: database.Secrets(),
		Runner:  &fakeRunner{},
	})
	client := NewClient(cfg)
	var ipcErr *Error

	if err := client.SetSecret("OPENROUTER_API_KEY", "sk-real-secret"); err != nil {
		t.Fatal(err)
	}
	if err := client.SetSecret("SLACK_WEBHOOK_URL", "https://hooks.slack.com/x"); err != nil {
		t.Fatal(err)
	}

	keys, err := client.ListSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "OPENROUTER_API_KEY" {
		t.Fatalf("keys = %v", keys)
	}
	// Values must never appear on the wire.
	raw, err := client.RawGet("/v1/secrets")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("sk-real-secret")) || bytes.Contains(raw, []byte("sk ")) {
		t.Fatalf("values leaked: %s", raw)
	}

	// Invalid keys rejected.
	if err := client.SetSecret("not-a-key", "v"); !errors.As(err, &ipcErr) || ipcErr.Code != "bad_request" {
		t.Fatalf("invalid key err = %v", err)
	}
	// Empty key never reaches the handler (no route); still an error, never ok.
	if err := client.SetSecret("", "v"); err == nil {
		t.Fatal("empty key must error")
	}

	// Delete removes; delete of missing is idempotent.
	if err := client.DeleteSecret("OPENROUTER_API_KEY"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteSecret("OPENROUTER_API_KEY"); err != nil {
		t.Fatal(err)
	}
	keys, _ = client.ListSecrets()
	if len(keys) != 1 {
		t.Fatalf("after delete keys = %v", keys)
	}
}

// ----- SPEC-13: task CRUD over the wire -----

// testFS is a real-filesystem TaskFilesystem for the CRUD e2e — mirrors
// internal/daemon.taskFS without importing the daemon (which imports ipc).
type testFS struct{ dir string }

func (f testFS) Parse(yaml []byte) (task.Task, error) { return task.Parse(yaml) }
func (f testFS) Write(t task.Task) error              { return task.Save(f.dir, t) }
func (f testFS) Remove(slug string) error {
	err := task.Delete(f.dir, slug)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return db.ErrNotFound
	}
	return err
}

func (f testFS) Load(slug string) (string, error) {
	data, err := os.ReadFile(filepath.Join(f.dir, slug+".yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", db.ErrNotFound
		}
		return "", err
	}
	return string(data), nil
}

// syncTasksToIndex re-scans a tasks dir into the index — the daemon's
// syncTasks() distilled to the rows the handlers depend on.
func syncTasksToIndex(t *testing.T, d *db.DB, dir string) func() error {
	t.Helper()
	return func() error {
		tasks, _ := task.Scan(dir)
		for _, tk := range tasks {
			parsed, _ := json.Marshal(tk)
			row := db.Task{
				ID: "id-" + tk.Slug, Slug: tk.Slug, Name: tk.Name,
				YAMLPath:   filepath.Join(dir, tk.Slug+".yaml"),
				ParsedJSON: string(parsed), Enabled: true,
				CreatedAt: db.Now(), UpdatedAt: db.Now(),
			}
			if ex, err := d.Tasks().Get(tk.Slug); err == nil {
				row.ID = ex.ID
				row.CreatedAt = ex.CreatedAt
				row.Enabled = ex.Enabled
			}
			if err := d.Tasks().Save(row); err != nil {
				return err
			}
		}
		return nil
	}
}

func startTasksServer(t *testing.T) (config.Config, *db.DB, string) {
	t.Helper()
	database := openDB(t)
	dir := t.TempDir()
	cfg := startTestServer(t, Deps{
		Health:    func() Health { return Health{Core: "healthy", LastHeartbeat: time.Now()} },
		Tasks:     database.Tasks(),
		Runs:      database.Runs(),
		Schedules: database.Schedules(),
		TaskFiles: testFS{dir: dir},
		SyncTasks: syncTasksToIndex(t, database, dir),
		Runner:    &fakeRunner{},
	})
	return cfg, database, dir
}

const goodYAML = `version: 1
slug: backup
name: backup
type: script
runtime: custom
script: backup.sh
`

func TestTaskCreateUpdateRead(t *testing.T) {
	cfg, database, dir := startTasksServer(t)
	client := NewClient(cfg)

	created, err := client.CreateTask(goodYAML)
	if err != nil {
		t.Fatal(err)
	}
	if created.Task.Slug != "backup" || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}
	// File on disk + index row present.
	if _, err := os.Stat(filepath.Join(dir, "backup.yaml")); err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if _, err := database.Tasks().Get("backup"); err != nil {
		t.Fatalf("index row missing: %v", err)
	}

	// Duplicate create → 409.
	var ipcErr *Error
	_, err = client.CreateTask(goodYAML)
	if !errors.As(err, &ipcErr) || ipcErr.Code != "conflict" {
		t.Fatalf("dup create err = %v", err)
	}

	// Update replaces name; slug must match.
	renamed, err := client.UpdateTask("backup",
		"version: 1\nslug: backup\nname: nightly-backup\ntype: script\nruntime: custom\nscript: backup.sh\n")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Task.Name != "nightly-backup" {
		t.Fatalf("renamed = %+v", renamed)
	}
	_, err = client.UpdateTask("backup", "version: 1\nslug: mismatched\nname: other\ntype: script\nruntime: custom\nscript: x.sh\n")
	if !errors.As(err, &ipcErr) || ipcErr.Code != "invalid_task" {
		t.Fatalf("slug mismatch err = %v", err)
	}
	_, err = client.UpdateTask("ghost", goodYAML)
	if !errors.As(err, &ipcErr) || ipcErr.Code != "not_found" {
		t.Fatalf("update missing err = %v", err)
	}

	// Raw YAML round-trip through the editor endpoint: canonical text carries
	// the updated name and the schema-mandated fields.
	text, err := client.TaskYAML("backup")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "slug: backup") || !strings.Contains(text, "name: nightly-backup") {
		t.Fatalf("yaml = %q", text)
	}
}

func TestTaskCreateValidation422(t *testing.T) {
	cfg, _, _ := startTasksServer(t)
	client := NewClient(cfg)
	var ipcErr *Error

	// Two violations at once: unknown runtime + missing script field.
	_, err := client.CreateTask("version: 1\nslug: broken\ntype: script\nruntime: not-a-runtime\n")
	if !errors.As(err, &ipcErr) || ipcErr.Code != "invalid_task" {
		t.Fatalf("err = %v", err)
	}
	var list []string
	if err := json.Unmarshal([]byte(ipcErr.Message), &list); err != nil || len(list) < 2 {
		t.Fatalf("422 list = %v (err %v)", ipcErr.Message, err)
	}
}

func TestTaskDelete(t *testing.T) {
	cfg, database, dir := startTasksServer(t)
	client := NewClient(cfg)
	if _, err := client.CreateTask(goodYAML); err != nil {
		t.Fatal(err)
	}
	// Pre-create run history that must survive deletion.
	if err := database.Runs().Create(db.Run{
		RunID: "run-hist", GroupID: "g", TaskSlug: "backup", Trigger: "manual",
		Status: "success", CreatedAt: db.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	// A schedule referencing the task blocks deletion with the slug named.
	if err := database.Schedules().Save(db.Schedule{
		ID: "sch-1", Slug: "nightly", TaskSlug: "backup", Kind: "recurring", Cron: "0 * * * *",
	}); err != nil {
		t.Fatal(err)
	}
	var ipcErr *Error
	err := client.DeleteTask("backup")
	if !errors.As(err, &ipcErr) || ipcErr.Code != "conflict" || !strings.Contains(ipcErr.Message, "nightly") {
		t.Fatalf("delete blocked err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backup.yaml")); err != nil {
		t.Fatalf("file must survive blocked delete")
	}

	// Free the task and delete for real.
	if err := database.Schedules().Delete("sch-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteTask("backup"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backup.yaml")); !os.IsNotExist(err) {
		t.Fatalf("file still present after delete: %v", err)
	}
	if _, err := database.Tasks().Get("backup"); err == nil {
		t.Fatal("index row still present")
	}
	// History survives with the task gone.
	runs, err := client.TaskRuns("backup", 10)
	if err != nil || len(runs) != 1 || runs[0].RunID != "run-hist" {
		t.Fatalf("history lost: %+v (err %v)", runs, err)
	}
}

func TestTaskListLastRunJoin(t *testing.T) {
	cfg, database, _ := startTasksServer(t)
	client := NewClient(cfg)

	seed := func(slug, name string) {
		tk := task.Task{Version: 1, Slug: slug, Name: name, Type: "script", Runtime: "custom", Script: "x.sh"}
		parsed, _ := json.Marshal(tk)
		if err := database.Tasks().Save(db.Task{ID: "id-" + slug, Slug: slug, Name: name,
			YAMLPath: "/tasks/" + slug + ".yaml", ParsedJSON: string(parsed), Enabled: true,
			CreatedAt: db.Now(), UpdatedAt: db.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	seed("has-runs", "Has Runs")
	seed("no-runs", "No Runs")

	now := db.Now()
	if err := database.Runs().Create(db.Run{RunID: "r1", GroupID: "g1", TaskSlug: "has-runs",
		Trigger: "manual", Status: "failed", StartedAt: &now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := database.Runs().Create(db.Run{RunID: "r2", GroupID: "g2", TaskSlug: "has-runs",
		Trigger: "schedule", Status: "success", StartedAt: &now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	summaries, err := client.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]TaskSummary{}
	for _, s := range summaries {
		bySlug[s.Slug] = s
	}
	if s := bySlug["has-runs"]; s.LastStatus != "success" || s.LastRunAt == "" {
		t.Fatalf("has-runs summary = %+v", s)
	}
	if s := bySlug["no-runs"]; s.LastStatus != "" || s.LastRunAt != "" {
		t.Fatalf("no-runs summary = %+v", s)
	}
}

func TestTaskValidateEndpoint(t *testing.T) {
	cfg, _, _ := startTasksServer(t)
	client := NewClient(cfg)

	errs, err := client.ValidateTaskYAML(goodYAML)
	if err != nil || len(errs) != 0 {
		t.Fatalf("valid yaml errs = %v (%v)", errs, err)
	}
	errs, err = client.ValidateTaskYAML("type: script\n")
	if err != nil || len(errs) == 0 {
		t.Fatalf("invalid yaml errs = %v (%v)", errs, err)
	}
}
