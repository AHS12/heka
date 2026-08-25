package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"heka/internal/config"
	"heka/internal/core/executor"
	"heka/internal/core/task"
	"heka/internal/db"
)

// TestMain isolates this package's tests onto their own named pipe (parallel
// package runs would otherwise collide on the shared per-user endpoint).
func TestMain(m *testing.M) {
	_ = os.Setenv("HEKA_PIPE_NAME", fmt.Sprintf("heka-daemon-test-%d", os.Getpid()))
	os.Exit(m.Run())
}

// testConfig returns a config rooted at a fresh temp dir, so tests never
// touch the real data location or collide with a user daemon's files.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(map[string]string{"LOCALAPPDATA": t.TempDir()}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// startRun launches Run in a goroutine and waits until it answers pings.
func startRun(t *testing.T, cfg config.Config) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		err := Run(cfg, "test-version")
		done <- err
		close(done)
	}()
	deadline := time.Now().Add(8 * time.Second)
	for {
		if _, err := Status(cfg); err == nil {
			return done
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRunLifecycle(t *testing.T) {
	cfg := testConfig(t)
	done := startRun(t, cfg)
	t.Cleanup(func() { _ = Stop(cfg) })

	h, err := Status(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != "test-version" || h.Core != "healthy" {
		t.Fatalf("health = %+v", h)
	}
	// Heartbeat is fresh (written at startup, refreshed by the loop).
	if age := time.Since(h.LastHeartbeat); age > 10*time.Second {
		t.Fatalf("heartbeat age = %v, want fresh", age)
	}

	if err := Stop(cfg); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after stop")
	}
	if _, err := Status(cfg); err == nil {
		t.Fatal("daemon still answers after stop")
	}
}

func TestSecondRunRejected(t *testing.T) {
	cfg := testConfig(t)
	done := startRun(t, cfg)
	t.Cleanup(func() { _ = Stop(cfg) })

	err := Run(cfg, "second") // synchronous: must fail on the endpoint bind
	if err == nil {
		t.Fatal("second Run succeeded, want bind error")
	}
	_ = Stop(cfg)
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("first daemon did not exit")
	}
}

func TestHeartbeatPersisted(t *testing.T) {
	cfg := testConfig(t)
	done := startRun(t, cfg)
	t.Cleanup(func() { _ = Stop(cfg) })

	// The daemon owns the DB; read the heartbeat the daemon wrote.
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	hb, ok, err := database.KV().Get("heartbeat")
	if err != nil || !ok {
		t.Fatalf("heartbeat missing: %q %v %v", hb, ok, err)
	}
	when, err := time.Parse(time.RFC3339, hb)
	if err != nil {
		t.Fatalf("heartbeat not RFC3339: %v", err)
	}
	if age := time.Since(when); age > 10*time.Second {
		t.Fatalf("heartbeat age = %v", age)
	}
	_ = Stop(cfg)
	<-done
}

func TestSyncTasks(t *testing.T) {
	cfg := testConfig(t)
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	d := newDaemon(cfg, "test", database)

	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(cfg.TasksDir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("alpha.yaml", "version: 1\nname: Alpha\nslug: alpha\nscript: a.sh\ntype: script\n")
	write("beta.yaml", "version: 1\nname: Beta\nslug: beta\nscript: b.sh\ntype: script\n")
	write("broken.yaml", "version: 1\nname: Broken\nslug: broken\n") // missing script → invalid

	d.syncTasks()

	// Healthy tasks indexed; broken one skipped.
	tasks, err := database.Tasks().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("index size = %d, want 2 (got %+v)", len(tasks), tasks)
	}

	// Enabled survives re-scans (SPEC-06 §5).
	if err := database.Tasks().SetEnabled("alpha", false); err != nil {
		t.Fatal(err)
	}
	d.syncTasks()
	alpha, err := database.Tasks().Get("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if alpha.Enabled {
		t.Fatal("re-scan clobbered enabled state")
	}

	// Deleted file → index row removed; others untouched.
	if err := os.Remove(filepath.Join(cfg.TasksDir, "alpha.yaml")); err != nil {
		t.Fatal(err)
	}
	d.syncTasks()
	if _, err := database.Tasks().Get("alpha"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("alpha still indexed: %v", err)
	}
	if _, err := database.Tasks().Get("beta"); err != nil {
		t.Fatalf("beta dropped: %v", err)
	}
}

// TestShutdownCancelsInFlight uses a long-running real process (ping -t on
// Windows, sleep elsewhere) so no helper harness is needed.
func TestShutdownCancelsInFlight(t *testing.T) {
	cfg := testConfig(t)
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	d := newDaemon(cfg, "test", database)
	d.exec = executor.New(database, 1<<20, 500*time.Millisecond, nil, "")
	d.execCtx, d.cancelExec = context.WithCancel(context.Background())
	if err := os.MkdirAll(cfg.TasksDir, 0o700); err != nil {
		t.Fatal(err)
	}

	cmd := "sleep"
	args := []string{"60"}
	if runtime.GOOS == "windows" {
		cmd = "ping"
		args = []string{"-t", "127.0.0.1"}
	}
	bin, err := exec.LookPath(cmd)
	if err != nil {
		t.Skipf("%s not available: %v", cmd, err)
	}
	tk := &task.Task{
		Version: 1, Name: "Long", Slug: "long",
		Type: "binary", Command: bin, Args: args,
	}
	h, err := d.exec.Start(context.Background(), tk, executor.Options{BaseDir: cfg.TasksDir})
	if err != nil {
		t.Fatal(err)
	}
	if d.exec.Active() != 1 {
		t.Fatalf("active = %d, want 1", d.exec.Active())
	}
	h.Cancel()

	d.shutdownAll()
	if d.exec.Active() != 0 {
		t.Fatalf("active after shutdown = %d, want 0", d.exec.Active())
	}

	runs, err := database.Runs().ListByTask("long", 5)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	if runs[0].Status != "cancelled" {
		t.Fatalf("status = %s, want cancelled: %+v", runs[0].Status, runs[0])
	}
}

func TestDaemonHealthScheduler(t *testing.T) {
	cfg := testConfig(t)
	// Seed a task + schedule before the daemon starts (its Sync picks them up).
	if err := os.MkdirAll(cfg.TasksDir, 0o700); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Tasks().Save(db.Task{
		ID: "id-daily", Slug: "daily", Name: "Daily",
		YAMLPath:   filepath.Join(cfg.TasksDir, "daily.yaml"),
		ParsedJSON: `{"version":1,"name":"Daily","slug":"daily","type":"script","runtime":"custom","script":"d.sh"}`,
		Enabled:    true, CreatedAt: db.Now(), UpdatedAt: db.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Schedules().Save(db.Schedule{
		ID: "s1", Slug: "daily-8am", TaskSlug: "daily", Kind: "recurring",
		Cron: "0 8 * * *", Enabled: true, MissedPolicy: "skip", CreatedAt: db.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	database.Close()

	done := startRun(t, cfg)
	t.Cleanup(func() { _ = Stop(cfg) })

	h, err := Status(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if h.Scheduler != "running" {
		t.Fatalf("scheduler = %q, want running", h.Scheduler)
	}
	if h.NextTaskSlug != "daily" || h.NextRunAt.IsZero() {
		t.Fatalf("health next run = %v %q", h.NextRunAt, h.NextTaskSlug)
	}

	_ = Stop(cfg)
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("daemon did not exit")
	}
}

func TestEnvResolverPrecedence(t *testing.T) {
	// Mirrors the daemon's resolver closure (SPEC-11 §4): process env first,
	// then the secret store.
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	t.Setenv("SHARED_NAME", "from-env")

	if err := database.Secrets().Set("SHARED_NAME", "from-secret"); err != nil {
		t.Fatal(err)
	}
	if err := database.Secrets().Set("ONLY_SECRET", "secret-value"); err != nil {
		t.Fatal(err)
	}

	resolve := func(name string) (string, bool) {
		if v, ok := os.LookupEnv(name); ok {
			return v, true
		}
		v, ok, _ := database.Secrets().Get(name)
		return v, ok
	}

	if v, ok := resolve("SHARED_NAME"); !ok || v != "from-env" {
		t.Fatalf("env must win: %q %v", v, ok)
	}
	if v, ok := resolve("ONLY_SECRET"); !ok || v != "secret-value" {
		t.Fatalf("secret fallback: %q %v", v, ok)
	}
	if _, ok := resolve("NEITHER"); ok {
		t.Fatal("unset name resolved")
	}
}

// ----- end-to-end over the real binary -----

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

// buildBinary compiles the real heka binary once for CLI-style e2e tests
// (SPEC-06 §8.6). Requires frontend/dist, which the build pipeline provides.
func buildBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "heka-e2e-bin")
		if err != nil {
			buildErr = err
			return
		}
		builtBin = filepath.Join(dir, "heka"+
			map[bool]string{true: ".exe", false: ""}[runtime.GOOS == "windows"])
		root, _ := filepath.Abs(filepath.Join("..", ".."))
		cmd := exec.Command("go", "build", "-o", builtBin, ".")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return builtBin
}

func TestCLIEndToEnd(t *testing.T) {
	binary := buildBinary(t)
	dataDir := t.TempDir()
	tasksDir := t.TempDir()
	// Seed a valid task so sync has something to index.
	if err := os.WriteFile(filepath.Join(tasksDir, "hello.yaml"),
		[]byte("version: 1\nname: Hello\nslug: hello\nscript: h.sh\ntype: script\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binary, "daemon")
	cmd.Env = append(os.Environ(),
		"HEKA_DATA_DIR="+dataDir,
		"HEKA_TASKS_DIR="+tasksDir,
	)
	logPath := filepath.Join(dataDir, "e2e.log")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// The daemon holds its own copies; release ours so the temp dir is never
	// locked on our side.
	logFile.Close()
	defer func() {
		_ = Stop(configForDirs(t, dataDir, tasksDir))
		_ = cmd.Process.Kill()
	}()

	cfg := configForDirs(t, dataDir, tasksDir)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := Status(cfg); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon did not start; log:\n%s", readFile(logPath))
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := Stop(cfg); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	select {
	case <-waitCh:
		// process exited cleanly after stop
	case <-time.After(15 * time.Second):
		t.Fatal("daemon process did not exit after stop")
	}
	if _, err := Status(cfg); err == nil {
		t.Fatal("daemon still reachable after stop")
	}
}

func configForDirs(t *testing.T, dataDir, tasksDir string) config.Config {
	t.Helper()
	cfg, err := config.Load(map[string]string{
		"HEKA_DATA_DIR":  dataDir,
		"HEKA_TASKS_DIR": tasksDir,
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func readFile(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}
