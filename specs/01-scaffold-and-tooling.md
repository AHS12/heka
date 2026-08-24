# SPEC-01 — Project Scaffold & Tooling

Status: **Draft** · Depends on: none · Master spec: §3, §4, §12

## Goal

Stand up the Heka skeleton: a single Go module with a mode-dispatching `main.go` (daemon/gui/CLI, per master spec D1), a Wails GUI that opens an empty shell, a stub daemon mode that starts and stops cleanly, a stub CLI mode, and a fully rewritten Makefile that drives everything. Everything is proven by `make check` and `make dev`.

## Toolchain (confirmed on this machine)

- Go 1.26.1 (windows/amd64); module `go` directive 1.25 (per Wails v2.15)
- Node 24.11.0, npm 11.6.1
- Wails CLI 2.15.0 (upgraded from 2.11.0; `go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

## Scope

**In:**
- `git init` + `.gitignore`
- `go.mod` (module `heka`), Wails v2 dependency
- `main.go` mode dispatcher
- `internal/app/` — Wails bootstrap + one stub binding
- Frontend from Wails' react-ts template, stripped to a minimal shell
- `wails.json`
- `internal/cli/` — stub so CLI mode has a home
- `scripts/dev.ps1` — combined daemon + GUI dev loop
- Makefile rewrite (replaces the copy-pasted dockless-deploy file)

**Out (later specs):** config/paths (02), SQLite (03), task model (04), executor (05), real daemon/health (06), IPC (07), CLI commands (08), scheduler (09), secrets/notifications (10), real frontend shell w/ Tailwind+HeroUI (11).

## 1. Mode dispatcher

`main.go` is the only entry point. `resolveMode(args []string)` is a pure function so it's unit-testable. Wails builds the root package, so `wails dev` / `wails build` invoke it with no args → GUI mode.

```text
heka                 → gui
heka gui             → gui            (Wails window)
heka daemon          → daemon         (foreground, logs to stdout, Ctrl-C exits)
heka daemon start|stop|status  → daemon-control (stubs)
heka <anything else> → cli            (stub: prints not-implemented, exit 1)
heka --help          → usage
```

Rules:
- Unknown/ambiguous mode → usage text to stderr, exit code 2.
- The daemon stub prints `heka daemon v0.1.0 starting (foreground)` and blocks until SIGINT/SIGTERM, then prints a clean shutdown line. Real logic arrives in SPEC-06.
- CLI stub prints: `CLI commands arrive in SPEC-08.` and exits 1.

## 2. Layout created

```text
heka/
├── main.go                 # dispatcher + usage
├── internal/
│   ├── app/                # app.go: Wails bootstrap, stub App bindings
│   └── cli/                # cli.go: Stub() called by dispatcher
├── frontend/               # wails react-ts template, minimized
│   ├── src/{main.tsx, App.tsx, App.css, index.css}
│   ├── wailsjs/            # generated bindings (gitignore)
│   ├── package.json
│   └── vite.config.ts
├── scripts/dev.ps1         # combined dev loop
├── tasks/                  # empty dir (task files arrive in SPEC-04)
├── wails.json
├── Makefile
├── go.mod
└── .gitignore
```

The `internal/config`, `internal/daemon`, `internal/db` etc. packages are **not** created until their specs.

## 3. Stub binding

`internal/app` binds one endpoint so the frontend proves the Go↔JS bridge works:

```text
AppInfo() → { name: "heka", version: "0.1.0", daemon: "not-running" }
```

Frontend renders: window title `Heka`, "heka v0.1.0", and a "Daemon: not running" pill. No Tailwind/HeroUI yet (SPEC-12).

## 4. Makefile

Replaces the current file (which references `dockless-deploy` and `internal/web/static`).

```text
make dev        → scripts/dev.ps1   (start daemon bg + wails dev; stop daemon on exit)
make dev-core   → go run . daemon   (foreground daemon, own terminal)
make build      → wails build -o build/heka  (+ on Windows: build/heka-gui.exe w/ -H windowsgui)
make test       → go test ./...
make check      → vet + lint + test (quality gate; lint skips if golangci-lint missing)
make format     → gofmt + goimports (no -local flag)
make clean      → remove build/, frontend/dist, dev daemon log
```

Note: `dev` initially just starts the daemon and GUI together; the "wait for daemon" wire-up lands with IPC in SPEC-07.

## 5. .gitignore

`build/`, `frontend/dist/`, `frontend/wailsjs/`, `node_modules/`, `*.log`, `.heka-dev-daemon.log`, editor dirs.

## 6. Acceptance criteria

1. `make build` succeeds; artifacts land in `build/` (`heka.exe`, and `heka-gui.exe` on Windows).
2. `make test` passes — dispatcher tests cover `gui`, `daemon`, `daemon start|stop|status`, unknown mode, `--help`.
3. `make dev` opens a window titled *Heka* showing name, version, "Daemon: not running"; closing dev stops the spawned daemon (no orphan process).
4. `heka daemon` runs in the foreground, prints startup line, exits cleanly on Ctrl-C.
5. `heka status` prints the CLI-stub message and exits non-zero.
6. `make check` is green.
7. `tasks/` exists and is empty; `specs/README.md` status table shows 01 as Approved after sign-off.

## DoD

1. Spec approved by user.
2. All acceptance criteria verified on this machine.
3. `make check` green; no build warnings from `wails build` beyond defaults.
4. Commit as the initial scaffold commit (with `Co-authored-by` trailer per repo convention, if we commit).