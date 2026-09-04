package ipc

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Backup surface (Settings → Backup): config CRUD, manual trigger, status,
// history, and destination connectivity tests. Secret material travels only
// through the existing /v1/secrets endpoints — nothing here returns values.

// handleBackupConfig serves GET/PUT /v1/backup/config.
func (s *Server) handleBackupConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.deps.GetBackupConfig == nil {
			writeError(w, http.StatusNotImplemented, "not_implemented", "backup not available")
			return
		}
		writeJSON(w, http.StatusOK, s.deps.GetBackupConfig())
	case http.MethodPut:
		limitBody(w, r)
		var dto BackupConfigDTO
		if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON: "+err.Error())
			return
		}
		if s.deps.UpdateBackupConfig == nil {
			writeError(w, http.StatusNotImplemented, "not_implemented", "backup not available")
			return
		}
		if err := s.deps.UpdateBackupConfig(dto); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_backup_config", err.Error())
			return
		}
		respondOK(w)
	default:
		requireMethod(w, r, http.MethodGet) // writes the 405 envelope
	}
}

// handleBackupRun serves POST /v1/backup/run.
func (s *Server) handleBackupRun(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if s.deps.RunBackup == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "backup not available")
		return
	}
	jobID, err := s.deps.RunBackup()
	if err != nil {
		writeError(w, http.StatusConflict, "backup_busy", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, BackupRunResponse{JobID: jobID})
}

// handleBackupStatus serves GET /v1/backup/status.
func (s *Server) handleBackupStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if s.deps.BackupStatus == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "backup not available")
		return
	}
	writeJSON(w, http.StatusOK, s.deps.BackupStatus())
}

// handleBackupHistory serves GET /v1/backup/history?limit=.
func (s *Server) handleBackupHistory(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if s.deps.BackupHistory == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "backup not available")
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	jobs, err := s.deps.BackupHistory(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": jobs})
}

// handleBackupTest serves POST /v1/backup/test.
func (s *Server) handleBackupTest(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if s.deps.TestBackupDestinations == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "backup not available")
		return
	}
	writeJSON(w, http.StatusOK, s.deps.TestBackupDestinations())
}
