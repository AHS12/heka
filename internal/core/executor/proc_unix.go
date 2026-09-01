//go:build !windows

package executor

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group so termination can
// reach the whole tree (POSIX).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup signals the child's whole process group. Negative pid = group.
func signalGroup(pid int, sig syscall.Signal) {
	_ = syscall.Kill(-pid, sig)
}
