//go:build windows

package executor

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000 // CREATE_NO_WINDOW (Windows)

func init() {
	// CREATE_NO_WINDOW: killing a tree must never flash a console window.
	taskkillCmd = func(args ...string) *exec.Cmd {
		cmd := exec.Command("taskkill", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
		return cmd
	}
}

// setProcessGroup puts the child in its own console process group (Windows)
// without a window: CREATE_NO_WINDOW stops console apps from flashing a
// terminal when the daemon runs them (SPEC-05 §4).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000200 | createNoWindow, // CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW
	}
}

// signalGroup is a no-op on Windows; termination goes through taskkill /T.
func signalGroup(pid int, sig syscall.Signal) {}
