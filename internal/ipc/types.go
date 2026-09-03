package ipc

import (
	"time"

	"heka/internal/core/backup"
	"heka/internal/core/task"
)

// Health is the daemon's status snapshot (SPEC-06 §3), now owned by the wire
// contract. Scheduler is "running" once the engine is wired (SPEC-09).
type Health struct {
	Version       string    `json:"version"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	Core          string    `json:"core"`      // "healthy" | error text
	Scheduler     string    `json:"scheduler"` // "starting" | "running" | "paused"
	NextRunAt     time.Time `json:"next_run_at,omitempty"`
	NextTaskSlug  string    `json:"next_task_slug,omitempty"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

// Schedule is the wire shape for the schedules surface (SPEC-09 §4).
type Schedule struct {
	ID              string `json:"id"`
	Slug            string `json:"slug"`
	TaskSlug        string `json:"task_slug"`
	Kind            string `json:"kind"` // "recurring" | "onetime"
	Cron            string `json:"cron,omitempty"`
	RunAt           string `json:"run_at,omitempty"`
	Enabled         bool   `json:"enabled"`
	MissedPolicy    string `json:"missed_policy"`
	NextRunAt       string `json:"next_run_at,omitempty"`
	LastRunAt       string `json:"last_run_at,omitempty"`
	LastStatus      string `json:"last_status,omitempty"`
	LatestRunID     string `json:"latest_run_id,omitempty"`
	LatestRunStatus string `json:"latest_run_status,omitempty"`
	LatestRunStart  string `json:"latest_run_started_at,omitempty"`
	LatestRunFinish string `json:"latest_run_finished_at,omitempty"`
	SkippedCount    int    `json:"skipped_count"`
	MissedCount     int    `json:"missed_count"`
}

// TaskSummary is a list row for GET /v1/tasks (SPEC-07 §3, extended SPEC-13).
type TaskSummary struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Runtime    string `json:"runtime"`
	Enabled    bool   `json:"enabled"`
	UpdatedAt  string `json:"updated_at"`
	LastStatus string `json:"last_status,omitempty"`
	LastRunAt  string `json:"last_run_at,omitempty"`
}

// TaskDetail is the editor payload for GET /v1/tasks/{slug}: the canonical
// task plus daemon-side index state (enabled/updated_at).
type TaskDetail struct {
	Enabled   bool      `json:"enabled"`
	UpdatedAt string    `json:"updated_at"`
	Task      task.Task `json:"task"`
}

// RunResponse is the immediate answer to POST run.
type RunResponse struct {
	GroupID string `json:"group_id"`
	Status  string `json:"status"` // "queued" | "running"
}

// RunList is the envelope for GET /v1/tasks/{slug}/runs.
type RunList struct {
	Runs []Run `json:"runs"`
}

// RunListWithTotal is the paginated envelope for GET /v1/runs (SPEC-14 §1).
type RunListWithTotal struct {
	Runs       []Run  `json:"runs"`
	Total      int    `json:"total"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// DaemonLog is one entry of the daemon's own event log (scheduler reconcile,
// lifecycle, wake detection) surfaced in the GUI Logs → System view.
type DaemonLog struct {
	ID      int64  `json:"id"`
	TS      string `json:"ts"`
	Level   string `json:"level"` // info | warn | error
	Event   string `json:"event"` // reconcile | daemon | scheduler
	Message string `json:"message"`
}

// Run is one attempt row on the wire (SPEC-05 group/attempt model).
type Run struct {
	RunID      string `json:"run_id"`
	GroupID    string `json:"group_id"`
	Attempt    int    `json:"attempt"`
	TaskSlug   string `json:"task_slug"`
	ScheduleID string `json:"schedule_id"`
	Trigger    string `json:"trigger"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	PID        int    `json:"pid,omitempty"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
}

// Error is the error envelope body (SPEC-07 §2). It implements error so
// client callers can switch on Code.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// Stats is the dashboard payload from GET /v1/stats (SPEC-16 §1).
type Stats struct {
	Tasks              int            `json:"tasks"`
	TasksEnabled       int            `json:"tasks_enabled"`
	SchedulesEnabled   int            `json:"schedules_enabled"`
	Running            int            `json:"running"`
	RunsToday          int            `json:"runs_today"`
	SuccessToday       int            `json:"success_today"`
	FailedToday        int            `json:"failed_today"`
	RunHistory         []DayStats     `json:"run_history"`
	StatusDistribution []StatusCount  `json:"status_distribution"`
	RecentActivity     []ActivityItem `json:"recent_activity"`
}

type DayStats struct {
	Date    string `json:"date"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
	Total   int    `json:"total"`
}

type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type ActivityItem struct {
	RunID    string `json:"run_id"`
	TaskSlug string `json:"task_slug"`
	Status   string `json:"status"`
	At       string `json:"at"`
}

// SettingsDTO is the wire shape for daemon settings (SPEC-16 §2).
type SettingsDTO struct {
	LogRetentionDays     int    `json:"log_retention_days"`
	SoundSuccess         string `json:"sound_success"`
	SoundFailure         string `json:"sound_failure"`
	SoundTimeout         string `json:"sound_timeout"`
	ReconcileIntervalMin int    `json:"reconcile_interval_min"`
	// WatchdogIntervalMin is the OS watchdog task's check cadence (1–60).
	// Saving a new value recreates the scheduled task.
	WatchdogIntervalMin int `json:"watchdog_interval_min"`
}

// SoundPreviewRequest is the request body for POST /v1/settings/sound-preview.
type SoundPreviewRequest struct {
	Preset string `json:"preset"`
}

// BackupConfigDTO is the wire shape for backup settings. Secret material
// (passphrase, S3 keys) never appears here — the UI manages those through
// the secrets API; PassphraseSet only tells it whether the vault entry
// exists.
type BackupConfigDTO struct {
	Schedule      backup.ScheduleSpec `json:"schedule"`
	LocalDir      string              `json:"local_dir"`
	KeepLastLocal int                 `json:"keep_last_local"`
	S3            backup.S3Config     `json:"s3"`
	Includes      backup.Includes     `json:"includes"`
	PassphraseSet bool                `json:"passphrase_set"`
}

// BackupDestinationResult is one destination's outcome inside a backup job.
type BackupDestinationResult struct {
	Type string `json:"type"` // local | s3
	OK   bool   `json:"ok"`
	Path string `json:"path,omitempty"` // local file path or remote object key
	Err  string `json:"error,omitempty"`
}

// BackupJobDTO is one backup history row on the wire.
type BackupJobDTO struct {
	ID           string                     `json:"id"`
	Trigger      string                     `json:"trigger"` // manual | scheduled
	Status       string                     `json:"status"`  // running | success | partial | failed
	StartedAt    string                     `json:"started_at"`
	FinishedAt   string                     `json:"finished_at,omitempty"`
	SizeBytes    int64                      `json:"size_bytes,omitempty"`
	LocalPath    string                     `json:"local_path,omitempty"`
	Destinations []BackupDestinationResult  `json:"destinations,omitempty"`
	Err          string                     `json:"error,omitempty"`
}

// BackupStatusDTO answers GET /v1/backup/status.
type BackupStatusDTO struct {
	Running   bool          `json:"running"`
	Current   *BackupJobDTO `json:"current,omitempty"`
	Last      *BackupJobDTO `json:"last,omitempty"`
	NextRunAt string        `json:"next_run_at,omitempty"`
}

// BackupTestDTO answers POST /v1/backup/test.
type BackupTestDTO struct {
	Local *BackupDestinationResult `json:"local,omitempty"`
	S3    *BackupDestinationResult `json:"s3,omitempty"`
}

// BackupRunResponse is the immediate answer to POST /v1/backup/run.
type BackupRunResponse struct {
	JobID string `json:"job_id"`
}

// errEnvelope is the wire shape for errors.
type errEnvelope struct {
	Error *Error `json:"error"`
}
