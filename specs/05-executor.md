# SPEC-05 — Executor (Process Runner)

Status: **Draft** · Depends on: SPEC-02 (config), SPEC-03 (runs repo), SPEC-04 (task model) · Master spec: §8, §21–25, §29, D11

## Goal

The daemon's execution engine: spawn tasks as direct processes (no shell), capture bounded output, enforce timeout/cancellation with graceful→force escalation, run tasks concurrently, prevent a task overlapping itself, and write every attempt to `runs` with retry support.

## Scope

**In:**
- `internal/core/executor`: runtime table + argv resolution, attempt runner (spawn/capture/timeout/cancel/process tree), retry loop, per-slug overlap lock
- `group_id` + `attempt` columns on `runs` (schema amendment — nothing is deployed yet, so `0001_init.sql` is edited directly rather than adding a 0002)
- Dependency: `github.com/oklog/ulid/v2` (run/group ids)

**Out:** streaming log tail (SPEC-14), schedule-driven execution decisions (SPEC-09), secrets in env resolution (SPEC-11 provides the resolver half), OS-native scheduling (post-v1).

## 1. Execution model

Direct `os/exec`, argv arrays, **never a shell** (PRD §22). Each *run group* = one logical run (one `POST run`). Each *attempt* = one process spawn and one `runs` row. Retries happen inside the group.

```text
Start(slug)
   │  acquire slug lock (ErrAlreadyRunning if busy)
   ▼
attempt 0 ──► process ──► runs row (group_id, attempt=0)
   │ success ───────────► done (status success)
   │ failure + retried ──► wait delay_seconds
   ▼
attempt 1 ──► process ──► runs row (group_id, attempt=1)
   ...
```

- **Concurrency**: one goroutine per group; different tasks run in parallel (PRD §28).
- **Overlap prevention** (D11): a per-slug in-memory lock. A second `Start` while the slug is running returns `ErrAlreadyRunning`; the scheduler (SPEC-09) turns that into skip-and-record. Manual duplicate requests surface as a conflict (IPC 409 in SPEC-07).
- **Scheduler independence**: failing attempts never affect other groups or the scheduler (PRD §29) — errors are captured in the run record.

## 2. API

```go
type EnvResolver func(name string) (string, bool)   // deamon env, then secrets (SPEC-11)

func New(db *db.DB, maxOutputBytes int64, grace time.Duration, env EnvResolver) *Executor

type Options struct {
    Trigger    string // "manual" | "schedule" | "cli" | "system"
    ScheduleID string // optional, schedule row id
}

func (e *Executor) Start(ctx context.Context, task *task.Task, opt Options) (*Handle, error)
// ErrAlreadyRunning when the slug's lock is held.

type Handle struct {
    GroupID string
    Done    <-chan struct{} // closes when the group finishes
    Cancel  func()          // graceful → force; safe to call twice
}
```

Returned immediately after the group's attempt 0 starts. Status is observed from `runs` rows (IPC later reads them). `Cancel` is safe to call repeatedly; it aborts retries too.

## 3. Runtime table & argv

| task.runtime | Windows | Linux/macOS |
|---|---|---|
| `powershell` | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File <script>` | `pwsh -NoProfile -File <script>` |
| `python` | `python <script>` (falls back to `py`) | `python3 <script>` |
| `node` | `node <script>` | `node <script>` |
| `bash` | unsupported → clear error | `bash <script>` |
| `custom` / `binary` | `command` + `args` verbatim | same |

- Working dir: `working_directory` resolved against the task file's dir (SPEC-04 `Resolve`); unset → task file's dir.
- Env: daemon env + task `environment` with `${VAR}` resolved via `EnvResolver` (task values override inherited).
- `timeout: 0` = no timeout.

## 4. Attempt runner

1. Build argv (runtime table) + env + cwd; spawn with an **own process group**.
2. Capture stdout/stderr into bounded buffers (`max_output_bytes` each). The buffer stops at the cap; a `\n…[truncated]\n` marker is appended. No unbounded growth (PRD §27).
3. On finish: write the `runs` row (group_id, attempt, trigger, status, started/finished, duration_ms, exit_code, pid, stdout, stderr).
4. Timeout (PRD §25): after `timeout` seconds → graceful terminate, escalate to force-kill after `grace` (default 5 s), status `timed_out`, capture available output.
5. Cancel (PRD §24): `Handle.Cancel()` → graceful, escalate after grace, status `cancelled`; remaining retries skipped.
6. Process tree: terminate the whole group (POSIX kill(-pgid, SIGTERM)/SIGKILL; Windows `taskkill /pid X /T /F` after a graceful `CTRL_BREAK` attempt). Grandchildren must not survive.

## 5. Retry (PRD §29)

- Semantics: **`retry.max_attempts` = total executions** (`0`/omitted = a single attempt, no retries; `3` = first try + 2 retries). Deliberate call — matches "0 = no retry" in the schema; flag for review if you read PRD §29's "retry N times" as N extra tries.
- Between attempts: wait `retry.delay_seconds` (default 30).
- Only non-timeout, non-cancel failures retry; timeout and cancel stop the group.
- Each attempt is its own `runs` row; the client-facing logical id is `group_id` (attempt rows keep distinct `run_id`s). This is what SPEC-07's `runs/{run_id}` returns per-attempt, and status endpoints may aggregate by `group_id`.

## 6. Runs schema amendment

`0001_init.sql` gains (safe — never applied anywhere yet):

```sql
group_id        TEXT NOT NULL,          -- logical run exposed to clients
attempt         INTEGER NOT NULL DEFAULT 0,
-- + CREATE INDEX idx_runs_group ON runs(group_id);
```

`run_id` remains the per-attempt PK. Both ids are ULIDs generated here.

## 7. Testing

Classic helper-process pattern (`-test.run=TestHelperProcess` with `GO_WANT_HELPER_PROCESS=1`), so tests spawn the test binary itself — cross-platform, zero external deps. Helper modes: sleep-forever, print-N-bytes, exit-N, stdout+stderr.

1. Success: exit 0 → `success`, exit_code 0, output captured.
2. Failure: exit 3 → `failed`, exit_code 3.
3. Timeout: 30 s helper + 1 s timeout → `timed_out` in ~grace; killed tree verified (no survivor).
4. Cancel: start + cancel → `cancelled`; second cancel is a no-op.
5. Overlap: two different slugs run concurrently; same slug twice → `ErrAlreadyRunning`.
6. Retry: always-fail helper, `max_attempts: 3`, `delay_seconds: 0` → `failed`, 3 rows, attempts 0/1/2, same `group_id`.
7. Retry stops on timeout/cancel.
8. Cap: 10 KB output, 4 KB cap → buffer = 4 KB + truncation marker.
9. Env: task env overrides inherited; `${VAR}` resolved via injected resolver.
10. Working dir resolution: relative `working_directory` resolves against task dir.

## 8. Files

```text
internal/core/executor/executor.go   # Executor, Start, overlap lock, retry loop
internal/core/executor/runtime.go    # runtime table + argv, cwd, env building
internal/core/executor/attempt.go    # one-attempt runner: spawn/capture/timeout/cancel/tree-kill
internal/core/executor/executor_test.go  # + helper process
internal/db/migrations/0001_init.sql # + group_id, attempt, index
go.mod                               # + github.com/oklog/ulid/v2
```

Dependency rule: executor imports `task`, `db`, `config`; nothing imports executor except the daemon (SPEC-06) and scheduler (SPEC-09). No circulars.

## 9. Acceptance criteria

1. All tests in §7 pass, including the process-tree kill assertion.
2. A same-slug second `Start` always returns `ErrAlreadyRunning` (concurrent test repeated with `-race`).
3. Retry semantics per §5 (group_id stable, attempts sequential).
4. `0001_init.sql` applies cleanly and contains the amended columns (SPEC-03 idempotency tests still green).
5. `make check` green (with `-race` on `go test ./...`).

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green.
3. Reviewer signs off the runtime table platform variants.