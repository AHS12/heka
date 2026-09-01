//go:build windows

package app

import (
	"fmt"
	"testing"
	"time"
)

func TestTryLockGUISingleInstance(t *testing.T) {
	UnlockGUI()
	name := fmt.Sprintf(`Local\Heka.GUI.Test.%d`, time.Now().UnixNano())
	if !tryLockGUI(name) {
		t.Fatal("first lock must succeed")
	}
	if tryLockGUI(name) {
		t.Fatal("second lock in the same process must fail")
	}
	UnlockGUI()
	if !tryLockGUI(name) {
		t.Fatal("lock must succeed again after unlock")
	}
	UnlockGUI()
}
