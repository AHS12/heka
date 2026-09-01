//go:build darwin

package osapp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const launchAgentID = "com.heka.daemon"

// launchdStartupRegistrar manages a LaunchAgent plist for the daemon
// (SPEC-15 §3, RunAtLoad).
type launchdStartupRegistrar struct{}

func newStartupRegistrar() StartupRegistrar { return &launchdStartupRegistrar{} }

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentID+".plist")
}

func (r *launchdStartupRegistrar) Enable(exePath string) error {
	dir := filepath.Dir(plistPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
`, launchAgentID, exePath)
	if err := os.WriteFile(plistPath(), []byte(plist), 0o600); err != nil {
		return err
	}
	// load the agent
	out, err := exec.Command("launchctl", "load", plistPath()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load: %w: %s", err, out)
	}
	return nil
}

func (r *launchdStartupRegistrar) Disable() error {
	// unload (ignore errors — may not be loaded)
	_ = exec.Command("launchctl", "unload", plistPath()).Run()
	return os.Remove(plistPath())
}

func (r *launchdStartupRegistrar) Enabled() (bool, error) {
	out, err := exec.Command("launchctl", "list").CombinedOutput()
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(out), launchAgentID), nil
}
