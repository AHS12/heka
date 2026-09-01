-- Heka v1 initial schema (master spec §6 + SPEC-05 amendments).
-- Immutable once applied: schema changes are new numbered files.

CREATE TABLE tasks (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    yaml_path   TEXT NOT NULL,
    parsed_json TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE schedules (
    id            TEXT PRIMARY KEY,
    slug          TEXT NOT NULL UNIQUE,
    task_slug     TEXT NOT NULL REFERENCES tasks(slug),
    kind          TEXT NOT NULL,
    cron          TEXT,
    run_at        TEXT,
    timezone      TEXT NOT NULL DEFAULT 'local',
    enabled       INTEGER NOT NULL DEFAULT 1,
    missed_policy TEXT NOT NULL DEFAULT 'skip',
    next_run_at   TEXT,
    last_run_at   TEXT,
    last_status   TEXT,
    created_at    TEXT NOT NULL
);

CREATE TABLE runs (
    run_id      TEXT PRIMARY KEY,
    group_id    TEXT NOT NULL,
    attempt     INTEGER NOT NULL DEFAULT 0,
    task_slug   TEXT NOT NULL,
    schedule_id TEXT,
    trigger     TEXT NOT NULL,
    status      TEXT NOT NULL,
    started_at  TEXT,
    finished_at TEXT,
    duration_ms INTEGER,
    exit_code   INTEGER,
    pid         INTEGER,
    stdout      TEXT,
    stderr      TEXT,
    created_at  TEXT NOT NULL
);

CREATE TABLE secrets (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE kv (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE INDEX idx_runs_task_started ON runs(task_slug, started_at DESC);
CREATE INDEX idx_runs_status      ON runs(status);
CREATE INDEX idx_runs_group       ON runs(group_id);
CREATE INDEX idx_schedules_due    ON schedules(enabled, next_run_at);