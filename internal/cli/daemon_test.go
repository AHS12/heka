package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"heka/internal/config"
	"heka/internal/ipc"
	"heka/internal/osapp"
)

// TestMain isolates this package on its own pipe: the daemon-status test
// must never talk to a real daemon.
func TestMain(m *testing.M) {
	_ = os.Setenv("HEKA_PIPE_NAME", fmt.Sprintf("heka-cli-test-%d", os.Getpid()))
	os.Exit(m.Run())
}

// stubClient is the APIClient seam (SPEC-08 §4).
type stubClient struct {
	tasks  []ipc.TaskSummary
	detail ipc.TaskDetail
	run    ipc.RunResponse
	runs   []ipc.Run
	runOne ipc.Run

	enabled  bool
	disabled bool
	err      error // when set, every call returns it
}

func (s *stubClient) ListTasks() ([]ipc.TaskSummary, error)        { return s.tasks, s.err }
func (s *stubClient) GetTask(string) (ipc.TaskDetail, error)       { return s.detail, s.err }
func (s *stubClient) RunTask(_, _ string) (ipc.RunResponse, error) { return s.run, s.err }
func (s *stubClient) Enable(slug string) error {
	if s.err != nil {
		return s.err
	}
	s.enabled = true
	return nil
}
func (s *stubClient) Disable(slug string) error {
	if s.err != nil {
		return s.err
	}
	s.disabled = true
	return nil
}
func (s *stubClient) TaskRuns(_ string, _ int) ([]ipc.Run, error) { return s.runs, s.err }
func (s *stubClient) Run(string) (ipc.Run, error)                 { return s.runOne, s.err }

// newTestApp wires a stub client into an App with captured output.
func newTestApp(stub *stubClient) (*App, *bytes.Buffer, *bytes.Buffer) {
	a := NewApp(config.Config{}, stub)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	a.stdout, a.stderr = out, errOut
	return a, out, errOut
}

func runArgs(t *testing.T, a *App, args ...string) error {
	t.Helper()
	return a.RunErr(args)
}

func TestList(t *testing.T) {
	stub := &stubClient{tasks: []ipc.TaskSummary{
		{Slug: "alpha", Name: "Alpha", Type: "script", Runtime: "custom", Enabled: true},
	}}
	a, out, _ := newTestApp(stub)

	if err := runArgs(t, a, "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "alpha") || !strings.Contains(out.String(), "yes") {
		t.Fatalf("human list:\n%s", out.String())
	}

	a2, out2, _ := newTestApp(stub)
	if err := runArgs(t, a2, "list", "--json"); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out2.Bytes()) {
		t.Fatalf("json list invalid:\n%s", out2.String())
	}
	var decoded []ipc.TaskSummary
	if err := json.Unmarshal(out2.Bytes(), &decoded); err != nil || len(decoded) != 1 {
		t.Fatalf("json list = %v (%v)", decoded, err)
	}
}

func TestRun(t *testing.T) {
	stub := &stubClient{run: ipc.RunResponse{GroupID: "01GFAKE", Status: "running"}}
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "run", "alpha"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "01GFAKE") || !strings.Contains(out.String(), "running") {
		t.Fatalf("human run:\n%s", out.String())
	}

	a2, out2, _ := newTestApp(stub)
	if err := runArgs(t, a2, "run", "alpha", "--json"); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out2.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	// run_id is the client-facing group id (SPEC-08 §3).
	if m["run_id"] != "01GFAKE" || m["success"] != true || m["status"] != "running" {
		t.Fatalf("json run = %v", m)
	}
}

func TestStatus(t *testing.T) {
	stub := &stubClient{
		detail: ipc.TaskDetail{Enabled: true},
		runs: []ipc.Run{
			{RunID: "01GR", Status: "success", FinishedAt: "2026-08-25T08:00:17Z", DurationMs: 16200},
		},
	}
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "status", "alpha"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "success") || !strings.Contains(out.String(), "enabled:  yes") {
		t.Fatalf("human status:\n%s", out.String())
	}
	// No runs yet.
	stub.runs = nil
	a2, out2, _ := newTestApp(stub)
	if err := runArgs(t, a2, "status", "alpha"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out2.String(), "none yet") {
		t.Fatalf("empty status:\n%s", out2.String())
	}
}

func TestLogs(t *testing.T) {
	stub := &stubClient{
		runs:   []ipc.Run{{RunID: "01GR"}},
		runOne: ipc.Run{RunID: "01GR", Status: "failed", ExitCode: 3, DurationMs: 900, Stdout: "out\n", Stderr: "boom\n"},
	}
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "logs", "alpha"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"exit code: 3", "STDOUT", "out", "STDERR", "boom"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("logs missing %q:\n%s", want, out.String())
		}
	}
	// Human duration formatting.
	if !strings.Contains(out.String(), "900ms") {
		t.Fatalf("duration missing:\n%s", out.String())
	}
}

func TestEnableDisable(t *testing.T) {
	stub := &stubClient{}
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "enable", "alpha"); err != nil {
		t.Fatal(err)
	}
	if !stub.enabled || stub.disabled {
		t.Fatalf("enable/disable flags = %v/%v", stub.enabled, stub.disabled)
	}
	if !strings.Contains(out.String(), "alpha enabled") {
		t.Fatalf("human enable:\n%s", out.String())
	}
	if err := runArgs(t, a, "disable", "alpha"); err != nil {
		t.Fatal(err)
	}
	if !stub.disabled {
		t.Fatal("disable not recorded")
	}
}

func TestFriendlyConflictMessage(t *testing.T) {
	stub := &stubClient{err: &ipc.Error{Code: "conflict", Message: "executor: task is already running"}}
	a, _, errOut := newTestApp(stub)
	err := runArgs(t, a, "run", "alpha")
	if err == nil {
		t.Fatal("expected error")
	}
	// Human mode: friendly text on stderr.
	if !strings.Contains(errOut.String(), "already running") {
		t.Fatalf("stderr:\n%s", errOut.String())
	}
}

func TestErrorJSONShape(t *testing.T) {
	stub := &stubClient{err: &ipc.Error{Code: "not_found", Message: "task \"ghost\" not found"}}
	a, out, errOut := newTestApp(stub)
	if err := runArgs(t, a, "status", "ghost", "--json"); err == nil {
		t.Fatal("expected error")
	}
	if errOut.Len() != 0 {
		t.Fatalf("json errors must go to stdout, stderr got:\n%s", errOut.String())
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("json error invalid:\n%s", out.String())
	}
	errEnv, ok := m["error"].(map[string]any)
	if !ok || errEnv["code"] != "not_found" {
		t.Fatalf("json error = %v", m)
	}
}

func TestDaemonNotRunningHint(t *testing.T) {
	stub := &stubClient{err: ipc.ErrDaemonNotRunning}
	a, _, errOut := newTestApp(stub)
	if err := runArgs(t, a, "list"); err == nil {
		t.Fatal("expected error")
	}
	msg := errOut.String()
	if !strings.Contains(msg, "heka daemon is not running.") ||
		!strings.Contains(msg, "heka daemon start") {
		t.Fatalf("hint missing:\n%s", msg)
	}
}

func TestSchedulesPlaceholder(t *testing.T) {
	a, out, _ := newTestApp(&stubClient{})
	if err := runArgs(t, a, "schedules"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "arrives in SPEC-09") {
		t.Fatalf("schedules output:\n%s", out.String())
	}
}

func TestDaemonStatusWhenDown(t *testing.T) {
	cfg, err := config.Load(map[string]string{"LOCALAPPDATA": t.TempDir()}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp(cfg, &stubClient{})
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	a.stdout, a.stderr = out, errOut

	err = runArgs(t, a, "daemon", "status")
	if err == nil {
		t.Fatal("expected not-running error")
	}
	// reportError prints the PRD §3.1 message.
	if !strings.Contains(errOut.String(), "heka daemon is not running.") {
		t.Fatalf("stderr:\n%s", errOut.String())
	}

	a2 := NewApp(cfg, &stubClient{})
	out2 := &bytes.Buffer{}
	a2.stdout, a2.stderr = out2, &bytes.Buffer{}
	if err := runArgs(t, a2, "daemon", "status", "--json"); err == nil {
		t.Fatal("expected error in json mode")
	}
	var m map[string]any
	if err := json.Unmarshal(out2.Bytes(), &m); err != nil {
		t.Fatalf("json error:\n%s", out2.String())
	}
	if e, ok := m["error"].(map[string]any); !ok || e["code"] != "daemon_not_running" {
		t.Fatalf("json error = %v", m)
	}
}

func TestUnknownCommand(t *testing.T) {
	a, _, _ := newTestApp(&stubClient{})
	err := runArgs(t, a, "frobnicate")
	if err == nil {
		t.Fatal("unknown command should error")
	}
	if !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "unknown command") &&
		!strings.Contains(err.Error(), "frobnicate") {
		// cobra may report differently by version; any error mentioning the arg is fine
		if !strings.Contains(err.Error(), "frobnicate") {
			t.Fatalf("err = %v", err)
		}
	}
}

func TestWatchOnceFlagsThroughCLI(t *testing.T) {
	cfg, err := config.Load(map[string]string{"LOCALAPPDATA": t.TempDir()}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp(cfg, &stubClient{})
	a.startDaemon = func(config.Config) error { return nil }
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	a.stdout, a.stderr = out, errOut
	swapWatchHooks(t,
		func(config.Config) error { return errors.New("down") },
	)

	if err := runArgs(t, a, "daemon", "watch", "--once"); err != nil {
		t.Fatalf("watch --once on a down daemon that recovers must exit 0: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", errOut.String())
	}
}

func TestWatchdogStatusCommand(t *testing.T) {
	cfg, err := config.Load(map[string]string{"LOCALAPPDATA": t.TempDir()}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	origInstaller := osapp.NewInstaller
	osapp.NewInstaller = func() osapp.Installer {
		return fakeInstaller{installed: true, interval: 5 * 60 * time.Second}
	}
	defer func() { osapp.NewInstaller = origInstaller }()

	a := NewApp(cfg, &stubClient{})
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	a.stdout, a.stderr = out, errOut
	if err := runArgs(t, a, "daemon", "watchdog", "status"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "installed (every 5m)") {
		t.Fatalf("status output:\n%s", out.String())
	}
}

type fakeInstaller struct {
	installed bool
	interval  time.Duration
}

func (f fakeInstaller) Install(time.Duration, string) error  { return nil }
func (f fakeInstaller) Uninstall() error                     { return nil }
func (f fakeInstaller) Status() (bool, time.Duration, error) { return f.installed, f.interval, nil }

// swapWatchHooks stubs the osapp watchdog seam for cli tests.
func swapWatchHooks(t *testing.T, check func(config.Config) error) {
	t.Helper()
	origCheck := osapp.CheckDaemon
	osapp.CheckDaemon = check
	t.Cleanup(func() {
		osapp.CheckDaemon = origCheck
	})
}
