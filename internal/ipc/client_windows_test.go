//go:build windows

package ipc

import (
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestClassifyErrnoWindows(t *testing.T) {
	cases := []struct {
		errno syscall.Errno
		want  error
	}{
		{windows.ERROR_FILE_NOT_FOUND, ErrDaemonNotRunning},
		{windows.ERROR_PATH_NOT_FOUND, ErrDaemonNotRunning},
		{windows.ERROR_ACCESS_DENIED, ErrDaemonAccessDenied},
		{windows.ERROR_PIPE_BUSY, ErrDaemonUnreachable},
		{windows.ERROR_TIMEOUT, ErrDaemonUnreachable},
	}
	for _, c := range cases {
		if got := classifyErrno(c.errno); got != c.want {
			t.Fatalf("classifyErrno(%d) = %v, want %v", c.errno, got, c.want)
		}
	}
	// Unknown errno → no opinion (caller falls back).
	if got := classifyErrno(windows.ERROR_NOT_READY); got != nil {
		t.Fatalf("unknown errno must return nil, got %v", got)
	}
}

func TestClassifyDialErrorWindows(t *testing.T) {
	// The winio error shape: *os.PathError{Op:"open", Path:"\\\\.\\pipe\\…"}
	// wrapping the errno. Access denied must NOT read as "not running".
	denied := &os.PathError{Op: "open", Path: `\\.\pipe\heka-AHS12`, Err: windows.ERROR_ACCESS_DENIED}
	if got := classifyDialError(denied); got != ErrDaemonAccessDenied {
		t.Fatalf("access denied classified as %v", got)
	}

	missing := &os.PathError{Op: "open", Path: `\\.\pipe\heka-AHS12`, Err: windows.ERROR_FILE_NOT_FOUND}
	if got := classifyDialError(missing); got != ErrDaemonNotRunning {
		t.Fatalf("file-not-found classified as %v", got)
	}

	// Unknown failure → unreachable, never the misleading "not running".
	if got := classifyDialError(&os.PathError{Op: "open", Path: `\\.\pipe\heka-AHS12`, Err: syscall.Errno(9999)}); got != ErrDaemonUnreachable {
		t.Fatalf("unknown classified as %v", got)
	}
}

func TestPipeSecurityDescriptor(t *testing.T) {
	sd := pipeSecurityDescriptor()
	if sd == fallbackPipeSD {
		t.Skip("user SID unavailable in this environment")
	}
	// SYSTEM + Administrators + the current user's SID.
	want := "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;"
	if len(sd) < len(want) || sd[:len(want)] != want {
		t.Fatalf("SDDL = %q, want prefix %q", sd, want)
	}
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	if got := sd[len(want):]; got != sid+")" {
		t.Fatalf("SDDL tail = %q, want %q)", got, sid)
	}
}
