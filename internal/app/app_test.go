package app

import (
	"errors"
	"testing"
	"time"

	"heka/internal/config"
	"heka/internal/ipc"
	"heka/internal/osapp"
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
func (s *stubCaller) DeleteTask(string) error                              { return s.err }
func (s *stubCaller) TaskYAML(string) (string, error)                      { return s.yaml, s.err }
func (s *stubCaller) ValidateTaskYAML(string) ([]string, error)            { return s.errs, s.err }
func (s *stubCaller) ParseTask(string) (ipc.TaskDetail, error)             { return s.detail, s.err }
func (s *stubCaller) RunTask(string, string) (ipc.RunResponse, error)      { return s.run, s.err }
func (s *stubCaller) SetTaskEnabled(string, bool) error                    { return s.err }
func (s *stubCaller) SetSecret(string, string) error                       { return s.err }
func (s *stubCaller) ListSecrets() ([]string, error)                       { return nil, s.err }
func (s *stubCaller) DeleteSecret(string) error                            { return s.err }
func (s *stubCaller) ListSchedulesFiltered(string) ([]ipc.Schedule, error) { return nil, s.err }
func (s *stubCaller) CreateSchedule(ipc.Schedule) (ipc.Schedule, error)    { return ipc.Schedule{}, s.err }
func (s *stubCaller) UpdateSchedule(string, ipc.Schedule) (ipc.Schedule, error) {
	return ipc.Schedule{}, s.err
}
func (s *stubCaller) DeleteSchedule(string) error  { return s.err }
func (s *stubCaller) EnableSchedule(string) error  { return s.err }
func (s *stubCaller) DisableSchedule(string) error { return s.err }
func (s *stubCaller) ListRuns(ipc.RunFilters) (ipc.RunListResult, error) {
	return ipc.RunListResult{}, s.err
}
func (s *stubCaller) Run(string) (ipc.Run, error)           { return ipc.Run{}, s.err }
func (s *stubCaller) Cancel(string) error                   { return s.err }
func (s *stubCaller) PauseScheduler() error                 { return s.err }
func (s *stubCaller) ResumeScheduler() error                { return s.err }
func (s *stubCaller) Stats() (ipc.Stats, error)             { return ipc.Stats{}, s.err }
func (s *stubCaller) GetSettings() (ipc.SettingsDTO, error) { return ipc.SettingsDTO{}, s.err }
func (s *stubCaller) UpdateSettings(ipc.SettingsDTO) error  { return s.err }
func (s *stubCaller) PreviewSound(string) error             { return s.err }

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

// fakeInstaller is the osapp.Installer seam for the watchdog bridge tests.
type fakeInstaller struct {
	installed bool
	interval  time.Duration
	err       error
}

func (f fakeInstaller) Install(time.Duration, string) error  { return f.err }
func (f fakeInstaller) Uninstall() error                     { return f.err }
func (f fakeInstaller) Status() (bool, time.Duration, error) { return f.installed, f.interval, f.err }

func TestWatchdogEnabledMapsStatusDTO(t *testing.T) {
	orig := osapp.NewInstaller
	osapp.NewInstaller = func() osapp.Installer {
		return fakeInstaller{installed: true, interval: 5 * time.Minute}
	}
	defer func() { osapp.NewInstaller = orig }()

	a := NewApp("Heka", "0.1.0")
	dto, err := a.WatchdogEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if !dto.Installed || dto.IntervalMinutes != 5 {
		t.Fatalf("dto = %+v, want installed + 5m", dto)
	}
}

func TestWatchdogEnabledNotInstalled(t *testing.T) {
	orig := osapp.NewInstaller
	osapp.NewInstaller = func() osapp.Installer {
		return fakeInstaller{installed: false}
	}
	defer func() { osapp.NewInstaller = orig }()

	a := NewApp("Heka", "0.1.0")
	dto, err := a.WatchdogEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if dto.Installed || dto.IntervalMinutes != 0 {
		t.Fatalf("dto = %+v, want not installed + 0m", dto)
	}
}

func TestWatchdogEnabledZeroIntervalFallsBackToDefault(t *testing.T) {
	// A platform Status() that reports installed with an unparseable interval
	// must never surface 0m to the Settings page.
	orig := osapp.NewInstaller
	osapp.NewInstaller = func() osapp.Installer {
		return fakeInstaller{installed: true, interval: 0}
	}
	defer func() { osapp.NewInstaller = orig }()

	a := NewApp("Heka", "0.1.0")
	dto, err := a.WatchdogEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if !dto.Installed || dto.IntervalMinutes != int64(osapp.DefaultWatchdogInterval.Minutes()) {
		t.Fatalf("dto = %+v, want installed + default %dm", dto, int(osapp.DefaultWatchdogInterval.Minutes()))
	}
}

func TestWatchdogEnabledErrorPropagates(t *testing.T) {
	orig := osapp.NewInstaller
	osapp.NewInstaller = func() osapp.Installer {
		return fakeInstaller{err: errors.New("schtasks boom")}
	}
	defer func() { osapp.NewInstaller = orig }()

	a := NewApp("Heka", "0.1.0")
	if _, err := a.WatchdogEnabled(); err == nil || err.Error() != "schtasks boom" {
		t.Fatalf("err = %v", err)
	}
}
