//go:build windows

package app

import "testing"

func TestTryLockGUISingleInstance(t *testing.T) {
	UnlockGUI() // clean slate in case a previous test left the mutex held
	if !TryLockGUI() {
		t.Fatal("first lock must succeed")
	}
	if TryLockGUI() {
		t.Fatal("second lock in the same process must fail")
	}
	UnlockGUI()
	if !TryLockGUI() {
		t.Fatal("lock must succeed again after unlock")
	}
	UnlockGUI()
}
