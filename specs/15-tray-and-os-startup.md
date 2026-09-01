# SPEC-15 — Tray & OS Startup

Status: **In Progress** · Depends on: SPEC-08 (daemon control), SPEC-09 (scheduler), SPEC-10 (watchdog), SPEC-12 (GUI) · Master spec: D6, §3, PRD §16–17

## Goal

The daemon becomes a proper background citizen: a system-tray icon it owns (D6 — the tray isn't the GUI's), OS startup registration (user-level, no admin), and the Settings wiring that later toggles startup + watchdog from the GUI.

## Scope

**In:**
- `internal/osapp/tray.go`: systray lifecycle owned by the **daemon** (getlantern/systray, D6), menu per PRD §16
- `internal/osapp/startup_{windows,linux,darwin}.go`: user-level startup registration
- Scheduler `Pause`/`Resume` (the tray's Pause Scheduler item)
- CLI: `heka daemon startup on|off|status`; tray is the second control surface
- Passthrough bindings for Settings-page integration (SPEC-16 consumes them)

**Out:** launch-at-login for the GUI itself (the daemon is what starts; the GUI is opened on demand via the tray), notifications from the tray, per-user multi-instance management.

## 1. Tray (daemon-owned, PRD §16)

```text
Heka
──────────────────
Open                      → spawns `heka gui` (os/exec, same exe) — the daemon brings the window up
Run Task ▸                → submenu: task list (slug + last status); click = executor Start (trigger "system")
Active Jobs              → submenu: currently running groups (task, elapsed); click → opens GUI /runs
Recent Runs ▸            → submenu: last 5 runs (status icon, task, time); click → GUI run detail
Pause Scheduler          → checkbox; toggles engine pause
──────────────────
Start with system        → checkbox; startup registration (this spec)
Watchdog guard           → checkbox; SPEC-10 installer
──────────────────
Quit                     → graceful shutdown (same sequence as SPEC-06)
```

**Threading**: systray demands its own thread — the daemon's `Run` starts the core in a goroutine and hands the main thread to `systray.Run(onReady, onExit)`; `onExit` triggers the graceful shutdown (IPC listeners stop, DB closes). The daemon's signal handling stays for console runs. This is the known systray integration wart; documented in the code.

- All menu data (tasks, running groups, recent runs) is read straight from the daemon's own db — the tray needs no IPC round-trips.
- `Open`/run links spawn the GUI via `exec.Command(self, "gui")`; failure → tray tooltip note, no crash.

## 2. Scheduler Pause/Resume (small extension to SPEC-09)

- `kv` flag `scheduler_paused`; `Pause()`/`Resume()` on the engine set it and `Sync()`.
- While paused: recurring ticks are skipped (a `skipped`-status run row is **not** written for pause — pause is operator intent, not overlap; the schedule's `next_run_at` simply stops advancing). One-time jobs don't fire either.
- Status visible via `/v1/health` (`Scheduler: "paused"` — SPEC-06/09 field gains the third value).

## 3. Startup registration (PRD §17, user-level only)

| Platform | Mechanism | Value registered |
|---|---|---|
| Windows | HKCU `Software\Microsoft\Windows\CurrentVersion\Run` → `Heka` | `"<abs path>\heka.exe" daemon` (direct long-lived daemon launch; the binary is built as a Windows GUI executable, so no console opens) |
| Linux | systemd **user** unit `heka-daemon.service` (WantedBy=default.target, `systemctl --user enable`) | `ExecStart=<abs path>/heka daemon` (fallback to autostart `.desktop` if systemd unavailable) |
| macOS | LaunchAgent `com.heka.daemon.plist` (RunAtLoad) | `<abs path>/heka daemon` |

- Registered value always uses `os.Executable()` at enable-time; moving the binary requires re-enabling (documented in the tray tooltip copy).
- No elevation anywhere (user scope, PRD §17 "must not require administrator privileges").

```go
// internal/osapp
type StartupRegistrar interface {
    Enable(exePath string) error
    Disable() error
    Enabled() (bool, error)
}
```

Windows impl uses `golang.org/x/sys/windows/registry` (new dep); exec-based fakes elsewhere.

## 4. CLI + bindings

```text
heka daemon startup on|off|status   # thin wrappers over the registrar
```

`internal/app` passthroughs (consumed by the Settings page in SPEC-16):

```go
StartupEnabled() (bool, error)   StartupSet(on bool) error
WatchdogEnabled() (bool, error)  WatchdogSet(on bool) error   // SPEC-10 installer
```

## 5. Testing

1. Menu builder: db rows → menu structure (pure function). Unit-test Run Task submenu contents, Recent Runs order/cap, Active Jobs contents.
2. Pause/Resume: paused → ticks stop, `next_run_at` frozen, health shows `paused`; resume → ticks restart (ticker-based engine test with `@every 1s`).
3. Windows registrar command generation is unit-tested without touching HKCU: enable writes the direct command string (`"…\heka.exe" daemon`), matching accepts path case differences, and the former `daemon start` wrapper is treated as stale. One *real* HKCU write happens only in manual acceptance.
4. Linux/macOS: unit + plist/systemd string assertions (verified-on-CI only, per SPEC-10 precedent).
5. Manual (with user): daemon shows tray; **Open** spawns the GUI; **Run Task** runs; **Pause** freezes next run; **Quit** exits cleanly; `startup on` then reading the Run key shows the entry.
6. `make check` green.

## 6. Files

```text
internal/osapp/tray.go            # systray lifecycle + menu building
internal/osapp/startup_windows.go # registry registrar
internal/osapp/startup_linux.go   # systemd user unit / autostart
internal/osapp/startup_darwin.go  # launchd plist
internal/core/scheduler/scheduler.go # + Pause/Resume + paused state
internal/cli/daemon.go            # + startup on|off|status
internal/app/app.go               # + Startup/Watchdog passthroughs
internal/daemon/daemon.go         # tray wiring, thread handoff
go.mod                            # + getlantern/systray, x/sys/windows/registry
build/appicon.png                 # reused as tray icon
```

## 7. Acceptance criteria

1. Tray present with daemon; all menu items functional per §1; Quit = graceful shutdown (IPC clients see daemon-not-running after).
2. Pause Scheduler: next_run freezes; health reports `paused`; resume works.
3. `daemon startup on|off|status` round-trips through the real HKCU Run key (verified by read-back).
4. Watchdog checkbox drives the SPEC-10 installer.
5. `make check` green.

## DoD

1. Spec approved by user.
2. Criteria verified on this machine (tray + HKCU run key are manual checks with the user).
3. Linux/macOS registrars code-complete with fake-exec tests; Windows is the verified platform.