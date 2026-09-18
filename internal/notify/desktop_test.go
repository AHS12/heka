package notify

import (
	"errors"
	"testing"
)

// swapDesktopSeams replaces the DesktopToast seams with fakes (restored on
// cleanup).
func swapDesktopSeams(t *testing.T, native func(string, string) bool, beep func(string, string) error) {
	t.Helper()
	origNative, origBeep := nativeToast, beepToast
	nativeToast = native
	beepToast = beep
	t.Cleanup(func() {
		nativeToast, beepToast = origNative, origBeep
	})
}

func TestDesktopToastNativeWins(t *testing.T) {
	n, b := 0, 0
	swapDesktopSeams(t,
		func(string, string) bool { n++; return true },
		func(string, string) error { b++; return errors.New("beeep must not run when native succeeds") })

	if err := DesktopToast("title", "message"); err != nil {
		t.Fatalf("DesktopToast: %v", err)
	}
	if n != 1 || b != 0 {
		t.Errorf("native=%d beep=%d, want native 1, beep 0", n, b)
	}
}

func TestDesktopToastFallsBackToBeeep(t *testing.T) {
	n, b := 0, 0
	swapDesktopSeams(t,
		func(string, string) bool { n++; return false },
		func(string, string) error { b++; return nil })

	if err := DesktopToast("title", "message"); err != nil {
		t.Fatalf("DesktopToast: %v", err)
	}
	if n != 1 || b != 1 {
		t.Errorf("native=%d beep=%d, want both 1 (native first, then fallback)", n, b)
	}
}

func TestDesktopToastNilNativeIsBeeepOnly(t *testing.T) {
	b := 0
	swapDesktopSeams(t, nil, func(string, string) error { b++; return nil })

	if err := DesktopToast("title", "message"); err != nil {
		t.Fatalf("DesktopToast: %v", err)
	}
	if b != 1 {
		t.Errorf("beep calls = %d, want 1 (no native path off-darwin)", b)
	}
}

func TestDesktopToastBeeepErrorPropagates(t *testing.T) {
	swapDesktopSeams(t,
		func(string, string) bool { return false },
		func(string, string) error { return errors.New("osascript failed") })

	if err := DesktopToast("title", "message"); err == nil {
		t.Error("DesktopToast swallowed the fallback error; callers log it")
	}
}
