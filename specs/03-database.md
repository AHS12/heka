# SPEC-03 — Database

Status: **Draft** · Depends on: SPEC-01, SPEC-02 (config) · Master spec: §4, §6, D4

## Goal

Stand up the daemon's SQLite layer: open a single database file in the configured data dir, apply embedded versioned migrations, and expose typed stores for every table. **Only the daemon ever opens this database** (PRD §4, master spec §4) — the GUI and CLI go through IPC.

## Scope

**In:**
- `internal/db`: connection (`Open`), migration runner, typed stores
- `internal/db/migrations/0001_init.sql`: full v1 schema (master spec §6)
- WAL + pragmas, foreign keys, busy timeout
- Integration tests against a real temp-file DB

**Out:** run-id generation (SPEC-05), retention *callsites* (SPEC-06/09), scheduler next-due queries (SPEC-09), dashboard stats queries (SPEC-13), anything GUI/CLI-facing.

## 1. Driver & connection

`modernc.org/sqlite` (pure Go, no CGO — D4). Connection:

```go
db.Open(dataDir) // dataDir from config.Load(env, home).DataDir (SPEC-02)
```

DSN (modernc query-param form so pragmas apply per connection):

```text
file:<dataDir>/heka.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)
```

- `journal_mode=WAL`: concurrent readers (IPC health polls) without blocking the writer.
- `busy_timeout=5000`: one writer (daemon), but goroutine floods (scheduler + CLI-triggered runs) must not deadlock.
- `foreign_keys(1)` is critical — a schedule must not orphan its task.

File created `0600`-equivalent: on Windows the file inherits the user's profile ACL; on unix chmod 0600 after create. The DB holds secrets in SPEC-11; restrict perms now, not later.

`*sql.DB` pool: default settings (modernc tolerates multiple conns). Single-writer semantics enforced by the daemon's own design (SPEC-06), not by the pool.

## 2. Migrations

Hand-rolled runner, no external dependency:

- `migrations/*.sql` embedded via `//go:embed`.
- `schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`.
- `Migrate(d)` reads embedded files sorted by version, applies each inside a transaction, records the row.
- Files are **append-only and immutable once applied** — schema changes are new numbered files, never edits.

`0001_init.sql` = the master spec §6 schema verbatim (tables `tasks`, `schedules`, `runs`, `secrets`, `kv` + indexes).

## 3. API shape

```go
type DB struct { sql *sql.DB }

func Open(dataDir string) (*DB, error)   // creates dir if needed, opens + migrates
func (d *DB) Close() error
func (d *DB) Tasks() *TaskStore
func (d *DB) Schedules() *ScheduleStore
func (d *DB) Runs() *RunStore
func (d *DB) Secrets() *SecretStore
func (d *DB) KV() *KVStore
```

`Open` = open + `Migrate` in one call; callers never run migrations themselves.

Store methods (stable-enough surface for later specs):

- `TaskStore`: `Save(t Task)`, `Get(slug)`, `List()`, `Delete(slug)`, `SetEnabled(slug, bool)`
- `ScheduleStore`: `Save(s Schedule)`, `Get(id)`, `List()`, `ListByTask(taskSlug)`, `Delete(id)`, `SetEnabled(id, bool)`
- `RunStore`: `Create(r)`, `Update(r)` (whole row — status/exit/out set at finish), `Get(runID)`, `ListByTask(slug, limit)`, `Prune(before time.Time)`
- `SecretStore`: `Set(key, value)`, `Get(key)`, `Delete(key)`, `Keys()`
- `KVStore`: `Get(key)`, `Set(key, value)`, `Delete(key)`

Entity structs live in `internal/db` for now; the task model becomes the canonical YAML type in SPEC-04 (task spec may then move, keeping `db.Task` only for the index row). Run and schedule structs are intentionally thin at this stage and gain fields in SPEC-05/09 if needed.

## 4. Time & id conventions

- All timestamps: **TEXT, ISO-8601 UTC** (RFC3339). Display conversion happens at the boundary (IPC/UI), never stored local.
- `run_id`: TEXT PK, ULID format (26-char Crockford base32, `01J…` per master spec). Generation lands with the executor in SPEC-05; the column is already sized for it.

## 5. Retention (data)

`RunStore.Prune(before)` deletes runs (and their captured stdout/stderr) older than `before`. Daemon calls it at startup and on a timer (SPEC-06/09) with `log_retention_days` from config. Per-run output cap (`max_output_bytes`) is enforced by the executor (SPEC-05), not the DB.

## 6. Testing

Integration tests, real files in `t.TempDir()` (modernc makes this trivia on any OS):

1. Fresh DB: `Open` migrates to latest; `schema_migrations` has version rows.
2. Re-open: idempotent, no re-apply, no error.
3. CRUD round-trip for each store.
4. Foreign keys: inserting a schedule with an unknown `task_slug` fails.
5. WAL verified: `PRAGMA journal_mode` returns `wal`.
6. `Prune(before)` removes old rows, keeps new; works on an empty table.
7. KV/secret stores survive close/re-open (persistence).

## 7. Files

```text
internal/db/db.go             # Open/Close, DSN, pragmas, perms
internal/db/migrate.go        # embed + migration runner
internal/db/migrations/0001_init.sql
internal/db/stores.go         # TaskStore, ScheduleStore, RunStore, SecretStore, KVStore
internal/db/db_test.go        # integration tests
go.mod                        # + modernc.org/sqlite
```

## 8. Acceptance criteria

1. `go test ./internal/db/...` passes all integration cases above.
2. A second `Open` on the same DB skips migrations (idempotent).
3. WAL mode and `foreign_keys=ON` confirmed active on a live connection.
4. FK violation for orphan schedules is returned as an error.
5. `Prune` behaves per §6.6.
6. `make check` stays green.
7. No daemon/GUI/CLI behavior changes — this spec is pure foundation.

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green.
3. Migration files immutable once committed (checked in the review).