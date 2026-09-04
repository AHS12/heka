-- backups: history of archive backup jobs (manual and scheduled). One row per
-- job attempt; per-destination outcomes live in destinations_json so a local
-- success with a failed remote upload is still visible. Config itself stays
-- in kv (backup_config) — this table is history only.
CREATE TABLE backups (
    id                TEXT PRIMARY KEY,
    trigger           TEXT NOT NULL,             -- manual | scheduled
    status            TEXT NOT NULL,             -- running | success | partial | failed
    started_at        TEXT NOT NULL,
    finished_at       TEXT,
    size_bytes        INTEGER,
    local_path        TEXT,
    destinations_json TEXT,
    error             TEXT
);

CREATE INDEX idx_backups_started ON backups(started_at DESC);
