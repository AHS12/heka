//go:build !windows

package ipc

import "syscall"

// classifyErrno maps a unix-socket errno onto a client sentinel. nil means
// "no strong opinion" — the caller falls back to message matching.
func classifyErrno(errno syscall.Errno) error {
	switch errno {
	case syscall.ENOENT:
		return ErrDaemonNotRunning
	case syscall.EACCES, syscall.EPERM:
		return ErrDaemonAccessDenied
	case syscall.ECONNREFUSED, syscall.ETIMEDOUT:
		return ErrDaemonUnreachable
	}
	return nil
}
