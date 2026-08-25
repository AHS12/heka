//go:build windows

package osapp

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const watchdogTaskName = "Heka Watchdog"

// runCommand is the exec seam (tests swap it for a fake).
var runCommand = exec.Command

// schtasksInstaller registers the watchdog as a Windows scheduled task,
// user-scope, no admin (SPEC-10 §3).
type schtasksInstaller struct{}

func newPlatformInstaller() Installer { return &schtasksInstaller{} }

func (i *schtasksInstaller) Install(interval time.Duration, hekaPath string) error {
	minutes := int(interval.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	cmd := runCommand("schtasks", "/Create", "/TN", watchdogTaskName,
		"/SC", "MINUTE", "/MO", strconv.Itoa(minutes),
		"/TR", fmt.Sprintf(`"%s" daemon watch --once`, hekaPath),
		"/F") // overwrite existing
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks create: %w: %s", err, out)
	}
	return nil
}

func (i *schtasksInstaller) Uninstall() error {
	cmd := runCommand("schtasks", "/Delete", "/TN", watchdogTaskName, "/F")
	if out, err := cmd.CombinedOutput(); err != nil {
		// Missing task = already uninstalled.
		return fmt.Errorf("schtasks delete: %w: %s", err, out)
	}
	return nil
}

func (i *schtasksInstaller) Status() (bool, time.Duration, error) {
	out, err := runCommand("schtasks", "/Query", "/TN", watchdogTaskName, "/FO", "LIST", "/V").
		CombinedOutput()
	if err != nil {
		return false, 0, nil // not installed (nonzero exit)
	}
	interval := time.Duration(0)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Repeat:") {
			// Format: "Repeat: Every 5 minute(s)"
			fields := strings.Fields(strings.TrimPrefix(line, "Repeat:"))
			if len(fields) >= 2 && fields[0] == "Every" {
				if n, err := strconv.Atoi(fields[1]); err == nil {
					interval = time.Duration(n) * time.Minute
				}
			}
		}
	}
	return true, interval, nil
}
