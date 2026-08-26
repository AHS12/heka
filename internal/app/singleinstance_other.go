//go:build !windows

// Single-instance guard stub: the requirement is Windows-only for now
// (a message box telling the user a GUI is already running). Other platforms
// always allow the GUI to start.
package app

// TryLockGUI reports whether this process may open the GUI. Off-Windows:
// always yes.
func TryLockGUI() bool { return true }

// UnlockGUI releases the GUI single-instance lock. Off-Windows: no-op.
func UnlockGUI() {}
