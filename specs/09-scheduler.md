# SPEC-09 — Scheduler

Status: **Draft** · Depends on: SPEC-03 (schedules repo), SPEC-05 (executor), SPEC-07 (schedules IPC routes), SPEC-08 · Master spec: §9, §12–13, §16–18, D5, D11

## Goal

The daemon's scheduling engine: recurring cron entries and one-time jobs that survive daemon restarts, respect the no-overlap rule, reconcile missed executions, and expose schedule CRUD over the IPC routes that have been 501 since SPEC-07.

## Scope

**In:**
- `internal/core/scheduler`: registry + `Sync`, tick dispatch (recurring + one-time), missed-run reconciliation at startup
- Schedules IPC handlers (create/list/update/delete/enable/disable) — replacing the 501s
- Health extension (scheduler state + next run)
- Dependency: `github.com/robfig/cron/v3` (D5)

**Out:** OS-native scheduler registration (post-v1, D10), complex overlap/queue policies, workflows, `--wait` semantic, notifications on schedule events (SPEC-11).

## 1. Model & semantics (repeating what's pinned)

- Schedules live **only** in SQLite (D3): `kind` recurring (`cron` field) or onetime (`run_at`). Slugs unique (PRD §13).
- A recurring schedule fires its **task**, not a separate command (schedule → task = one step).
- **Overlap rule** (D11): if the task's slug lock is held, the tick is **skipped and recorded** — a `runs` row with `status='skipped'`, `trigger='schedule'`, no pid. `runs.status` is a free TEXT column (SPEC-03), so no schema change.
- **missed_policy** governs *downtime* misses (daemon was down during a fire time) — distinct from overlap skips. Per schedule: `skip` (record `status='missed'` row) or `run_now` (one immediate execution). Synchronous with the existing schema, same free-text status.
- Time zone: local only (master §15 — the `timezone` column stays default `'local'` as a documented warm-seat for the future).

## 2. Engine

`robfig/cron/v3`, one instance, 5-field specs + predefined (`@every 2h`, `@daily`, `@weekly`, `@monthly`, `@hourly`) — which covers every "every N minutes/hours/daily/weekly/monthly" option in PRD §12 with zero custom grammar.

```go
type Runner interface {   // consumer-side seam; production = *executor.Executor
    Start(ctx context.Context, task *task.Task, opt executor.Options) (*executor.Handle, error)
}
```

- **Registry**: in-memory view rebuilt from `schedules` on daemon start and after every CRUD/enable/disable change (`Sync`). Rebuild-in-place, not a restart (cron.Remove + Add).
- **Recurring tick**: `Runner.Start(trigger="schedule", ScheduleID=id)`; `ErrAlreadyRunning` → skipped row (§1).
- **One-time**: `time.AfterFunc` until `run_at`; on fire → start, then mark the schedule done (`enabled=0`) so it never fires again; it stays visible in the Jobs view (PRD §16). One-time jobs whose `run_at` is already in the past at load fire immediately (they were missed while down).
- **next_run_at / last_run_at / last_status** maintained on the row after each fire/finish; `GET /v1/schedules` returns them (health "next run" too).

## 3. Missed-run reconciliation (daemon start, PRD §7.1)

For each enabled recurring schedule:

```text
window = [last_run_at or created_at, now]
expected = occurrences of the cron expr in window, per cron.Next() iteration
fired     = runs rows with trigger='schedule' in window for this schedule
missed    = expected - fired   (a REAL count, not a boolean)
```

- `missed > 0` → apply `missed_policy` once:
  - `run_now` → single immediate run.
  - `skip` → one `runs` row `status='missed'` (count collapsed to one row; the schedule's real history already shows gaps).
- Onetime jobs: covered by §2's "past run_at → fire now".
- Enabled=false schedules are not reconciled.

## 4. Schedule CRUD (IPC)

All previously-501 `/v1/schedules` routes become live, thin over the repo + `Scheduler.Sync`:

| Route | Rules |
|---|---|
| `POST /v1/schedules` | body: `slug, task_slug, kind, cron|run_at, missed_policy`; validates slug format, task exists (404/422), cron parses (400 `bad_request` with the cron error), onetime `run_at` in the future (400); FK guarantees wired in SPEC-03 |
| `GET /v1/schedules` | all schedules + jobs, with `next_run_at`/`last_run_at`/`last_status` |
| `PUT /v1/schedules/{id}` | same validation; re-Sync |
| `DELETE /v1/schedules/{id}` | remove + re-Sync; run history untouched |
| `POST /v1/schedules/{id}/enable\|disable` | flip + re-Sync |

Every mutation ends with `Sync()` so the running engine and the DB can never disagree for more than one call.

## 5. Health extension

`daemon.Health` (SPEC-06 §3) gains fields — still v1-minimal:

```go
Scheduler   string    // "running" (was the reserved seam — now real)
NextRunAt   time.Time
NextTaskSlug string
```

The daemon computes these from the registry on each health read (cheap: next `Entry.Next()` over active schedules).

## 6. Testing

Fake `Runner` records `Start` calls; helper-process task for the one integration test.

1. Recurring `@every 1s` fires: `Start` called with `trigger='schedule'`, row status `running`→`success`, `last_run_at` set, `next_run_at` advanced.
2. Overlap: fake Runner returns `ErrAlreadyRunning` → `skipped` row, no further calls.
3. One-time: fires once, schedule marked disabled afterward; second `Sync` doesn't re-fire.
4. Missed reconcile: cron every minute, `last_run_at` 10 min ago — `run_now` → exactly one `Start`; `skip` → one `missed` row. `enabled=0` → nothing.
5. CRUD e2e over a real `ipc` server (SPEC-07 harness): create with bad cron → 400 envelope; unknown task_slug → 404; create → auto-Sync → tick fires; delete stops ticking.
6. Health: `Scheduler='running'`, `NextRunAt` matches the nearest entry; with zero schedules, `NextRunAt` zero + no task.
7. No-fire-when-disabled across a daemon restart (reload registry).
8. `make check` green (`-race`).

## 7. Files

```text
internal/core/scheduler/scheduler.go      # engine: registry, Sync, tick dispatch, onetime
internal/core/scheduler/reconcile.go      # missed-run window computation
internal/core/scheduler/scheduler_test.go
internal/ipc/schedules.go                 # CRUD handlers (replaces the 501s)
internal/daemon/daemon.go                 # scheduler wiring + health next-run
go.mod                                    # + github.com/robfig/cron/v3
```

Dependency rule: scheduler imports `db`, `task`, `executor` (via `Runner`); nothing imports the scheduler except the daemon and the ipc handlers.

## 8. Acceptance criteria

1. Recurring + one-time behavior per §2/§6, including restart survival (registry reload).
2. Overlap ticks produce `skipped` rows; downtime produces `missed`/`run_now` per policy.
3. CRUD e2e with validation errors mapped to the §4 envelope codes.
4. Health: `Scheduler="running"` + correct next run; zero-schedule case handled.
5. SPEC-07's schedules 501 tests updated to expect 200s.
6. `make check` green.

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green.
3. The SPEC-07 "owner" table (§5) updated: schedules = SPEC-09 ✓.