//go:build windows

package executor

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own console process group (Windows).
// Tree termination is handled by taskkill /T in terminate.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}

// signalGroup is a no-op on Windows; termination goes through taskkill /T.
func signalGroup(pid int, sig syscall.Signal) {}
