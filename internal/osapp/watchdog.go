// Package osapp holds OS-level integration the daemon needs while it runs
// (SPEC-10 watchdog, SPEC-15 startup registration/tray). Only watchdog code
// lives here today.
package osapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"heka/internal/config"
	"heka/internal/ipc"
)

// Watchdog interval default (minutes).
const (
	DefaultWatchdogInterval = 5 * time.Minute
	maxAttemptsPerMinute    = 2
)

// Hooks are the seams WatchOnce uses; tests swap them for fakes.
var (
	// CheckDaemon reports whether the daemon answers health pings.
	CheckDaemon = func(cfg config.Config) error {
		_, err := ipc.NewClient(cfg).Health()
		return err
	}
)

// StartFunc is the function signature for starting the daemon.
type StartFunc func(cfg config.Config) error

// Installer manages the OS-level watchdog entry (SPEC-10 §3).
type Installer interface {
	Install(interval time.Duration, hekaPath string) error
	Uninstall() error
	Status() (Installed bool, Interval time.Duration, err error)
}

// RepairEntries reconciles OS registration (watchdog + startup) with the
// currently running binary. After an upgrade the recorded exe path can point
// at a deleted/renamed binary, so entries whose path differs are re-created
// with the current executable path. Runs once at daemon startup (SPEC-10 §3).
func RepairEntries() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	repairWatchdog(exe)
	repairStartup(exe)
}

func repairWatchdog(exe string) {
	installed, interval, err := NewInstaller().Status()
	if err != nil || !installed {
		return
	}
	// The task points at this same binary? Nothing to do.
	if taskPointsAt(exe) {
		return
	}
	_ = NewInstaller().Install(interval, exe)
}

func repairStartup(exe string) {
	enabled, err := NewStartupRegistrar().Enabled()
	if err != nil || !enabled {
		return
	}
	// The Run value points at this same binary? Nothing to do.
	if startupPointsAt(exe) {
		return
	}
	_ = NewStartupRegistrar().Enable(exe)
}

// taskPointsAt reports whether the watchdog scheduled task's command line
// references the given binary path. Platform-specific (parses `schtasks /Query`).
var taskPointsAt = func(exe string) bool {
	return taskPointsAtImpl(exe)
}

// startupPointsAt reports whether the startup Run value references the given
// binary path. Platform-specific (reads the registry).
var startupPointsAt = func(exe string) bool {
	return startupPointsAtImpl(exe)
}

// WatchOnce is the whole `heka daemon watch --once` command (SPEC-10 §1):
//
//  1. daemon alive?        → exit 0
//  2. backoff in effect?   → exit 0 (don't pile on a crash loop)
//  3. start, record attempt → exit 0 on success, 1 if it fails to come up
func WatchOnce(cfg config.Config, startDaemon StartFunc) error {
	if CheckDaemon(cfg) == nil {
		return nil
	}
	state := readWatchdogState(cfg)
	if state.backedOff() {
		writeWatchdogState(cfg, state)
		return nil
	}
	state.attempt()
	writeWatchdogState(cfg, state)
	return startDaemon(cfg)
}

// watchdogState is the backoff bookkeeping. A file, not the DB: when the
// daemon is down, nothing else is readable (SPEC-10 §2).
type watchdogState struct {
	LastAttempt        time.Time `json:"last_attempt"`
	AttemptsLastMinute int       `json:"attempts_last_minute"`
}

func (s watchdogState) backedOff() bool {
	if time.Since(s.LastAttempt) > time.Minute {
		return false
	}
	return s.AttemptsLastMinute >= maxAttemptsPerMinute
}

func (s *watchdogState) attempt() {
	if time.Since(s.LastAttempt) > time.Minute {
		s.AttemptsLastMinute = 0
	}
	s.AttemptsLastMinute++
	s.LastAttempt = time.Now()
}

func watchdogStatePath(cfg config.Config) string {
	return filepath.Join(cfg.DataDir, "watchdog.state")
}

func readWatchdogState(cfg config.Config) watchdogState {
	var st watchdogState
	data, err := os.ReadFile(watchdogStatePath(cfg))
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

func writeWatchdogState(cfg config.Config, st watchdogState) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.WriteFile(watchdogStatePath(cfg), data, 0o600)
}

// NewInstaller returns the platform's watchdog installer. It is a var so
// tests (and the CLI) can substitute a fake.
var NewInstaller = func() Installer {
	return newPlatformInstaller()
}
