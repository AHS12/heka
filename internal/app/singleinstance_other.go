//go:build !windows

// GUI single-instance guard (SPEC-17 §4.4): an exclusive flock on
// <DataDir>/gui.lock held for the lifetime of the GUI process. A second
// `heka gui` fails the non-blocking lock, shows a notice, and exits without
// creating a window. The kernel releases the lock when the process dies, so
// a crashed GUI can never leave the lock stuck. The daemon keeps its own
// singleton (the IPC socket bind, SPEC-06 §1); this guard covers the window
// only.
package app

import (
	"os"
	"path/filepath"
	"syscall"

	"heka/internal/config"
)

var guiLockFile *os.File

// guiLockPath resolves the lock file's location. Var-seamed for tests.
var guiLockPath = func() (string, error) {
	cfg, err := config.LoadDefault()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg.DataDir, "gui.lock"), nil
}

// TryLockGUI attempts to take the GUI single-instance lock. It reports
// whether this process is the one that owns it.
func TryLockGUI() bool {
	if guiLockFile != nil {
		return false
	}
	f, err := openGUILockFile()
	if err != nil {
		// No lock file location (config failure) — let the GUI start rather
		// than lock the user out of the window.
		return true
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false
	}
	guiLockFile = f
	return true
}

// UnlockGUI releases the GUI lock. Safe to call even when the lock was
// never taken.
func UnlockGUI() {
	if guiLockFile != nil {
		_ = syscall.Flock(int(guiLockFile.Fd()), syscall.LOCK_UN)
		_ = guiLockFile.Close()
		guiLockFile = nil
	}
}

func openGUILockFile() (*os.File, error) {
	path, err := guiLockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}
