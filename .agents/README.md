# Heka — AI Agent Guidelines

> **Heka** is a local task runner and scheduler for programmers. A single Go binary with
> three modes: **daemon** (authoritative runtime), **GUI** (Wails desktop app), and **CLI**
> (Cobra commands). The daemon owns all state — GUI and CLI are IPC clients.

## Quick Reference

- **Architecture:** `main.go` → daemon / GUI (Wails) / CLI (Cobra). Daemon owns SQLite.
- **Tech:** Go 1.25, Wails v2.15, React 19, HeroUI v3, Tailwind v4, SQLite (modernc)
- **Build:** `make dev` (daemon + GUI), `make test`, `make check`
- **Frontend:** `frontend/src/` — components, pages, lib, layouts
- **Backend:** `internal/` — app, cli, config, core, daemon, db, ipc, notify, osapp
- **Specs:** `specs/` — 18 design docs driving implementation order

## Key Invariants

1. **Daemon owns state** — GUI/CLI never touch SQLite directly
2. **IPC everywhere** — All mutations go through HTTP/1.1 over named pipe/socket
3. **Encrypted vault** — Secrets encrypted at rest with per-install AES-256-GCM key
4. **YAML canonical** — Task definitions are YAML files on disk, parsed strictly
5. **Single binary** — Frontend embedded in Go binary via `//go:embed`
6. **Desktop feel** — App must feel native, not web-like

## Common Pitfalls

| Pitfall | Solution |
|---------|----------|
| HeroUI Select in tests | Target hidden `<select>` element, not labels |
| Tailwind dark mode broken | Keep `@custom-variant dark (&:where(.dark, .dark *))` |
| Scrollbar black/gray | Use ONLY `::-webkit-scrollbar` (no standard properties) |
| Frontend embed fails | Build frontend first (`make dev` or `npm run build`) |
| IPC client stale | Opens fresh pipe/socket per call — never cache |
| Secret values leaked | API returns keys only, frontend uses `${KEY}` refs |
| YAML round-trip broken | Use `draftToYAML()` deterministic emitter |
| HeroUI colors wrong | Bridge `--foreground`, `--background` in theme blocks |

## File Structure

```
heka/
├── main.go                    # Entry point (mode dispatcher)
├── Makefile                   # Build orchestration
├── AGENTS.md                  # AI agent guidelines
├── internal/                  # Go backend
│   ├── app/                   # Wails bridge
│   ├── cli/                   # Cobra commands
│   ├── config/                # Runtime paths
│   ├── core/executor/         # Process execution
│   ├── core/scheduler/        # Cron + one-time
│   ├── core/task/             # YAML model + store
│   ├── daemon/                # Lifecycle + IPC server
│   ├── db/                    # SQLite + migrations
│   ├── ipc/                   # HTTP/1.1 over pipe/socket
│   ├── notify/                # Desktop toasts + webhooks
│   └── osapp/                 # OS watchdog
├── frontend/                  # React + TypeScript
│   └── src/
│       ├── components/        # UI components
│       ├── components/tasks/  # Task-specific components
│       ├── layouts/           # AppLayout shell
│       ├── pages/             # Route pages
│       ├── lib/               # Shared logic
│       └── test/              # Test setup
├── specs/                     # Technical design docs
├── tasks/                     # Example task YAML files
└── .commandcode/rules/        # Domain-specific rules
```

## Development Workflow

```bash
make dev            # Start daemon + Wails GUI (HMR)
make test           # Run all tests (Go + Vitest)
make frontend-test  # Vitest only
make check          # Quality gate (vet + lint + test)
make build          # Production build
```

## Testing

- **Go:** `go test ./...` + `go test -race ./internal/...`
- **Frontend:** `cd frontend && npm test` (Vitest + jsdom)
- **Pattern:** Tests co-located with source (`Foo.tsx` + `Foo.test.tsx`)
- **Mocks:** Wails bindings mocked in `src/test/setup.ts`

## Code Conventions

- **Go:** Per-OS files, consumer-side interfaces, error envelope, clean code
- **React:** Component co-location, React Query for server state, Zustand for UI state
- **CSS:** Tailwind v4 + HeroUI v3, `inputCls` pattern, custom scrollbar
- **Forms:** `TaskDraft` model, deterministic YAML, validation at save time
