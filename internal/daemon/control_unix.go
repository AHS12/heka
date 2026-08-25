//go:build !windows

package daemon

import "syscall"

// detachedAttrs makes the spawned daemon a session leader (POSIX), so it
// outlives the launching terminal.
func detachedAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
