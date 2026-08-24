# Heka — Master Technical Spec

Status: **Draft** · Last updated: 2026-08-24 · Scope: v1 (MVP per PRD §36)

This spec turns the [Product Requirements Specification](../Heka%20—%20Product%20Requirements%20Specification.md) into concrete engineering decisions. Where the PRD leaves a choice open, this document makes a decision and records the rationale.

Product principle (PRD §37): **Heka schedules and executes. Scripts decide what the work actually does.**

---

## 1. Architecture Overview

Three interfaces around one daemon (PRD §2):

```text
┌─────────────────────────────────────────────────────────┐
│                     heka (single codebase)              │
│                                                         │
│  ┌────────────────────  daemon mode  ─────────────────┐ │
│  │ Scheduler │ Executor │ Process Manager            │ │
│  │ SQLite │ Logs │ Notifications │ Recovery │ Health │ │
│  └────────────────────────┬──────────────────────────┘ │
│                           │                            │
│                 IPC (HTTP/1.1 over                      │
│                 named pipe / unix socket)               │
│             ┌─────────────┼──────────────┐             │
│             ▼             ▼              ▼             │
│          GUI mode      CLI mode      (future agents)   │
│          (Wails)      (commands)                       │
└─────────────────────────────────────────────────────────┘
```

- The **daemon is the single authoritative runtime**. Only it owns the scheduler, process execution, and SQLite (PRD §4).
- **GUI and CLI are clients.** They never execute tasks or touch the database directly.
- **Local-first.** Everything runs on localhost-class IPC; no cloud, no account, no network listener by default (PRD §31).

---

## 2. Key Decisions

Each decision below is a deliberate choice. If any of them is wrong for you, this is where to push back.

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | **One Go module at the repo root**, `module heka`. Single `main.go` dispatches modes. | Lets GUI, CLI, and daemon share core packages without `go.work`/`replace` gymnastics. Deviates from PRD §35's `app/`+`cli/` separate-module layout. |
| D2 | **IPC = HTTP/1.1 served over a named pipe (Windows) or unix socket (Linux/macOS)**. | `net/http` is battle-tested, debuggable (`curl` against the socket), versioned easily, and the PRD's future local HTTP API (§32) becomes near-free. Requests/responses are JSON. |
| D3 | **Schedules are runtime state in SQLite, not part of task YAML.** | Runs and scheduler state don't belong in the portable task definition — keeps a task file portable without carrying orchestration baggage. The visual form's Schedule section writes schedule records, which the spec documents (PRD §11.1 lists schedule in the form; the form is not 1:1 YAML for this one field). |
| D4 | **SQLite driver: `modernc.org/sqlite` (pure Go, no CGO).** | Wails cross-compilation on Windows is painless without a C toolchain. |
| D5 | **Cron engine: `robfig/cron/v3`**, five-field spec strings. | Mature, supports a scheduler sandbox independently testable from wall-clock time. |
| D6 | **Tray icon is owned by the daemon** (via `getlantern/systray`), not the Wails GUI. | The tray must exist when the GUI is closed (PRD §16). Wails' tray lives in the window process. |
| D7 | **Desktop notifications from the daemon** via `gen2brain/beeep`. | Notifications must work with the GUI closed (PRD §7.3). Beeep does WinRT toast / dbus / osascript without a window. |
| D8 | **One codebase, two Windows build flavors** (Makefile targets): `heka.exe` (console subsystem: daemon + CLI + `heka gui`) and `heka-gui.exe` (windowsgui subsystem via `-H windowsgui`: double-click launch without a console window). | PRD §5's "single executable, multiple modes" — one source, packaging nuance only. |
| D9 | **Secrets: v1 stores them in SQLite with user-only file permissions, masked in the UI; OS keychain deferred.** | Documented limitation (PRD §14 says "where avoidable" — raw secrets stay out of YAML). Keychain integration is a clean future swap. |
| D10 | **OS-native scheduler registration (Task Scheduler/systemd/launchd) is post-v1.** Interface stubbed, nothing registered in MVP. | MVP acceptance (PRD §36) requires in-daemon reliability, missed-run detection, and recovery — not native registration. PRD §7 lists it as layered reliability "where practical". |
| D11 | **Overlap prevention**: same task never overlaps itself by default (PRD §23); different tasks run concurrently. | Explicit PRD behavior; enforced by a per-slug execution lock in the daemon. |
| D12 | **OS-level watchdog**: a Task Scheduler / systemd timer / launchd entry checks the daemon every N minutes and restarts it if down. | The daemon's liveness *is* the product's trust point; the watchdog closes dead-daemon gaps in minutes. Watchdog ≠ OS task scheduling (still post-v1, D10). |
| D13 | **Webhook notification channels in MVP**: Slack, Discord, Pumble (Slack-compatible), Telegram Bot API, generic HTTP. Per-task inline `notify.webhooks`; tokens/URLs via the secret store. | User requirement (PRD §30 updated): chat integrations are MVP. One notifier with preset payload shapes — small, well-scoped, and it doesn't disturb the custom-script story. Delivery is async + best-effort; failures never touch task state. |

---

## 3. Process Model

One codebase, three modes, separate processes (PRD §5):

```text
heka                        # no subcommand → GUI mode
heka gui                    # GUI mode (Wails window)
heka daemon                 # daemon mode (foreground, logs to stdout)
heka daemon start|stop|status
heka list | run | status | logs | enable | disable | schedules ...   # CLI mode
```

Rules:

- **Daemon is a singleton.** A fixed pipe/socket name is the lock: a second `heka daemon` fails fast with "already running".
- `heka daemon start` spawns a detached daemon process and returns; `stop` asks the daemon to shut down via IPC.
- GUI and CLI connect to the daemon over IPC. If unreachable (PRD §3.1): report `Heka daemon is not running.` GUI shows a "Start Daemon" action; CLI offers `heka daemon start`.
- `wails dev` (dev only) runs the GUI in-process; the daemon is started separately by `make dev` (see §12).

---

## 4. Repository Layout

```text
heka/
├── main.go                   # mode dispatcher (guì / daemon / CLI)
├── internal/
│   ├── app/                  # Wails app bootstrap: bindings + window
│   ├── cli/                  # CLI commands (cobra), human + JSON output
│   ├── config/               # paths, defaults, settings (PRD §2 manual but simple)
│   ├── core/
│   │   ├── task/             # Task model, YAML schema v1, validation, file store
│   │   ├── executor/         # process runner: spawn, capture, timeout, cancel
│   │   └── scheduler/        # cron engine, overlap lock, missed-run reconciler
│   ├── daemon/               # lifecycle, singleton, health, wiring
│   ├── db/                   # SQLite open, embedded migrations, repositories
│   ├── ipc/                  # server (HTTP over pipe/socket) + typed client
│   ├── notify/               # beeep wrapper, per-task policy
│   ├── osapp/                # startup registration, tray, OS-scheduler stub
│   └── secret/               # secret store: masked values, ${VAR} resolution
├── frontend/                 # React + TS + Vite (Wails-managed)
│   └── src/
│       ├── components/  pages/  hooks/  lib/  types/
│       ├── App.tsx  main.tsx
├── tasks/                    # canonical YAML task files (default tasks dir)
├── specs/                    # this folder
├── wails.json
├── Makefile
└── go.mod
```

Note on tasks: YAML files in a configured **tasks directory** are the canonical task definitions (PRD §10, §11.4). The daemon watches/reads this directory; the DB stores an index of parsed fields plus runtime state. This makes tasks portable, diffable, and copy-pasteable.

### Paths (D1 location policy)

| | Windows | Linux/macOS |
|---|---|---|
| Data dir (SQLite, logs) | `%LOCALAPPDATA%\heka` | `~/.local/share/heka` |
| Tasks dir (default) | `%LOCALAPPDATA%\heka\tasks` | `~/.local/share/heka/tasks` |
| IPC endpoint | `\\.\pipe\heka\<user>` | `$XDG_RUNTIME_DIR/heka.sock` or data dir |

All paths overridable via `--config` and env vars. Manageable via `Heka Core` config in the GUI (Settings).

---

## 5. IPC Contract

Transport: HTTP/1.1 over named pipe/unix socket (D2). Base path is versioned: `/v1`. All bodies JSON.

### Error envelope

```json
{ "error": { "code": "not_found", "message": "task 'foo' not found" } }
```

HTTP status mirrors the error class (404, 400, 409, 500).

### Endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/health` | daemon + scheduler health, version, next run |
| GET | `/v1/tasks` | list tasks (indexed from YAML files) |
| POST | `/v1/tasks` | create task (body = YAML task or object) |
| GET | `/v1/tasks/{slug}` | get task definition (as YAML + parsed form) |
| PUT | `/v1/tasks/{slug}` | update task |
| DELETE | `/v1/tasks/{slug}` | delete task |
| POST | `/v1/tasks/{slug}/run` | run now → `{run_id, status}` |
| POST | `/v1/tasks/{slug}/enable` / `disable` | toggle |
| POST | `/v1/tasks/{slug}/cancel` | cancel active run |
| GET | `/v1/tasks/{slug}/runs` | run history |
| GET | `/v1/runs/{run_id}` | run detail (stdout/stderr, exit code) |
| GET | `/v1/schedules` | list schedules + one-time jobs |
| POST | `/v1/schedules` | create schedule/job |
| PUT | `/v1/schedules/{id}` | update |
| DELETE | `/v1/schedules/{id}` | delete |
| POST | `/v1/schedules/{id}/enable` / `disable` | toggle |
| GET | `/v1/secrets` | list secret keys (values masked) |
| PUT | `/v1/secrets/{key}` | set secret |
| DELETE | `/v1/secrets/{key}` | delete secret |
| POST | `/v1/daemon/shutdown` | graceful stop |

### Run semantics

`POST /v1/tasks/{slug}/run` returns immediately:

```json
{ "success": true, "slug": "daily-research", "run_id": "01J…", "status": "queued" }
```

Statuses (PRD §15): `queued`, `running`, `success`, `failed`, `timed_out`, `cancelled`.

### Security of the endpoint (PRD §31)

- Windows: go-winio pipe with a restrictive DACL (current user only).
- Unix: socket `0600` inside user runtime/data dir; client not needed beyond that.
- Never bound to a network interface. No unauthenticated network API (PRD §32).
- GUI/CLI never receive secret values — only masked keys.

---

## 6. SQLite Schema

Driver: `modernc.org/sqlite` (D4). Single DB file at `data/heka.db`. Embedded, versioned migrations (`schema_migrations` table, SQL files via `embed`).

```sql
tasks (
  id              TEXT PRIMARY KEY,          -- uuid
  slug            TEXT NOT NULL UNIQUE,      -- ^[a-z0-9]+(-[a-z0-9]+)*$
  name            TEXT NOT NULL,
  yaml_path       TEXT NOT NULL,             -- canonical file path
  parsed_json     TEXT NOT NULL,             -- indexed/cached parse of the YAML
  enabled         INTEGER NOT NULL DEFAULT 1,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
)

schedules (
  id              TEXT PRIMARY KEY,
  slug            TEXT NOT NULL UNIQUE,
  task_slug       TEXT NOT NULL REFERENCES tasks(slug),
  kind            TEXT NOT NULL,             -- 'recurring' | 'onetime'
  cron            TEXT,                      -- recurring: 5-field cron
  run_at          TEXT,                      -- onetime: ISO timestamp
  timezone        TEXT NOT NULL DEFAULT 'local',
  enabled         INTEGER NOT NULL DEFAULT 1,
  missed_policy   TEXT NOT NULL DEFAULT 'skip',  -- 'skip' | 'run_now'
  next_run_at     TEXT,
  last_run_at     TEXT,
  last_status     TEXT,
  created_at      TEXT NOT NULL
)

runs (
  run_id          TEXT PRIMARY KEY,          -- run_id exposes as 01J… ulid-ish
  task_slug       TEXT NOT NULL,
  schedule_id     TEXT,                      -- NULL for manual
  trigger         TEXT NOT NULL,             -- 'manual' | 'schedule' | 'cli' | 'system'
  status          TEXT NOT NULL,
  started_at      TEXT,
  finished_at     TEXT,
  duration_ms     INTEGER,
  exit_code       INTEGER,
  pid             INTEGER,
  stdout          TEXT,                      -- capped, see retention
  stderr          TEXT,
  created_at      TEXT NOT NULL
)

secrets (
  key             TEXT PRIMARY KEY,
  value           TEXT NOT NULL
)

kv (
  key             TEXT PRIMARY KEY,          -- health heartbeat etc.
  value           TEXT NOT NULL
)
```

Indexes: `runs(task_slug, started_at DESC)`, `runs(status)`, `schedules(enabled, next_run_at)`.

Log retention (PRD §27: "no unbounded growth"): per-run output capped (default 1 MB each); runs pruned after N days (default 90) with a per-task override.

---

## 7. Task YAML Schema — v1

Canonical portable task definition (PRD §10). File name convention: `<slug>.yaml` in the tasks dir.

```yaml
version: 1
name: Daily AI Research
slug: daily-ai-research
type: script                       # script | binary

# type: script
runtime: powershell                # powershell | python | node | bash | custom
script: ./scripts/daily-ai-research.ps1

# type: binary
command: ./bin/backup.exe
args: [--database, main]

working_directory: ./scripts       # relative to task file's dir
environment:
  OPENROUTER_API_KEY: ${OPENROUTER_API_KEY}   # ${VAR} → daemon env or secret store
timeout: 300                       # seconds, 0 = none
retry:
  max_attempts: 3                  # 0/omitted = no retry
  delay_seconds: 30
capture_output: true
notify_on: [failure, timeout]      # success | failure | timeout
```

### Validation rules

| Field | Rule |
|---|---|
| `version` | required, must equal `1` |
| `name` | required, non-empty |
| `slug` | required, `^[a-z0-9]+(-[a-z0-9]+)*$`, unique |
| `type` | required, enum `script`/`binary` |
| `runtime` | script only, enum, defaults `custom` |
| `script`/`command` | required per type; path validated to exist or a clear error otherwise |
| `args` | array of strings |
| `timeout` | non-negative integer |
| `retry.max_attempts` | non-negative integer (v1: only linear delay) |
| `notify_on` | list, subset of `success`/`failure`/`timeout`; empty/omitted = no notifications |
| `notify.webhooks` | optional list; `format` ∈ `slack`/`discord`/`telegram`/`generic`, `url` required (may contain `${VAR}` refs), `chat_id` required for telegram; tokens live in the secret store, never plaintext in YAML |
| unknown fields | rejected with a schema error (PRD §11.3) |

Field resolution for `${VAR}`: daemon process env first, then `secrets` table (D9). Never written back into YAML.

Runtime detection: daemon checks the interpreter exists (`powershell`/`python`/`node`/…); missing → task marked invalid with message like *"Python was not found on this system."* (PRD §12).

---

## 8. Executor (Process Runner)

Go `os/exec`, direct execution — no shell (PRD §22). Per run (PRD §21):

1. Resolve argv (runtime + script + args, or binary + args), env (task env over daemon env), working dir.
2. Spawn; capture stdout/stderr to buffers **and** stream to the run's log.
3. Enforce timeout: graceful terminate (`os.Interrupt`/CTRL_BREAK on Windows) → escalate to Kill after grace (default 5 s).
4. Cancellation (PRD §24): graceful → forced, recorded `cancelled`.
5. Record exit code, duration, status. Failure never affects the scheduler (PRD §29).
6. Overlap guard (D11): per-slug lock; if a run is active, a scheduled tick is **skipped and recorded**; manual runs report "already running", `409`.

Retry (PRD §29): on failure, linear retry with `delay_seconds` up to `max_attempts`; each attempt is its own `runs` row sharing a `run_id` group (add `attempt` column at migration time per SPEC-05).

---

## 9. Scheduler

- `robfig/cron/v3` (D5). One cron instance per daemon.
- Each enabled recurring schedule registers a job that:
  1. Checks overlap lock (skip if running, per D11).
  2. `POST /v1/tasks/{slug}/run` internally (trigger `schedule`).
- One-time jobs: daemon-side timer; on fire → run, mark schedule `done` (excluded from active lists but kept in Jobs view, PRD §16).
- **Missed-run reconciler at daemon start** (PRD §7.1): for each enabled recurring schedule, compute expected occurrences since `last_run_at` (or `created_at`). If missed: apply `missed_policy` — `skip` (record a missed-run note) or `run_now` (execute once immediately).
- Next-run computation: exposed on the schedule and by `/v1/health`.
- Scheduler state persisted in `kv` (last tick, heartbeat) so health can be displayed even after restart.

---

## 10. Notifications & Secrets

- **Notifications** (D7 + D13): two MVP channels — native desktop toasts (beeep) and outgoing webhooks (Slack/Discord/Pumble-Slack-compatible/Telegram/generic, see SPEC-11). Both fire on `notify_on` (success/failure/timeout) at group completion. Async and best-effort: delivery failures are logged, never affect task state or retries. No dependency on GUI or agents (PRD §7.3).
- **Secrets** (D9): `secrets` table; GUI shows masked keys only; `PUT` writes, `DELETE` removes; used only for `${VAR}` resolution at execution time.

---

## 11. Frontend

Stack (PRD §8): React + TypeScript + Vite (Wails-managed), Tailwind CSS, HeroUI.

- **Data flow**: React → typed API client (`frontend/src/lib`) → Wails bindings (`window.go.app.*`) → shared Go IPC client → daemon. Frontend never talks to SQLite or processes (PRD §2).
- **Routing** (PRD §24): Dashboard, Tasks, Schedules, Jobs, Runs, Logs, Settings.
- **Task editor** (PRD §11): tabbed `Form | YAML`. Form maps to parsed task fields; YAML editor edits the canonical definition with live schema validation. No silent field dropping: unknown/advanced YAML fields are preserved and flagged.
- **Design** (PRD §23): dark + light mode, dense developer-tool aesthetic, keyboard-friendly, no gratuitous charts. Dashboard = counters + recent activity.
- **Runtime state**: TanStack Query for server state (polls `/v1/health` and run lists via bindings); zustand for small UI state.

---

## 12. Makefile & Dev Loop

Makefile is the single entry point for tooling. Current Makefile is a copy-paste from another project (references `dockless-deploy`, `internal/web/static`) and is replaced wholesale.

```text
make dev        # start local daemon (background) + wails dev (GUI); Ctrl-C stops both
make dev-core   # daemon only, foreground (go run . daemon)
make build      # build heka (console) + heka-gui (windowsgui on Windows)
make test       # go test ./...
make check      # vet + lint + test (quality gate)
make format     # gofmt + goimports
make clean
```

Windows notes: `wails dev` needs a working Go + Node toolchain; daemon spawns as detached process for `heka daemon start`. The named pipe name includes the username, so `make dev` daemon and `wails dev` GUI always find each other.

---

## 13. Test Strategy

- Go unit tests: YAML parse/validate (golden files), executor (fake long-running scripts, timeout, cancel), scheduler (virtual clock), IPC round-trip over an in-memory listener, secret masking.
- Go e2e smoke: start real daemon on a temp dir, run task via IPC client, assert run row/status.
- Frontend: Vitest for the API client and YAML↔Form mapping helpers; minimal component tests. No heavyweight e2e in v1.

---

## 14. Bite-Size Spec Roadmap

Each line becomes its own numbered spec in `specs/`, written just-in-time (not all up front), implemented strictly in order.

| # | Spec | Contents | Depends on |
|---|------|----------|-----------|
| 01 | Scaffold & tooling | go.mod, main dispatcher, Wails skeleton, frontend shell, Makefile rewrite, dirs, config paths skeleton | — |
| 02 | Config & paths | settings resolution, data/tasks dirs, env overrides | 01 |
| 03 | Database | schema, migrations, repositories | 02 |
| 04 | Task model | YAML v1 schema, validation, file store, import/export helpers | 02, 03 |
| 05 | Executor | process runner, capture, timeout, cancel, retry, overlap lock | 03 |
| 06 | Daemon & health | singleton, lifecycle, `daemon start/stop/status`, health | 03, 05 |
| 07 | IPC | transport, server routes, client lib, error envelope | 06 |
| 08 | CLI | commands, human/JSON output, daemon-unavailable UX | 07 |
| 09 | Scheduler | cron engine, one-time jobs, missed runs, recovery | 05, 07 |
| 10 | Watchdog | schtasks/systemd timer/launchd entry + `heka daemon watch`; restart-if-down with backoff | 08 |
| 11 | Notifications & secrets | beeep desktop toasts + webhook channels (slack/discord/telegram/generic), notify_on, secret store, env resolver hookup | 03, 05, 07, 08 |
| 12 | GUI shell | Wails bindings, React+TS+Tailwind+HeroUI shell, routing, theme | 07 |
| 13 | Tasks UI | list, form editor, YAML editor w/ validation | 12 |
| 14 | Schedules/Jobs/Runs UI | schedule + one-time creation, run history, log viewer | 12 |
| 15 | Tray & OS startup | daemon tray, startup registration (Windows v1), watchdog install UI | 09, 10 |
| 16 | Polish & package | import/export UI, settings, dark/light, installers, retention config | 13–15 |

DoD for any spec: tests pass, `make check` green, acceptance criteria in that spec met, code review by the user.

---

## 15. Open Items / Risks

- **Windows console/GUI subsystem split** (D8) — verify `heka gui` from the console-subsystem binary doesn't steal focus/console weirdness; fallback is shipping `heka-gui.exe` as the primary desktop entry.
- **HeroUI + Tailwind v4** integration in the Wails Vite template — pinned during SPEC-11; possible downgrade to Tailwind v3 if the plugin fights us.
- **beeep on Windows** — WinRT toast without a packaged identity can be flaky in dev; fallback is the OS-scheduler-less Wails runtime notification, or log-only.
- **Watch directory** — v1 polls the tasks dir (2 s) rather than fsnotify; avoids cross-platform watching bugs. fsnotify later if needed.
- **Time zones** — v1: schedules in local time only; cron strings stored raw. A `timezone` column exists for future validation.

---

## 16. Out of Scope for v1 (echoing PRD §34)

No visual workflow editor, no n8n/Windmill features, no LLM framework, no secrets platform, no container orchestration, no network API, no OS-native scheduler registration (D10), no MCP server (PRD §33 — design leaves room; the IPC client is the seam).