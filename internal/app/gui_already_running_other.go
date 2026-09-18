//go:build !windows

// GUI-already-running notice (SPEC-17 §4.4): a bare binary has no native
// message-box API on macOS, so the notice goes through osascript. Best
// effort — the process exits immediately afterwards regardless.
package app

import "os/exec"

// ShowGUIAlreadyRunning displays the "already running" notice. Failure is
// ignored: the second GUI is about to exit either way.
func ShowGUIAlreadyRunning() {
	script := `display dialog "A Heka GUI window is already running." with title "Heka" buttons {"OK"} default button "OK" with icon note`
	_ = exec.Command("osascript", "-e", script).Run()
}
