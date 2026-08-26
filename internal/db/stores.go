package db

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned by Get-style methods when a row is absent.
var ErrNotFound = errors.New("db: not found")

// Store accessors (SPEC-03 §3).
func (d *DB) Tasks() *TaskStore         { return &TaskStore{db: d} }
func (d *DB) Schedules() *ScheduleStore { return &ScheduleStore{db: d} }
func (d *DB) Runs() *RunStore           { return &RunStore{db: d} }
func (d *DB) Secrets() *SecretStore     { return &SecretStore{db: d} }
func (d *DB) KV() *KVStore              { return &KVStore{db: d} }

// Task is the index row derived from a canonical task YAML (SPEC-04 §5).
// The YAML file remains the source of truth; this is a cached index.
type Task struct {
	ID         string
	Slug       string
	Name       string
	YAMLPath   string
	ParsedJSON string
	Enabled    bool
	CreatedAt  string
	UpdatedAt  string
}

// Schedule is a recurring or one-time execution rule (runtime state, D3).
type Schedule struct {
	ID           string
	Slug         string
	TaskSlug     string
	Kind         string // "recurring" | "onetime"
	Cron         string
	RunAt        string
	Timezone     string
	Enabled      bool
	MissedPolicy string // "skip" | "run_now"
	NextRunAt    string
	LastRunAt    string
	LastStatus   string
	CreatedAt    string
}

// Run is one attempt row. A logical run (group) may own several attempts
// sharing GroupID (SPEC-05).
type Run struct {
	RunID      string
	GroupID    string
	Attempt    int
	TaskSlug   string
	ScheduleID string
	Trigger    string // "manual" | "schedule" | "cli" | "system"
	Status     string // queued|running|success|failed|timed_out|cancelled|skipped|missed
	StartedAt  *string
	FinishedAt *string
	DurationMs *int64
	ExitCode   *int
	PID        *int
	Stdout     string
	Stderr     string
	CreatedAt  string
}

// TaskStore is the tasks index.
type TaskStore struct {
	db *DB
}

func (s *TaskStore) Save(t Task) error {
	const q = `
INSERT INTO tasks (id, slug, name, yaml_path, parsed_json, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (slug) DO UPDATE SET
    name = excluded.name,
    yaml_path = excluded.yaml_path,
    parsed_json = excluded.parsed_json,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at`
	_, err := s.db.sql.Exec(q, t.ID, t.Slug, t.Name, t.YAMLPath, t.ParsedJSON, b(t.Enabled), t.CreatedAt, Now())
	return err
}

func (s *TaskStore) Get(slug string) (Task, error) {
	var t Task
	err := s.db.sql.QueryRow(
		`SELECT id, slug, name, yaml_path, parsed_json, enabled, created_at, updated_at
		   FROM tasks WHERE slug = ?`, slug,
	).Scan(&t.ID, &t.Slug, &t.Name, &t.YAMLPath, &t.ParsedJSON, &t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return Task{}, ErrNotFound
	}
	return t, err
}

func (s *TaskStore) List() ([]Task, error) {
	rows, err := s.db.sql.Query(
		`SELECT id, slug, name, yaml_path, parsed_json, enabled, created_at, updated_at
		   FROM tasks ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.YAMLPath, &t.ParsedJSON, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// TaskWithRun is a task index row plus its latest run, for list surfaces
// (SPEC-13 §1). Zero values mean the task has never run.
type TaskWithRun struct {
	Task
	LastStatus string
	LastRunAt  string
}

// ListWithLastRun lists tasks with a LEFT JOIN onto each task's latest run
// (idx_runs_task_started makes the correlated subquery cheap; run_id breaks
// started_at ties).
func (s *TaskStore) ListWithLastRun() ([]TaskWithRun, error) {
	rows, err := s.db.sql.Query(`
SELECT t.id, t.slug, t.name, t.yaml_path, t.parsed_json, t.enabled, t.created_at, t.updated_at,
       COALESCE(r.status, ''),
       CASE WHEN r.started_at IS NULL THEN '' ELSE r.started_at END
  FROM tasks t
  LEFT JOIN runs r ON r.task_slug = t.slug AND r.run_id = (
        SELECT r2.run_id FROM runs r2
         WHERE r2.task_slug = t.slug
         ORDER BY r2.started_at DESC, r2.run_id DESC LIMIT 1)
 ORDER BY t.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskWithRun
	for rows.Next() {
		var tw TaskWithRun
		if err := rows.Scan(&tw.ID, &tw.Slug, &tw.Name, &tw.YAMLPath, &tw.ParsedJSON,
			&tw.Enabled, &tw.CreatedAt, &tw.UpdatedAt, &tw.LastStatus, &tw.LastRunAt); err != nil {
			return nil, err
		}
		out = append(out, tw)
	}
	return out, rows.Err()
}

func (s *TaskStore) Delete(slug string) error {
	_, err := s.db.sql.Exec(`DELETE FROM tasks WHERE slug = ?`, slug)
	return err
}

func (s *TaskStore) SetEnabled(slug string, enabled bool) error {
	_, err := s.db.sql.Exec(`UPDATE tasks SET enabled = ?, updated_at = ? WHERE slug = ?`, b(enabled), Now(), slug)
	return err
}

// ScheduleStore is the schedules + one-time jobs table.
type ScheduleStore struct {
	db *DB
}

func (s *ScheduleStore) Save(sch Schedule) error {
	const q = `
INSERT INTO schedules (id, slug, task_slug, kind, cron, run_at, timezone, enabled,
                       missed_policy, next_run_at, last_run_at, last_status, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    slug = excluded.slug,
    task_slug = excluded.task_slug,
    kind = excluded.kind,
    cron = excluded.cron,
    run_at = excluded.run_at,
    timezone = excluded.timezone,
    enabled = excluded.enabled,
    missed_policy = excluded.missed_policy,
    next_run_at = excluded.next_run_at,
    last_run_at = excluded.last_run_at,
    last_status = excluded.last_status`
	_, err := s.db.sql.Exec(q, sch.ID, sch.Slug, sch.TaskSlug, sch.Kind, sch.Cron, sch.RunAt,
		sch.Timezone, b(sch.Enabled), sch.MissedPolicy, sch.NextRunAt, sch.LastRunAt,
		sch.LastStatus, sch.CreatedAt)
	return err
}

func (s *ScheduleStore) Get(id string) (Schedule, error) {
	var sch Schedule
	err := s.db.sql.QueryRow(
		`SELECT id, slug, task_slug, kind, cron, run_at, timezone, enabled,
		        missed_policy, next_run_at, last_run_at, last_status, created_at
		   FROM schedules WHERE id = ?`, id,
	).Scan(&sch.ID, &sch.Slug, &sch.TaskSlug, &sch.Kind, &sch.Cron, &sch.RunAt, &sch.Timezone,
		&sch.Enabled, &sch.MissedPolicy, &sch.NextRunAt, &sch.LastRunAt, &sch.LastStatus, &sch.CreatedAt)
	if err == sql.ErrNoRows {
		return Schedule{}, ErrNotFound
	}
	return sch, err
}

func (s *ScheduleStore) List() ([]Schedule, error) {
	rows, err := s.db.sql.Query(
		`SELECT id, slug, task_slug, kind, cron, run_at, timezone, enabled,
		        missed_policy, next_run_at, last_run_at, last_status, created_at
		   FROM schedules ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Schedule
	for rows.Next() {
		var sch Schedule
		if err := rows.Scan(&sch.ID, &sch.Slug, &sch.TaskSlug, &sch.Kind, &sch.Cron, &sch.RunAt,
			&sch.Timezone, &sch.Enabled, &sch.MissedPolicy, &sch.NextRunAt, &sch.LastRunAt,
			&sch.LastStatus, &sch.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

func (s *ScheduleStore) ListByTask(taskSlug string) ([]Schedule, error) {
	rows, err := s.db.sql.Query(
		`SELECT id, slug, task_slug, kind, cron, run_at, timezone, enabled,
		        missed_policy, next_run_at, last_run_at, last_status, created_at
		   FROM schedules WHERE task_slug = ? ORDER BY slug`, taskSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Schedule
	for rows.Next() {
		var sch Schedule
		if err := rows.Scan(&sch.ID, &sch.Slug, &sch.TaskSlug, &sch.Kind, &sch.Cron, &sch.RunAt,
			&sch.Timezone, &sch.Enabled, &sch.MissedPolicy, &sch.NextRunAt, &sch.LastRunAt,
			&sch.LastStatus, &sch.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

func (s *ScheduleStore) Delete(id string) error {
	result, err := s.db.sql.Exec(`DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ScheduleStore) SetEnabled(id string, enabled bool) error {
	_, err := s.db.sql.Exec(`UPDATE schedules SET enabled = ? WHERE id = ?`, b(enabled), id)
	return err
}

// RunStore is the execution history (one row per attempt, SPEC-05).
type RunStore struct {
	db *DB
}

func (s *RunStore) Create(r Run) error {
	const q = `
INSERT INTO runs (run_id, group_id, attempt, task_slug, schedule_id, trigger, status,
                  started_at, finished_at, duration_ms, exit_code, pid, stdout, stderr, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.sql.Exec(q, r.RunID, r.GroupID, r.Attempt, r.TaskSlug,
		nullableStr(r.ScheduleID), r.Trigger, r.Status,
		nullableStrPtr(r.StartedAt), nullableStrPtr(r.FinishedAt),
		nullableInt64(r.DurationMs), nullableInt(r.ExitCode), nullableInt(r.PID),
		r.Stdout, r.Stderr, r.CreatedAt)
	return err
}

func (s *RunStore) Update(r Run) error {
	const q = `
UPDATE runs SET status = ?, finished_at = ?, duration_ms = ?, exit_code = ?, stdout = ?, stderr = ?
 WHERE run_id = ?`
	result, err := s.db.sql.Exec(q, r.Status, nullableStrPtr(r.FinishedAt),
		nullableInt64(r.DurationMs), nullableInt(r.ExitCode), r.Stdout, r.Stderr, r.RunID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *RunStore) Get(runID string) (Run, error) {
	var r Run
	err := s.db.sql.QueryRow(
		`SELECT run_id, group_id, attempt, task_slug, COALESCE(schedule_id, '') AS schedule_id,
		        trigger, status, started_at, finished_at, duration_ms, exit_code, pid,
		        stdout, stderr, created_at
		   FROM runs WHERE run_id = ?`, runID,
	).Scan(&r.RunID, &r.GroupID, &r.Attempt, &r.TaskSlug, &r.ScheduleID, &r.Trigger, &r.Status,
		&r.StartedAt, &r.FinishedAt, &r.DurationMs, &r.ExitCode, &r.PID, &r.Stdout, &r.Stderr,
		&r.CreatedAt)
	if err == sql.ErrNoRows {
		return Run{}, ErrNotFound
	}
	return r, err
}

// ListByTask returns the latest runs for a task, newest first. run_id is a
// time-ordered ULID, so it also breaks same-second started_at ties
// deterministically.
func (s *RunStore) ListByTask(slug string, limit int) ([]Run, error) {
	rows, err := s.db.sql.Query(
		`SELECT run_id, group_id, attempt, task_slug, COALESCE(schedule_id, '') AS schedule_id,
		        trigger, status, started_at, finished_at, duration_ms, exit_code, pid,
		        stdout, stderr, created_at
		   FROM runs WHERE task_slug = ?
		  ORDER BY started_at DESC, run_id DESC LIMIT ?`, slug, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

// ListBySchedule returns the latest runs originating from one schedule
// (feed for scheduler last_status, SPEC-09).
func (s *RunStore) ListBySchedule(scheduleID string, limit int) ([]Run, error) {
	rows, err := s.db.sql.Query(
		`SELECT run_id, group_id, attempt, task_slug, COALESCE(schedule_id, '') AS schedule_id,
		        trigger, status, started_at, finished_at, duration_ms, exit_code, pid,
		        stdout, stderr, created_at
		   FROM runs WHERE schedule_id = ?
		  ORDER BY started_at DESC, run_id DESC LIMIT ?`, scheduleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

// CountBySchedule counts schedule-triggered runs started at or after since
// (missed-run reconciliation, SPEC-09 §3).
func (s *RunStore) CountBySchedule(scheduleID string, since time.Time) (int, error) {
	var n int
	err := s.db.sql.QueryRow(
		`SELECT COUNT(*) FROM runs WHERE schedule_id = ? AND trigger = 'schedule' AND started_at >= ?`,
		scheduleID, since.UTC().Format(time.RFC3339),
	).Scan(&n)
	return n, err
}

// RunsFilter holds optional filters for the global runs listing (SPEC-14 §1).
type RunsFilter struct {
	Task   string // task slug
	Status string // run status
	From   string // RFC3339 lower bound (inclusive)
	To     string // RFC3339 upper bound (inclusive)
	Q      string // substring search over stdout/stderr
	Cursor string // base64-encoded cursor (started_at:run_id)
	Limit  int
	Order  string // "desc" (default, newest first) or "asc" (oldest first)
}

// RunsResult is the paginated runs response (SPEC-14 §1).
type RunsResult struct {
	Runs      []Run
	Total     int
	NextCursor string
}

// ListRuns returns filtered runs with total count and cursor pagination.
// The q parameter is a plain substring; % and _ in user input are escaped so
// they are treated as literals (SPEC-14 §1, PRD §31 injection care).
// Cursor is a base64-encoded "started_at:run_id" for stable pagination.
func (s *RunStore) ListRuns(f RunsFilter) (RunsResult, error) {
	where := []string{}
	args := []any{}

	if f.Task != "" {
		where = append(where, "task_slug = ?")
		args = append(args, f.Task)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.From != "" {
		where = append(where, "started_at >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, "started_at <= ?")
		args = append(args, f.To)
	}
	if f.Q != "" {
		escaped := escapeLike(f.Q)
		where = append(where, "(stdout LIKE ? ESCAPE '\\' OR stderr LIKE ? ESCAPE '\\')")
		args = append(args, "%"+escaped+"%", "%"+escaped+"%")
	}

	// Decode cursor for keyset pagination.
	if f.Cursor != "" {
		cursorSID, err := decodeCursor(f.Cursor)
		if err == nil {
			if f.Order == "asc" {
				// Oldest first: want rows after the cursor
				where = append(where, "(started_at > ? OR (started_at = ? AND run_id > ?))")
			} else {
				// Newest first (default): want rows before the cursor
				where = append(where, "(started_at < ? OR (started_at = ? AND run_id < ?))")
			}
			args = append(args, cursorSID.StartedAt, cursorSID.StartedAt, cursorSID.RunID)
		}
	}

	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	// Count total.
	var total int
	countQ := "SELECT COUNT(*) FROM runs" + clause
	if err := s.db.sql.QueryRow(countQ, args...).Scan(&total); err != nil {
		return RunsResult{}, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 25
	}

	order := "DESC"
	if f.Order == "asc" {
		order = "ASC"
	}

	query := `SELECT run_id, group_id, attempt, task_slug, COALESCE(schedule_id, '') AS schedule_id,
	                 trigger, status, started_at, finished_at, duration_ms, exit_code, pid,
	                 stdout, stderr, created_at
	            FROM runs` + clause + ` ORDER BY started_at ` + order + `, run_id ` + order + ` LIMIT ?`
	args = append(args, limit+1) // fetch one extra to detect next page

	rows, err := s.db.sql.Query(query, args...)
	if err != nil {
		return RunsResult{}, err
	}
	defer rows.Close()
	allRuns, err := scanRuns(rows)
	if err != nil {
		return RunsResult{}, err
	}

	var nextCursor string
	if len(allRuns) > limit {
		runs := allRuns[:limit]
		last := runs[len(runs)-1]
		if last.StartedAt != nil {
			nextCursor = encodeCursor(*last.StartedAt, last.RunID)
		}
		return RunsResult{Runs: runs, Total: total, NextCursor: nextCursor}, nil
	}
	return RunsResult{Runs: allRuns, Total: total}, nil
}

// cursorSID holds the decoded cursor components.
type cursorSID struct {
	StartedAt string
	RunID     string
}

func encodeCursor(startedAt, runID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(startedAt + ":" + runID))
}

func decodeCursor(cursor string) (cursorSID, error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return cursorSID{}, err
	}
	s := string(b)
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return cursorSID{}, fmt.Errorf("invalid cursor")
	}
	return cursorSID{StartedAt: s[:i], RunID: s[i+1:]}, nil
}

// escapeLike escapes % and _ so LIKE treats them as literals.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// ListByKind filters schedules by kind ("recurring" or "onetime").
func (s *ScheduleStore) ListByKind(kind string) ([]Schedule, error) {
	rows, err := s.db.sql.Query(
		`SELECT id, slug, task_slug, kind, cron, run_at, timezone, enabled,
		        missed_policy, next_run_at, last_run_at, last_status, created_at
		   FROM schedules WHERE kind = ? ORDER BY slug`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Schedule
	for rows.Next() {
		var sch Schedule
		if err := rows.Scan(&sch.ID, &sch.Slug, &sch.TaskSlug, &sch.Kind, &sch.Cron, &sch.RunAt,
			&sch.Timezone, &sch.Enabled, &sch.MissedPolicy, &sch.NextRunAt, &sch.LastRunAt,
			&sch.LastStatus, &sch.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

// Prune deletes finished runs (and their captured output) finished before the
// cutoff. Running and queued rows are never touched (SPEC-03 §5).
func (s *RunStore) Prune(before time.Time) error {
	cutoff := before.UTC().Format(time.RFC3339)
	_, err := s.db.sql.Exec(
		`DELETE FROM runs WHERE finished_at IS NOT NULL AND finished_at < ?`, cutoff)
	return err
}

// Stats aggregates dashboard counters and chart data (SPEC-16 §1).
func (s *RunStore) Stats() (StatsResult, error) {
	var out StatsResult

	// Task counts
	s.db.sql.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&out.Tasks)
	s.db.sql.QueryRow(`SELECT COUNT(*) FROM tasks WHERE enabled = 1`).Scan(&out.TasksEnabled)

	// Schedule counts
	s.db.sql.QueryRow(`SELECT COUNT(*) FROM schedules WHERE enabled = 1`).Scan(&out.SchedulesEnabled)

	// Currently running
	s.db.sql.QueryRow(`SELECT COUNT(*) FROM runs WHERE status IN ('queued','running')`).Scan(&out.Running)

	// Today counts (local midnight boundary)
	today := time.Now().UTC().Format("2006-01-02")
	s.db.sql.QueryRow(`SELECT COUNT(*) FROM runs WHERE date(created_at) = ?`, today).Scan(&out.RunsToday)
	s.db.sql.QueryRow(`SELECT COUNT(*) FROM runs WHERE date(created_at) = ? AND status = 'success'`, today).Scan(&out.SuccessToday)
	s.db.sql.QueryRow(`SELECT COUNT(*) FROM runs WHERE date(created_at) = ? AND status IN ('failed','timed_out')`, today).Scan(&out.FailedToday)

	// Run history: last 7 days aggregated by date+status
	rows, err := s.db.sql.Query(`
		SELECT date(created_at) AS d, status, COUNT(*) AS n
		FROM runs
		WHERE created_at >= date('now', '-7 days')
		GROUP BY d, status
		ORDER BY d`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	type dayStatus struct {
		Date   string
		Status string
		Count  int
	}
	var ds []dayStatus
	for rows.Next() {
		var d dayStatus
		if err := rows.Scan(&d.Date, &d.Status, &d.Count); err != nil {
			return out, err
		}
		ds = append(ds, d)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	// Build run_history map
	historyMap := map[string]*DayStats{}
	for _, d := range ds {
		if _, ok := historyMap[d.Date]; !ok {
			historyMap[d.Date] = &DayStats{Date: d.Date}
		}
		entry := historyMap[d.Date]
		entry.Total += d.Count
		switch d.Status {
		case "success":
			entry.Success += d.Count
		case "failed", "timed_out":
			entry.Failed += d.Count
		}
	}
	out.RunHistory = make([]DayStats, 0, len(historyMap))
	for _, v := range historyMap {
		out.RunHistory = append(out.RunHistory, *v)
	}

	// Status distribution (all time)
	sRows, err := s.db.sql.Query(`SELECT status, COUNT(*) AS n FROM runs GROUP BY status`)
	if err != nil {
		return out, err
	}
	defer sRows.Close()
	for sRows.Next() {
		var s struct {
			Status string
			Count  int
		}
		if err := sRows.Scan(&s.Status, &s.Count); err != nil {
			return out, err
		}
		out.StatusDistribution = append(out.StatusDistribution, StatusCount{
			Status: s.Status, Count: s.Count,
		})
	}

	// Recent activity (last 10 runs)
	aRows, err := s.db.sql.Query(`
		SELECT run_id, task_slug, status, created_at
		FROM runs ORDER BY created_at DESC LIMIT 10`)
	if err != nil {
		return out, err
	}
	defer aRows.Close()
	for aRows.Next() {
		var a ActivityItem
		if err := aRows.Scan(&a.RunID, &a.TaskSlug, &a.Status, &a.At); err != nil {
			return out, err
		}
		out.RecentActivity = append(out.RecentActivity, a)
	}

	return out, nil
}

// StatsResult is the full dashboard payload.
type StatsResult struct {
	Tasks              int             `json:"tasks"`
	TasksEnabled       int             `json:"tasks_enabled"`
	SchedulesEnabled   int             `json:"schedules_enabled"`
	Running            int             `json:"running"`
	RunsToday          int             `json:"runs_today"`
	SuccessToday       int             `json:"success_today"`
	FailedToday        int             `json:"failed_today"`
	RunHistory         []DayStats      `json:"run_history"`
	StatusDistribution []StatusCount   `json:"status_distribution"`
	RecentActivity     []ActivityItem  `json:"recent_activity"`
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

// SecretStore is the secrets vault (SPEC-11). Values are AES-GCM-sealed with
// the per-install key at rest; names stay plaintext (they are references in
// task YAML). Get decrypts on read with a legacy-plaintext fallback.
type SecretStore struct {
	db *DB
}

func (s *SecretStore) Set(key, value string) error {
	sealed, err := s.db.encryptSecret(value)
	if err != nil {
		return err
	}
	_, err = s.db.sql.Exec(
		`INSERT INTO secrets (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, sealed)
	return err
}

func (s *SecretStore) Get(key string) (string, bool, error) {
	var value string
	err := s.db.sql.QueryRow(`SELECT value FROM secrets WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	plain, _ := s.db.decryptSecret(value)
	return plain, true, nil
}

func (s *SecretStore) Delete(key string) error {
	_, err := s.db.sql.Exec(`DELETE FROM secrets WHERE key = ?`, key)
	return err
}

func (s *SecretStore) Keys() ([]string, error) {
	rows, err := s.db.sql.Query(`SELECT key FROM secrets ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// KVStore is the daemon's small state table (heartbeat, scheduler flags).
type KVStore struct {
	db *DB
}

func (s *KVStore) Set(key, value string) error {
	_, err := s.db.sql.Exec(
		`INSERT INTO kv (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *KVStore) Get(key string) (string, bool, error) {
	var value string
	err := s.db.sql.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *KVStore) Delete(key string) error {
	_, err := s.db.sql.Exec(`DELETE FROM kv WHERE key = ?`, key)
	return err
}

func scanRuns(rows *sql.Rows) ([]Run, error) {
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.RunID, &r.GroupID, &r.Attempt, &r.TaskSlug, &r.ScheduleID,
			&r.Trigger, &r.Status, &r.StartedAt, &r.FinishedAt, &r.DurationMs, &r.ExitCode,
			&r.PID, &r.Stdout, &r.Stderr, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// b converts a bool to SQLite's integer representation.
func b(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullableStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullableStrPtr(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
