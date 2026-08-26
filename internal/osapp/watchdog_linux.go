//go:build linux

package osapp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runCommand is the exec seam (tests swap it for a fake).
var runCommand = exec.Command

const (
	unitName  = "heka-watchdog.service"
	timerName = "heka-watchdog.timer"
)

// systemdInstaller registers a user-level systemd timer (SPEC-10 §3). If
// systemd is unavailable, the CLI documents a crontab fallback.
type systemdInstaller struct{}

func newPlatformInstaller() Installer { return &systemdInstaller{} }

func systemdUserDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func writeUnit(dir, hekaPath string, interval time.Duration) error {
	unit := fmt.Sprintf(`[Unit]
Description=Heka daemon watchdog

[Service]
Type=oneshot
ExecStart=%s daemon watch --once

[Install]
WantedBy=default.target
`, hekaPath)
	timer := fmt.Sprintf(`[Unit]
Description=Heka daemon watchdog timer

[Timer]
OnUnitActiveSec=%ds
Persistent=true

[Install]
WantedBy=timers.target
`, int(interval.Seconds()))
	if err := os.WriteFile(filepath.Join(dir, unitName), []byte(unit), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, timerName), []byte(timer), 0o600)
}

func (i *systemdInstaller) Install(interval time.Duration, hekaPath string) error {
	dir, err := systemdUserDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := writeUnit(dir, hekaPath, interval); err != nil {
		return err
	}
	cmd := runCommand("systemctl", "--user", "daemon-reload")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w: %s", err, out)
	}
	cmd = runCommand("systemctl", "--user", "enable", "--now", timerName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable %s: %w: %s", timerName, err, out)
	}
	return nil
}

func (i *systemdInstaller) Uninstall() error {
	cmd := runCommand("systemctl", "--user", "disable", "--now", timerName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl disable: %w: %s", err, out)
	}
	dir, err := systemdUserDir()
	if err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dir, unitName))
	_ = os.Remove(filepath.Join(dir, timerName))
	return nil
}

func (i *systemdInstaller) Status() (bool, time.Duration, error) {
	out, err := runCommand("systemctl", "--user", "list-timers", timerName).CombinedOutput()
	if err != nil {
		return false, 0, nil
	}
	interval := time.Duration(0)
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, timerName) {
			interval = DefaultWatchdogInterval // cadence lives in the unit file; existence is the signal
			break
		}
	}
	return interval > 0, interval, nil
}
