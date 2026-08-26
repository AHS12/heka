//go:build windows

package osapp

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const watchdogTaskName = "Heka Watchdog"

// runCommand is the exec seam (tests swap it for a fake).
var runCommand = exec.Command

// hiddenCmd is the CREATE_NO_WINDOW exec seam: schtasks and taskkill must
// never flash a console window (Settings-page queries, tray opens, and
// cancellations all route through here — SPEC-10 §3).
var hiddenCmd = func(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

// schtasksInstaller registers the watchdog as a Windows scheduled task,
// user-scope, no admin (SPEC-10 §3).
type schtasksInstaller struct{}

func newPlatformInstaller() Installer { return &schtasksInstaller{} }

// taskPointsAtImpl reports whether the watchdog scheduled task's command line
// references the given binary path. `schtasks /Query /V` shows the "Task To
// Run" line; a task pointing elsewhere (old install dir) must be re-created.
func taskPointsAtImpl(exe string) bool {
	out, err := hiddenCmd("schtasks", "/Query", "/TN", watchdogTaskName, "/FO", "LIST", "/V").
		CombinedOutput()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Task To Run:") {
			continue
		}
		target := strings.TrimSpace(strings.TrimPrefix(line, "Task To Run:"))
		return strings.Contains(target, exe)
	}
	return false
}

func (i *schtasksInstaller) Install(interval time.Duration, hekaPath string) error {
	minutes := int(interval.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	cmd := hiddenCmd("schtasks", "/Create", "/TN", watchdogTaskName,
		"/SC", "MINUTE", "/MO", strconv.Itoa(minutes),
		"/TR", fmt.Sprintf(`"%s" daemon watch --once`, hekaPath),
		"/F") // overwrite existing
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks create: %w: %s", err, out)
	}
	return nil
}

func (i *schtasksInstaller) Uninstall() error {
	cmd := hiddenCmd("schtasks", "/Delete", "/TN", watchdogTaskName, "/F")
	if out, err := cmd.CombinedOutput(); err != nil {
		// Missing task = already uninstalled.
		return fmt.Errorf("schtasks delete: %w: %s", err, out)
	}
	return nil
}

func (i *schtasksInstaller) Status() (bool, time.Duration, error) {
	out, err := hiddenCmd("schtasks", "/Query", "/TN", watchdogTaskName, "/FO", "LIST", "/V").
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
	// The task exists but the cadence line didn't parse (locale variance of
	// `schtasks /Query /V`). Fall back to the default install interval so the
	// Settings page never shows 0m.
	if interval == 0 {
		interval = DefaultWatchdogInterval
	}
	return true, interval, nil
}
