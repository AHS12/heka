# SPEC-06 — Daemon & Health

Status: **Draft** · Depends on: SPEC-02 (config), SPEC-03 (DB), SPEC-05 (executor) · Master spec: §3, §4, §18–19, §2.2

## Goal

Turn the daemon stub into the real thing: a singleton background process that owns the DB, the tasks index, and the executor; reports health; runs headless forever; and can be started, stopped, and interrogated from the CLI. IPC gets its full HTTP protocol in SPEC-07 — this spec introduces a **transitional newline-JSON admin channel** on the same endpoint so `daemon stop/status` work today and SPEC-07 replaces the protocol without touching the endpoint.

## Scope

**In:**
- `internal/daemon`: `Run` (lifecycle/wiring/health/heartbeat), `Start`/`Stop`/`Status` (CLI control), endpoint transport (Windows named pipe / unix socket), tasks-dir poll → index sync
- `main.go`: real `runDaemon` + `daemon start|stop|status`
- Dependency: `github.com/Microsoft/go-winio` (named pipes)

**Out:** full IPC API (SPEC-07), scheduler (SPEC-09), secrets in env (SPEC-11), tray + OS startup (SPEC-15), production log rotation (SPEC-16). The admin channel dies in SPEC-07.

## 1. Process model

- `heka daemon` runs in the foreground (console attached, logs to stdout).
- `heka daemon start` spawns a **detached** instance of the same exe in `daemon` mode and returns after readiness:
  - Windows: `SysProcAttr{CreationFlags: CREATE_NO_WINDOW}`; stdout/stderr → `<data>/daemon.log`.
  - POSIX: `Setsid: true`; same log redirect.
- **Singleton**: the daemon binds its IPC endpoint first thing. A second instance loses the bind → exits with `heka: daemon is already running` (exit 1). The endpoint is the lock (master spec §5 / §3.1); no separate lockfile.
- Shutdown paths: the admin `shutdown` op, SIGINT/SIGTERM (foreground), and a failed-health self-exit if the DB breaks. In-flight run groups are cancelled (recorded `cancelled`) before the DB closes (master spec §24).

## 2. Lifecycle (`Run`)

```text
1. config.Load → data/tasks dirs
2. db.Open(dataDir)                        # SPEC-03
3. admin endpoint listen (bind = singleton check)
4. executor.New(db, cfg.MaxOutputBytes, grace, envResolver)
   envResolver = daemon env first; secrets seam reserved (SPEC-11)
5. heartbeat loop (kv: last heartbeat, every 5s)
6. tasks poll loop: task.Scan(tasksDir) every 2s → sync index (below)
7. serve admin channel until shutdown/exit
8. graceful: cancel executor ctx → wait groups → close DB → close endpoint → exit 0
```

Failure in 1–4 is a startup error (message on stderr, non-zero exit).

## 3. Health & heartbeat

```go
type Health struct {
    Version        string
    Uptime         time.Duration
    Core           string   // "healthy" (or error text if DB ping fails)
    Scheduler      string   // "running" | "starting" — real value from SPEC-09
    LastHeartbeat  time.Time
}
```

- `kv` keys: `heartbeat` (RFC3339 UTC), `daemon_pid`, `daemon_version`.
- Scheduler field is the SPEC-09 seam — always reports `starting` until the scheduler wires in (documented, so the UI never has to guess later).

## 4. Transitional admin channel

Newline-delimited JSON on the endpoint the full IPC will use:

```text
→ {"op":"ping"}
← {"ok":true,"health":{...}}

→ {"op":"shutdown"}
← {"ok":true}        (daemon then shuts down)
```

- Bind: Windows `\\.\pipe\heka-<user>` via go-winio (user-scoped DACL — security from day one, PRD §31); POSIX `<SocketDir>/heka.sock` (0600). Socket dir from SPEC-02.
- Dial: same path; connect refused/timeout → "daemon is not running".
- **SPEC-07 replaces this protocol with HTTP/1.1 on the identical endpoint** (`GET /v1/health`, `POST /v1/daemon/shutdown`); the transport moves to `internal/ipc`. Explicitly throwaway code — keep it tiny.

## 5. Tasks index sync

`task.Scan` → `db.Tasks()` upsert:

- New/changed YAML → upsert index row (slug, name, `yaml_path`, parsed fields, `updated_at`).
- `enabled` is daemon-side state: **never clobbered** by re-scans (SPEC-04 §5).
- Deleted YAML → index row removed; `runs` history keeps `task_slug` as-is.
- Broken YAML → the task is skipped from the index but recorded (per-file error kept for the UI later); daemon stays up.

## 6. CLI control

```text
heka daemon start   → spawn detached, poll ping (≤5s), print
                      "heka daemon started (pid N)" or "failed to start: …" (exit 1)
heka daemon status  → running: prints §7 block, exit 0
                      not running: "heka daemon is not running." (PRD §3.1), exit 1
heka daemon stop    → ping, send shutdown, wait for process exit (≤10s), exit 0;
                      daemon not running → clear message, exit 1
```

## 7. Status output

```text
Heka daemon: running
version: 0.1.0
uptime: 42s
core: healthy
heartbeat: 2s ago
```

## 8. Testing

In-process lifecycle tests (temp data dir, injected env/config):

1. `Run` on a temp dir → endpoint binds; `Status` → running + fresh heartbeat.
2. Second `Run` on the same dir → `already running` error.
3. Admin shutdown → `Run` returns nil; in-flight helper-process group recorded `cancelled`.
4. Heartbeat: after a short sleep, `kv` heartbeat is fresh; `Status` after shutdown → not running.
5. Index sync: create/update/delete YAML files → index follows; `enabled` survives re-scans; one broken YAML skips only that task.
6. CLI-style e2e using the helper-process pattern: exec `os.Executable() daemon` with env overrides, then `start`/`status`/`stop` from the test process.

## 9. Files

```text
internal/daemon/daemon.go      # Run: wiring, heartbeat, shutdown sequence
internal/daemon/control.go     # Start/Stop/Status + admin client
internal/daemon/transport.go   # endpoint listen/dial (moves to internal/ipc in SPEC-07)
internal/daemon/sync.go        # tasks poll + index sync
internal/daemon/daemon_test.go
main.go                        # real daemon modes
go.mod                         # + go-winio
```

## 10. Acceptance criteria

1. Foreground daemon starts, logs to console, binds endpoint, pings healthy, exits cleanly on shutdown op and Ctrl-C.
2. Second instance exits with `already running`.
3. `daemon start` returns after readiness; `status` shows running + recent heartbeat; `stop` cleanly terminates; subsequent `status` says not running.
4. Detached daemon writes `<data>/daemon.log` (no console window on Windows).
5. Index sync per §5; broken YAML doesn't take the daemon down.
6. `make check` green (with `-race`).

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green.
3. The admin protocol is documented as transitional so SPEC-07 replaces it without regrets.