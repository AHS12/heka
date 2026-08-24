# SPEC-08 — CLI

Status: **Draft** · Depends on: SPEC-06 (daemon control), SPEC-07 (ipc client) · Master spec: §3.1, §14, §18–20, PRD §13, §20

## Goal

The agent-friendly command surface: every daemon capability exposed as `heka <command>` with human output by default and stable JSON output (`--json`) for agents (PRD §20). The CLI is a pure client — it never executes tasks or touches SQLite; every command goes through the `ipc.Client` (PRD §13).

## Scope

**In:**
- `internal/cli` as a cobra command tree (`list`, `run`, `status`, `logs`, `enable`, `disable`, `schedules`, `daemon start|stop|status`)
- `--json` global flag; human + JSON rendering; daemon-unavailable UX
- Dispatcher simplification (below); client-interface seam for tests
- Dependency: `github.com/spf13/cobra`

**Out:** task create/edit via CLI (not needed for MVP acceptance), `--wait` on run (future nicety — the run → status → logs flow already covers agents), schedules listing beyond what the 501 lets us (SPEC-09).

## 1. Dispatcher simplification

`daemon start|stop|status` moves from `main.go` into the cobra tree, calling the **same** `daemon.Start/Stop/Status` functions from SPEC-06. `main.go` becomes:

```text
(no args) | heka gui        → GUI
heka daemon                 → foreground daemon (unchanged)
anything else               → cli.Run(os.Args[1:])   # cobra, incl. daemon start|stop|status
```

- `resolveMode` loses `modeDaemonControl`; `heka daemon start` now resolves to CLI mode. `main_test.go`'s mode table updates accordingly (SPEC-01's "daemon start → daemon-control" row becomes "daemon start → cli").
- One help source: cobra. `heka --help` prints the CLI tree through cobra's usage. Dispatcher `usage()` survives only for the GUI/foreground-daemon summary in `heka`'s help text.
- All SPEC-06 daemon-control behaviors (detached spawn, readiness ping, status block, stop timeout) stay green — same code, new home.

## 2. Command surface

| Command | Behavior | Human output (exit 0) |
|---|---|---|
| `heka list` | `GET /v1/tasks` | table: slug, name, type, runtime, enabled |
| `heka run <slug>` | `POST run` | `queued <group_id>` |
| `heka status <slug>` | latest attempt + enabled state | slug, enabled, last run status/time/duration or "no runs yet" |
| `heka logs <slug>` | newest attempt detail | run_id, started/finished, duration, exit code, stdout, stderr |
| `heka enable <slug>` / `heka disable <slug>` | flip | `heka: <slug> enabled` |
| `heka schedules` | `GET /v1/schedules` | "schedules not available yet (arrives in SPEC-09)" — server 501 mapped to friendly text |
| `heka daemon start/stop/status` | SPEC-06 control | SPEC-06 §7 wording |

`--json` is a persistent root flag on every command.

## 3. Output contract (agents are the client, PRD §20)

- **Human mode**: aligned text to stdout; errors to stderr; non-zero exit on any error.
- **JSON mode**: the *stable contract*. Shapes are the IPC payloads + a `success` bool:

```json
// heka run daily-research --json
{ "success": true, "slug": "daily-research", "run_id": "01J…", "status": "running" }
```

- **run_id = IPC `group_id`** (client-facing logical run; retry attempts stay internal). Documented mapping, matches PRD's JSON examples.
- **Errors in JSON mode print to stdout** (agents parse stdout), exit code non-zero:
  - daemon down → `{"error":{"code":"daemon_not_running","message":"heka daemon is not running."}}` + hint `heka daemon start` in human mode
  - 404/409/501 IPC codes pass through 1:1 (`not_found`, `conflict`, `not_implemented`)
- No spurious prose in JSON mode: no banners, no color, no progress bars.

## 4. Client seam

CLI depends on a small consumer-side interface, so tests never spin up a daemon:

```go
type APIClient interface {
    Health() (daemon.Health, error)
    ListTasks() ([]ipc.TaskSummary, error)
    GetTask(slug string) (ipc.Task, error)
    RunTask(slug, trigger string) (ipc.RunResponse, error)
    Enable/Disable(slug) (…)
    Cancel(slug) (…)
    TaskRuns(slug string, limit int) ([]ipc.Run, error)
    Run(runID string) (ipc.Run, error)
    Shutdown() error
}
```

Production: `ipc.NewClient(cfg)`. Tests: stub implementing the subset a command needs.

## 5. Structure

```text
internal/cli/root.go     # cobra root, --json, client wiring
internal/cli/tasks.go    # list, run, status, logs, enable, disable, schedules
internal/cli/daemon.go   # daemon start|stop|status (calls daemon.*; moves from main)
internal/cli/output.go   # human renderers + JSON encoder helpers
internal/cli/cli_test.go # stub-client tests per command
main.go, main_test.go    # dispatcher simplified
go.mod                   # + github.com/spf13/cobra
```

## 6. Testing

1. Per command with the stub client: human output golden, JSON output golden (incl. `run_id` mapping), exit codes, stderr-vs-stdout separation.
2. Error paths: `ErrDaemonNotRunning` (both modes + hint), 404 unknown slug, 409 duplicate run, 501 schedules friendly text.
3. `heka daemon status` with a stubbed daemon control (existing SPEC-06 tests keep covering the real one).
4. One full e2e: helper-process daemon on a temp dir (SPEC-06 pattern) → real client → `heka run` a task defined as a YAML file → `heka status` → `heka logs` → `heka daemon stop`. Asserts the whole vertical slice.
5. `--json` output parses as valid JSON in every command test (json.Valid assertion).
6. `make check` green (`-race`).

## 7. Acceptance criteria

1. Every command works against a live daemon (e2e §6.4).
2. JSON mode: valid JSON, stable field names, `run_id` == group_id.
3. Daemon unavailable → correct message/hint and non-zero exit in both modes.
4. 501 schedules → friendly, not an error dump.
5. Dispatcher simplification done; `main_test.go` passes with the updated table.
6. `make check` green.

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green.
3. SPEC-06 daemon-control code reused, not duplicated.