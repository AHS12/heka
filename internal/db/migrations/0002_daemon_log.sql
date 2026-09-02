-- daemon_log: internal daemon diagnostics (scheduler reconcile, lifecycle,
-- wake detection) surfaced in the GUI Logs → System tab. Task output lives
-- in runs; this table is the daemon's own event log.
CREATE TABLE daemon_log (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    ts      TEXT NOT NULL,
    level   TEXT NOT NULL,
    event   TEXT NOT NULL,
    message TEXT NOT NULL
);

CREATE INDEX idx_daemon_log_ts ON daemon_log(ts DESC);
