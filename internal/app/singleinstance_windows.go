//go:build windows

// GUI single-instance guard: a per-user named mutex held for the lifetime of
// the GUI process. A second `heka gui` finds the mutex already owned, shows a
// message box, and exits without creating a window. The daemon keeps its own
// singleton (the IPC pipe bind, SPEC-06 §1); this guard covers the window
// only. `Local\` keeps the mutex per-user — no admin needed, and it matches
// the per-user IPC pipe name.
package app

import (
	"golang.org/x/sys/windows"
)

const guiMutexName = `Local\Heka.GUI`

var guiMutex windows.Handle

// TryLockGUI attempts to take the GUI single-instance mutex. It reports
// whether this process is the one that owns it.
func TryLockGUI() bool {
	h, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(guiMutexName))
	if err != nil {
		// ERROR_ALREADY_EXISTS: another GUI holds the mutex. The returned
		// handle is still valid — close it so it doesn't keep the object alive.
		if err == windows.ERROR_ALREADY_EXISTS {
			_ = windows.CloseHandle(h)
			return false
		}
		// Any other error (e.g. security) — let the GUI start rather than
		// lock the user out of the window.
		return true
	}
	guiMutex = h
	return true
}

// UnlockGUI releases the GUI mutex. Safe to call even when the lock was
// never taken (the handle is zero).
func UnlockGUI() {
	if guiMutex != 0 {
		_ = windows.ReleaseMutex(guiMutex)
		_ = windows.CloseHandle(guiMutex)
		guiMutex = 0
	}
}
