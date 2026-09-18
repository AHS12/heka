//go:build darwin

package osapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"heka/internal/config"
	"heka/internal/ipc"
)

// The daemon's launchd agent (SPEC-15 §3, SPEC-17 §4.12). One plist, two
// orthogonal toggles: Startup (RunAtLoad) and Watchdog
// (KeepAlive{SuccessfulExit:false} — respawn on crash/signal/non-zero exit
// only, so `heka daemon stop` and tray Quit stay down).
const launchAgentID = "com.heka.daemon"

// agentFlags is the four-state startup×watchdog matrix over one plist.
type agentFlags struct {
	RunAtLoad bool
	KeepAlive bool
}

// launchctlTimeout bounds every launchctl invocation: launchd is normally
// instant, but a bootout of a wedged job or a busy bootstrap can block
// indefinitely — and ApplyAgent runs inside a synchronous GUI binding, so an
// unbounded exec would hang the Settings toggle forever.
const launchctlTimeout = 10 * time.Second

// runLaunchctl is the launchctl seam (tests substitute a recorder).
var runLaunchctl = func(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), launchctlTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "launchctl", args...).CombinedOutput()
	return string(out), err
}

func launchDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

func agentPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentID+".plist")
}

// agentStdioPath is where launchd captures the daemon's stdout/stderr when
// it spawns the agent (boot-time failures were invisible before this —
// StandardErrorPath is the only way to see why a login start never ran).
func agentStdioPath() string {
	if cfg, err := config.LoadDefault(); err == nil && cfg.DataDir != "" {
		return filepath.Join(cfg.DataDir, "daemon.launchd.log")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "heka", "daemon.launchd.log")
}

// writeAgentPlist renders the agent definition for the given flag state.
// The binary path is the only other variable; keep the key order stable so
// content comparisons detect real changes only.
func writeAgentPlist(exePath string, f agentFlags) error {
	if err := os.MkdirAll(filepath.Dir(agentPlistPath()), 0o700); err != nil {
		return err
	}
	stdio := agentStdioPath()
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.heka.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>` + exePath + `</string>
        <string>daemon</string>
    </array>
    <key>StandardOutPath</key>
    <string>` + stdio + `</string>
    <key>StandardErrorPath</key>
    <string>` + stdio + `</string>
`)
	if f.RunAtLoad {
		sb.WriteString(`    <key>RunAtLoad</key>
    <true/>
`)
	}
	if f.KeepAlive {
		// Respawn on abnormal exit only: crashes and signals come back,
		// clean exits (heka daemon stop, tray Quit) stay down.
		sb.WriteString(`    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
`)
	}
	sb.WriteString(`</dict>
</plist>
`)
	return os.WriteFile(agentPlistPath(), []byte(sb.String()), 0o600)
}

// readAgentFlags parses the installed plist. A missing file is all-false;
// a legacy plist (plain `KeepAlive: true`) maps to KeepAlive on.
func readAgentFlags() agentFlags {
	data, err := os.ReadFile(agentPlistPath())
	if err != nil {
		return agentFlags{}
	}
	s := string(data)
	f := agentFlags{
		RunAtLoad: strings.Contains(s, "<key>RunAtLoad</key>"),
		KeepAlive: strings.Contains(s, "<key>KeepAlive</key>"),
	}
	return f
}

// agentLoaded reports whether the agent is bootstrapped into the user's
// launchd GUI domain. Var so tests can script the loaded/unloaded states.
var agentLoaded = func() bool {
	_, err := runLaunchctl("print", launchDomain(), launchAgentID)
	return err == nil
}

// reloadAgent re-applies the plist to launchd: bootout (ignore errors —
// covers "not loaded"), then bootstrap. launchd is known to reject a
// bootstrap that lands immediately after a bootout of the same label
// ("Input/output error"), so bootstrap is retried with a short backoff; the
// pre-v2.15 `load` verb is the last resort for older hosts. Does not start
// anything by itself unless the plist says so (RunAtLoad / KeepAlive).
func reloadAgent() error {
	_, _ = runLaunchctl("bootout", launchDomain()+"/"+launchAgentID)
	var last string
	for attempt := 0; attempt < 3; attempt++ {
		out, err := runLaunchctl("bootstrap", launchDomain(), agentPlistPath())
		if err == nil {
			return nil
		}
		last = fmt.Sprintf("launchctl bootstrap: %v: %s", err, strings.TrimSpace(out))
		// A failed bootstrap can still mean "already bootstrapped" — the
		// job being registered is what matters, not who registered it.
		if agentLoaded() {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	if _, err := runLaunchctl("load", agentPlistPath()); err == nil {
		return nil
	}
	return errors.New(last)
}

// kickstartAgent starts (or restarts) the daemon under launchd supervision.
func kickstartAgent() error {
	_, err := runLaunchctl("kickstart", launchDomain()+"/"+launchAgentID)
	return err
}

// AgentLoaded reports whether the launchd agent is bootstrapped — daemons
// started while it is go through kickstart so they run supervised
// (SPEC-17 §4.12).
func AgentLoaded() bool { return agentLoaded() }

// KickstartDaemon starts the daemon via launchd (supervised start).
func KickstartDaemon() error { return kickstartAgent() }

// RepairAgentPath rewrites the agent plist to launch the given binary if it
// currently points elsewhere (app moved/renamed after an upgrade). No-op
// when the path is already current or no plist exists.
func RepairAgentPath(exePath string) error {
	data, err := os.ReadFile(agentPlistPath())
	if err != nil {
		return nil // no plist — nothing to repair
	}
	if strings.Contains(string(data), "<string>"+exePath+"</string>") {
		return nil // current
	}
	return writeAgentPlist(exePath, readAgentFlags())
}

// ---- Daemon process transitions (complement rule: a toggle never leaves
// you with a different daemon state than before — running stays running,
// supervised matches the new flags).

// stopDaemon asks the running daemon for a clean IPC shutdown and waits for
// the endpoint to disappear. Clean exits leave launchd quiet (SuccessfulExit
// guard), so this is safe against a supervised daemon too.
func stopDaemon(cfg config.Config) bool {
	client := ipc.NewClient(cfg)
	if err := client.Shutdown(); err != nil {
		return false
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Health(); err != nil {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

// daemonProbe reports whether a daemon answers IPC health. Var so tests can
// script the running/stopped transitions.
var daemonProbe = func(cfg config.Config) bool {
	_, err := ipc.NewClient(cfg).Health()
	return err == nil
}

// daemonRunning reports whether an IPC health ping answers.
func daemonRunning(cfg config.Config) bool { return daemonProbe(cfg) }

// ApplyAgent writes the plist for f, reloads launchd, and reconciles the
// running daemon: it was running before the call → it runs after (kickstarted
// under launchd when the flags supervise it, detached otherwise). exePath is
// the binary the agent should launch; startDetached is the manual-spawn seam
// (daemon.Start) used when supervision is off.
//
// Guard rails:
//   - A plist already matching f with the agent loaded is a no-op — the
//     toggle neither stops nor restarts the daemon (the historical freeze:
//     a redundant toggle cost a full stop/restart cycle, ~10–25 s).
//   - A plist already matching f with the agent NOT loaded falls through to
//     the full reload: that is the broken state a failed reload leaves
//     behind, and re-applying the toggle heals it.
//   - On any failure after the daemon was stopped, the daemon is restored
//     detached — a failed toggle must never leave Heka down.
func ApplyAgent(cfg config.Config, exePath string, f agentFlags, startDetached func(config.Config) error) error {
	wasRunning := daemonRunning(cfg)

	if readAgentFlags() == f {
		neitherFlag := !f.RunAtLoad && !f.KeepAlive
		if neitherFlag || agentLoaded() {
			return nil // desired state already in place; daemon untouched
		}
		// plist matches but launchd lost the job — fall through and reload.
	}

	err := applyAgent(cfg, exePath, f, startDetached, wasRunning)
	if err != nil && wasRunning && !daemonRunning(cfg) {
		_ = startDetached(cfg) // best-effort restore; surface the real error
	}
	return err
}

func applyAgent(cfg config.Config, exePath string, f agentFlags, startDetached func(config.Config) error, wasRunning bool) error {
	if wasRunning {
		stopDaemon(cfg)
	}
	if f.RunAtLoad || f.KeepAlive {
		if err := writeAgentPlist(exePath, f); err != nil {
			return err
		}
		if err := reloadAgent(); err != nil {
			return err
		}
		if f.RunAtLoad || wasRunning {
			if err := kickstartAgent(); err != nil {
				return fmt.Errorf("launchctl kickstart: %w", err)
			}
			if err := waitDaemon(cfg); err != nil {
				return err
			}
		}
		return nil
	}
	// Both off: remove the agent entirely.
	_, _ = runLaunchctl("bootout", launchDomain()+"/"+launchAgentID)
	if err := os.Remove(agentPlistPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	if wasRunning {
		return startDetached(cfg)
	}
	return nil
}

// waitDaemon polls IPC health until the daemon answers (≤15 s). Var so
// tests can skip the real poll.
var waitDaemon = func(cfg config.Config) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if daemonProbe(cfg) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready within 15s")
}

// SetStartupAgent flips the Startup toggle (RunAtLoad), preserving the
// current Watchdog bit, and reconciles the running daemon. macOS path used
// by Settings + CLI; other platforms keep their plain registrar.
func SetStartupAgent(cfg config.Config, exePath string, on bool, startDetached func(config.Config) error) error {
	f := readAgentFlags()
	f.RunAtLoad = on
	return ApplyAgent(cfg, exePath, f, startDetached)
}

// SetWatchdogAgent flips the Watchdog toggle (KeepAlive guard), preserving
// the current Startup bit. When turning the guard on while a daemon runs
// unsupervised, the daemon is restarted under launchd (launchd cannot adopt
// a running process).
func SetWatchdogAgent(cfg config.Config, exePath string, on bool, startDetached func(config.Config) error) error {
	f := readAgentFlags()
	f.KeepAlive = on
	return ApplyAgent(cfg, exePath, f, startDetached)
}

// EnsureAgentLoaded re-bootstraps the launchd agent when the plist
// registers the daemon (RunAtLoad or KeepAlive) but launchd has lost the
// job — a failed reload after a toggle, a manual bootout, or an agent
// never loaded since the plist was written. Called at GUI startup so the
// heal runs in a process that is NOT the daemon's job: reloading launchd
// from inside the daemon would bootout (kill) the caller. When a detached
// daemon already serves IPC, the heal is skipped — bootstrapping then
// spawns a second daemon that loses the bind race and gets respawn-looped
// by KeepAlive; the next toggle or reboot repairs it instead.
// Idempotent: a loaded agent or a missing/flag-less plist is a no-op.
func EnsureAgentLoaded(cfg config.Config) error {
	f := readAgentFlags()
	if !f.RunAtLoad && !f.KeepAlive {
		return nil
	}
	if agentLoaded() {
		return nil
	}
	if daemonRunning(cfg) {
		return nil
	}
	return reloadAgent()
}

// ---- StartupRegistrar (SPEC-15 §3) — plist content is the source of truth.

type launchdStartupRegistrar struct{}

func newStartupRegistrar() StartupRegistrar { return &launchdStartupRegistrar{} }

// Enable turns Startup on, preserving the Watchdog bit. The daemon keeps
// running as-is (no transition here — the tray path is conservative; the
// GUI path uses SetStartupAgent for the full dance).
func (r *launchdStartupRegistrar) Enable(exePath string) error {
	f := readAgentFlags()
	f.RunAtLoad = true
	if err := writeAgentPlist(exePath, f); err != nil {
		return err
	}
	if agentLoaded() {
		return reloadAgent()
	}
	domain := launchDomain()
	if out, err := runLaunchctl("bootstrap", domain, agentPlistPath()); err != nil {
		if out2, err2 := runLaunchctl("load", agentPlistPath()); err2 != nil {
			return fmt.Errorf("launchctl bootstrap: %w: %s; load fallback: %w: %s", err, out, err2, out2)
		}
	}
	return nil
}

// Disable turns Startup off. With the Watchdog guard still on, the plist is
// rewritten (KeepAlive only) and reloaded — the supervised daemon survives
// via the SuccessfulExit guard (a reload's bootout kills it, launchd
// respawns it). With the guard off, the agent is removed entirely.
func (r *launchdStartupRegistrar) Disable() error {
	f := readAgentFlags()
	f.RunAtLoad = false
	if f.KeepAlive {
		exe := agentExecutable()
		if err := writeAgentPlist(exe, f); err != nil {
			return err
		}
		return reloadAgent()
	}
	_, _ = runLaunchctl("bootout", launchDomain()+"/"+launchAgentID)
	_, _ = runLaunchctl("unload", agentPlistPath())
	return os.Remove(agentPlistPath())
}

// Enabled reads plist *content* — RunAtLoad is what starts the daemon at
// login; a watchdog-only plist must not report startup on (SPEC-17 §4.12).
func (r *launchdStartupRegistrar) Enabled() (bool, error) {
	return readAgentFlags().RunAtLoad, nil
}

// agentExecutable extracts the binary path from the installed plist (the
// first ProgramArguments entry) so flag rewrites preserve it.
func agentExecutable() string {
	fallback := func() string {
		exe, _ := os.Executable()
		return exe
	}
	data, err := os.ReadFile(agentPlistPath())
	if err != nil {
		return fallback()
	}
	s := string(data)
	args := strings.Index(s, "<key>ProgramArguments</key>")
	if args < 0 {
		return fallback()
	}
	first := strings.Index(s[args:], "<string>")
	if first < 0 {
		return fallback()
	}
	seg := s[args+first+len("<string>"):]
	end := strings.Index(seg, "</string>")
	if end < 0 {
		return fallback()
	}
	return strings.TrimSpace(seg[:end])
}

// startupPointsAtImpl reports whether the agent plist's first
// ProgramArguments entry references the given binary. RepairEntries uses it
// to detect upgrades: a stale path triggers an idempotent re-Enable.
func startupPointsAtImpl(exe string) bool {
	data, err := os.ReadFile(agentPlistPath())
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "<string>"+exe+"</string>")
}
