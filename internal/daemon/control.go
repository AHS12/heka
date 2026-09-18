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
	"heka/internal/osapp"
)

// Start spawns a detached daemon and returns once it answers a health ping
// (readiness ≤ 15 s). If a daemon is already running, returns nil immediately.
// When a launchd agent is bootstrapped (macOS watchdog/startup, SPEC-17
// §4.12), the daemon is started via `launchctl kickstart` instead of a raw
// fork so it runs under launchd supervision. Windows: no console window.
// POSIX: new session.
func Start(cfg config.Config) error {
	client := ipc.NewClient(cfg)
	if _, err := client.Health(); err == nil {
		return nil // already running
	}

	// Supervised start: the agent plist is the launch instruction, so make
	// sure it points at this binary before kickstarting (an app move/upgrade
	// leaves a stale path that would respawn the dead version forever).
	if osapp.AgentLoaded() {
		if exe, err := osapp.ConsoleExecutable(); err == nil {
			_ = osapp.RepairAgentPath(exe)
		}
		if err := osapp.KickstartDaemon(); err == nil {
			if err := awaitHealthy(client, 15*time.Second); err == nil {
				return nil
			}
		}
		// Kickstart failed — fall through to the manual spawn.
	}

	binary, err := osapp.ConsoleExecutable()
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

	return awaitHealthy(client, 15*time.Second)
}

// awaitHealthy polls the IPC health endpoint until it answers or the
// deadline passes.
func awaitHealthy(client *ipc.Client, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, err := client.Health(); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready within %s", d)
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
