# SPEC-14 — Schedules, Jobs, Runs & Logs UI

Status: **Draft** · Depends on: SPEC-09 (schedules CRUD), SPEC-12 (shell), SPEC-13 (patterns) · Master spec: §9, §12–13, §16–17, PRD §12, §16, §21, §28

## Goal

The remaining management surfaces: recurring schedules + one-time jobs (creation + monitoring), execution history with filtering and full-text search, and the run-detail view (output, exit code, duration, retry attempts). This spec also extends the IPC with a queryable global runs listing.

## Scope

**In:**
- Schedules page: list + create/edit/delete/enable, friendly recurrence builder → cron
- Jobs page: one-time jobs (upcoming + done)
- Runs page: filterable history (task/status/date) → run detail
- Logs page: same history, output-first: free-text search over captured stdout/stderr
- IPC additions: `GET /v1/runs` (filtered + `?q=`), `GET /v1/schedules?kind=onetime`
- Passthrough bindings for the above

**Out:** schedule cloning, calendar visualizations, log streaming/tailing (live follow), export of run output (SPEC-16 or later).

## 1. IPC additions

```text
GET /v1/runs?task={slug}&status={}&from={rfc3339}&to={rfc3339}&q={}&limit={}&offset={}
    → {runs: [...], total: n}            # newest first; q = substring over stdout|stderr (daemon-side LIKE, escaped)
GET /v1/schedules?kind=onetime|recurring  # filter param; default all
```

- `q` is the only new query machinery (LIKE with `%`/`_` escaped — user input never reaches SQL raw; PRD §31 injection care).
- Existing `GET /v1/tasks/{slug}/runs` (CLI) stays untouched.
- Runs list items: group_id, task_slug, trigger, status, started/finished, duration, exit_code; attempt rows under the `group_id` via detail.

## 2. Schedules page

- Table: slug, task, kind, expression, enabled, next_run, last_run, last_status (+ skipped/missed counts surfaced from the runs rows introduced in SPEC-09).
- **Recurrence builder** → cron (SPEC-09 syntax incl. `@every`):

| UI control | cron |
|---|---|
| Every N minutes / hours | `@every 5m` / `@every 2h` |
| Daily at HH:MM | `0 HH * * *` |
| Weekly: day + time | `0 HH * * DOW` |
| Monthly: day + time | `0 HH D * *` |
| Custom | raw passthrough (validated by the daemon, SPEC-09) |

- One-time job creation on the same page (date+time picker → `POST /v1/schedules {kind: onetime, run_at}`).
- Enable toggle + delete (409-worthy? deleting a schedule is never blocked — it doesn't hold history; runs keep `schedule_id`).
- next_run updates come from SPEC-09's stored/computed fields; the page polls schedules every 15 s (cheap) while visible.

## 3. Jobs page

One-time jobs, two sections: **Upcoming** (pending, shows run_at, cancelable) and **Done** (fire → marked done by the scheduler; shows result + link to the run detail). Backed by `GET /v1/schedules?kind=onetime`.

## 4. Runs page

- Filters: task (dropdown from tasks), status, date range. All server-side via the §1 query — not client-side, so the URL/shareable-filter story stays honest.
- Table → **run detail**: run metadata (group_id, task, trigger, schedule, started/finished, duration, exit_code) + **STDOUT / STDERR** blocks (monospace, scrollable, copy button) + **Attempts** for retried groups (each attempt row → its own output; final status callout).
- Live refresh: 3 s poll while any visible row is `queued`/`running`; stops when idle.
- URL state: filters live in the query string (shareable, back-button friendly).

## 5. Logs page

Same filtered history as Runs, but output-first:

- Free-text search box → `?q=` (the daemon-side search); result rows show a snippet with the match highlighted.
- Click row → the same run-detail view (§4).
- The two pages share table + detail components; Logs just defaults to search mode (PRD §21: "searchable output, task filtering, success/failure, date filtering").

## 6. Testing

Vitest:
1. Recurrence builder → cron mapping table (§2, incl. custom passthrough + validation errors surfaced from the daemon).
2. Runs filters build the right query params (task/status/range/q/limit).
3. OutputViewer: stdout/stderr rendering, copy button, retry group attempts list.
4. Schedules page: toggle optimistic, delete, one-time cancel.

Go (IPC):
5. `GET /v1/runs`: each filter alone + combined; `q` matches substring, `%`/`_` in `q` are literals (no wildcard injection); `limit`/`offset` + `total` correct; ordering newest-first.
6. `?kind=onetime` returns only one-time jobs.
7. Runs detail returns all attempts of a group under `group_id`.
8. `make check` green.

## 7. Files

```text
frontend/src/pages/{SchedulesPage,JobsPage,RunsPage,LogsPage,RunDetailPage}.tsx
frontend/src/components/schedules/{ScheduleTable,RecurrenceBuilder,ScheduleForm}.tsx
frontend/src/components/runs/{RunsTable,RunFilters,OutputViewer,AttemptList}.tsx
frontend/src/lib/{schedules,runs}.ts   # hooks + filter state → query params
internal/ipc/runs.go                   # GET /v1/runs filtered + search
internal/ipc/handlers.go               # schedules kind filter
internal/app/app.go                    # passthroughs for the new surfaces
internal/ipc/client.go                 # + ListRuns(filters) / ListSchedules(kind)
```

## 8. Acceptance criteria

1. Create a recurring schedule via the builder → next_run shown → fires on schedule (daemon running) → last_run/last_status update; custom cron passthrough works.
2. Create a one-time job → appears in Jobs → fires at run_at → moves to Done.
3. Runs: task/status/date filters + search hit highlighting; a retried run's detail shows all attempts with per-attempt output.
4. Logs free-text search escapes wildcards (tested).
5. Live table refresh stops when nothing is running.
6. `make check` green (+ Vitest).

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green.
3. SPEC-07 owner table: schedules already live (09); no new 501s introduced.