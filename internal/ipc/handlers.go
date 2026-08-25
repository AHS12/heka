package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"

	"heka/internal/core/executor"
	"heka/internal/core/task"
	"heka/internal/db"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "GET") {
		return
	}
	writeJSON(w, http.StatusOK, s.deps.Health())
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "POST") {
		return
	}
	// Respond first so the client receives the ack, then shut down.
	respondOK(w)
	if s.deps.Shutdown != nil {
		s.deps.Shutdown()
	}
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		rows, err := s.deps.Tasks.ListWithLastRun()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		summaries := make([]TaskSummary, 0, len(rows))
		for _, row := range rows {
			summaries = append(summaries, summarizeTask(row.Task, row.LastStatus, row.LastRunAt))
		}
		writeJSON(w, http.StatusOK, summaries)
	case "POST":
		s.createTask(w, r)
	default:
		requireMethod(w, r, "GET")
	}
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	switch r.Method {
	case "GET":
		row, err := s.deps.Tasks.Get(slug)
		if err != nil {
			writeTaskError(w, err, slug)
			return
		}
		t, err := taskFromIndex(row)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, TaskDetail{
			Enabled:   row.Enabled,
			UpdatedAt: row.UpdatedAt,
			Task:      t,
		})
	case "PUT":
		s.updateTask(w, r)
	case "DELETE":
		s.deleteTask(w, r)
	default:
		requireMethod(w, r, "GET")
	}
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "POST") {
		return
	}
	limitBody(w, r)
	slug := r.PathValue("slug")

	row, err := s.deps.Tasks.Get(slug)
	if err != nil {
		writeTaskError(w, err, slug)
		return
	}
	t, err := taskFromIndex(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	var body struct {
		Trigger string `json:"trigger"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "request_too_large",
					"request body exceeds 1 MiB")
				return
			}
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
			return
		}
	}

	// Runs outlive the HTTP request: WithoutCancel keeps tracing values but
	// drops the request's cancellation, so finishing the response can never
	// kill the process group (caught by the live SPEC-08 smoke).
	handle, err := s.deps.Runner.Start(context.WithoutCancel(r.Context()), &t, executor.Options{
		Trigger: body.Trigger,
		BaseDir: filepath.Dir(row.YAMLPath),
	})
	if errors.Is(err, executor.ErrAlreadyRunning) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, RunResponse{GroupID: handle.GroupID, Status: "running"})
}

func (s *Server) handleEnable(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "POST") {
		return
	}
	if err := s.deps.Tasks.SetEnabled(r.PathValue("slug"), true); err != nil {
		writeTaskError(w, err, r.PathValue("slug"))
		return
	}
	respondOK(w)
}

func (s *Server) handleDisable(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "POST") {
		return
	}
	if err := s.deps.Tasks.SetEnabled(r.PathValue("slug"), false); err != nil {
		writeTaskError(w, err, r.PathValue("slug"))
		return
	}
	respondOK(w)
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "POST") {
		return
	}
	slug := r.PathValue("slug")
	if err := s.deps.Runner.Cancel(slug); err != nil {
		if errors.Is(err, executor.ErrNotRunning) {
			writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no active run for %q", slug))
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	respondOK(w)
}

func (s *Server) handleTaskRuns(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "GET") {
		return
	}
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = min(n, 200)
		}
	}
	slug := r.PathValue("slug")
	rows, err := s.deps.Runs.ListByTask(slug, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, RunList{Runs: toRuns(rows)})
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "GET") {
		return
	}
	row, err := s.deps.Runs.Get(r.PathValue("run_id"))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toRun(row))
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "GET") {
		return
	}
	q := r.URL.Query()
	f := db.RunsFilter{
		Task:   q.Get("task"),
		Status: q.Get("status"),
		From:   q.Get("from"),
		To:     q.Get("to"),
		Q:      q.Get("q"),
		Cursor: q.Get("cursor"),
		Order:  q.Get("order"),
	}
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			f.Limit = n
		}
	}
	result, err := s.deps.Runs.ListRuns(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, RunListWithTotal{
		Runs:       toRuns(result.Runs),
		Total:      result.Total,
		NextCursor: result.NextCursor,
	})
}

// writeTaskError maps db errors onto the envelope table.
func writeTaskError(w http.ResponseWriter, err error, slug string) {
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("task %q not found", slug))
		return
	}
	writeError(w, http.StatusInternalServerError, "internal", err.Error())
}

// summarizeTask decodes the indexed task into a list row. lastStatus/lastRunAt
// come from the latest-run join (SPEC-13 §1).
func summarizeTask(row db.Task, lastStatus, lastRunAt string) TaskSummary {
	t, err := taskFromIndex(row)
	if err != nil {
		return TaskSummary{Slug: row.Slug, Name: row.Name, Enabled: row.Enabled,
			LastStatus: lastStatus, LastRunAt: lastRunAt}
	}
	return TaskSummary{
		Slug:       t.Slug,
		Name:       t.Name,
		Type:       t.Type,
		Runtime:    t.Runtime,
		Enabled:    row.Enabled,
		UpdatedAt:  row.UpdatedAt,
		LastStatus: lastStatus,
		LastRunAt:  lastRunAt,
	}
}

// newTaskFromIndex decodes the canonical task JSON stored by the sync loop.
func taskFromIndex(row db.Task) (task.Task, error) {
	var t task.Task
	if err := json.Unmarshal([]byte(row.ParsedJSON), &t); err != nil {
		return task.Task{}, fmt.Errorf("task %q index corrupt: %w", row.Slug, err)
	}
	return t, nil
}

func toRuns(rows []db.Run) []Run {
	out := make([]Run, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRun(r))
	}
	return out
}

func toRun(r db.Run) Run {
	out := Run{
		RunID:    r.RunID,
		GroupID:  r.GroupID,
		Attempt:  r.Attempt,
		TaskSlug: r.TaskSlug,
		Trigger:  r.Trigger,
		Status:   r.Status,
		Stdout:   r.Stdout,
		Stderr:   r.Stderr,
	}
	if r.ScheduleID != "" {
		out.ScheduleID = r.ScheduleID
	}
	if r.StartedAt != nil {
		out.StartedAt = *r.StartedAt
		out.FinishedAt = ""
		if r.FinishedAt != nil {
			out.FinishedAt = *r.FinishedAt
		}
	}
	if r.DurationMs != nil {
		out.DurationMs = *r.DurationMs
	}
	if r.ExitCode != nil {
		out.ExitCode = *r.ExitCode
	}
	if r.PID != nil {
		out.PID = *r.PID
	}
	return out
}
