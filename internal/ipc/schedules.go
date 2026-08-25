package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/oklog/ulid/v2"

	"heka/internal/core/scheduler"
	"heka/internal/db"
)

// scheduleDTO mirrors db.Schedule for the wire.
type scheduleDTO struct {
	Slug         string `json:"slug"`
	TaskSlug     string `json:"task_slug"`
	Kind         string `json:"kind"`
	Cron         string `json:"cron,omitempty"`
	RunAt        string `json:"run_at,omitempty"`
	MissedPolicy string `json:"missed_policy"`
}

func toSchedule(s db.Schedule) Schedule {
	return Schedule{
		ID: s.ID, Slug: s.Slug, TaskSlug: s.TaskSlug, Kind: s.Kind,
		Cron: s.Cron, RunAt: s.RunAt, Enabled: s.Enabled,
		MissedPolicy: s.MissedPolicy,
		NextRunAt:    s.NextRunAt, LastRunAt: s.LastRunAt, LastStatus: s.LastStatus,
	}
}

func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		rows, err := s.deps.Schedules.List()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		out := make([]Schedule, 0, len(rows))
		for _, row := range rows {
			out = append(out, toSchedule(row))
		}
		writeJSON(w, http.StatusOK, out)
	case "POST":
		s.createSchedule(w, r)
	default:
		requireMethod(w, r, "GET")
	}
}

func (s *Server) createSchedule(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)
	var body scheduleDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	sch := db.Schedule{
		ID:           ulid.Make().String(),
		Slug:         body.Slug,
		TaskSlug:     body.TaskSlug,
		Kind:         body.Kind,
		Cron:         body.Cron,
		RunAt:        body.RunAt,
		Timezone:     "local",
		Enabled:      true,
		MissedPolicy: "skip",
		CreatedAt:    db.Now(),
	}
	if body.MissedPolicy != "" {
		sch.MissedPolicy = body.MissedPolicy
	}
	if err := scheduler.ValidateSchedule(sch); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if _, err := s.deps.Tasks.Get(sch.TaskSlug); err != nil {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("task %q not found", sch.TaskSlug))
		return
	}
	if err := s.deps.Schedules.Save(sch); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if s.deps.SyncSchedules != nil {
		if err := s.deps.SyncSchedules(); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, toSchedule(sch))
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	switch r.Method {
	case "DELETE":
		if err := s.deps.Schedules.Delete(id); err != nil {
			writeScheduleError(w, err, id)
			return
		}
		if s.deps.SyncSchedules != nil {
			_ = s.deps.SyncSchedules()
		}
		respondOK(w)
	case "PUT":
		s.updateSchedule(w, r, id)
	default:
		requireMethod(w, r, "PUT")
	}
}

func (s *Server) updateSchedule(w http.ResponseWriter, r *http.Request, id string) {
	limitBody(w, r)
	sch, err := s.deps.Schedules.Get(id)
	if err != nil {
		writeScheduleError(w, err, id)
		return
	}
	var body scheduleDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	sch.Slug, sch.Kind = body.Slug, body.Kind
	sch.TaskSlug, sch.Cron, sch.RunAt = body.TaskSlug, body.Cron, body.RunAt
	if body.MissedPolicy != "" {
		sch.MissedPolicy = body.MissedPolicy
	}
	if err := scheduler.ValidateSchedule(sch); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if _, err := s.deps.Tasks.Get(sch.TaskSlug); err != nil {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("task %q not found", sch.TaskSlug))
		return
	}
	if err := s.deps.Schedules.Save(sch); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if s.deps.SyncSchedules != nil {
		if err := s.deps.SyncSchedules(); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, toSchedule(sch))
}

func (s *Server) handleScheduleEnable(w http.ResponseWriter, r *http.Request) {
	s.toggleSchedule(w, r, true)
}

func (s *Server) handleScheduleDisable(w http.ResponseWriter, r *http.Request) {
	s.toggleSchedule(w, r, false)
}

func (s *Server) toggleSchedule(w http.ResponseWriter, r *http.Request, enabled bool) {
	if !requireMethod(w, r, "POST") {
		return
	}
	id := r.PathValue("id")
	if err := s.deps.Schedules.SetEnabled(id, enabled); err != nil {
		writeScheduleError(w, err, id)
		return
	}
	if s.deps.SyncSchedules != nil {
		_ = s.deps.SyncSchedules()
	}
	respondOK(w)
}

func writeScheduleError(w http.ResponseWriter, err error, id string) {
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("schedule %q not found", id))
		return
	}
	writeError(w, http.StatusInternalServerError, "internal", err.Error())
}
