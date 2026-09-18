//go:build !windows && !darwin

package osapp

// On non-Windows, non-darwin platforms the watchdog installer always
// rewrites its unit file on Install, so the path is inherently current.
// Report "does not point at exe" so RepairEntries re-registers
// (idempotently). Windows parses schtasks output; darwin reads the launchd
// agent plist directly.
func taskPointsAtImpl(exe string) bool { return false }
