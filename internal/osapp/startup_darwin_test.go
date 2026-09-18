//go:build darwin

package osapp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"heka/internal/config"
	"heka/internal/ipc"
)

// isolateAgent points the agent plist + launchctl seam at a temp sandbox so
// tests never touch the real ~/Library/LaunchAgents or run launchctl.
func isolateAgent(t *testing.T) *[]string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USER", "test")
	var calls []string
	runLaunchctl = func(args ...string) (string, error) {
		calls = append(calls, args[0])
		return "", nil
	}
	// agentLoaded consults runLaunchctl("print", …) → nil error → loaded;
	// tests that need "not loaded" override agentLoaded themselves.
	t.Cleanup(func() { calls = nil })
	return &calls
}

func TestWriteAgentPlistMatrix(t *testing.T) {
	exe := "/Applications/Heka.app/Contents/MacOS/heka"
	cases := []struct {
		name          string
		flags         agentFlags
		wantRunAtLoad bool
		wantKeepAlive bool
	}{
		{"none", agentFlags{}, false, false},
		{"startup only", agentFlags{RunAtLoad: true}, true, false},
		{"watchdog only", agentFlags{KeepAlive: true}, false, true},
		{"both", agentFlags{RunAtLoad: true, KeepAlive: true}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateAgent(t)
			if err := writeAgentPlist(exe, c.flags); err != nil {
				t.Fatalf("write: %v", err)
			}
			data, err := os.ReadFile(agentPlistPath())
			if err != nil {
				t.Fatal(err)
			}
			s := string(data)
			if got := strings.Contains(s, "<key>RunAtLoad</key>"); got != c.wantRunAtLoad {
				t.Errorf("RunAtLoad = %v, want %v", got, c.wantRunAtLoad)
			}
			if got := strings.Contains(s, "<key>KeepAlive</key>"); got != c.wantKeepAlive {
				t.Errorf("KeepAlive = %v, want %v", got, c.wantKeepAlive)
			}
			if got := readAgentFlags(); got != c.flags {
				t.Errorf("readAgentFlags = %+v, want %+v", got, c.flags)
			}
			if !strings.Contains(s, "<string>"+exe+"</string>") {
				t.Errorf("plist missing exe path:\n%s", s)
			}
			// The guard must be the SuccessfulExit flavor: launchd respawns
			// crashes but leaves clean exits down.
			if c.wantKeepAlive && !strings.Contains(s, "<key>SuccessfulExit</key>") {
				t.Errorf("KeepAlive guard missing SuccessfulExit:\n%s", s)
			}
		})
	}
}

func TestReadAgentFlagsLegacyPlist(t *testing.T) {
	// Pre-§4.12 installs wrote a plain boolean KeepAlive — it must read as
	// the watchdog being on, not break parsing.
	isolateAgent(t)
	legacy := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
    <key>Label</key><string>com.heka.daemon</string>
    <key>ProgramArguments</key><array>
        <string>/old/heka</string><string>daemon</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
</dict></plist>
`
	if err := os.MkdirAll(filepath.Dir(agentPlistPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentPlistPath(), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	f := readAgentFlags()
	if !f.RunAtLoad || !f.KeepAlive {
		t.Errorf("legacy flags = %+v, want both on", f)
	}
	if exe := agentExecutable(); exe != "/old/heka" {
		t.Errorf("agentExecutable = %q, want /old/heka", exe)
	}
}

func TestSetStartupAgentPreservesWatchdog(t *testing.T) {
	isolateAgent(t)
	exe := "/Applications/Heka.app/Contents/MacOS/heka"
	cfg := config.Config{}
	daemonProbe = func(config.Config) bool { return false }
	waitDaemon = func(config.Config) error { return nil }

	// Watchdog on first (daemon not running: no kickstart expected).
	if err := SetWatchdogAgent(cfg, exe, true, func(config.Config) error {
		t.Fatal("startDetached must not fire when the daemon is down")
		return nil
	}); err != nil {
		t.Fatalf("watchdog on: %v", err)
	}
	if got := readAgentFlags(); got != (agentFlags{KeepAlive: true}) {
		t.Fatalf("flags = %+v, want KeepAlive only", got)
	}

	// Startup on → RunAtLoad added, KeepAlive preserved.
	if err := SetStartupAgent(cfg, exe, true, func(config.Config) error {
		t.Fatal("startDetached must not fire when the daemon is down")
		return nil
	}); err != nil {
		t.Fatalf("startup on: %v", err)
	}
	if got := readAgentFlags(); got != (agentFlags{RunAtLoad: true, KeepAlive: true}) {
		t.Fatalf("flags = %+v, want both", got)
	}

	// Startup off → RunAtLoad removed, KeepAlive preserved.
	if err := SetStartupAgent(cfg, exe, false, func(config.Config) error {
		t.Fatal("startDetached must not fire when the daemon is down")
		return nil
	}); err != nil {
		t.Fatalf("startup off: %v", err)
	}
	if got := readAgentFlags(); got != (agentFlags{KeepAlive: true}) {
		t.Fatalf("flags = %+v, want KeepAlive only", got)
	}

	// Watchdog off with nothing else → agent removed entirely.
	if err := SetWatchdogAgent(cfg, exe, false, func(config.Config) error {
		t.Fatal("startDetached must not fire when the daemon is down")
		return nil
	}); err != nil {
		t.Fatalf("watchdog off: %v", err)
	}
	if _, err := os.Stat(agentPlistPath()); !os.IsNotExist(err) {
		t.Errorf("plist still present after both toggles off: %v", err)
	}
}

func TestStartupEnabledReadsContent(t *testing.T) {
	isolateAgent(t)
	r := newStartupRegistrar()
	exe := "/Applications/Heka.app/Contents/MacOS/heka"

	// Watchdog-only plist: startup must report off.
	if err := writeAgentPlist(exe, agentFlags{KeepAlive: true}); err != nil {
		t.Fatal(err)
	}
	if on, _ := r.Enabled(); on {
		t.Error("watchdog-only plist reported startup enabled")
	}

	if err := writeAgentPlist(exe, agentFlags{RunAtLoad: true, KeepAlive: true}); err != nil {
		t.Fatal(err)
	}
	if on, _ := r.Enabled(); !on {
		t.Error("RunAtLoad plist reported startup disabled")
	}
}

func TestRepairAgentPathRewritesStaleExe(t *testing.T) {
	isolateAgent(t)
	exe := "/Applications/Heka.app/Contents/MacOS/heka"
	if err := writeAgentPlist("/old/path/heka", agentFlags{RunAtLoad: true, KeepAlive: true}); err != nil {
		t.Fatal(err)
	}
	if err := RepairAgentPath(exe); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if !taskPointsAtImpl(exe) {
		t.Error("plist still points at the stale path after repair")
	}
	if got := readAgentFlags(); got != (agentFlags{RunAtLoad: true, KeepAlive: true}) {
		t.Errorf("flags changed during repair: %+v", got)
	}
	// Repairing the current path is a no-op (file untouched).
	before, _ := os.Stat(agentPlistPath())
	if err := RepairAgentPath(exe); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(agentPlistPath())
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("repair rewrote an already-current plist")
	}
}

func TestApplyAgentRunningTransitions(t *testing.T) {
	isolateAgent(t)
	exe := "/Applications/Heka.app/Contents/MacOS/heka"
	cfg := config.Config{}

	// Script the daemon's answers: down throughout, so no transitions fire.
	daemonProbe = func(config.Config) bool { return false }

	started := 0
	startDetached := func(config.Config) error { started++; return nil }

	// Watchdog on while the daemon is down: plist written, no detached start.
	if err := SetWatchdogAgent(cfg, exe, true, startDetached); err != nil {
		t.Fatalf("watchdog on: %v", err)
	}
	if started != 0 {
		t.Errorf("startDetached fired with the daemon down")
	}
	if got := readAgentFlags(); got != (agentFlags{KeepAlive: true}) {
		t.Fatalf("flags = %+v, want KeepAlive only", got)
	}

	// Turning everything off while down removes the plist without a respawn.
	if err := SetWatchdogAgent(cfg, exe, false, startDetached); err != nil {
		t.Fatalf("watchdog off: %v", err)
	}
	if started != 0 {
		t.Errorf("startDetached fired although no daemon was running")
	}
	if _, err := os.Stat(agentPlistPath()); !os.IsNotExist(err) {
		t.Errorf("plist survived both toggles off: %v", err)
	}
}

func TestApplyAgentRunningDetachedRespawn(t *testing.T) {
	isolateAgent(t)
	exe := "/Applications/Heka.app/Contents/MacOS/heka"
	cfg := config.Config{}

	// Script the daemon: down while registering (no kickstart), then up for
	// the removal (the supervised daemon must be respawned detached).
	daemonProbe = func(config.Config) bool { return false }
	waitDaemon = func(config.Config) error { return nil }
	if err := SetStartupAgent(cfg, exe, true, nil); err != nil {
		t.Fatalf("startup on: %v", err)
	}

	probes := 0
	daemonProbe = func(config.Config) bool {
		probes++
		return probes == 1 // running for the removal call, down afterwards
	}
	defer func() { daemonProbe = func(cfg config.Config) bool {
		_, err := ipc.NewClient(cfg).Health()
		return err == nil
	}}()

	started := 0
	startDetached := func(config.Config) error { started++; return nil }
	if err := SetStartupAgent(cfg, exe, false, startDetached); err != nil {
		t.Fatalf("startup off: %v", err)
	}
	if started != 1 {
		t.Errorf("startDetached calls = %d, want 1 (daemon was running)", started)
	}
	if _, err := os.Stat(agentPlistPath()); !os.IsNotExist(err) {
		t.Errorf("plist survived the removal: %v", err)
	}
}

// A toggle whose desired flags already match the plist, with the agent
// loaded, must not stop or restart the daemon (the historical freeze).
func TestApplyAgentNoopWhenStateMatches(t *testing.T) {
	isolateAgent(t)
	exe := "/Applications/Heka.app/Contents/MacOS/heka"
	cfg := config.Config{}
	daemonProbe = func(config.Config) bool { return false }
	waitDaemon = func(config.Config) error { return nil }

	if err := SetStartupAgent(cfg, exe, true, nil); err != nil {
		t.Fatalf("startup on: %v", err)
	}
	calls := isolateAgentRestore(t) // fresh recorder from here on

	// Re-applying the same toggle: no transitions — only the cheap
	// agentLoaded probe ("print") is allowed.
	if err := SetStartupAgent(cfg, exe, true, nil); err != nil {
		t.Fatalf("redundant toggle: %v", err)
	}
	for _, c := range *calls {
		if c != "print" {
			t.Errorf("redundant toggle ran %q", c)
		}
	}
}

// The broken state a failed reload used to leave: plist present, agent not
// bootstrapped. Re-applying the toggle must reload (not no-op) so the
// registration heals.
func TestApplyAgentHealsUnloadedAgent(t *testing.T) {
	isolateAgent(t)
	exe := "/Applications/Heka.app/Contents/MacOS/heka"
	cfg := config.Config{}
	daemonProbe = func(config.Config) bool { return false }
	waitDaemon = func(config.Config) error { return nil }
	agentLoaded = func() bool { return false }

	if err := writeAgentPlist(exe, agentFlags{RunAtLoad: true}); err != nil {
		t.Fatal(err)
	}
	calls := isolateAgentRestore(t)
	if err := SetStartupAgent(cfg, exe, true, nil); err != nil {
		t.Fatalf("toggle with unloaded agent: %v", err)
	}
	for _, c := range *calls {
		if c == "bootstrap" {
			return // agent was re-bootstrapped — healed
		}
	}
	t.Errorf("agent was not re-bootstrapped; launchctl calls: %v", *calls)
}

// A failed reload must never leave the daemon down: with wasRunning=true,
// ApplyAgent restores it detached and still surfaces the error.
func TestApplyAgentRestoresDaemonOnFailure(t *testing.T) {
	isolateAgent(t)
	exe := "/Applications/Heka.app/Contents/MacOS/heka"
	cfg := config.Config{}
	probes := 0
	daemonProbe = func(config.Config) bool {
		probes++
		return probes == 1 // running before, gone after the failed reload
	}
	defer func() { daemonProbe = func(cfg config.Config) bool {
		_, err := ipc.NewClient(cfg).Health()
		return err == nil
	}}()

	runLaunchctl = func(args ...string) (string, error) {
		if args[0] == "bootstrap" || args[0] == "load" {
			return "launchctl boom", errors.New("boom")
		}
		return "", nil
	}

	started := 0
	startDetached := func(config.Config) error { started++; return nil }
	err := SetStartupAgent(cfg, exe, true, startDetached)
	if err == nil {
		t.Fatal("reload failure must be surfaced")
	}
	if started != 1 {
		t.Errorf("startDetached calls = %d, want 1 (daemon restored)", started)
	}
}

func TestReloadAgentRetriesBootstrap(t *testing.T) {
	isolateAgent(t)
	agentLoaded = func() bool { return false }
	attempts := 0
	runLaunchctl = func(args ...string) (string, error) {
		switch args[0] {
		case "bootstrap":
			attempts++
			if attempts < 3 {
				return "5: Input/output error", errors.New("5: Input/output error")
			}
		case "load":
			t.Error("load fallback used although bootstrap succeeded on retry")
		}
		return "", nil
	}
	if err := reloadAgent(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if attempts != 3 {
		t.Errorf("bootstrap attempts = %d, want 3", attempts)
	}
}

func TestEnsureAgentLoaded(t *testing.T) {
	cfg := config.Config{}
	t.Run("no plist", func(t *testing.T) {
		isolateAgent(t)
		agentLoaded = func() bool { return false }
		daemonProbe = func(config.Config) bool { return false }
		calls := isolateAgentRestore(t)
		if err := EnsureAgentLoaded(cfg); err != nil {
			t.Fatalf("EnsureAgentLoaded: %v", err)
		}
		if len(*calls) != 0 {
			t.Errorf("launchctl traffic without a plist: %v", *calls)
		}
	})
	t.Run("flagless plist", func(t *testing.T) {
		isolateAgent(t)
		agentLoaded = func() bool { return false }
		daemonProbe = func(config.Config) bool { return false }
		if err := writeAgentPlist("/x/heka", agentFlags{}); err != nil {
			t.Fatal(err)
		}
		calls := isolateAgentRestore(t)
		if err := EnsureAgentLoaded(cfg); err != nil {
			t.Fatalf("EnsureAgentLoaded: %v", err)
		}
		if len(*calls) != 0 {
			t.Errorf("launchctl traffic for a flagless plist: %v", *calls)
		}
	})
	t.Run("registered but unloaded reloads", func(t *testing.T) {
		isolateAgent(t)
		agentLoaded = func() bool { return false }
		daemonProbe = func(config.Config) bool { return false }
		if err := writeAgentPlist("/x/heka", agentFlags{KeepAlive: true}); err != nil {
			t.Fatal(err)
		}
		calls := isolateAgentRestore(t)
		if err := EnsureAgentLoaded(cfg); err != nil {
			t.Fatalf("EnsureAgentLoaded: %v", err)
		}
		for _, c := range *calls {
			if c == "bootstrap" {
				return
			}
		}
		t.Errorf("unloaded registered agent was not re-bootstrapped; calls: %v", *calls)
	})
	t.Run("loaded agent is a no-op", func(t *testing.T) {
		isolateAgent(t)
		agentLoaded = func() bool { return true }
		if err := writeAgentPlist("/x/heka", agentFlags{RunAtLoad: true}); err != nil {
			t.Fatal(err)
		}
		calls := isolateAgentRestore(t)
		if err := EnsureAgentLoaded(cfg); err != nil {
			t.Fatalf("EnsureAgentLoaded: %v", err)
		}
		if len(*calls) != 0 {
			t.Errorf("loaded agent triggered launchctl traffic: %v", *calls)
		}
	})
	t.Run("running daemon skips the heal", func(t *testing.T) {
		isolateAgent(t)
		agentLoaded = func() bool { return false }
		daemonProbe = func(config.Config) bool { return true }
		if err := writeAgentPlist("/x/heka", agentFlags{KeepAlive: true}); err != nil {
			t.Fatal(err)
		}
		calls := isolateAgentRestore(t)
		if err := EnsureAgentLoaded(cfg); err != nil {
			t.Fatalf("EnsureAgentLoaded: %v", err)
		}
		for _, c := range *calls {
			if c == "bootstrap" {
				t.Errorf("bootstrapped although a daemon is running (spawn-collision guard): %v", *calls)
			}
		}
	})
}

// isolateAgentRestore installs a fresh recorder without resetting the
// environment seams, returning the live call log.
func isolateAgentRestore(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	runLaunchctl = func(args ...string) (string, error) {
		calls = append(calls, args[0])
		return "", nil
	}
	return &calls
}
