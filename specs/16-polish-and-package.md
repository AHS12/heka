# SPEC-16 — Dashboard, Settings, Retention & Packaging

Status: **Draft** · Depends on: SPEC-12 (shell/theme), SPEC-13 (tasks UI), SPEC-14 (schedules/runs), SPEC-15 (startup toggles) · Master spec: §12, §22, PRD §22, §27, §37

## Goal

The last spec: the Dashboard page (promised by the shell but never detailed) with charts and graphs, the Settings page (startup/watchdog/[data dirs]/retention), config-file persistence from the GUI, daemon-side retention enforcement, versioning through the build, and the installer packaging with PATH option. After this, everything in PRD §36's acceptance criteria has an owner.

## Scope

**In:**
- `GET /v1/stats` + Dashboard page with charts: task counts, run success/failure trends, schedule overview
- Charts: use a lightweight chart library (e.g. Recharts, Nivo, or Visx) for run history sparklines, status distribution, and recent activity timeline
- Settings page: startup toggle, watchdog toggle, paths display + open data dir, retention fields
- `config.Write` (GUI-persisted settings → `config.yaml`), daemon retention prune timer
- Version plumbing: `-ldflags -X` + `heka --version`
- Windows installer (NSIS, per-user, no admin); `make package`
- **PATH option in installer**: checkbox "Add Heka to PATH" so CLI is available system-wide
- Micro-polish: window size/minsize, empty-state components, consistent toasts, about dialog

**Out:** auto-update, code signing, custom fonts beyond the system stack, HiDPI tuning, Linux/macOS GUI packaging (documented, not built — Windows is the MVP platform per the DoD).

## 1. Dashboard (PRD §22) — Charts & Graphs

`GET /v1/stats`:

```json
{ "tasks": 12, "schedules": 8, "running": 1, "failed_today": 2,
  "runs_today": 15, "success_today": 12,
  "recent_activity": [
    { "kind": "run", "status": "success", "task_slug": "daily-research", "at": "…", "group_id": "…" }
  ],
  "run_history": [
    { "date": "2026-08-25", "success": 12, "failed": 2, "total": 14 }
  ]
}
```

**Stat tiles (top row):**
- Tasks: total count (with enabled breakdown)
- Schedules: enabled count
- Running: currently active runs
- Today: runs_today / failed_today ratio

**Charts (using Recharts — lightweight, tree-shakeable, React-native):**

1. **Run Success/Failure Bar Chart** — last 7 days, stacked bars (green=success, red=failed). Shows trends at a glance.
2. **Status Distribution Donut** — success/failed/timed_out/cancelled breakdown of all runs. Quick health indicator.
3. **Recent Activity Timeline** — last 10 runs with status chips, task names, timestamps. Clickable → logs page.

**Chart library:** Recharts (`recharts` npm package). It's React-native, minimal bundle (~40KB gzipped), supports dark mode via CSS variables, and has no native dependencies. Fits the project's stack perfectly.

**Dark mode:** Charts use CSS custom properties from the existing accent/theme system. Recharts accepts `fill`/`stroke` colors, so we pass theme-aware colors.

## 2. Settings page

| Section | Control | Backing |
|---|---|---|
| Daemon | data dir + tasks dir display, **Open data dir** button | `App.OpenDataDir()` — spawns explorer/finder (user-level) |
| Startup | Start with system (SPEC-15) | `StartupEnabled/StartupSet` |
| Reliability | Watchdog guard (SPEC-10) | `WatchdogEnabled/WatchdogSet` |
| Retention | log retention days, max output bytes | `GetSettings/UpdateSettings` → `config.Write` |

- Settings bindings round-trip through `config` (SPEC-02): `GetSettings() SettingsDTO`, `UpdateSettings(patch)` re-marshals `config.yaml` (atomic temp+rename, same discipline as task files) and hot-applies what's live (retention timer, output cap) with **no daemon restart**.
- Display-only fields (paths) are never editable — changing them is a deliberate advanced action, still env/file only (SPEC-02 precedence unchanged).

## 3. Retention enforcement (callsite that's been reserved since SPEC-03/09)

- Daemon starts a nightly timer (and one run at startup) calling `RunStore.Prune(now - log_retention_days)`.
- Window config via Settings applies to the next prune; `max_output_bytes` flows into the executor at wiring (already a constructor arg, SPEC-05).
- Prune is safe to run side-by-side with active runs (WHERE on finished_at; never touches `running` rows).

## 4. Version plumbing

- `main.appVersion` becomes a `var` (was `const 0.1.0`) set by `-ldflags "-X main.appVersion=<ver>"`.
- `make build`/`make package` pass the version from a single source: `VERSION` var in the Makefile (default `0.1.0`), mirrored into `wails.json` (`info.productVersion`).
- `heka --version` prints it; `/v1/health` and about dialog show the same value.
- Test: build with an overridden `VERSION=9.9.9-test` → `heka --version` reports it.

## 5. Packaging (Windows MVP)

- `make release-windows` builds `heka.exe` with Wails `-windowsconsole`, builds `heka-gui.exe` with the GUI subsystem, and packages both in the NSIS installer.
- NSIS config lives in `build/windows/installer/`. Start-menu, desktop, and finish-page actions launch `heka-gui.exe`; startup/watchdog registration and terminal PATH usage resolve to `heka.exe`.
- **PATH checkbox:** Installer includes an optional checkbox "Add Heka to PATH" which appends the install directory to the user's PATH environment variable. Default: checked. Implementation: manual registry edit for `HKCU\Environment\Path`.
- Upgrades stop stale processes and restore the existing startup/watchdog entries against the new console executable; Settings/tray remain the runtime controls.
- Linux/macOS: documented recipes in `README` (AppImage/deb + .app); not built or verified.

## 6. Micro-polish

- Window `MinWidth/MinHeight` in `wails.json`/options (e.g. 800×560); remember window size is out (OS handles).
- Shared empty-states for tables (no tasks/schedules/runs yet — each with a primary action).
- Consistent toast behavior (hero toast in one place, no ad-hoc alerts).
- About dialog: name/version/daemon status/health (from the existing bindings).
- Dark/light refinements from the design pass (SPEC-12 tokens — no new colors in this spec).

## 7. Testing

1. Stats endpoint: counters correct with seeded data; failed_today boundary at local midnight (inject clock); run_history returns correct daily aggregates.
2. Dashboard: chart rendering with mocked data; dark mode colors correct; empty state when no runs.
3. Settings: `UpdateSettings` writes `config.yaml`, survives `config.Load` round-trip; atomic write verifiable (no partial file); invalid values (0/negative retention, capped output) rejected client+server side.
4. Retention: prune timer fires with the configured cutoff; active `running` rows untouched; changing settings mid-flight takes effect on next prune.
5. Version: ldflags override test per §4.
6. Installer smoke: `make release-windows` runs end-to-end, packages both executables, and produces the versioned installer.
7. Binary smoke: `heka.exe help` and `heka.exe --version` write to the terminal; PE headers identify `heka.exe` as console and `heka-gui.exe` as GUI.
8. PATH: installer with PATH checkbox checked modifies user PATH; `heka` command works from a new terminal.
9. `make check` green.

## 8. Files

```text
frontend/src/pages/{DashboardPage,SettingsPage}.tsx
frontend/src/components/{StatTile,EmptyState,RunChart,StatusDonut,ActivityTimeline}.tsx
frontend/src/lib/{dashboard,settings}.ts     # hooks
internal/ipc/stats.go                         # GET /v1/stats
internal/config/config.go                     # + Write(cfg) atomic, + hot-applied fields
internal/daemon/daemon.go                     # retention timer
internal/app/app.go                           # + settings/startup/watchdog/dialog passthroughs
internal/cli/root.go                          # + --version
main.go                                      # appVersion → var
Makefile                                     # VERSION, package target, ldflags
build/windows/nsis/…                         # per-user installer config + PATH checkbox
```

## 9. Acceptance criteria

1. Dashboard shows live counters + charts (PRD §22), refreshed on visibility. Charts render correctly in light and dark mode.
2. Settings toggles persist (startup + watchdog, verified by read-back); retention change survives restart and prunes on the new cutoff.
3. `heka --version` reflects the Makefile `VERSION` (ldflags).
4. `make release-windows` produces an installer containing console `heka.exe` and GUI `heka-gui.exe`; installed app opens and reports the right version (manual install → launch → version check).
5. PATH checkbox works: after install with checkbox, `heka` is available in new terminal sessions and help/version output is visible.
6. Empty states + about dialog render.
7. `make check` green.

## DoD

1. Spec approved by user.
2. Criteria verified on this machine; one full install→launch cycle with the user.
3. With this spec, every PRD §36 MVP acceptance item maps to at least one spec in this repository — the mapping is noted in the final review.