# SPEC-04 — Task Model (YAML Schema v1 + File Store)

Status: **Draft** · Depends on: SPEC-02 (tasks dir), SPEC-03 (index record shape) · Master spec: §7 (schema), D3, PRD §10–11

## Goal

Deliver the canonical, portable task definition: Go model, YAML v1 parsing with strict validation, and the file store over the tasks directory. After this spec, a task is something the daemon and UI can load, save, validate, and round-trip — **without touching the database** (YAML is the source of truth; the DB index is derived and synced by the daemon in SPEC-06).

## Scope

**In:**
- `internal/core/task`: model, YAML parse/marshal (strict), validation, file store (`Scan`, `LoadFile`, `Save`, `Delete`), import/export helpers
- `tasks/example.yaml`
- Dependency: `gopkg.in/yaml.v3` (first schema-defining dep)

**Out:** `${VAR}` resolution (executor, SPEC-05), runtime existence checks (SPEC-05), DB syncing and tasks-dir polling (daemon, SPEC-06), visual form ↔ YAML mapping (SPEC-13), schedules (never part of the YAML — D3), logged-in watching via fsnotify (master spec §15: v1 polls).

## 1. Schema — canonical example

```yaml
version: 1
name: Daily AI Research
slug: daily-ai-research
type: script
runtime: powershell          # script only: powershell|python|node|bash|custom
script: ./scripts/daily-ai-research.ps1
working_directory: ./scripts   # relative → task file's dir
environment:
  OPENROUTER_API_KEY: ${OPENROUTER_API_KEY}
timeout: 300                  # seconds, 0 = none
retry:
  max_attempts: 3             # 0/omitted = no retry
  delay_seconds: 30
capture_output: true
notify_on: [failure, timeout]  # subset of success|failure|timeout
notify:
  webhooks:
    - format: slack            # slack | discord | telegram | generic
      url: ${SLACK_WEBHOOK_URL}
```

Binary tasks use `command:` + `args:` instead of `runtime:`/`script:` (master spec §7).

## 2. Parsing & strictness

- `yaml.v3` decoder with `KnownFields(true)` — **unknown fields are errors** (PRD §11.3). No silent field dropping, ever.
- Schema version: `version` required and must equal `1`. Anything else → error naming the version.
- Comments are preserved? No — YAML is canonical config, not prose; marshal writes normalized output (fields in schema order, values defaulted where the schema has defaults).

## 3. Validation rules

| Field | Rule |
|---|---|
| `version` | required, == 1 |
| `name` | required, non-empty, trimmed |
| `slug` | required, `^[a-z0-9]+(-[a-z0-9]+)*$`, unique within the tasks dir |
| `type` | required, enum `script`\|`binary` |
| `runtime` | script only; enum `powershell`\|`python`\|`node`\|`bash`\|`custom`; default `custom` |
| `script` / `command` | required per type, exactly one of the two present |
| `args` | array of strings |
| `working_directory` | optional string |
| `environment` | string→string; values either literal or `\${NAME}` (`^[A-Za-z_][A-Za-z0-9_]*$`); malformed reference (`${` without `}`) → error |
| `timeout` | non-negative integer; default `0` (none) |
| `retry.max_attempts` | non-negative integer; default `0` (v1 = linear delay only) |
| `retry.delay_seconds` | non-negative integer; default `30`, ignored when `max_attempts == 0` |
| `capture_output` | bool; default `true` |
| `notify_on` | list, subset of `success\|failure\|timeout`; empty/omitted = no notifications |
| `notify.webhooks` | optional list; `format` ∈ `slack\|discord\|telegram\|generic`, `url` required (may contain `${VAR}` refs — tokens live in the secret store, never plaintext), `chat_id` required for `telegram` |

Interpreter existence (`python` not installed…) is **not** a validation error — it's flagged per-task by the executor at run time and surfaced in the UI (PRD §12). Tasks stay importable even when their runtime is missing on this machine.

Path rule (master spec §26): relative paths resolve against the YAML file's directory. `task.Resolve(base string)` provides it; the executor calls it in SPEC-05.

## 4. File store

- A task's file is `<tasks-dir>/<slug>.yaml`. The slug in the file must match the filename slug, or the file is rejected on load.
- `Scan(dir)` → `([]Task, []LoadError)`: files sorted by name; per-file parse errors returned individually, never aborting the whole scan (one bad YAML must not hide the rest).
- `LoadFile(path)`, `Save(dir, task)` (writes normalized YAML atomically via temp+rename), `Delete(dir, slug)` (fails if the slug doesn't match the filename).
- Duplicate slugs across files → both files reported as a load error.
- `Import(bytes)` / `Export(task)`: same parse/marshal codecs — round-trip guarantee: `parse(export(t)) == t`.

The 2s polling loop that calls `Scan` is the daemon's (SPEC-06). `Scan` itself is synchronous and testable.

## 5. Index record (bridge to SPEC-03)

The daemon syncs each canonical task into the `tasks` index row, storing the **full task JSON** as `parsed_json` (not just a projection) so IPC handlers can execute it without re-reading files (specified in SPEC-07, implemented by the daemon sync loop). The task package does not import the db package (one-way dependency: daemon composes them). `enabled` is runtime state — the YAML has no `enabled` field in v1; the daemon keeps it in the index row (D3 spirit).

## 6. Example task

`tasks/example.yaml` — a PowerShell script task demonstrating the schema (name/slug/type/runtime/script/env/timeout). It is documentation, not necessarily runnable on every OS.

## 7. Testing

1. Table-driven validation: every rule row above has a valid + invalid case (unknown field, bad slug `Bad_Slug`, missing name, negative timeout, bad runtime, malformed `${` env ref, `retry.delay_seconds` negative, `notify_on: [nope]`).
2. Round-trip: the two canonical examples (script + binary) survive `export(parse(x)) == x`.
3. Store: save creates `<slug>.yaml`; scan loads it back equal; duplicate-slug files both reported; delete removes; filename/slug mismatch rejected.
4. `Scan` with one broken file still returns the healthy tasks + the error list.
5. `Import` rejects v2 headers with a clear version error.

## 8. Files

```text
internal/core/task/model.go      # Task struct, defaults, json tags (wire format)
internal/core/task/yaml.go       # parse/marshal (strict decoder) + Import/Export
internal/core/task/validate.go   # Validate(Task) []error
internal/core/task/store.go      # Scan/LoadFile/Save/Delete
internal/core/task/model_test.go # validation + round-trip tests
internal/core/task/store_test.go # file store tests (t.TempDir())
tasks/example.yaml
go.mod                           # + gopkg.in/yaml.v3
```

## 9. Acceptance criteria

1. All validation tests pass; strict decoder rejects unknown fields.
2. Script and binary canonical examples round-trip exactly.
3. Store behavior per §7.3–7.4.
4. `tasks/example.yaml` loads cleanly via `LoadFile`.
5. No DB imports in `internal/core/task` (verified by the review; architecture stays one-way).
6. `make check` green.

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green.
3. `gopkg.in/yaml.v3` pinned in go.mod; migration files untouched.