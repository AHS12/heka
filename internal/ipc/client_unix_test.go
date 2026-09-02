//go:build !windows

package ipc

import (
	"os"
	"syscall"
	"testing"
)

func TestClassifyErrnoUnix(t *testing.T) {
	cases := []struct {
		errno syscall.Errno
		want  error
	}{
		{syscall.ENOENT, ErrDaemonNotRunning},
		{syscall.EACCES, ErrDaemonAccessDenied},
		{syscall.EPERM, ErrDaemonAccessDenied},
		{syscall.ECONNREFUSED, ErrDaemonUnreachable},
		{syscall.ETIMEDOUT, ErrDaemonUnreachable},
	}
	for _, c := range cases {
		if got := classifyErrno(c.errno); got != c.want {
			t.Fatalf("classifyErrno(%d) = %v, want %v", c.errno, got, c.want)
		}
	}
	if got := classifyErrno(syscall.EAGAIN); got != nil {
		t.Fatalf("unknown errno must return nil, got %v", got)
	}
}

func TestClassifyDialErrorUnix(t *testing.T) {
	// net.Dial's error shape: *net.OpError → *os.SyscallError → Errno, but
	// the classifier accepts the PathError form too (winio parity).
	denied := &os.PathError{Op: "dial", Path: "/run/heka/heka.sock", Err: syscall.EACCES}
	if got := classifyDialError(denied); got != ErrDaemonAccessDenied {
		t.Fatalf("access denied classified as %v", got)
	}

	missing := &os.PathError{Op: "dial", Path: "/run/heka/heka.sock", Err: syscall.ENOENT}
	if got := classifyDialError(missing); got != ErrDaemonNotRunning {
		t.Fatalf("ENOENT classified as %v", got)
	}
}
