//go:build !windows

package app

import (
	"path/filepath"
	"testing"
)

// TestTryLockGUIExclusive pins the flock guard (SPEC-17 §4.4): one holder at
// a time, released by UnlockGUI. The kernel releases the lock on process
// death, so no cross-process case needs simulating here.
func TestTryLockGUIExclusive(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "gui.lock")
	orig := guiLockPath
	guiLockPath = func() (string, error) { return lockPath, nil }
	defer func() { guiLockPath = orig }()

	UnlockGUI() // start clean regardless of earlier tests
	defer UnlockGUI()

	if !TryLockGUI() {
		t.Fatal("first TryLockGUI failed, want the lock taken")
	}
	if TryLockGUI() {
		t.Fatal("second TryLockGUI succeeded while the lock is held")
	}
	UnlockGUI()
	if !TryLockGUI() {
		t.Fatal("TryLockGUI failed after the lock was released")
	}
}
