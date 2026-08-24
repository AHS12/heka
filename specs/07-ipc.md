# SPEC-07 — IPC

Status: **Draft** · Depends on: SPEC-02 (endpoint path), SPEC-03 (repos), SPEC-05 (executor), SPEC-06 (endpoint transport) · Master spec: §3, §5, §32

## Goal

Replace the SPEC-06 admin channel with the real thing: HTTP/1.1 over the named pipe / unix socket, a typed Go client shared by CLI (SPEC-08) and GUI bindings (SPEC-12), the full route contract pinned (including routes that stay 501 until their specs), and the JSON error envelope everywhere.

## Scope

**In:**
- `internal/ipc`: transport (moves from `daemon/transport.go`), HTTP server (routes, error envelope, recover, limits), typed client
- Route contract for **all** of master spec §5 (unimplemented ones return 501 `not_implemented`)
- Live handlers: health, shutdown, task list/get, run, enable/disable, cancel, runs list/get (fully backed by db + executor)
- Enumeration of the deferred handlers and who implements them

**Out:** schedules/secrets handlers (SPEC-09/11), task create/update/delete API (SPEC-13, task-file writes), listener-auth beyond current DACL/perms (out of scope entirely), network exposure (never).

## 1. Transport (relocated)

`daemon/transport.go` moves to `internal/ipc` unchanged in behavior:

```go
func Listen(cfg config.Config) (net.Listener, error)  // winio / unix socket, 0600
func Dial(cfg config.Config) (net.Conn, error)        // same path; DialTimeout 2s
```

The endpoint path comes from SPEC-02 (`PipeName` / `SocketDir`). The daemon's singleton check stays exactly the same — it now binds via `ipc.Listen`.

## 2. Server

`net/http` on the listener (winio conns satisfy `net.Conn`, so `http.Serve` just works). Go 1.22+ pattern routing:

```text
GET    /v1/health
GET    /v1/tasks                      GET    /v1/tasks/{slug}
POST   /v1/tasks                      PUT    /v1/tasks/{slug}
DELETE /v1/tasks/{slug}               POST   /v1/tasks/{slug}/run
POST   /v1/tasks/{slug}/enable        POST   /v1/tasks/{slug}/disable
POST   /v1/tasks/{slug}/cancel        GET    /v1/tasks/{slug}/runs
GET    /v1/runs/{run_id}
GET    /v1/schedules                  POST   /v1/schedules
PUT    /v1/schedules/{id}             DELETE /v1/schedules/{id}
POST   /v1/schedules/{id}/enable      POST   /v1/schedules/{id}/disable
GET    /v1/secrets                    PUT    /v1/secrets/{key}
DELETE /v1/secrets/{key}              POST   /v1/daemon/shutdown
```

Middleware in order: recover (panic → 500 envelope), content-type (JSON in/out), body size limit (1 MiB → 413 envelope). No CORS, no auth — the socket's OS permissions ARE the auth (PRD §31; CLI/GUI/agents all run as the same user).

### Error envelope (master spec §5)

```json
{ "error": { "code": "not_found", "message": "task 'foo' not found" } }
```

| Code | HTTP | Where |
|---|---|---|
| `not_implemented` | 501 | schedules/secrets/task-write handlers |
| `not_found` | 404 | unknown slug/run/schedule |
| `method_not_allowed` | 405 | ServeMux default |
| `conflict` | 409 | `executor.ErrAlreadyRunning` (duplicate manual run) |
| `bad_request` | 400 | malformed JSON / invalid args |
| `invalid_task` | 422 | body that fails task validation (for the 501 later: already defined) |
| `internal` | 500 | unexpected (recovered panics, DB errors) |

Success bodies standard JSON, no envelope wrapping (e.g. `POST /v1/tasks/{slug}/run` → `{"group_id":"01J…","status":"running"}` — master §5).

## 3. Live handlers (backed today)

- `GET /v1/health` → `daemon.Health` JSON (SPEC-06 shape).
- `GET /v1/tasks` → index rows (slug, name, enabled, type, runtime, updated_at).
- `GET /v1/tasks/{slug}` → index row + parsed task definition (for the editor later).
- `POST /v1/tasks/{slug}/run` → executor `Start`; `ErrAlreadyRunning` → 409. Body: `{"trigger":"cli"|"manual"}` optional, default `manual`.
- `POST /v1/tasks/{slug}/enable|disable` → flip index `enabled`, immediate SQL commit.
- `POST /v1/tasks/{slug}/cancel` → executor `Cancel(slug)` (new method: returns the active handle or 404 when idle).
- `GET /v1/tasks/{slug}/runs` → recent runs for the slug (`?limit=`, default 25; newest first).
- `GET /v1/runs/{run_id}` → one attempt row (stdout/stderr/exit/duration).
- `POST /v1/daemon/shutdown` → graceful shutdown then close (SPEC-06 sequence).

Executor gains one method: `Cancel(slug) error` — map `slug → active handle` (the daemon already tracks this for overlap; the map lives in the executor).

## 4. Typed client

```go
c := ipc.NewClient(cfg)
c.Health() (daemon.Health, error)
c.ListTasks() ([]TaskSummary, error)
c.GetTask(slug) (Task, error)          c.RunTask(slug, trigger) (RunResponse, error)
c.Enable(slug), c.Disable(slug)        c.Cancel(slug)
c.TaskRuns(slug, limit) ([]Run, error) c.Run(runID) (Run, error)
c.Shutdown() error
```

- `http.Client` with a `DialContext` that returns `ipc.Dial(cfg)` conns — every call opens a fresh pipe/socket connection (cheap for local IPC).
- Responses: 2xx → decode body; envelope → typed `*ipc.Error{Code, Message}` so callers switch on `Code`.
- Connection refused → `ErrDaemonNotRunning` sentinel (CLI/GUI both branch on it — PRD §3.1).

## 5. What stays 501 (and who owns it)

| Route | Owner |
|---|---|
| `POST/PUT/DELETE /v1/tasks…` | SPEC-13 (task-file writes via SPEC-04 store + index sync) |
| `/v1/schedules/*` | SPEC-09 (scheduler) |
| `/v1/secrets/*` | SPEC-11 (secret store) |

The contract is authoritative *now*: any client written against SPEC-07 keeps working as handlers light up.

## 6. Testing

In-process e2e over the real platform transport (temp socket dir):

1. Health round-trip; shutdown round-trip returns and closes.
2. Tasks: seeded index → list/get; unknown slug → 404 envelope; run on unknown slug → 404; duplicate run with fake busy executor → 409 envelope.
3. Enable/disable round-trip; cancel with fake active handle; cancel idle → 404.
4. Runs list/detail with a seeded run row.
5. 405 (wrong method), 404 JSON shape for unknown route, panic → 500 envelope (handler that panics in test), >1 MiB body → 413.
6. Client dial with nothing listening → `ErrDaemonNotRunning`.
7. Envelope table test: every code row in §2 maps from the right error.
8. 501s: schedules GET returns `not_implemented` envelope.
9. `-race` over the whole package (server handles concurrent clients).

## 7. Files

```text
internal/ipc/transport.go   # Listen/Dial (moved from daemon)
internal/ipc/server.go      # mux, middleware, envelope
internal/ipc/handlers.go    # live handlers over a deps struct
internal/ipc/client.go      # typed client + ErrDaemonNotRunning
internal/ipc/ipc_test.go
internal/daemon/transport.go  # deleted (moved)
internal/executor/executor.go # + Cancel(slug)
```

Deps struct is thin wiring (db, executor, task store, health func) — daemon composes it; tests pass fakes. No new dependencies (stdlib `net/http`).

## 8. Acceptance criteria

1. Full contract registered; 501s return the envelope.
2. e2e §6 passes on the real pipe/socket with `-race`.
3. Admin channel (SPEC-06 §4) is gone — `daemon status/stop` now use the client; foreground daemon serves HTTP.
4. `heka daemon stop` still works end-to-end (now over HTTP shutdown).
5. Error envelope JSON for 404/405/409/413/500 — verified by test.
6. `make check` green.

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green.
3. SPEC-06 admin channel deleted, not left as dead code.