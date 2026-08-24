# SPEC-13 — Tasks UI

Status: **Draft** · Depends on: SPEC-04 (task model/validation/store), SPEC-07 (write 501s), SPEC-12 (shell) · Master spec: §7, §11, §25, PRD §11, §27

## Goal

The task management surface: a filterable list, the two-way **Form | YAML** editor (PRD §11), and the IPC write handlers that have been 501 since SPEC-07 (`POST/PUT/DELETE /v1/tasks`). Import/export of task files is included — PRD §36 requires it and the helpers already exist (SPEC-04).

## Scope

**In:**
- Tasks page (table + filters + enable toggle + Run Now + delete)
- Task editor: `/tasks/new` + `/tasks/:slug`, tabs `Form | YAML`, validation feedback, advanced-field preservation
- IPC handlers: `POST/PUT/DELETE /v1/tasks` (+ `GET /v1/tasks` gains last-run info)
- Import/export (file dialogs, validation on import)
- Passthrough bindings: `CreateTask`, `UpdateTask`, `DeleteTask`, `GetTaskYAML`, `ValidateTaskYAML`

**Out:** schedule creation from the editor (Schedules page owns that, SPEC-14 — the form shows a pointer, not a control), drag-and-drop import, diffing, batch operations (all SPEC-16).

## 1. Data contract additions (SPEC-07 owner table: tasks write = SPEC-13 ✓)

```text
POST   /v1/tasks              body: task YAML text  → 201 {task}          # creates <tasks>/<slug>.yaml, resyncs index
PUT    /v1/tasks/{slug}       body: task YAML text  → 200 {task}          # validates slug body vs path
DELETE /v1/tasks/{slug}       → 200 {ok}                                  # deletes file + index row
GET    /v1/tasks              → summaries extended: last_status, last_run_at (LEFT JOIN latest run)
```

Validation failures → `422 invalid_task` + the error list from SPEC-04's validator (the code exists since SPEC-04; this is its first API consumer).

**Delete with schedules → `409 conflict`** listing the blocking schedule slugs. Run history is never deleted with the task (`runs.task_slug` is plain text; SPEC-03).

Rename = `POST` copy + `DELETE` old when the slug changed (the editor enforces this; no PUT-rename magic).

## 2. Binding passthrough

Same pattern as SPEC-12 §1:

```go
CreateTask(yaml string) (TaskDTO, error)      // POST
UpdateTask(slug, yaml string) (TaskDTO, error) // PUT
DeleteTask(slug string) error                  // DELETE
GetTaskYAML(slug string) (string, error)       // canonical YAML for the editor tab
ValidateTaskYAML(yaml string) ([]string, error) // wire-format error list for the Form↔YAML tab switch
```

Frontend never touches the filesystem — file *paths* never reach JS; only YAML *text* does (PRD §2).

## 3. Tasks page

- Table: name (+ slug), type, runtime, enabled, last run status/time. Enabled = toggle (optimistic via Query cache); Run Now = `useRunNow` (toast the `group_id`); row click → editor.
- Filters: by type, enabled, runtime (client-side in this spec — the list is small; server-side filters only if it grows).
- Auto-refresh: 5 s poll only while any row shows an active run state (same pattern as SPEC-12 health polling, scoped query).

## 4. Task editor — Form | YAML tabs

One canonical in-memory draft: the parsed `TaskDTO`. Tabs are two *views* of the same draft (PRD §11.4 — no hidden source of truth):

- **Form tab**: sections — Basics (name, slug, type, runtime), Script/Binary (script|command), Execution (args, working dir, timeout, capture_output), Retry (max_attempts, delay_seconds), Environment (key/value rows + ${} helper), Notifications (notify_on checkboxes, webhook list editor: format/url/chat_id with secret-ref hint). A callout links to Schedules for recurring setup.
- **YAML tab**: textarea of the draft serialized to canonical YAML. On tab switch back / on save → `ValidateTaskYAML` + parse; errors render inline (PRD §11.2's error style: "`timeout:` must be a positive integer").
- **Preservation rule** (PRD §11.3): a draft that fails validation in the YAML tab saves *nothing* and shows the errors — the YAML text is kept exactly as typed, so nothing is silently dropped or rewritten. (For v1 the schema rejects unknown fields outright, so there are no "unknown-but-preserved" fields to carry; the rule still protects against partial form loss.)
- Slug edit on an existing task = copy+delete flow (§1).

## 5. Import / Export (PRD §27)

- **Import** (`Import Task` on the tasks page): `OpenFileDialog` (wails runtime) → read text (JS) → `POST /v1/tasks` — the daemon validates; errors show the 422 list.
- **Export** (on the editor): canonical YAML text → `SaveFileDialog` → `WriteExport(slug, path)` binding (daemon-side write of a user-chosen path).
- Import/export never carries run history or (by construction) plaintext secrets: webhook URLs/chat_ids reference `${SECRET}` or as-entered values; plaintext env values export as-is (documented, matches schema).

## 6. Testing

Vitest (mocked bindings):
1. `taskForm` mapping: form→YAML and YAML→form round-trip for script + binary fixtures; slug change detection (rename = copy+delete flag).
2. YAML tab with invalid text: errors surface, draft text preserved byte-for-byte, no save signal sent.
3. Tasks page: table renders seeded data; enable toggle optimistic + revert on error; Run Now toast contains `group_id`.
4. Import: dialog → text → create call; create error list shown.

Go (IPC write handlers):
5. e2e: create → file exists on disk + index row; GET returns it; update replaces; delete removes (history rows intact).
6. 422 carries all validation errors (2+ violations).
7. Delete with a schedule → 409 with schedule slugs.
8. `GET /v1/tasks` last-run join correct with and without runs.
9. `make check` green.

## 7. Files

```text
frontend/src/pages/{TasksPage,TaskEditorPage}.tsx
frontend/src/components/tasks/{TaskTable,TaskForm,YamlEditor,EnvEditor,WebhookEditor,RetryEditor}.tsx
frontend/src/lib/tasks.ts      # useTasks/useTask/useCreate/useUpdate/useDelete/useRunNow
frontend/src/lib/taskForm.ts   # draft model, form↔yaml views, rename detection
internal/app/app.go            # Task CRUD + Validate/GetYAML/WriteExport passthrough
internal/ipc/tasks.go          # POST/PUT/DELETE handlers
internal/ipc/handlers.go       # GET /v1/tasks last-run join
internal/ipc/client.go         # + Create/Update/DeleteTask, GetTaskYAML
```

## 8. Acceptance criteria

1. Create via form → listed + YAML on disk; edit via YAML tab round-trips; invalid YAML blocks save with inline errors (nothing dropped).
2. Run Now executes (see status update in the row); enable toggle persists across restart.
3. Delete with schedules → 409 message listing them; delete without → gone, history intact.
4. Import a valid YAML file → task appears; import of a broken file → 422 list.
5. `make check` green (+ Vitest).

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green.
3. SPEC-07 owner table: tasks write = SPEC-13 ✓ (no 501s left on tasks).