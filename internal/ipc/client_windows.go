//go:build windows

package ipc

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// classifyErrno maps a Windows pipe/socket errno onto a client sentinel.
// nil means "no strong opinion" — the caller falls back to message matching.
func classifyErrno(errno syscall.Errno) error {
	switch errno {
	case windows.ERROR_FILE_NOT_FOUND, windows.ERROR_PATH_NOT_FOUND:
		return ErrDaemonNotRunning
	case windows.ERROR_ACCESS_DENIED:
		return ErrDaemonAccessDenied
	case windows.ERROR_PIPE_BUSY, windows.ERROR_TIMEOUT, windows.ERROR_SEM_TIMEOUT:
		return ErrDaemonUnreachable
	}
	return nil
}
