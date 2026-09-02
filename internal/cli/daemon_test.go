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

	reconciled   int
	reconcileErr error
	schedules    []ipc.Schedule
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
func (s *stubClient) ListRuns(_ ipc.RunFilters) (ipc.RunListResult, error) {
	return ipc.RunListResult{Runs: s.runs, Total: len(s.runs)}, s.err
}
func (s *stubClient) ListSchedules() ([]ipc.Schedule, error)      { return s.schedules, s.err }
func (s *stubClient) ReconcileSchedules() error {
	if s.reconcileErr != nil {
		return s.reconcileErr
	}
	s.reconciled++
	return nil
}

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

func TestHelpAndVersionOutput(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}} {
		a, out, errOut := newTestApp(&stubClient{})
		if err := runArgs(t, a, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(out.String(), "Available Commands:") {
			t.Fatalf("%v output:\n%s", args, out.String())
		}
		if errOut.Len() != 0 {
			t.Fatalf("%v stderr:\n%s", args, errOut.String())
		}
	}

	for _, args := range [][]string{{"-v"}, {"--version"}} {
		a, out, errOut := newTestApp(&stubClient{})
		a.root.Version = "9.9.9"
		if err := runArgs(t, a, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if got := strings.TrimSpace(out.String()); got != "heka version 9.9.9" {
			t.Fatalf("%v output = %q", args, got)
		}
		if errOut.Len() != 0 {
			t.Fatalf("%v stderr:\n%s", args, errOut.String())
		}
	}
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

func TestDaemonAccessDeniedHint(t *testing.T) {
	stub := &stubClient{err: ipc.ErrDaemonAccessDenied}
	a, _, errOut := newTestApp(stub)
	if err := runArgs(t, a, "list"); err == nil {
		t.Fatal("expected error")
	}
	msg := errOut.String()
	if !strings.Contains(msg, "denied access") || !strings.Contains(msg, "elevated") {
		t.Fatalf("access-denied hint missing:\n%s", msg)
	}
}

func TestDaemonAccessDeniedJSONCode(t *testing.T) {
	stub := &stubClient{err: ipc.ErrDaemonAccessDenied}
	a, out, errOut := newTestApp(stub)
	if err := runArgs(t, a, "list", "--json"); err == nil {
		t.Fatal("expected error")
	}
	if errOut.Len() != 0 {
		t.Fatalf("json errors must go to stdout, stderr got:\n%s", errOut.String())
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("json error invalid:\n%s", out.String())
	}
	if e, ok := m["error"].(map[string]any); !ok || e["code"] != "daemon_access_denied" {
		t.Fatalf("json error = %v", m)
	}
}

func TestDaemonUnreachableHint(t *testing.T) {
	stub := &stubClient{err: ipc.ErrDaemonUnreachable}
	a, _, errOut := newTestApp(stub)
	if err := runArgs(t, a, "list"); err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(errOut.String(), "did not respond") {
		t.Fatalf("unreachable hint missing:\n%s", errOut.String())
	}
}

func TestSchedulesPlaceholder(t *testing.T) {
	a, _, errOut := newTestApp(&stubClient{})
	if err := runArgs(t, a, "schedules"); err == nil {
		t.Fatal("expected error for missing subcommand")
	}
	if !strings.Contains(errOut.String(), "specify") {
		t.Fatalf("schedules stderr:\n%s", errOut.String())
	}
}

func TestSchedulesReconcile(t *testing.T) {
	stub := &stubClient{}
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "schedules", "reconcile"); err != nil {
		t.Fatal(err)
	}
	if stub.reconciled != 1 {
		t.Fatalf("reconciled = %d, want 1", stub.reconciled)
	}
	if !strings.Contains(out.String(), "reconciled") {
		t.Fatalf("output = %q", out.String())
	}

	stub2 := &stubClient{}
	a2, out2, _ := newTestApp(stub2)
	if err := runArgs(t, a2, "schedules", "reconcile", "--json"); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out2.Bytes(), &m); err != nil {
		t.Fatalf("json parse: %v (%q)", err, out2.String())
	}
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("json = %v", m)
	}
}

func TestSchedulesMissed(t *testing.T) {
	stub := &stubClient{
		schedules: []ipc.Schedule{
			{ID: "schA", Slug: "daily-check", TaskSlug: "openrouter", Kind: "recurring"},
			{ID: "schB", Slug: "monthly-ims", TaskSlug: "ims-reminder", Kind: "recurring"},
		},
		runs: []ipc.Run{
			{
				RunID: "01HABCDEFGHJKMNPQRSTVWX", TaskSlug: "openrouter",
				ScheduleID: "schA", Trigger: "schedule", Status: "missed",
				StartedAt:  "2026-09-02T09:00:00Z",
				FinishedAt: "2026-09-02T09:00:00Z",
			},
			{
				RunID: "01HMISSEDJKMMNPQRSTVWXY", TaskSlug: "openrouter",
				ScheduleID: "schA", Trigger: "schedule", Status: "skipped",
				StartedAt: "2026-09-02T09:30:00Z",
			},
			{
				RunID: "01HOTHERRUNJKMMNPQRSTVW", TaskSlug: "ims-reminder",
				ScheduleID: "schB", Trigger: "schedule", Status: "success",
				StartedAt: "2026-09-01T10:00:00Z",
			},
		},
	}
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "schedules", "missed"); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, expect := range []string{
		"WHEN", "TASK", "SCHEDULE", "STATUS", "TRIGGER", "RUN_ID",
		"openrouter", "daily-check", "missed", "skipped",
		"01HABCDEFGHJ…", "01HMISSEDJKM…",
	} {
		if !strings.Contains(body, expect) {
			t.Fatalf("output missing %q\n%s", expect, body)
		}
	}
	// Schedule ID → slug resolution worked; the schedule's own slug appears even
	// when there's no schedule_id match for a stale run.
	if !strings.Contains(body, "monthly-ims") {
		t.Fatalf("output missing mapped slug\n%s", body)
	}
	// Footer summary.
	if !strings.Contains(body, "3 row(s)") {
		t.Fatalf("output missing row count\n%s", body)
	}
}

func TestSchedulesMissedEmpty(t *testing.T) {
	stub := &stubClient{}
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "schedules", "missed"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no missed/skipped schedule runs") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSchedulesMissedJSON(t *testing.T) {
	stub := &stubClient{
		runs: []ipc.Run{
			{RunID: "r1", TaskSlug: "t", ScheduleID: "s", Status: "missed",
				StartedAt: "2026-09-02T09:00:00Z"},
		},
	}
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "schedules", "missed", "--json"); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("json parse: %v (%q)", err, out.String())
	}
	if m["status"] != "missed,skipped" {
		t.Fatalf("status filter = %v", m["status"])
	}
	if total, _ := m["total"].(float64); int(total) != 1 {
		t.Fatalf("total = %v", m["total"])
	}
}

func TestSchedulesMissedBadSince(t *testing.T) {
	stub := &stubClient{}
	a, _, _ := newTestApp(stub)
	if err := runArgs(t, a, "schedules", "missed", "--since", "nope"); err == nil {
		t.Fatal("expected error")
	} else if !strings.Contains(err.Error(), "--since") {
		t.Fatalf("err = %v", err)
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
