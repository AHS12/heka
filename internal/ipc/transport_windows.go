//go:build windows

package ipc

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"heka/internal/config"
)

// EndpointPath is the IPC endpoint for a configuration (SPEC-06 §4).
func EndpointPath(config.Config) string {
	return `\\.\pipe\` + pipeName()
}

// pipeName is the named pipe for this process: HEKA_PIPE_NAME overrides it
// (useful for running several instances per user, and for tests); otherwise
// it derives from the OS username, e.g. heka-alice.
func pipeName() string {
	if overridden := os.Getenv("HEKA_PIPE_NAME"); overridden != "" {
		return overridden
	}
	u := os.Getenv("USERNAME")
	if u == "" {
		if current, err := user.Current(); err == nil {
			u = current.Username
		}
	}
	return "heka-" + sanitizeUser(u)
}

// sanitizeUser normalizes a username for use inside a pipe name. os/user on
// Windows can return DOMAIN\user — backslashes are illegal in pipe names —
// and the bare form must match the USERNAME env form so processes whose
// environment lacks USERNAME still resolve the same endpoint.
func sanitizeUser(u string) string {
	if i := strings.LastIndexAny(u, `\/`); i >= 0 {
		return u[i+1:]
	}
	return u
}

// fallbackPipeSD is the owner-only descriptor used when the current user's
// SID cannot be resolved. Better than the permissive default (which grants
// read to Everyone), but see pipeSecurityDescriptor for its flaw.
const fallbackPipeSD = "D:P(A;;GA;;;OW)"

// pipeSecurityDescriptor builds the pipe's SDDL at runtime: SYSTEM and
// Administrators keep full control, and the running user's SID is granted
// generic access so every process of that user — elevated or not — can reach
// the endpoint.
//
// A static owner-only descriptor (D:P(A;;GA;;;OW)) cannot express this:
// objects created by an elevated process are owned by BUILTIN\Administrators
// (the elevated token's default owner), so the same user's non-elevated
// CLI/GUI/watchdog get ERROR_ACCESS_DENIED on dial and bind — which reads as
// "daemon is not running".
func pipeSecurityDescriptor() string {
	sid, err := currentUserSID()
	if err != nil {
		return fallbackPipeSD
	}
	return fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;%s)", sid)
}

// currentUserSID resolves the process token's user SID. The current-process
// pseudo-token needs no close and is valid for TOKEN_QUERY.
func currentUserSID() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

// Listen binds the IPC endpoint. A successful bind doubles as the daemon's
// singleton lock (SPEC-06 §1): a second daemon fails here.
func Listen(cfg config.Config) (net.Listener, error) {
	return winio.ListenPipe(EndpointPath(cfg), &winio.PipeConfig{
		SecurityDescriptor: pipeSecurityDescriptor(),
	})
}

// Dial connects to a running daemon's endpoint.
func Dial(cfg config.Config) (net.Conn, error) {
	return winio.DialPipe(EndpointPath(cfg), nil)
}
