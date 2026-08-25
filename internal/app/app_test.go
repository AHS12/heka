package app

import (
	"errors"
	"testing"

	"heka/internal/config"
	"heka/internal/ipc"
)

// stubCaller fakes the ipc client for the bridge tests. Task-surface methods
// return zero values plus s.err unless a specific stub is wired.
type stubCaller struct {
	health ipc.Health
	err    error
	calls  int

	list   []ipc.TaskSummary
	detail ipc.TaskDetail
	yaml   string
	errs   []string
	run    ipc.RunResponse
}

func (s *stubCaller) Health() (ipc.Health, error) {
	s.calls++
	return s.health, s.err
}

func (s *stubCaller) ListTasks() ([]ipc.TaskSummary, error)     { return s.list, s.err }
func (s *stubCaller) GetTask(string) (ipc.TaskDetail, error)    { return s.detail, s.err }
func (s *stubCaller) CreateTask(string) (ipc.TaskDetail, error) { return s.detail, s.err }
func (s *stubCaller) UpdateTask(string, string) (ipc.TaskDetail, error) {
	return s.detail, s.err
}
func (s *stubCaller) DeleteTask(string) error                         { return s.err }
func (s *stubCaller) TaskYAML(string) (string, error)                 { return s.yaml, s.err }
func (s *stubCaller) ValidateTaskYAML(string) ([]string, error)       { return s.errs, s.err }
func (s *stubCaller) ParseTask(string) (ipc.TaskDetail, error)        { return s.detail, s.err }
func (s *stubCaller) RunTask(string, string) (ipc.RunResponse, error) { return s.run, s.err }
func (s *stubCaller) SetTaskEnabled(string, bool) error               { return s.err }
func (s *stubCaller) SetSecret(string, string) error                  { return s.err }
func (s *stubCaller) ListSecrets() ([]string, error)                  { return nil, s.err }
func (s *stubCaller) DeleteSecret(string) error                       { return s.err }

func newAppWith(caller ipcCaller, started *int, startErr error) *App {
	a := NewApp("Heka", "0.1.0")
	a.client = caller
	a.start = func(config.Config) error { *started++; return startErr }
	return a
}

func TestHealthMapsDTO(t *testing.T) {
	a := newAppWith(&stubCaller{health: ipc.Health{
		Version: "0.1.0", UptimeSeconds: 42, Core: "healthy", Scheduler: "running",
	}}, new(int), nil)
	dto, err := a.Health()
	if err != nil {
		t.Fatal(err)
	}
	if dto.Version != "0.1.0" || dto.UptimeSeconds != 42 || dto.Core != "healthy" || dto.Scheduler != "running" {
		t.Fatalf("dto = %+v", dto)
	}
}

func TestHealthPreservesErrorCode(t *testing.T) {
	a := newAppWith(&stubCaller{err: &ipc.Error{Code: "bad_request", Message: "nope"}}, new(int), nil)
	_, err := a.Health()
	if err == nil || err.Error() != "bad_request: nope" {
		t.Fatalf("err = %v", err)
	}
}

func TestDaemonStatusMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		want   string
		wantID error
	}{
		{"running", nil, "running", nil},
		{"down", ipc.ErrDaemonNotRunning, "not-running", nil},
		{"other error", &ipc.Error{Code: "internal", Message: "boom"}, "", errors.New("internal: boom")},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			a := newAppWith(&stubCaller{err: tt.err}, new(int), nil)
			got, err := a.DaemonStatus()
			if tt.wantID != nil {
				if err == nil || err.Error() != tt.wantID.Error() {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("status = %q, %v", got, err)
			}
		})
	}
}

func TestStartDaemonDelegates(t *testing.T) {
	started := 0
	a := newAppWith(&stubCaller{err: ipc.ErrDaemonNotRunning}, &started, nil)
	if err := a.StartDaemon(); err != nil {
		t.Fatal(err)
	}
	if started != 1 {
		t.Fatalf("start calls = %d", started)
	}
}

func TestStartDaemonError(t *testing.T) {
	started := 0
	a := newAppWith(&stubCaller{}, &started, errors.New("spawn failed"))
	if err := a.StartDaemon(); err == nil || err.Error() != "spawn failed" {
		t.Fatalf("err = %v", err)
	}
}

func TestClientIsCached(t *testing.T) {
	caller := &stubCaller{health: ipc.Health{Core: "healthy"}}
	a := NewApp("Heka", "0.1.0")
	a.client = caller // pre-set beats lazy construction
	a.loadCfg = func() (config.Config, error) {
		t.Fatal("loadCfg must not run when a client is injected")
		return config.Config{}, nil
	}
	if _, err := a.Health(); err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d", caller.calls)
	}
}
