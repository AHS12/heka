package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"heka/internal/core/executor"
	"heka/internal/core/task"
	"heka/internal/db"
)

// Deps is everything the server needs. The daemon composes it from real
// objects; tests inject fakes (SPEC-07 §7).
type Deps struct {
	Health        func() Health
	Tasks         *db.TaskStore
	Runs          *db.RunStore
	Schedules     *db.ScheduleStore
	SyncSchedules func() error // scheduler registry refresh after mutations
	Secrets       *db.SecretStore
	TaskFiles     TaskFilesystem // task YAML read/write (SPEC-13 §1)
	SyncTasks     func() error   // reindex tasks dir after file mutations
	Runner        Runner
	Shutdown      func()            // graceful daemon shutdown (SPEC-06 sequence)
	Pause         func()            // scheduler pause (SPEC-15 §2)
	Resume        func()            // scheduler resume (SPEC-15 §2)
	IsPaused      func() bool       // scheduler paused query (SPEC-15 §2)
}

// TaskFilesystem is the filesystem half of task CRUD (SPEC-13 §1). The daemon
// implements it over the canonical YAML store; handlers own index state and
// conflict rules.
type TaskFilesystem interface {
	Parse(yaml []byte) (task.Task, error) // strict parse + validate
	Write(t task.Task) error              // atomic save at <dir>/<slug>.yaml
	Load(slug string) (string, error)     // raw canonical YAML text
	Remove(slug string) error             // delete the YAML file
}

// Runner is the execution seam (consumer-side interface).
type Runner interface {
	Start(ctx context.Context, t *task.Task, opt executor.Options) (*executor.Handle, error)
	Cancel(slug string) error
}

// Server routes the versioned API. Wire format: JSON everywhere, envelope
// errors, 1 MiB request bodies (SPEC-07 §2).
type Server struct {
	deps Deps
}

func NewServer(deps Deps) *Server {
	return &Server{deps: deps}
}

// Handler returns the fully-wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health + daemon control.
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/daemon/shutdown", s.handleShutdown)

	// Tasks.
	mux.HandleFunc("/v1/tasks", s.handleTasks)
	mux.HandleFunc("/v1/tasks/{slug}", s.handleTask)
	mux.HandleFunc("/v1/tasks/parse", s.handleTaskParse)
	mux.HandleFunc("/v1/tasks/validate", s.handleTaskValidate)
	mux.HandleFunc("/v1/tasks/{slug}/yaml", s.handleTaskYAML)
	mux.HandleFunc("/v1/tasks/{slug}/run", s.handleRun)
	mux.HandleFunc("/v1/tasks/{slug}/enable", s.handleEnable)
	mux.HandleFunc("/v1/tasks/{slug}/disable", s.handleDisable)
	mux.HandleFunc("/v1/tasks/{slug}/cancel", s.handleCancel)
	mux.HandleFunc("/v1/tasks/{slug}/runs", s.handleTaskRuns)

	// Runs.
	mux.HandleFunc("/v1/runs", s.handleRuns)
	mux.HandleFunc("/v1/runs/{run_id}", s.handleRunDetail)

	// Schedules (SPEC-09).
	mux.HandleFunc("/v1/schedules", s.handleSchedules)
	mux.HandleFunc("/v1/schedules/{id}", s.handleSchedule)
	mux.HandleFunc("/v1/schedules/{id}/enable", s.handleScheduleEnable)
	mux.HandleFunc("/v1/schedules/{id}/disable", s.handleScheduleDisable)

	// Secrets (SPEC-11).
	mux.HandleFunc("/v1/secrets", s.handleSecrets)
	mux.HandleFunc("/v1/secrets/{key}", s.handleSecret)

	// Scheduler control (SPEC-15 §2).
	mux.HandleFunc("/v1/scheduler/pause", s.handleSchedulerPause)
	mux.HandleFunc("/v1/scheduler/resume", s.handleSchedulerResume)

	// Unknown routes → JSON 404 envelope.
	mux.HandleFunc("/", notFound)

	return recoverMiddleware(http.Handler(mux))
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "no such route")
}

// recoverMiddleware turns handler panics into 500 envelopes (SPEC-07 §2).
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeError(w, http.StatusInternalServerError, "internal",
					fmt.Sprintf("internal error: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// writeJSON marshals a success body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError sends the SPEC-07 error envelope.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errEnvelope{Error: &Error{Code: code, Message: message}})
}

// respondOK is the canonical {"ok":true}.
func respondOK(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// requireMethod gates a handler; on mismatch it writes the 405 envelope and
// reports false. (Routes are registered without method prefixes so errors
// stay JSON, unlike ServeMux's plain-text 405.)
func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed",
			fmt.Sprintf("%s not allowed for %s (use %s)", r.Method, r.URL.Path, method))
		return false
	}
	return true
}

// limitBody wraps the request body with the 1 MiB cap → 413 on overflow.
func limitBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
}
