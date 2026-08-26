//go:build windows

package osapp

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunnerCommand replaces runCommand: records invocations, emits canned
// output, exits 0.
func fakeRunnerCommand(t *testing.T, cans []string) *[]string {
	t.Helper()
	orig := runCommand
	var calls []string
	var mu sync.Mutex
	runCommand = func(name string, args ...string) *exec.Cmd {
		mu.Lock()
		calls = append(calls, strings.Join(append([]string{name}, args...), " "))
		mu.Unlock()
		out := "exit 0"
		if len(cans) > 0 {
			out = cans[len(calls)-1]
		}
		return exec.Command("cmd", "/c", "echo "+out)
	}
	t.Cleanup(func() { runCommand = orig })
	return &calls
}

// fakeHiddenCommand replaces hiddenCmd: records invocations, emits canned
// output, exits 0.
func fakeHiddenCommand(t *testing.T, cans []string) *[]string {
	t.Helper()
	orig := hiddenCmd
	var calls []string
	var mu sync.Mutex
	hiddenCmd = func(name string, args ...string) *exec.Cmd {
		mu.Lock()
		calls = append(calls, strings.Join(append([]string{name}, args...), " "))
		mu.Unlock()
		out := "exit 0"
		if len(cans) > 0 {
			out = cans[len(calls)-1]
		}
		return exec.Command("cmd", "/c", "echo "+out)
	}
	t.Cleanup(func() { hiddenCmd = orig })
	return &calls
}

func TestSchtasksInstall(t *testing.T) {
	calls := fakeHiddenCommand(t, nil)
	inst := &schtasksInstaller{}
	if err := inst.Install(5*time.Minute, `C:\heka\heka.exe`); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(*calls, "\n")
	for _, want := range []string{
		`schtasks /Create /TN Heka Watchdog /SC MINUTE /MO 5`,
		`/TR "C:\heka\heka.exe" daemon watch --once`,
		`/F`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install args missing %q:\n%s", want, got)
		}
	}
}

func TestSchtasksIntervalClipped(t *testing.T) {
	calls := fakeHiddenCommand(t, nil)
	inst := &schtasksInstaller{}
	if err := inst.Install(2*time.Second, "heka.exe"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains((*calls)[0], " /MO 1 ") {
		t.Fatalf("sub-minute interval must clip to 1m: %s", (*calls)[0])
	}
}

func TestSchtasksStatus(t *testing.T) {
	calls := fakeHiddenCommand(t, []string{"Repeat: Every 5 minute(s)"})
	inst := &schtasksInstaller{}
	installed, interval, err := inst.Status()
	if err != nil || !installed {
		t.Fatalf("status = %v %v %v", installed, interval, err)
	}
	if interval != 5*time.Minute {
		t.Fatalf("interval = %v, want 5m", interval)
	}
	if !strings.Contains((*calls)[0], "/Query") {
		t.Fatalf("status query missing: %s", (*calls)[0])
	}
}

func TestSchtasksNotInstalled(t *testing.T) {
	// A nonzero exit means the task doesn't exist.
	orig := hiddenCmd
	defer func() { hiddenCmd = orig }()
	hiddenCmd = func(name string, args ...string) *exec.Cmd {
		return exec.Command("cmd", "/c", "exit 1")
	}
	inst := &schtasksInstaller{}
	installed, _, err := inst.Status()
	if err != nil || installed {
		t.Fatalf("status for missing task = %v, %v", installed, err)
	}
}

func TestSchtasksUninstall(t *testing.T) {
	calls := fakeHiddenCommand(t, nil)
	inst := &schtasksInstaller{}
	if err := inst.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains((*calls)[0], "/Delete") || !strings.Contains((*calls)[0], "/F") {
		t.Fatalf("uninstall args: %s", (*calls)[0])
	}
}
