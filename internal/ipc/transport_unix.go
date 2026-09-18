//go:build !windows

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"heka/internal/config"
)

// EndpointPath is the IPC endpoint for a configuration (SPEC-06 §4).
func EndpointPath(cfg config.Config) string {
	return filepath.Join(cfg.SocketDir, "heka.sock")
}

// Listen binds the IPC endpoint. A successful bind doubles as the daemon's
// singleton lock (SPEC-06 §1): a second daemon fails here. Security is by
// filesystem permissions — 0600 socket inside a 0700 directory.
//
// A daemon killed hard (crash, kill -9, reboot) leaves the socket file
// behind and unix bind then fails on the existing path — permanently
// locking the daemon out. A bind failure nobody answers on is therefore
// treated as stale: unlink once and retry (SPEC-17 §4.1).
func Listen(cfg config.Config) (net.Listener, error) {
	path := EndpointPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil && staleSocket(path) {
		_ = os.Remove(path)
		ln, err = net.Listen("unix", path)
	}
	if err != nil {
		return nil, fmt.Errorf("%w (is the daemon already running?)", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// staleSocket reports whether the endpoint exists but no daemon answers on
// it — i.e. the file is a leftover from a killed daemon, not a live
// singleton lock.
func staleSocket(path string) bool {
	conn, err := net.DialTimeout("unix", path, 500*time.Millisecond)
	if err != nil {
		return true
	}
	conn.Close()
	return false
}

// Dial connects to a running daemon's endpoint.
func Dial(cfg config.Config) (net.Conn, error) {
	return net.DialTimeout("unix", EndpointPath(cfg), 2*time.Second)
}
