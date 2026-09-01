package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"heka/internal/core/task"
	"heka/internal/db"
)

// createTask handles POST /v1/tasks (SPEC-13 §1): raw task YAML in the body,
// strict parse + validate, conflict on existing slug, atomic file write, then
// a reindex so the list/detail surfaces see it immediately.
func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	yaml, ok := readTaskBody(w, r)
	if !ok {
		return
	}
	t, ok := parseTaskBody(w, yaml)
	if !ok {
		return
	}
	if _, err := s.deps.Tasks.Get(t.Slug); err == nil {
		writeError(w, http.StatusConflict, "conflict", fmt.Sprintf("task %q already exists", t.Slug))
		return
	}
	s.writeTask(w, *t, http.StatusCreated)
}

// updateTask handles PUT /v1/tasks/{slug}: the body's slug must match the
// path (rename is copy+delete in the editor, SPEC-13 §1).
func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	// The path task must exist before anything else: a missing target is a
	// 404 even when the body is malformed or renames something.
	if _, err := s.deps.Tasks.Get(slug); err != nil {
		writeTaskError(w, err, slug)
		return
	}
	yaml, ok := readTaskBody(w, r)
	if !ok {
		return
	}
	t, ok := parseTaskBody(w, yaml)
	if !ok {
		return
	}
	if t.Slug != slug {
		writeTaskValidation(w, []string{
			fmt.Sprintf("slug %q does not match task path %q", t.Slug, slug),
		})
		return
	}
	s.writeTask(w, *t, http.StatusOK)
}

// writeTask persists the canonical file, reindexes, and answers with the
// resulting detail envelope.
func (s *Server) writeTask(w http.ResponseWriter, t task.Task, status int) {
	if err := s.deps.TaskFiles.Write(t); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "write task: "+err.Error())
		return
	}
	if err := s.deps.SyncTasks(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "reindex: "+err.Error())
		return
	}
	detail, err := s.taskDetail(t.Slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, status, map[string]TaskDetail{"task": detail})
}

// deleteTask handles DELETE /v1/tasks/{slug}: schedules block deletion with a
// 409 naming them; run history is never removed (SPEC-13 §1).
func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if _, err := s.deps.Tasks.Get(slug); err != nil {
		writeTaskError(w, err, slug)
		return
	}
	schedules, err := s.deps.Schedules.ListByTask(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if len(schedules) > 0 {
		refs := make([]string, 0, len(schedules))
		for _, sch := range schedules {
			refs = append(refs, sch.Slug)
		}
		writeError(w, http.StatusConflict, "conflict",
			fmt.Sprintf("cannot delete task %q: schedule(s) still reference it: %s",
				slug, joinRefs(refs)))
		return
	}
	if err := s.deps.TaskFiles.Remove(slug); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeTaskError(w, db.ErrNotFound, slug)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "delete task file: "+err.Error())
		return
	}
	if err := s.deps.Tasks.Delete(slug); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := s.deps.SyncTasks(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	respondOK(w)
}

// handleTaskParse serves POST /v1/tasks/parse: strict parse + validate
// without persisting — the editor rebuilds its Form-tab draft from YAML-tab
// edits through this (SPEC-13 §4: one canonical draft, two views).
func (s *Server) handleTaskParse(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "POST") {
		return
	}
	yaml, ok := readTaskBody(w, r)
	if !ok {
		return
	}
	t, ok := parseTaskBody(w, yaml)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]TaskDetail{"task": TaskDetail{Task: *t}})
}

// handleTaskYAML serves GET /v1/tasks/{slug}/yaml — the raw canonical YAML
// text for the editor (SPEC-13 §2). File paths never leave the daemon.
func (s *Server) handleTaskYAML(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "GET") {
		return
	}
	slug := r.PathValue("slug")
	if _, err := s.deps.Tasks.Get(slug); err != nil {
		writeTaskError(w, err, slug)
		return
	}
	text, err := s.deps.TaskFiles.Load(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "read task: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"yaml": text})
}

// handleTaskValidate serves POST /v1/tasks/validate — the wire-format error
// list for the Form↔YAML handoff (SPEC-13 §4).
func (s *Server) handleTaskValidate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "POST") {
		return
	}
	limitBody(w, r)
	var body struct {
		YAML string `json:"yaml"`
	}
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
	writeJSON(w, http.StatusOK, map[string][]string{"errors": task.ValidateYAML([]byte(body.YAML))})
}

// taskDetail rebuilds the editor payload from the index after a mutation.
func (s *Server) taskDetail(slug string) (TaskDetail, error) {
	row, err := s.deps.Tasks.Get(slug)
	if err != nil {
		return TaskDetail{}, err
	}
	t, err := taskFromIndex(row)
	if err != nil {
		return TaskDetail{}, err
	}
	return TaskDetail{Enabled: row.Enabled, UpdatedAt: row.UpdatedAt, Task: t}, nil
}

// readTaskBody reads the 1 MiB-capped raw YAML body.
func readTaskBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	limitBody(w, r)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large",
				"request body exceeds 1 MiB")
			return nil, false
		}
		writeError(w, http.StatusBadRequest, "bad_request", "read body: "+err.Error())
		return nil, false
	}
	return data, true
}

// parseTaskBody runs the strict parse and answers 422 with the JSON-encoded
// problem list on failure (SPEC-13 §1).
func parseTaskBody(w http.ResponseWriter, yaml []byte) (*task.Task, bool) {
	t, err := task.Parse(yaml)
	if err == nil {
		return &t, true
	}
	writeTaskValidation(w, task.ValidateYAML(yaml))
	return nil, false
}

// writeTaskValidation sends 422 invalid_task with the problem list JSON-
// encoded in the envelope message so the editor can render each line.
func writeTaskValidation(w http.ResponseWriter, errs []string) {
	if len(errs) == 0 {
		errs = []string{"invalid task"}
	}
	payload, _ := json.Marshal(errs)
	writeError(w, http.StatusUnprocessableEntity, "invalid_task", string(payload))
}

// joinRefs renders the blocking-schedule list for the 409 message.
func joinRefs(refs []string) string {
	out := ""
	for i, ref := range refs {
		if i > 0 {
			out += ", "
		}
		out += ref
	}
	return out
}
