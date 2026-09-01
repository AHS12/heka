//go:build linux

package osapp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const systemdUnitName = "heka-daemon"

// systemdStartupRegistrar manages a systemd --user unit for the daemon
// (SPEC-15 §3). Falls back to XDG autostart .desktop if systemd is unavailable.
type systemdStartupRegistrar struct{}

func newStartupRegistrar() StartupRegistrar { return &systemdStartupRegistrar{} }

func unitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", systemdUnitName+".service")
}

func autostartPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "autostart", "heka-daemon.desktop")
}

func hasSystemd() bool {
	out, err := exec.Command("systemctl", "--user", "status").CombinedOutput()
	if err != nil {
		return false
	}
	s := string(out)
	return !strings.Contains(s, "Failed to connect") && !strings.Contains(s, "not found")
}

func (r *systemdStartupRegistrar) Enable(exePath string) error {
	if hasSystemd() {
		return r.enableSystemd(exePath)
	}
	return r.enableAutostart(exePath)
}

func (r *systemdStartupRegistrar) enableSystemd(exePath string) error {
	dir := filepath.Dir(unitPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=Heka Task Daemon
After=graphical-session.target

[Service]
ExecStart=%s daemon
Restart=on-failure

[Install]
WantedBy=default.target
`, exePath)
	if err := os.WriteFile(unitPath(), []byte(unit), 0o600); err != nil {
		return err
	}
	// daemon-reload + enable + start
	for _, args := range [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", systemdUnitName},
		{"systemctl", "--user", "start", systemdUnitName},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, out)
		}
	}
	return nil
}

func (r *systemdStartupRegistrar) enableAutostart(exePath string) error {
	dir := filepath.Dir(autostartPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Heka Daemon
Exec=%s daemon
X-GNOME-Autostart-enabled=true
`, exePath)
	return os.WriteFile(autostartPath(), []byte(desktop), 0o600)
}

func (r *systemdStartupRegistrar) Disable() error {
	if hasSystemd() {
		_ = exec.Command("systemctl", "--user", "disable", systemdUnitName).Run()
		_ = exec.Command("systemctl", "--user", "stop", systemdUnitName).Run()
	}
	_ = os.Remove(unitPath())
	_ = os.Remove(autostartPath())
	return nil
}

func (r *systemdStartupRegistrar) Enabled() (bool, error) {
	if hasSystemd() {
		out, err := exec.Command("systemctl", "--user", "is-enabled", systemdUnitName).CombinedOutput()
		if err != nil {
			return false, nil
		}
		return strings.TrimSpace(string(out)) == "enabled", nil
	}
	_, err := os.Stat(autostartPath())
	return err == nil, nil
}
