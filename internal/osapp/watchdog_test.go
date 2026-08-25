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

func withHooks(t *testing.T, check func(config.Config) error, start func(config.Config) error) {
	t.Helper()
	origCheck, origStart := CheckDaemon, StartDaemon
	CheckDaemon, StartDaemon = check, start
	t.Cleanup(func() {
		CheckDaemon, StartDaemon = origCheck, origStart
	})
}

func TestWatchOnceHealthyDoesNothing(t *testing.T) {
	cfg := testConfig(t)
	started := false
	withHooks(t,
		func(config.Config) error { return nil },
		func(config.Config) error { started = true; return nil },
	)
	if err := WatchOnce(cfg); err != nil {
		t.Fatal(err)
	}
	if started {
		t.Fatal("healthy daemon must not be restarted")
	}
}

func TestWatchOnceDownStarts(t *testing.T) {
	cfg := testConfig(t)
	started := 0
	withHooks(t,
		func(config.Config) error { return errDaemonFake },
		func(config.Config) error { started++; return nil },
	)
	if err := WatchOnce(cfg); err != nil {
		t.Fatal(err)
	}
	if started != 1 {
		t.Fatalf("starts = %d, want 1", started)
	}
}

func TestWatchOnceStartFailureReported(t *testing.T) {
	cfg := testConfig(t)
	withHooks(t,
		func(config.Config) error { return errDaemonFake },
		func(config.Config) error { return errStartFake },
	)
	if err := WatchOnce(cfg); err == nil {
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
	withHooks(t,
		func(config.Config) error { return errDaemonFake },
		func(config.Config) error { started++; return errStartFake },
	)

	if err := WatchOnce(cfg); err == nil {
		t.Fatal("first call should attempt and fail")
	}
	if err := WatchOnce(cfg); err == nil {
		t.Fatal("second call should attempt and fail")
	}
	// Third and fourth calls within the minute are backed off: no trip, no error.
	if err := WatchOnce(cfg); err != nil {
		t.Fatalf("backed-off call must exit 0, got %v", err)
	}
	if err := WatchOnce(cfg); err != nil {
		t.Fatalf("backed-off call must exit 0, got %v", err)
	}
	if started != 2 {
		t.Fatalf("starts = %d, want exactly 2 (crash-loop guard)", started)
	}
}

func TestBackoffExpires(t *testing.T) {
	cfg := testConfig(t)
	started := 0
	withHooks(t,
		func(config.Config) error { return errDaemonFake },
		func(config.Config) error { started++; return errStartFake },
	)
	_ = WatchOnce(cfg)
	_ = WatchOnce(cfg)
	// Simulate the window passing.
	st := readWatchdogState(cfg)
	st.LastAttempt = time.Now().Add(-2 * time.Minute)
	writeWatchdogState(cfg, st)
	if err := WatchOnce(cfg); err == nil {
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
	// Fresh reads are zero.
	if st := readWatchdogState(cfg); st.AttemptsLastMinute != 0 {
		t.Fatalf("fresh state = %+v", st)
	}
}

var (
	errDaemonFake = errorFake("daemon down")
	errStartFake  = errorFake("start failed")
)

type errorFake string

func (e errorFake) Error() string { return string(e) }
