package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"heka/internal/config"
	"heka/internal/ipc"
)

// Start spawns a detached daemon and returns once it answers a health ping
// (readiness ≤ 15 s). If a daemon is already running, returns nil immediately.
// Windows: no console window. POSIX: new session.
func Start(cfg config.Config) error {
	client := ipc.NewClient(cfg)
	if _, err := client.Health(); err == nil {
		return nil // already running
	}

	binary, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}

	logPath := filepath.Join(cfg.DataDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(binary, "daemon")
	cmd.Dir = cfg.DataDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = detachedAttrs()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	_ = cmd.Process.Release() // hand the process to the OS

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Health(); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready within 15s")
}

// Stop asks the daemon to shut down gracefully and waits until the endpoint
// is gone (≤ 10 s).
func Stop(cfg config.Config) error {
	client := ipc.NewClient(cfg)
	if err := client.Shutdown(); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Health(); errors.Is(err, ipc.ErrDaemonNotRunning) {
			return nil // endpoint gone → daemon down
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("daemon did not exit within 10s")
}

// Status pings the daemon and returns its health.
func Status(cfg config.Config) (ipc.Health, error) {
	return ipc.NewClient(cfg).Health()
}
