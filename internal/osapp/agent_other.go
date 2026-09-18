//go:build !darwin

package osapp

import (
	"errors"

	"heka/internal/config"
)

// The launchd agent machinery (SPEC-17 §4.12) is macOS-only. Other
// platforms keep their plain registrar seams: Windows/Linux toggles need no
// daemon transition, and there is no supervisor to kickstart.

// AgentLoaded reports whether a launchd agent exists (false off-macOS).
func AgentLoaded() bool { return false }

// KickstartDaemon is unavailable off-macOS.
func KickstartDaemon() error { return errors.New("launchd kickstart not supported on this platform") }

// RepairAgentPath is a no-op off-macOS (no agent plist).
func RepairAgentPath(exePath string) error { return nil }

// EnsureAgentLoaded is a no-op off-macOS (no launchd agent to re-bootstrap).
func EnsureAgentLoaded(_ config.Config) error { return nil }

// SetStartupAgent falls back to the platform registrar (Windows registry,
// Linux systemd) — no daemon transition needed there.
func SetStartupAgent(cfg config.Config, exePath string, on bool, _ func(config.Config) error) error {
	r := NewStartupRegistrar()
	if on {
		return r.Enable(exePath)
	}
	return r.Disable()
}

// SetWatchdogAgent is unavailable off-macOS (the Installer paths handle
// Windows/Linux).
func SetWatchdogAgent(cfg config.Config, exePath string, on bool, _ func(config.Config) error) error {
	if on {
		return NewInstaller().Install(DefaultWatchdogInterval, exePath)
	}
	return NewInstaller().Uninstall()
}
