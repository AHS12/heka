package osapp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"heka/internal/config"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(map[string]string{"LOCALAPPDATA": t.TempDir()}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func withCheckDaemon(t *testing.T, check func(config.Config) error) {
	t.Helper()
	orig := CheckDaemon
	CheckDaemon = check
	t.Cleanup(func() { CheckDaemon = orig })
}

func TestWatchOnceHealthyDoesNothing(t *testing.T) {
	cfg := testConfig(t)
	started := false
	withCheckDaemon(t, func(config.Config) error { return nil })
	if err := WatchOnce(cfg, func(config.Config) error { started = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if started {
		t.Fatal("healthy daemon must not be restarted")
	}
}

func TestWatchOnceDownStarts(t *testing.T) {
	cfg := testConfig(t)
	started := 0
	withCheckDaemon(t, func(config.Config) error { return errDaemonFake })
	if err := WatchOnce(cfg, func(config.Config) error { started++; return nil }); err != nil {
		t.Fatal(err)
	}
	if started != 1 {
		t.Fatalf("starts = %d, want 1", started)
	}
}

func TestWatchOnceStartFailureReported(t *testing.T) {
	cfg := testConfig(t)
	withCheckDaemon(t, func(config.Config) error { return errDaemonFake })
	if err := WatchOnce(cfg, func(config.Config) error { return errStartFake }); err == nil {
		t.Fatal("expected start failure to surface")
	}
	st := readWatchdogState(cfg)
	if st.AttemptsLastMinute != 1 {
		t.Fatalf("attempts = %d, want 1 recorded", st.AttemptsLastMinute)
	}
}

func TestWatchOnceBackoff(t *testing.T) {
	cfg := testConfig(t)
	started := 0
	withCheckDaemon(t, func(config.Config) error { return errDaemonFake })
	fakeStart := func(config.Config) error { started++; return errStartFake }

	if err := WatchOnce(cfg, fakeStart); err == nil {
		t.Fatal("first call should attempt and fail")
	}
	if err := WatchOnce(cfg, fakeStart); err == nil {
		t.Fatal("second call should attempt and fail")
	}
	// Third and fourth calls within the minute are backed off: no trip, no error.
	if err := WatchOnce(cfg, fakeStart); err != nil {
		t.Fatalf("backed-off call must exit 0, got %v", err)
	}
	if err := WatchOnce(cfg, fakeStart); err != nil {
		t.Fatalf("backed-off call must exit 0, got %v", err)
	}
	if started != 2 {
		t.Fatalf("starts = %d, want exactly 2 (crash-loop guard)", started)
	}
}

func TestBackoffExpires(t *testing.T) {
	cfg := testConfig(t)
	started := 0
	withCheckDaemon(t, func(config.Config) error { return errDaemonFake })
	fakeStart := func(config.Config) error { started++; return errStartFake }

	_ = WatchOnce(cfg, fakeStart)
	_ = WatchOnce(cfg, fakeStart)
	// Simulate the window passing.
	st := readWatchdogState(cfg)
	st.LastAttempt = time.Now().Add(-2 * time.Minute)
	writeWatchdogState(cfg, st)
	if err := WatchOnce(cfg, fakeStart); err == nil {
		t.Fatal("expired window should allow a new attempt")
	}
	if started != 3 {
		t.Fatalf("starts = %d, want 3 after window expiry", started)
	}
}

func TestStateFileSurvives(t *testing.T) {
	cfg := testConfig(t)
	path := watchdogStatePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if st := readWatchdogState(cfg); st.AttemptsLastMinute != 0 {
		t.Fatalf("fresh state = %+v", st)
	}
}

// ---- RepairEntries (path-change reconciliation on daemon start) ----

type fakeInstaller struct {
	installed bool
	interval  time.Duration
	install   []string
	uninstall []string
}

func (f *fakeInstaller) Install(interval time.Duration, hekaPath string) error {
	f.install = append(f.install, hekaPath)
	return nil
}
func (f *fakeInstaller) Uninstall() error {
	f.uninstall = append(f.uninstall, "x")
	return nil
}
func (f *fakeInstaller) Status() (bool, time.Duration, error) {
	return f.installed, f.interval, nil
}

type fakeRegistrar struct {
	enabled bool
	enable  []string
	disable []string
}

func (f *fakeRegistrar) Enable(exePath string) error {
	f.enable = append(f.enable, exePath)
	return nil
}
func (f *fakeRegistrar) Disable() error {
	f.disable = append(f.disable, "x")
	return nil
}
func (f *fakeRegistrar) Enabled() (bool, error) {
	return f.enabled, nil
}

// swapSeams points the package-level constructor/path vars at fakes.
func swapSeams(t *testing.T, inst *fakeInstaller, reg *fakeRegistrar, taskPoints, startupPoints bool) {
	t.Helper()
	origInst := NewInstaller
	origReg := NewStartupRegistrar
	origTask := taskPointsAt
	origStartup := startupPointsAt
	NewInstaller = func() Installer { return inst }
	NewStartupRegistrar = func() StartupRegistrar { return reg }
	taskPointsAt = func(string) bool { return taskPoints }
	startupPointsAt = func(string) bool { return startupPoints }
	t.Cleanup(func() {
		NewInstaller = origInst
		NewStartupRegistrar = origReg
		taskPointsAt = origTask
		startupPointsAt = origStartup
	})
}

func TestRepairEntriesNoopWhenPathsMatch(t *testing.T) {
	inst := &fakeInstaller{installed: true, interval: 5 * time.Minute}
	reg := &fakeRegistrar{enabled: true}
	swapSeams(t, inst, reg, true, true) // both entries already point at exe

	RepairEntries()

	if len(inst.install) != 0 || len(inst.uninstall) != 0 {
		t.Fatalf("watchdog touched when paths match: %+v", inst)
	}
	if len(reg.enable) != 0 || len(reg.disable) != 0 {
		t.Fatalf("startup touched when paths match: %+v", reg)
	}
}

func TestRepairEntriesReinstallsOnPathChange(t *testing.T) {
	inst := &fakeInstaller{installed: true, interval: 5 * time.Minute}
	reg := &fakeRegistrar{enabled: true}
	swapSeams(t, inst, reg, false, false) // entries point at old install dir

	RepairEntries()

	if len(inst.install) != 1 {
		t.Fatalf("watchdog re-registrations = %d, want 1", len(inst.install))
	}
	if len(reg.enable) != 1 {
		t.Fatalf("startup re-registrations = %d, want 1", len(reg.enable))
	}
}

func TestRepairEntriesSkipsDisabled(t *testing.T) {
	inst := &fakeInstaller{installed: false}
	reg := &fakeRegistrar{enabled: false}
	swapSeams(t, inst, reg, false, false)

	RepairEntries()

	if len(inst.install) != 0 {
		t.Fatalf("disabled watchdog must not be installed: %+v", inst)
	}
	if len(reg.enable) != 0 {
		t.Fatalf("disabled startup must not be enabled: %+v", reg)
	}
}

var (
	errDaemonFake = errorFake("daemon down")
	errStartFake  = errorFake("start failed")
)

type errorFake string

func (e errorFake) Error() string { return string(e) }
