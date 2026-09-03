package ipc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

// secretKeyRe enforces the env-var-name rule (SPEC-11 §2): secrets resolve
// into process environment, so keys must be valid identifiers.
var secretKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// handleSecrets serves GET /v1/secrets — keys only, values never returned
// (SPEC-11 §2, PRD §14).
func (s *Server) handleSecrets(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "GET") {
		return
	}
	keys, err := s.deps.Secrets.Keys()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"keys": keys})
}

// handleSecret serves PUT/DELETE /v1/secrets/{key}.
func (s *Server) handleSecret(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !secretKeyRe.MatchString(key) {
		writeError(w, http.StatusBadRequest, "bad_request",
			fmt.Sprintf("%q is not a valid secret key", key))
		return
	}
	switch r.Method {
	case "PUT":
		s.setSecret(w, r, key)
	case "DELETE":
		// Idempotent: missing keys still return ok (SPEC-11 §2).
		if err := s.deps.Secrets.Delete(key); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		respondOK(w)
	default:
		requireMethod(w, r, "PUT")
	}
}

// handleSecretsUsage serves GET /v1/secrets/usage — for each vault key, the
// task slugs that reference it. Keys only; values never leave the daemon.
func (s *Server) handleSecretsUsage(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "GET") {
		return
	}
	if s.deps.SecretsUsage == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "usage map not available")
		return
	}
	usage, err := s.deps.SecretsUsage()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (s *Server) setSecret(w http.ResponseWriter, r *http.Request, key string) {
	limitBody(w, r)
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if err := s.deps.Secrets.Set(key, body.Value); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	respondOK(w)
}
