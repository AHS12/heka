//go:build windows

package daemon

import "syscall"

// detachedAttrs spawns the daemon without a console window (Windows), so
// `heka daemon start` doesn't flash a terminal.
func detachedAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
