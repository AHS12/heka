// Package ipc is Heka's local API (SPEC-07): HTTP/1.1 over a named pipe
// (Windows) or unix socket (POSIX). The transport moved here from the daemon
// so the endpoint is owned by the contract layer, not the runtime.
package ipc

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"

	"heka/internal/config"
)

// EndpointPath is the IPC endpoint for a configuration (SPEC-06 §4).
func EndpointPath(cfg config.Config) string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\` + pipeName()
	}
	return filepath.Join(cfg.SocketDir, "heka.sock")
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

// Listen binds the IPC endpoint. A successful bind doubles as the daemon's
// singleton lock (SPEC-06 §1): a second daemon fails here.
func Listen(cfg config.Config) (net.Listener, error) {
	if runtime.GOOS == "windows" {
		return winio.ListenPipe(EndpointPath(cfg), &winio.PipeConfig{
			SecurityDescriptor: pipeSecurityDescriptor(),
		})
	}
	path := EndpointPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("%w (is the daemon already running?)", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// Dial connects to a running daemon's endpoint.
func Dial(cfg config.Config) (net.Conn, error) {
	if runtime.GOOS == "windows" {
		return winio.DialPipe(EndpointPath(cfg), nil)
	}
	return net.DialTimeout("unix", EndpointPath(cfg), 2*time.Second)
}
