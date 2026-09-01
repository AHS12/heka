//go:build darwin

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

const plistName = "com.heka.watchdog.plist"

type launchdInstaller struct{}

func newPlatformInstaller() Installer { return &launchdInstaller{} }

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", plistName), nil
}

func writePlist(path, hekaPath string, interval time.Duration) error {
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>com.heka.watchdog</string>
	<key>ProgramArguments</key>
	<array><string>%s</string><string>daemon</string><string>watch</string><string>--once</string></array>
	<key>StartInterval</key><integer>%d</integer>
	<key>RunAtLoad</key><true/>
</dict>
</plist>
`, hekaPath, int(interval.Seconds()))
	return os.WriteFile(path, []byte(content), 0o600)
}

func (i *launchdInstaller) Install(interval time.Duration, hekaPath string) error {
	path, err := plistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := writePlist(path, hekaPath, interval); err != nil {
		return err
	}
	cmd := runCommand("launchctl", "load", "-w", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %w: %s", err, out)
	}
	return nil
}

func (i *launchdInstaller) Uninstall() error {
	path, err := plistPath()
	if err != nil {
		return err
	}
	if _, err := runCommand("launchctl", "unload", "-w", path).CombinedOutput(); err != nil {
		// already gone is fine
	}
	_ = os.Remove(path)
	return nil
}

func (i *launchdInstaller) Status() (bool, time.Duration, error) {
	out, err := runCommand("launchctl", "list").CombinedOutput()
	if err != nil {
		return false, 0, nil
	}
	if strings.Contains(string(out), "com.heka.watchdog") {
		return true, DefaultWatchdogInterval, nil
	}
	return false, 0, nil
}
