package ipc

import (
	"time"

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
	ID           string `json:"id"`
	Slug         string `json:"slug"`
	TaskSlug     string `json:"task_slug"`
	Kind         string `json:"kind"` // "recurring" | "onetime"
	Cron         string `json:"cron,omitempty"`
	RunAt        string `json:"run_at,omitempty"`
	Enabled      bool   `json:"enabled"`
	MissedPolicy string `json:"missed_policy"`
	NextRunAt    string `json:"next_run_at,omitempty"`
	LastRunAt    string `json:"last_run_at,omitempty"`
	LastStatus   string `json:"last_status,omitempty"`
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

// errEnvelope is the wire shape for errors.
type errEnvelope struct {
	Error *Error `json:"error"`
}
