//go:build darwin

package osapp

import (
	"os"
	"strings"
	"time"
)

// The darwin watchdog is launchd's KeepAlive guard on the shared agent
// plist (SPEC-17 §4.12): a crash or signal respawns the daemon within
// seconds; a clean exit (`heka daemon stop`, tray Quit) stays down. The
// periodic-check machinery (WatchOnce/schtasks) stays Windows/Linux-only —
// launchd supervision is instantaneous, so there is no interval.
//
// Installer semantics (repair-safe): Install/Status/Uninstall only manage
// the plist — they never stop or restart the daemon (RepairEntries runs
// from inside the daemon; killing itself would flap). The GUI toggle path
// uses SetWatchdogAgent for the full transition.

type launchdInstaller struct{}

func newPlatformInstaller() Installer { return &launchdInstaller{} }

// Install rewrites the plist with the KeepAlive guard (preserving
// RunAtLoad) and reloads launchd — safe to call from inside the running
// daemon. The interval parameter is ignored: launchd supervision is
// event-driven, not periodic.
//
// Guard: when the plist already carries the guard, points at exePath and
// the agent is live, this is a no-op — a reload's bootout would kill the
// calling daemon (it IS the job's process) and the post-bootout bootstrap
// race ("5: Input/output error") can leave the job unregistered, which is
// how supervised starts used to destroy themselves.
func (launchdInstaller) Install(_ time.Duration, exePath string) error {
	if f := readAgentFlags(); f.KeepAlive && agentLoaded() && taskPointsAtImpl(exePath) {
		return nil
	}
	f := readAgentFlags()
	f.KeepAlive = true
	if err := writeAgentPlist(exePath, f); err != nil {
		return err
	}
	if agentLoaded() {
		return reloadAgent()
	}
	if _, err := runLaunchctl("bootstrap", launchDomain(), agentPlistPath()); err != nil {
		if _, err2 := runLaunchctl("load", agentPlistPath()); err2 != nil {
			return err
		}
	}
	return nil
}

// Uninstall removes the KeepAlive guard (preserving RunAtLoad) and reloads.
// A supervised daemon survives the reload via respawn — call sites that
// want the daemon running detached must handle that (SetWatchdogAgent
// false → ApplyAgent boots it detached).
func (launchdInstaller) Uninstall() error {
	f := readAgentFlags()
	f.KeepAlive = false
	if f.RunAtLoad {
		if err := writeAgentPlist(agentExecutable(), f); err != nil {
			return err
		}
		return reloadAgent()
	}
	_, _ = runLaunchctl("bootout", launchDomain()+"/"+launchAgentID)
	_, _ = runLaunchctl("unload", agentPlistPath())
	return os.Remove(agentPlistPath())
}

// Status reports whether the KeepAlive guard is present in the plist.
func (launchdInstaller) Status() (bool, time.Duration, error) {
	return readAgentFlags().KeepAlive, 0, nil
}

// watchdogSupportedImpl: launchd supervision replaces the periodic check.
func watchdogSupportedImpl() bool { return true }

func watchdogModeImpl() string { return "launchd" }

// taskPointsAtImpl reports whether the agent plist launches the given
// binary. Where it does, RepairEntries leaves the plist alone; a stale path
// (app moved/renamed after an upgrade) triggers an idempotent re-Install.
func taskPointsAtImpl(exe string) bool {
	data, err := os.ReadFile(agentPlistPath())
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "<string>"+exe+"</string>")
}
