package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"heka/internal/core/task"
	"heka/internal/db"
)

// TestRunArtifactsWrittenToGroupFolder verifies the per-run folder capture:
// stdout.log/stderr.log tee the attempt output and run.json carries the
// manifest (task output_dir overrides the global config root).
func TestRunArtifactsWrittenToGroupFolder(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	artifactsRoot := t.TempDir()
	exe := New(database, 1<<20, time.Second, nil, "")

	task := helperTask("artifact", "out-err")
	task.OutputDir = artifactsRoot

	h, err := exe.Start(context.Background(), task, Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)

	runs, _ := database.Runs().ListByTask("artifact", 5)
	if len(runs) != 1 || runs[0].Status != "success" {
		t.Fatalf("runs = %+v", runs)
	}
	dir := filepath.Join(artifactsRoot, runs[0].GroupID)
	for _, name := range []string{"stdout.log", "stderr.log", "run.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing artifact %s: %v", name, err)
		}
	}

	so, err := os.ReadFile(filepath.Join(dir, "stdout.log"))
	if err != nil || string(so) != "stdout-line\n" {
		t.Fatalf("stdout.log = %q (%v)", so, err)
	}
	se, err := os.ReadFile(filepath.Join(dir, "stderr.log"))
	if err != nil || string(se) != "stderr-line\n" {
		t.Fatalf("stderr.log = %q (%v)", se, err)
	}

	var m runManifest
	data, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.GroupID != runs[0].GroupID || m.TaskSlug != "artifact" || len(m.Attempts) != 1 {
		t.Fatalf("manifest = %+v", m)
	}
	if m.Attempts[0].Status != "success" || m.Attempts[0].ExitCode != 0 {
		t.Fatalf("attempt = %+v", m.Attempts[0])
	}
}

// TestNoArtifactsWithoutConfig ensures capture stays DB-only when the
// artifacts root is empty.
func TestNoArtifactsWithoutConfig(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	exe := New(database, 1<<20, time.Second, nil, "")
	h, err := exe.Start(context.Background(), helperTask("noart", "out-err"), Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)

	// A non-existent root never appears.
	if _, err := os.Stat(exe.artifactsDir); !os.IsNotExist(err) {
		t.Fatalf("artifacts dir appeared: %v", err)
	}
}

// TestOutputDirWritesToWorkspace verifies that a task's output_dir overrides
// the global artifacts root, writing logs relative to the working directory.
func TestOutputDirWritesToWorkspace(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	workspace := t.TempDir()
	exe := New(database, 1<<20, time.Second, nil, "")

	task := helperTask("wdir", "out-err")
	task.OutputDir = "./run-logs"

	h, err := exe.Start(context.Background(), task, Options{BaseDir: workspace})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)

	runs, _ := database.Runs().ListByTask("wdir", 5)
	if len(runs) != 1 || runs[0].Status != "success" {
		t.Fatalf("runs = %+v", runs)
	}

	dir := filepath.Join(workspace, "run-logs", runs[0].GroupID)
	for _, name := range []string{"stdout.log", "stderr.log", "run.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing artifact %s in workspace: %v", name, err)
		}
	}
	so, _ := os.ReadFile(filepath.Join(dir, "stdout.log"))
	if string(so) != "stdout-line\n" {
		t.Fatalf("stdout.log = %q", so)
	}
}

// TestDefaultLogDirUsesWorkingDirectory verifies that when output_dir is
// empty, logs go to the task's working directory.
func TestDefaultLogDirUsesWorkingDirectory(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	workspace := t.TempDir()
	exe := New(database, 1<<20, time.Second, nil, "")

	task := helperTask("worklog", "out-err")
	h, err := exe.Start(context.Background(), task, Options{BaseDir: workspace})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)

	runs, _ := database.Runs().ListByTask("worklog", 5)
	if len(runs) != 1 || runs[0].Status != "success" {
		t.Fatalf("runs = %+v", runs)
	}

	// Logs land directly in the working directory (base dir).
	dir := filepath.Join(workspace, runs[0].GroupID)
	for _, name := range []string{"stdout.log", "run.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing artifact %s in working dir: %v", name, err)
		}
	}
}

// TestHelperProcess is the cross-platform process under test (SPEC-05 §7):
// the executor's "tasks" re-run this same test binary in a helper mode.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	exe := mustAbs(t, os.Args[0])
	mode := os.Getenv("HELPER_MODE")
	switch mode {
	case "sleep":
		time.Sleep(time.Hour)
	case "exit":
		code, _ := strconv.Atoi(os.Getenv("HELPER_EXIT"))
		os.Exit(code)
	case "print":
		n, _ := strconv.Atoi(os.Getenv("HELPER_BYTES"))
		fmt.Fprint(os.Stdout, strings.Repeat("x", n))
	case "out-err":
		fmt.Fprintln(os.Stdout, "stdout-line")
		fmt.Fprintln(os.Stderr, "stderr-line")
	case "print-env":
		fmt.Fprintln(os.Stdout, os.Getenv(os.Getenv("HELPER_ENV")))
	case "cwd":
		fmt.Fprintln(os.Stdout, mustAbs(t, "."))
	case "fail-until":
		countFile := os.Getenv("HELPER_COUNT_FILE")
		failUntil, _ := strconv.Atoi(os.Getenv("HELPER_FAIL_UNTIL"))
		count := 0
		if b, err := os.ReadFile(countFile); err == nil {
			count, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		count++
		_ = os.WriteFile(countFile, []byte(strconv.Itoa(count)), 0o600)
		if count <= failUntil {
			os.Exit(3)
		}
		os.Exit(0)
	case "spawn-child":
		// Parent: spawn a grandchild that would write a file after 10s, then
		// sleep forever. A proper process-tree kill means the file never
		// appears.
		child := exec.Command(exe, "-test.run=TestHelperProcess")
		child.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_MODE=child-done",
			"HELPER_DONE_FILE="+os.Getenv("HELPER_DONE_FILE"),
		)
		if err := child.Start(); err != nil {
			os.Exit(9)
		}
		_ = os.WriteFile(os.Getenv("HELPER_PID_FILE"), []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		time.Sleep(time.Hour)
	case "child-done":
		time.Sleep(10 * time.Second)
		_ = os.WriteFile(os.Getenv("HELPER_DONE_FILE"), []byte("done"), 0o600)
	}
	os.Exit(0)
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// newTestExecutor opens a temp DB and builds an executor with a small grace
// period and a friendly resolver.
func newTestExecutor(t *testing.T, grace time.Duration) (*Executor, *db.DB) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	exe := New(database, 1<<20, grace, func(name string) (string, bool) {
		if name == "TOKEN" {
			return "resolved-secret", true
		}
		if name == "DAEMON_VAR" {
			return "from-task-override", true
		}
		return "", false
	}, "")
	return exe, database
}

// helperTask builds a binary-type task whose command is this test binary.
func helperTask(slug, mode string, envs ...string) *task.Task {
	env := map[string]string{
		"GO_WANT_HELPER_PROCESS": "1",
		"HELPER_MODE":            mode,
	}
	for _, pair := range envs {
		k, v, _ := strings.Cut(pair, "=")
		env[k] = v
	}
	return &task.Task{
		Version: 1, Name: slug, Slug: slug, Type: "binary",
		Command: os.Args[0], Args: []string{"-test.run=TestHelperProcess"},
		Environment: env,
	}
}

// waitDone blocks until the handle finishes or the test times out.
func waitDone(t *testing.T, h *Handle) {
	t.Helper()
	select {
	case <-h.Done:
	case <-time.After(60 * time.Second):
		t.Fatal("run group did not finish in time")
	}
}

func TestRunSuccess(t *testing.T) {
	exe, database := newTestExecutor(t, time.Second)
	h, err := exe.Start(context.Background(), helperTask("success", "out-err"), Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)
	runs, _ := database.Runs().ListByTask("success", 5)
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	r := runs[0]
	if r.Status != "success" || r.ExitCode == nil || *r.ExitCode != 0 {
		t.Fatalf("run = %+v", r)
	}
	if r.Stdout != "stdout-line\n" || r.Stderr != "stderr-line\n" {
		t.Fatalf("output: %q / %q", r.Stdout, r.Stderr)
	}
	if *r.DurationMs < 0 {
		t.Fatalf("negative duration: %d", *r.DurationMs)
	}
}

func TestRunFailure(t *testing.T) {
	exe, database := newTestExecutor(t, time.Second)
	h, err := exe.Start(context.Background(), helperTask("fail", "exit", "HELPER_EXIT=3"), Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)
	runs, _ := database.Runs().ListByTask("fail", 5)
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("runs = %+v", runs)
	}
	if runs[0].ExitCode == nil || *runs[0].ExitCode != 3 {
		t.Fatalf("exit code = %v, want 3", runs[0].ExitCode)
	}
}

func TestRunTimeout(t *testing.T) {
	exe, database := newTestExecutor(t, 500*time.Millisecond)
	tk := helperTask("slow", "sleep")
	tk.Timeout = 1
	h, err := exe.Start(context.Background(), tk, Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	waitDone(t, h)
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
	runs, _ := database.Runs().ListByTask("slow", 5)
	if len(runs) != 1 || runs[0].Status != "timed_out" {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestRunCancel(t *testing.T) {
	exe, database := newTestExecutor(t, 500*time.Millisecond)
	h, err := exe.Start(context.Background(), helperTask("cancel-me", "sleep"), Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	h.Cancel()
	h.Cancel() // safe to repeat
	waitDone(t, h)
	runs, _ := database.Runs().ListByTask("cancel-me", 5)
	if len(runs) != 1 || runs[0].Status != "cancelled" {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestOverlapPrevention(t *testing.T) {
	exe, _ := newTestExecutor(t, time.Second)
	dir := t.TempDir()
	h1, err := exe.Start(context.Background(), helperTask("busy", "sleep"), Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer h1.Cancel()

	_, err = exe.Start(context.Background(), helperTask("busy", "exit", "HELPER_EXIT=0"), Options{BaseDir: dir})
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Start err = %v, want ErrAlreadyRunning", err)
	}

	// A different slug runs concurrently.
	h2, err := exe.Start(context.Background(), helperTask("other", "exit", "HELPER_EXIT=0"), Options{BaseDir: dir})
	if err != nil {
		t.Fatalf("concurrent slug: %v", err)
	}
	waitDone(t, h2)
	h1.Cancel()
	waitDone(t, h1)

	// After the group finishes, the slug is free again.
	h3, err := exe.Start(context.Background(), helperTask("busy", "exit", "HELPER_EXIT=0"), Options{BaseDir: dir})
	if err != nil {
		t.Fatalf("slug not released: %v", err)
	}
	waitDone(t, h3)
}

func TestRetrySucceedsOnThirdAttempt(t *testing.T) {
	exe, database := newTestExecutor(t, time.Second)
	dir := t.TempDir()
	countFile := filepath.Join(dir, "count.txt")
	tk := helperTask("flaky", "fail-until",
		"HELPER_COUNT_FILE="+countFile, "HELPER_FAIL_UNTIL=2")
	tk.Retry.MaxAttempts = 3
	h, err := exe.Start(context.Background(), tk, Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)
	if err := h.Err(); err != nil {
		t.Fatal(err)
	}
	runs, _ := database.Runs().ListByTask("flaky", 10)
	if len(runs) != 3 {
		t.Fatalf("attempts = %d, want 3 (got %+v)", len(runs), runs)
	}
	if runs[0].Status != "success" {
		t.Fatalf("final status = %s, want success: %+v", runs[0].Status, runs)
	}
	// Same group across attempts.
	if runs[0].GroupID != runs[2].GroupID {
		t.Fatalf("group ids differ: %s vs %s", runs[0].GroupID, runs[2].GroupID)
	}
	if runs[0].Attempt != 2 || runs[2].Attempt != 0 {
		t.Fatalf("attempt numbers wrong: %+v", runs)
	}
}

func TestRetryExhausted(t *testing.T) {
	exe, database := newTestExecutor(t, time.Second)
	dir := t.TempDir()
	countFile := filepath.Join(dir, "count.txt")
	task := helperTask("doomed", "fail-until",
		"HELPER_COUNT_FILE="+countFile, "HELPER_FAIL_UNTIL=99")
	task.Retry.MaxAttempts = 2
	h, err := exe.Start(context.Background(), task, Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)
	runs, _ := database.Runs().ListByTask("doomed", 10)
	if len(runs) != 2 {
		t.Fatalf("attempts = %d, want 2", len(runs))
	}
	if runs[0].Status != "failed" {
		t.Fatalf("final status = %s, want failed", runs[0].Status)
	}
}

func TestOutputCap(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	exe := New(database, 4096, time.Second, nil, "")
	h, err := exe.Start(context.Background(), helperTask("verbose", "print", "HELPER_BYTES=10000"), Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)
	runs, _ := database.Runs().ListByTask("verbose", 5)
	if len(runs) != 1 {
		t.Fatal("no run recorded")
	}
	out := runs[0].Stdout
	if !strings.Contains(out, truncationMarker) {
		t.Fatal("truncation marker missing")
	}
	if got := len(out) - len(truncationMarker) - 1; got > 4096 {
		t.Fatalf("captured %d bytes, cap 4096 (marker=%q)", len(out), truncationMarker)
	}
}

func TestEnvResolution(t *testing.T) {
	exe, database := newTestExecutor(t, time.Second)
	dir := t.TempDir()

	tk := helperTask("env-ok", "print-env", "HELPER_ENV=VALUE")
	tk.Environment["VALUE"] = "${TOKEN}"
	h, err := exe.Start(context.Background(), tk, Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)
	runs, _ := database.Runs().ListByTask("env-ok", 5)
	if len(runs) != 1 || runs[0].Status != "success" {
		t.Fatalf("env-ok runs = %+v", runs)
	}
	if runs[0].Stdout != "resolved-secret\n" {
		t.Fatalf("stdout = %q, want resolved-secret", runs[0].Stdout)
	}

	// Task env overrides an inherited daemon value.
	ov := helperTask("env-override", "print-env", "HELPER_ENV=MY_VAR")
	ov.Environment["MY_VAR"] = "${DAEMON_VAR}"
	h2, err := exe.Start(context.Background(), ov, Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h2)
	runs, _ = database.Runs().ListByTask("env-override", 5)
	if runs[0].Stdout != "from-task-override\n" {
		t.Fatalf("override stdout = %q", runs[0].Stdout)
	}

	// Unresolvable ref fails the attempt with the message as stderr.
	bad := helperTask("env-bad", "print-env", "HELPER_ENV=X")
	bad.Environment["X"] = "${NOPE}"
	h3, err := exe.Start(context.Background(), bad, Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h3)
	runs, _ = database.Runs().ListByTask("env-bad", 5)
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("env-bad runs = %+v", runs)
	}
	if !strings.Contains(runs[0].Stderr, "NOPE") {
		t.Fatalf("stderr = %q, want missing-var message", runs[0].Stderr)
	}
}

func TestWorkingDirectoryResolution(t *testing.T) {
	exe, database := newTestExecutor(t, time.Second)
	base := t.TempDir()
	sub := filepath.Join(base, "scripts")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	tk := helperTask("cwd", "cwd")
	tk.WorkingDirectory = "../scripts"
	h, err := exe.Start(context.Background(), tk, Options{BaseDir: filepath.Join(base, "deep")})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)
	runs, _ := database.Runs().ListByTask("cwd", 5)
	if len(runs) != 1 || runs[0].Status != "success" {
		t.Fatalf("runs = %+v", runs)
	}
	if strings.TrimSpace(runs[0].Stdout) != sub {
		t.Fatalf("cwd = %q, want %q", strings.TrimSpace(runs[0].Stdout), sub)
	}
}

func TestTreeKillOnTimeout(t *testing.T) {
	exe, _ := newTestExecutor(t, 500*time.Millisecond)
	dir := t.TempDir()
	doneFile := filepath.Join(dir, "grandchild-done")
	pidFile := filepath.Join(dir, "grandchild.pid")
	tk := helperTask("tree", "spawn-child",
		"HELPER_DONE_FILE="+doneFile, "HELPER_PID_FILE="+pidFile)
	tk.Timeout = 2
	h, err := exe.Start(context.Background(), tk, Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)
	// Wait beyond the grandchild's 10s write window; if the tree kill worked
	// the grandchild is dead long before then.
	time.Sleep(3 * time.Second)
	if _, err := os.Stat(doneFile); err == nil {
		t.Fatal("grandchild wrote its file — process tree survived termination")
	}
}

func TestOnGroupFinishedFiresOnce(t *testing.T) {
	exe, database := newTestExecutor(t, time.Second)
	dir := t.TempDir()
	countFile := filepath.Join(dir, "count.txt")

	var mu sync.Mutex
	var results []GroupResult
	exe.OnGroupFinished(func(r GroupResult) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})

	tk := helperTask("callback", "fail-until",
		"HELPER_COUNT_FILE="+countFile, "HELPER_FAIL_UNTIL=99")
	tk.Retry.MaxAttempts = 3
	h, err := exe.Start(context.Background(), tk, Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, h)

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 1 {
		t.Fatalf("callback fired %d times, want exactly 1 (final group result)", len(results))
	}
	r := results[0]
	if r.TaskSlug != "callback" || r.FinalStatus != "failed" {
		t.Fatalf("result = %+v", r)
	}
	if r.ExitCode != 3 || r.Duration <= 0 {
		t.Fatalf("result fields = %+v", r)
	}
	runs, _ := database.Runs().ListByTask("callback", 10)
	if len(runs) != 3 {
		t.Fatalf("attempts = %d, want 3", len(runs))
	}
}

func TestRunIDAndGroupIDAreULIDish(t *testing.T) {
	exe, database := newTestExecutor(t, time.Second)
	h, err := exe.Start(context.Background(), helperTask("ids", "exit", "HELPER_EXIT=0"), Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.GroupID) != 26 {
		t.Fatalf("group id %q not ULID-sized", h.GroupID)
	}
	waitDone(t, h)
	runs, _ := database.Runs().ListByTask("ids", 5)
	if len(runs) != 1 || runs[0].RunID == "" || runs[0].GroupID != h.GroupID {
		t.Fatalf("runs = %+v", runs)
	}
}
