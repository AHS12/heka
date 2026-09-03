# Changelog

All notable changes to Heka are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.8.0] - 2026-09-04

Backup & restore, plus a proper home for a growing vault. Every archive is an
AES-256-encrypted zip that captures the complete Heka state — a consistent
SQLite snapshot, the vault key, the canonical task YAML files, and the user
config — so a restore brings back exactly what was there, secrets included.

### Added
- Automatic backups on a configurable schedule — every N hours (up to a
  year), daily, weekly, or monthly at a local time — with missed-run
  catch-up: if Heka was closed at the scheduled time, the missed backup
  runs once at startup.
- Local destination (custom folder + keep-latest retention, default inside
  the data directory) and an S3-compatible remote destination — AWS S3,
  Cloudflare R2, Backblaze B2, MinIO — with its own keep-latest pruning and
  a one-click "Test connection" that exercises the bucket end to end.
- S3 credentials live in the secrets vault (`BACKUP_S3_ACCESS_KEY_ID`,
  `BACKUP_S3_SECRET_ACCESS_KEY`), same write-only discipline as every other
  secret.
- Archive encryption with a passphrase stored in the vault
  (`BACKUP_PASSPHRASE`) so scheduled backups run unattended — with a loud
  warning that losing it means losing the backups, since restore on a new
  machine asks for it.
- Manual "Back up now" with live status, a job history view (trigger,
  status, size, per-destination outcomes), and desktop notifications when a
  scheduled backup fails.
- A guided restore flow: pick an archive, verify it with the passphrase,
  preview exactly what it contains (created when, which Heka version, task
  counts), choose optional parts (config.yaml, run artifacts), and let the
  app stop the daemon, restore, and prompt to start it again. A safety
  backup of the previous state is written automatically before anything is
  overwritten; archives from newer Heka versions are refused, and checksums
  guard against truncated or tampered archives.
- A dedicated Backup section in Settings with all of the above, and
  `heka backup create|status|history|test` plus `heka restore` in the CLI.
- A dedicated secrets manager page (`/secrets`, linked from Settings →
  Secrets) built for large vaults: search, sort, "unused only" filter,
  per-key usage tracking (which tasks reference `${KEY}`), bulk delete with
  confirmation, pagination, and per-key value replacement. Keys stay the
  only thing ever displayed — values remain write-only.

### Changed
- Backup catch-up is now part of the missed-run reconcile pipeline: a missed
  auto-backup runs once on daemon start, on wake from sleep, on the periodic
  reconcile cadence, and from the "Reconcile now" button — not only via the
  backup loop's own tick. A reconciled backup is labeled "catch-up" in the
  job history, and the missed window is recorded in the system log
  (Logs → System), alongside new events for manual starts and schedule
  arming.
- The backup time-of-day picker is a themed segmented field (HeroUI
  TimeField) instead of the native browser time dropdown, which overflowed
  the panel.

### Fixed
- The Schedules page no longer shows a stale NEXT RUN after a missed run
  was caught up. Reconcile closed the miss window without re-deriving
  `next_run_at`, and the startup path read a cron entry that only carries a
  value once the engine starts — so a daily 9:00 schedule kept showing
  yesterday's 9:00 as next. Both paths now compute the next occurrence
  directly from the cron spec.
- S3 "Test connection" now saves pending edits first (endpoint, bucket,
  keys, Use HTTPS), so it tests exactly what is on screen. Enabling the S3
  destination also defaults to HTTPS — plain HTTP is 301-redirected by
  Cloudflare and friends, which previously surfaced as a wall of HTML; the
  redirect now produces a clear, actionable error instead.

## [0.7.7] - 2026-09-04

A fresh coat of paint: the app icon — window, executable, installer, tray,
and About page — is replaced with the new artwork (yellow "H" with the robot,
clock, browser, and gear).

### Changed
- New application icon everywhere it appears: the executable and installer
  icons (multi-size ICO rebuilt at 16–256 px), the Wails app icon, the system
  tray icon (now generated from an exact 32 px source for crisper results),
  and the About page logo.

### Fixed
- The dashboard "Last 7 Days" chart no longer reshuffles on every poll. The
  stats endpoint flattened its per-date build map in randomized Go map
  order, so the days arrived scrambled (e.g. 09-03 first) each time the
  dashboard re-fetched; run history is now sorted by date ascending.

## [0.7.6] - 2026-09-03

A reliability patch for the missed-run machinery, prompted by a field report:
a daily 9:00 schedule missed its activation while the daemon was down, and
reconcile kept insisting there was nothing to catch up.

### Fixed
- Missed-run reconciliation no longer masks the next occurrence when the
  previous run finished within the same second it started. Such sub-second
  runs store `started_at == finished_at == last_run_at` (RFC 3339 has second
  precision), and the inclusive window boundary counted that already-
  accounted run as "fired" in the next window — so `missed` computed as
  `1 - 1 = 0` and the missed activation was never caught up. The boundary is
  now exclusive.
- The periodic reconcile pass now logs its outcome every tick to
  Logs → System (`reconcile (periodic): checked N schedule(s), M caught up`),
  proving the loop is alive; previously it stayed silent unless it actually
  caught something.

## [0.7.5] - 2026-09-03

This release is about the part of Heka you see first. The Home screen, the
task list, and the task dialogs got a full pass to feel like a proper desktop
app — quicker to read, quicker to act.

### Added
- A reworked Home screen. Up top you'll find the state of the engine at a
  glance (is the daemon up, is the scheduler running) and three quick
  actions: create a task, create a schedule, or run any task right from a
  small picker — no need to visit other pages for the everyday stuff.
- The next scheduled run now gets the spotlight it deserves: which task,
  when it fires, and how long from now ("in 14h 2m"), with a one-click
  "Run now" if you can't wait.
- Recent activity rows now open the exact run they refer to — one click from
  the dashboard straight to that run's output — and show friendly times like
  "2m ago" (the full timestamp appears when you hover).
- Hovering the status dot in the top bar — or the engine badge on Home —
  now tells you what's actually going on: the version, the state of the
  core and scheduler, and how long the daemon has been up. When the
  scheduler is paused, it says so plainly.
- The task list is now made of cards instead of a dense table. Each card
  shows the task, its type, when it last ran and how it went, with Run,
  enable/disable and delete right on the card. Clicking a card opens the
  editor.

### Changed
- Creating and editing tasks now happens in a dialog, the same way for both.
  Editing no longer navigates away from the task list, and importing a task
  file drops you straight into the editor to review it.
- The task dialog is wider, so the form breathes and the YAML editor has
  room to work.
- The default window size is larger, and Tasks and Schedules use the extra
  room. On small laptop screens the window quietly shrinks to fit — the
  shape stays the same.
- Statuses are written the way people say them — "Timed out" instead of
  "timed_out" — everywhere, including the charts and the log filters. The
  status chart now shows readable labels with aligned counts, and the log
  filters use the same wording.

### Fixed
- The status dot in the top bar ignored the mouse entirely — hovering it
  never showed anything. It now has a proper hover area and reveals the
  health details described above.

## [0.7.2] - 2026-09-02

### Added
- Heka now remembers your window. The size and position you leave it in are
  restored the next time you open the app, including whether it was
  maximized. If the monitor you last used is no longer connected, the window
  reopens centered on a visible screen instead of getting lost off-screen.
- A "Pause scheduler" switch in Settings → Reliability. Pausing used to be
  possible only from the tray icon; the switch mirrors that state, so you
  can pause and resume without hunting for the tray. Both controls stay in
  sync, and a paused scheduler survives daemon restarts.

## [0.7.1] - 2026-09-02

This release makes Heka more dependable day to day. The daemon — the
background part of Heka that runs and schedules your tasks — now stays
reachable no matter how it was started, and it explains itself clearly when
something is wrong.

### Fixed
- Fixed the confusing "heka daemon is not running" error that could appear
  even when the daemon was actually running. If Heka had been started as
  administrator (for example, to get notification sounds working), the CLI
  and GUI could not connect to it. Heka now grants your Windows account
  permission to reach the daemon regardless of how it was started.
- `heka daemon stop` now actually stops the daemon. Previously it was
  silently ignored whenever the daemon was running with a tray icon.
- Fixed a small race when logging into Windows: the automatic startup and
  the watchdog could briefly overlap when bringing the daemon up. That can
  no longer lead to tasks running twice.
- `heka daemon status` now tells you what is actually wrong and what to do
  next, instead of always reporting "not running".

### Added
- You can now choose how often the watchdog checks on the daemon, in
  Settings → Reliability. Pick anywhere from every minute to once an hour —
  the change applies right away, no restart needed.

### Changed
- Command-line error messages are clearer: they now tell you whether the
  daemon needs to be started, was started with administrator rights, or is
  simply busy.

## [0.7.0] - 2026-09-02

### Added
- Schedules now catch up automatically. If your computer was off, asleep, or
  the app was in the background when a recurring task should have fired,
  Heka will run the missed schedule as soon as you return — no more wondering
  why the 9 AM task never happened. You can choose how often to check
  (every 2, 5, 8, or 10 minutes) in Settings → Reliability.
- Heka now notices when your PC wakes from sleep and catches up missed runs
  right away, instead of waiting for the next check. Resuming the scheduler
  after a pause does the same.
- A new System view on the Logs page shows what the scheduler has been up
  to: when the daemon started, when it woke from sleep, and exactly what it
  caught up — which schedule missed how many runs and what was done about it.
  Manual checks and the check at startup always report their result, even
  when nothing was missed.
- If a missed task fails to start (for example the script was temporarily
  unavailable), the catch-up stays pending and is retried on the next check
  instead of being silently dropped.
- "Reconcile now" button on the Schedules page lets you catch up on missed
  runs without waiting for the next tick.
- New `heka schedules reconcile` command for the same on demand from the
  terminal. It now points you to Logs → System to see what was caught up.
- `heka schedules missed` command for debugging. Lists schedule runs the
  daemon recorded as missed or skipped, so you can see exactly which
  schedules didn't fire (and when) after the PC was off, asleep, or when an
  overlap was skipped. Supports `--since` (default 168h/7d), `--status` (default
  `missed,skipped`), `--task`, and `--limit`, with both human-readable and
  `--json` output.
- Each schedule row now shows whether missed runs will fire (Run now) or
  just be logged (Skip), so you know what will happen after downtime.

### Changed
- Missed-run checks now run every 2 minutes by default (was 10), so
  catch-up after a nap or short sleep happens within moments of returning.
- The log retention and notification settings now also include the new
  reconciliation interval, keeping the settings shape consistent.

## [0.6.9] - 2026-09-01

### Added
- README with project overview, features, and use cases, including images for dashboard, tasks, schedules, and settings.
- GitHub badges for downloads, stars, and latest release.

### Fixed
- Watchdog terminal flash issue on Windows.

## [0.6.5] - 2026-09-01

### Added
- Latest run ID, status, start and finish times, skipped count, and missed count to the Schedule model.
- `ListWithLatestRun` method in ScheduleStore to retrieve schedules with their latest run information.
- Search functionality on the SchedulesPage and display of latest run details.
- Tests validating new schedule behavior.

### Changed
- IPC types and conversion functions to handle the new schedule structure.

## [0.6.4] - 2026-09-01

### Changed
- Executable handling refactored for improved OS compatibility.
- OS startup registration now uses `GUIExecutable`.

## [0.6.3] - 2026-09-01

### Changed
- Enhanced daemon startup logging.

## [0.6.1] - 2026-08-28

### Added
- Sound notification settings.

## [0.6.0] - 2026-08-27

### Added
- Light and dark theme variants with specific visual styles.
- Animation utility to manage user-configurable animation preferences.
- Animation toggle in SettingsPage for user preference management.
- Sound notification settings groundwork.

### Changed
- AppLayout enhanced with scanline overlay and gradient background.
- Theme management extended to include light and dark variants.
- LogsPage uses SelectField for pagination and improved filter handling.
- TaskEditorPage uses Tabs component for better tab navigation.
- main.css updated with new theme styles and animation suppression utilities.

### Fixed
- Watchdog installation error handling on Windows for better user feedback.

### Tests
- Theme tests updated to reflect new theme variants and data attributes.
- ResizeObserver and getAnimations mocks added for HeroUI components in test setup.

## [0.5.1] - 2026-08-26

### Added
- Script to automate version bumps across multiple files.
- Tests for watchdog and startup registration functionality.

### Changed
- Windows installer handles upgrades gracefully by closing running instances.
- Watchdog and startup entry reconciliation to ensure correct paths after upgrades.
- Daemon status handling in the frontend reflects changes in real time.

### Fixed
- Improved watchdog interval handling.

## [0.5.0] - 2026-08-26

### Added
- Data directory and retention settings to the settings page.
- System tray support, OS startup registration, and installer.
- Schedules and runs management in the app.
- Comprehensive guidelines for AI agent development and styling rules.
- Backend, daemon, and frontend task module.

### Changed
- Routing refactored to replace the placeholder page with the dashboard page.
- API calls extended with `getSettings` and `updateSettings` for managing settings.
- Backend enhanced to support new settings and statistics retrieval for the dashboard.

### Fixed
- Watchdog interval handling.

### Tests
- Updated to reflect changes in routing and settings management.

## [0.1.0] - 2026-08-25

### Added
- Basic Wails skeleton and daemon foundation.
