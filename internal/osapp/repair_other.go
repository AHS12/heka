//go:build !windows

package osapp

// On non-Windows platforms the startup registrar and watchdog installer always
// rewrite their unit/plist files on Enable, so the path is inherently current.
// Report "does not point at exe" so RepairEntries re-registers (idempotently).
func taskPointsAtImpl(exe string) bool { return false }

func startupPointsAtImpl(exe string) bool { return false }
