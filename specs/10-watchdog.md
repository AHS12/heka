# SPEC-10 — Watchdog (OS-level daemon reliability)

Status: **Draft** · Depends on: SPEC-06/08 (daemon status/start), SPEC-02 (data dir) · Master spec: D12, §7 layer 10

## Goal

An OS-registered watchdog that catches a dead daemon and brings it back — because a stopped daemon means missed schedules, and the user's trust in Heka is exactly "it just runs, effortlessly" (PRD §7 reliability). Missed-run reconciliation (SPEC-09) heals gaps after the next start; the watchdog makes sure the next start happens in minutes, not whenever someone notices.

Scope is narrow and deliberate: **watchdog ≠ OS task scheduling** (that stays post-v1, D10). The watchdog only watches the *daemon process*, never task execution.

## Scope

**In:**
- `heka daemon watch [--once]` — the check-and-restart command (Go, testable; no .cmd/.sh scripting)
- `heka daemon watchdog install|uninstall|status` — OS entry management
- `internal/osapp/watchdog.go` (shared logic) + platform files
- Backoff state file to prevent restart storms

**Out:** OS-native task scheduling (D10), watching task execution itself, admin/UAC elevation (never needed — user-level), anything GUI-side (SPEC-16 surfaces a toggle).

## 1. Mechanism

OS schedulers run a check on a cadence (default 5 minutes):

| Platform | Entry | Action |
|---|---|---|
| Windows | Task Scheduler task `Heka Watchdog`, `schtasks /SC MINUTE /MO 5` | `<heka-gui> daemon watch --once` (GUI-subsystem binary, so no console window flashes) |
| Linux | systemd **user** timer `heka-watchdog.{service,timer}` (OnUnitActiveSec=5min); fallback: user crontab `*/5 * * * *` (documented in the spec's README notes) | same |
| macOS | launchd LaunchAgent `heka.watchdog.plist` (StartInterval=300) | same |

`daemon watch --once` logic (the whole command):

```text
1. osapp.DaemonAlive(cfg)?  → exit 0                      # daemon responds to IPC ping
2. backoff check:          if state file shows a restart attempt < 1 min ago → exit 0  # don't pile on
3. daemon.Start(cfg)       → success                       → record attempt in state file, exit 0
                            → failure (readiness ping timed out)                     → record, exit 1
```

Exit codes matter: `0` in cases the OS entry should treat as "fine" (no Task Scheduler failure spam), `1` when a restart genuinely failed (visible in the OS entry's last-result for diagnostics).

## 2. Backoff file

`<data>/watchdog.state` (JSON, owned by the watchdog):

```json
{ "last_restart_attempt": "2026-08-24T19:00:00Z", "attempts_last_minute": 1 }
```

- Reason for a file, not the DB/kv: when the daemon is down, **nothing else is readable**.
- Intent: if the daemon crash-loops, the watchdog backs off instead of bouncing it every 5 minutes. Beyond 2 attempts/minute → skip restart (exit 0) and leave diagnostics to `daemon status` + the GUI (SPEC-16).
- The daemon itself may clear the file on a healthy start (optional, in SPEC-10 if cheap — otherwise SPEC-16).

## 3. Install / uninstall / status

```text
heka daemon watchdog install     → creates the OS entry (default 5 min interval; --interval flag)
heka daemon watchdog uninstall   → removes it
heka daemon watchdog status      → installed? cadence? last result? (from schtasks query / systemctl list-timers / launchctl)
```

- All user-level, no admin (schtasks default /RU current user; systemd `--user`; launchd user agent) — matches PRD §17's "no administrator privileges" rule.
- Platform implementations sit behind one interface:

```go
type Installer interface {
    Install(interval time.Duration, hekaPath string) error
    Uninstall() error
    Status() (Installed bool, Interval time.Duration, err error)
}
```

Wrapped exec calls (`schtasks.exe`, `systemctl --user`, `launchctl`) run through an injectable `execRunner` so unit tests fake the OS.

## 4. CLI placement

Added under the existing cobra `daemon` subcommand (SPEC-08):

```text
heka daemon watch              # foreground loop (for manual testing / container-style runs)
heka daemon watch --once       # what the OS entries call
heka daemon watchdog install/--interval 10
heka daemon watchdog uninstall
heka daemon watchdog status
```

## 5. Testing

1. `WatchOnce` with fake alive/start fns: alive → no start; down → start called once; restart fails → error (exit 1); backoff file fresh → no start, no error.
2. Backoff: 3 rapid `WatchOnce` calls with a failing start → only the first restart attempt; subsequent exit 0.
3. Windows installer with fake exec runner: correct `schtasks /Create /SC MINUTE /MO N /TN "Heka Watchdog" /TR "<hekaPath> daemon watch --once"`; `Uninstall` `/Delete /F`; `Status` parses `/Query` output.
4. Generated systemd unit / launchd plist contain the right ExecStart/cadence (string assertions) + install via `systemctl --user enable --now` in fake exec.
5. Manual acceptance (do with the user): install → daemon stop → wait ≤ interval → daemon status shows running again.
6. `watch --once` exit codes verified in-process.
7. `make check` green.

## 6. Files

```text
internal/osapp/watchdog.go            # WatchOnce, backoff state file
internal/osapp/watchdog_windows.go    # schtasks Installer
internal/osapp/watchdog_linux.go      # systemd user units (+ crontab note)
internal/osapp/watchdog_darwin.go     # launchd plist
internal/cli/daemon.go                # + watch / watchdog subcommands
go.mod                                # no new deps
```

## 7. Acceptance criteria

1. `watchdog install` creates a working OS entry; `status` reports it; `uninstall` removes it (verified on this Windows machine).
2. Live test: kill the daemon → within the interval the watchdog restores it (`daemon status` → running).
3. Crash-loop case: watchdog backs off and does not ping-pong (unit test + observable last-result).
4. `watch --once` exit-code contract per §1.
5. No admin prompts anywhere; no elevation.
6. `make check` green.

## DoD

1. Spec approved by user.
2. Criteria verified **on this machine** (the kill-and-recover test is the point of this spec).
3. Windows is the only required platform for the acceptance run; linux/darwin installers ship code-complete with fake-exec tests (documented as verified-on-CI-only).