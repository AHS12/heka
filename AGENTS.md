# AGENTS.md — AI Agent Guidelines for Heka

> **Heka** is a local task runner and scheduler for programmers. A single Go binary with
> three modes: **daemon** (authoritative runtime), **GUI** (Wails desktop app), and **CLI**
> (Cobra commands). The daemon owns all state — GUI and CLI are IPC clients.

---

## Architecture

```
heka (single binary, go:embed frontend/dist)
│
├── daemon mode      ── orchestrator (DB, executor, scheduler, notifier)
│   ├── internal/daemon       lifecycle, heartbeat, tasks-dir sync
│   ├── internal/core/executor   process spawn, retry, capture, cancellation
│   ├── internal/core/scheduler  cron + one-time + missed-run reconciliation
│   ├── internal/core/task       YAML model, validation, file store
│   ├── internal/db              SQLite (WAL, encrypted vault, embedded migrations)
│   ├── internal/ipc             HTTP/1.1 over named pipe (Win) / unix socket (POSIX)
│   └── internal/notify          desktop toasts (beeep) + webhooks
│
├── GUI mode         ── Wails v2 window → React 19 + HeroUI v3 + Tailwind v4
│   └── internal/app            Wails bindings → IPC client → daemon
│
└── CLI mode         ── Cobra tree → pure IPC client
    └── internal/cli            list, run, status, logs, enable/disable, daemon mgmt
```

**Key invariant:** The daemon is the single source of truth. The GUI never touches SQLite
directly. All mutations go through IPC (HTTP/1.1 over named pipe/socket).

---

## Tech Stack

| Layer | Technology | Version |
|-------|-----------|---------|
| Language | Go | 1.25 |
| GUI framework | Wails | v2.15 |
| Frontend | React | 19.2 |
| Component library | HeroUI | v3.2 (`@heroui/react` + `@heroui/styles`) |
| Styling | Tailwind CSS | v4 (Vite plugin, NOT PostCSS) |
| Routing | react-router-dom | v6.30 (HashRouter for Wails) |
| Server state | TanStack React Query | v5.66 |
| UI state | Zustand | v5.0 |
| Code editor | CodeMirror 6 | @uiw/react-codemirror + @codemirror/lang-yaml |
| Database | SQLite | modernc.org/sqlite (pure Go, no CGO) |
| Scheduler | robfig/cron | v3 |
| CLI | Cobra | v1.10 |
| Testing (Go) | go test + go test -race | built-in |
| Testing (FE) | Vitest + jsdom + @testing-library/react | v3.2 |
| Build | Makefile + Wails CLI | |
| Notifications | beeep (desktop) + outgoing webhooks | |

---

## Development Workflow

```bash
make dev            # Primary: daemon (background) + wails dev (GUI with HMR)
make test           # Both Go tests + Vitest
make frontend-test  # Vitest only
make check          # vet + lint + test (quality gate)
make build          # wails build → build/bin/heka.exe
```

**Critical:** `main.go` embeds `all:frontend/dist`. The frontend MUST be built before any
`go build` or `go test`. The Makefile handles this automatically.

**Version bumps:** run `node scripts/bump-version.js <version>` — it updates all six
version sources (see [docs/version-bump.md](docs/version-bump.md)). Never hand-edit
`main.go` / `wails.json` / `Makefile` / `frontend` version fields for a release.

**Daily flow:**
1. `make dev` — starts daemon + Wails GUI
2. Edit frontend code → Vite HMR updates live
3. Edit Go code → restart needed (`make dev` restarts)
4. `make check` before committing

---

## Project Structure

### Go Backend (`internal/`)

| Package | Purpose |
|---------|---------|
| `app` | Wails bridge — thin passthrough to IPC client |
| `cli` | Cobra command tree (root, tasks, daemon) |
| `config` | Runtime path resolution (DataDir, TasksDir, etc.) |
| `core/executor` | Process execution engine (spawn, retry, capture, cancel) |
| `core/scheduler` | Cron + one-time scheduling with missed-run reconciliation |
| `core/task` | YAML v1 task model, validation, file store |
| `daemon` | Daemon lifecycle, IPC server, heartbeat, tasks-dir sync |
| `db` | SQLite layer (5 stores, AES-256-GCM vault, embedded migrations) |
| `ipc` | HTTP/1.1 over named pipe/socket, client, handlers |
| `notify` | Desktop toasts (beeep with sound) + outgoing webhooks |
| `osapp` | OS-specific watchdog (schtasks/systemd/launchd) |

### Frontend (`frontend/src/`)

| Directory | Purpose |
|-----------|---------|
| `layouts/` | AppLayout — single-column shell with TopNav + scrollable Outlet |
| `components/` | TopNav, DaemonStatusIcon, DaemonDownBanner, controls |
| `components/tasks/` | TaskTable, TaskForm, EnvEditor, SecretValue, WebhookEditor, YamlEditor |
| `pages/` | TasksPage, TaskEditorPage, SettingsPage, Placeholder |
| `lib/` | api.ts, query.ts, tasks.ts, taskForm.ts, theme.ts, accent.ts, secrets.ts |

### Specs (`specs/`)

18 technical design documents (all Approved). Specs drive implementation order:
`Draft → Approved → In Progress → Done`. Always reference the relevant spec when
implementing a feature.

---

## Code Conventions

### Go Conventions

- **Per-OS files:** Platform-specific code goes in `_windows.go` / `_unix.go` build-tagged files
- **Interfaces:** Dependencies behind small consumer-side interfaces for testability
- **Error envelope:** IPC errors use `{"code": "…", "message": "…"}` JSON format
- **Embedded migrations:** SQL migrations in `internal/db/migrations/`, applied in version order
- **Secrets:** Values encrypted at rest (AES-256-GCM), keys exposed, values never on the wire
- **Race testing:** `go test -race` on concurrency-sensitive packages (notify, executor, ipc, daemon)
- **Single implementation:** Shared logic (cron parser, error rendering) lives in one place, used everywhere
- **Clean code:** Remove placeholder/dead code before finishing features

### TypeScript/React Conventions

- **Component co-location:** Tests sit next to components (`Foo.tsx` + `Foo.test.tsx`)
- **Wails bindings:** All daemon calls go through `@wailsjs/go/app/App` wrappers in `lib/api.ts`
- **State management:** React Query for server state, Zustand for UI state (theme, accent)
- **Styling:** Tailwind v4 utilities + HeroUI components. Custom controls in `components/controls.tsx`
- **Form pattern:** Canonical `TaskDraft` model with `emptyDraft()`, `draftFromTask()`, `draftToTask()`
- **Dark mode:** `@custom-variant dark (&:where(.dark, .dark *))` — NOT OS preference
- **Theme tokens:** CSS custom properties with `[data-theme="…"]` selectors for extensibility
- **HeroUI v3:** Compound component API — `Select`, `Select.Trigger`, `Select.Value`, etc.

### CSS/Styling Conventions

- **Control styling:** All form controls use the `inputCls` pattern from `controls.tsx`
- **Select dropdowns:** Use HeroUI v3 `SelectField` from `controls.tsx` — NEVER native `<select>`
- **Scrollbar:** Custom accent-colored scrollbar via `::-webkit-scrollbar` (NOT `scrollbar-width`)
- **Layout:** `flex h-screen flex-col` on shell, `min-h-0 flex-1 overflow-y-auto` on scroll area
- **Body:** `overflow-hidden bg-background text-foreground` — prevents body scroll
- **HeroUI bridge:** Override `--foreground`, `--background`, `--field-foreground`, `--field-background` in theme blocks

---

## Testing Patterns

### Go Tests

```bash
go test ./...                    # Run all
go test -race ./internal/...    # Race detection on concurrent packages
```

- Tests use real SQLite (in-memory or temp file)
- IPC tests use real named pipe/socket
- Executor tests use real process spawning
- Mock at interface boundaries, not internal packages

### Frontend Tests

```bash
cd frontend && npm test          # Vitest
```

- Environment: jsdom
- Wails bindings mocked in `src/test/setup.ts`
- All `@wailsjs/go/app/App` functions mocked with `vi.mock`
- Tests use `@testing-library/react` + `screen` queries
- HeroUI Select hidden native `<select>` in jsdom — test via hidden select element

### Test File Locations

- Go: `*_test.go` next to source files
- Frontend: `*.test.tsx` / `*.test.ts` next to source files
- No separate `test/` directories (except `src/test/setup.ts` for global setup)

---

## Common Pitfalls

### 1. HeroUI Select in Tests
HeroUI v3 Select renders a hidden `<select>` for form submission. In jsdom, `fireEvent.change`
on labels doesn't trigger the React Aria state. Target the hidden `<select>` element instead.

### 2. Tailwind Dark Mode
If `@custom-variant dark` is removed or misconfigured, Tailwind falls back to
`prefers-color-scheme: dark` (OS preference). Light mode text becomes invisible when OS is
in dark mode. Always keep: `@custom-variant dark (&:where(.dark, .dark *))`.

### 3. Scrollbar Styling (Chromium 121+)
Setting `scrollbar-width` / `scrollbar-color` disables `::-webkit-scrollbar` in Chromium.
To keep custom accent scrollbars, use ONLY `::-webkit-scrollbar` pseudo-elements (no
standard properties) and provide Firefox fallback via `@supports not selector(::-webkit-scrollbar)`.

### 4. Frontend Embed
`main.go` embeds `all:frontend/dist`. If `frontend/dist` doesn't exist, `go build` and
`go test` (for the root package) will fail. Always run `make dev` or build frontend first.

### 5. IPC Client
The IPC client opens a fresh pipe/socket for EVERY call. Never cache connections. The daemon
is the single source of truth — all mutations go through IPC.

### 6. Secret Values
The API never returns secret values — only keys. The frontend stores secrets as `${KEY}`
references in task YAML. Never add a literal/free-text input for secret values.

### 7. YAML Round-Trip
Task YAML must survive parse → export → parse cycles byte-for-byte. The `draftToYAML()`
function uses a deterministic emitter. Never use `json.Marshal` or non-deterministic YAML
serialization.

### 8. HeroUI Theme Tokens
HeroUI v3 sets its own CSS variables (`--foreground`, `--background`, etc.). These MUST be
bridged to the app's palette in the theme selector blocks. Without this bridge, HeroUI
components render with incorrect colors in light/dark mode.

### 9. Windows Pipe ACL Across Elevation
Objects created by an elevated process are owned by `BUILTIN\Administrators` (the
elevated token's default owner). A static owner-only pipe SDDL
(`D:P(A;;GA;;;OW)`) therefore locks the same user's non-elevated CLI/GUI/watchdog
out of an elevated daemon with `ERROR_ACCESS_DENIED` — which reads as "daemon is
not running". The pipe SDDL MUST be built at runtime with the user's SID
(`internal/ipc/transport_windows.go`): `D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;<userSID>)`.
Dial failures are classified by errno in `internal/ipc/client.go`
(`ErrDaemonNotRunning` / `ErrDaemonAccessDenied` / `ErrDaemonUnreachable`) — never
revert to string-matching error text.

---

## Spec-Driven Development

All features are defined in `specs/` numbered documents. When implementing:
1. Read the relevant spec first
2. Follow its acceptance criteria exactly
3. Update the spec if implementation reveals design adjustments
4. Mark spec status: `Draft → Approved → In Progress → Done`

**Spec numbers map to areas:**
- 00-03: Core architecture, config, database
- 04-05: Task model, executor
- 06-07: Daemon, IPC
- 08-09: CLI, scheduler
- 10-11: Watchdog, notifications
- 12-13: GUI shell, tasks UI
- 14-16: Schedules/jobs/runs UI, tray, polish

---

## File Editing Rules

1. **Never create files unless absolutely necessary** — prefer editing existing files
2. **Follow existing patterns** — look at neighboring files for conventions
3. **No unnecessary comments** — code should be self-documenting
4. **No over-engineering** — only make changes directly requested
5. **Verify after changes** — run `make check` (or `make test` at minimum)
6. **Read before writing** — always read a file before editing it
7. **Platform sensitivity** — Windows/POSIX differences in Go code

---

## Important Invariants

- **Daemon owns state:** GUI/CLI never touch SQLite directly
- **IPC everywhere:** All mutations go through HTTP/1.1 over named pipe/socket
- **Encrypted vault:** Secrets encrypted at rest with per-install AES-256-GCM key
- **YAML canonical:** Task definitions are YAML files on disk, parsed strictly
- **Single binary:** Frontend embedded in Go binary via `//go:embed`
- **Desktop feel:** App must feel native, not web-like — viewport-constrained, custom scrollbars, no body scroll
