package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"heka/internal/config"
)

// ErrDaemonNotRunning is returned by every client call when the daemon
// endpoint cannot be reached (PRD §3.1). CLI and GUI both branch on it.
var ErrDaemonNotRunning = errors.New("ipc: heka daemon is not running")

// Client is the typed IPC client (SPEC-07 §4). Each call opens a fresh
// pipe/socket connection through a custom dialer.
type Client struct {
	cfg  config.Config
	http *http.Client
}

// NewClient builds a client for the given configuration's endpoint.
func NewClient(cfg config.Config) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return Dial(cfg)
		},
		DisableKeepAlives: true,
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
	}
}

// do executes a request and decodes the JSON body. Envelope errors come back
// as *Error; dial failures as ErrDaemonNotRunning.
func (c *Client) do(method, path string, body any, out any) error {
	data, err := c.raw(method, path, body)
	if err != nil {
		return err
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("ipc: decode %s: %w", path, err)
		}
	}
	return nil
}

// raw executes a request and returns the undecoded body. Used by tests and
// by security assertions (values must never appear on the wire).
func (c *Client) raw(method, path string, body any) ([]byte, error) {
	return c.rawCT(method, path, "application/json", body)
}

// rawCT is raw with an explicit Content-Type (task YAML bodies are not JSON).
func (c *Client) rawCT(method, path, contentType string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		if r, ok := body.(io.Reader); ok {
			reader = r
		} else {
			b, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			reader = bytes.NewReader(b)
		}
	}
	req, err := http.NewRequest(method, "http://heka"+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		if isDialError(err) {
			return nil, ErrDaemonNotRunning
		}
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var env errEnvelope
		if json.Unmarshal(data, &env) == nil && env.Error != nil {
			return nil, env.Error
		}
		return nil, fmt.Errorf("ipc: %s %s returned %s", method, path, resp.Status)
	}
	return data, nil
}

// RawGet returns an undecoded response body (agent/test helper).
func (c *Client) RawGet(path string) ([]byte, error) {
	return c.raw("GET", path, nil)
}

// isDialError maps transport failures onto ErrDaemonNotRunning.
func isDialError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "pipe") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "connect: ") ||
		strings.Contains(msg, "Cannot create a file when that file already exists")
}

// Health hits GET /v1/health.
func (c *Client) Health() (Health, error) {
	var h Health
	err := c.do("GET", "/v1/health", nil, &h)
	return h, err
}

// ListTasks hits GET /v1/tasks.
func (c *Client) ListTasks() ([]TaskSummary, error) {
	var out []TaskSummary
	err := c.do("GET", "/v1/tasks", nil, &out)
	return out, err
}

// GetTask hits GET /v1/tasks/{slug}.
func (c *Client) GetTask(slug string) (TaskDetail, error) {
	var out TaskDetail
	err := c.do("GET", "/v1/tasks/"+slug, nil, &out)
	return out, err
}

// RunTask hits POST /v1/tasks/{slug}/run.
func (c *Client) RunTask(slug, trigger string) (RunResponse, error) {
	var out RunResponse
	body := map[string]string{}
	if trigger != "" {
		body["trigger"] = trigger
	}
	err := c.do("POST", "/v1/tasks/"+slug+"/run", body, &out)
	return out, err
}

// Enable / Disable flip the index state.
func (c *Client) Enable(slug string) error {
	return c.do("POST", "/v1/tasks/"+slug+"/enable", nil, nil)
}
func (c *Client) Disable(slug string) error {
	return c.do("POST", "/v1/tasks/"+slug+"/disable", nil, nil)
}

// SetTaskEnabled is the single-toggle form used by the GUI bridge.
func (c *Client) SetTaskEnabled(slug string, enabled bool) error {
	if enabled {
		return c.Enable(slug)
	}
	return c.Disable(slug)
}

// Task CRUD (SPEC-13 §1): bodies are raw canonical YAML text, not JSON.
func (c *Client) CreateTask(yaml string) (TaskDetail, error) {
	return c.writeTask("POST", "/v1/tasks", yaml)
}

func (c *Client) UpdateTask(slug, yaml string) (TaskDetail, error) {
	return c.writeTask("PUT", "/v1/tasks/"+slug, yaml)
}

func (c *Client) writeTask(method, path, yaml string) (TaskDetail, error) {
	data, err := c.rawCT(method, path, "text/yaml", strings.NewReader(yaml))
	if err != nil {
		// On 422 the envelope message is the JSON-encoded problem list the
		// editor renders line by line (SPEC-13 §1).
		return TaskDetail{}, err
	}
	var out struct {
		Task TaskDetail `json:"task"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return TaskDetail{}, fmt.Errorf("ipc: decode %s: %w", path, err)
	}
	return out.Task, nil
}

// DeleteTask removes the task file + index; run history is kept.
func (c *Client) DeleteTask(slug string) error {
	return c.do("DELETE", "/v1/tasks/"+slug, nil, nil)
}

// ParseTask validates and parses YAML without persisting — the Form↔YAML
// tab handoff (SPEC-13 §4).
func (c *Client) ParseTask(yaml string) (TaskDetail, error) {
	return c.writeTask("POST", "/v1/tasks/parse", yaml)
}

// TaskYAML returns the raw canonical YAML for the editor tab.
func (c *Client) TaskYAML(slug string) (string, error) {
	var out struct {
		YAML string `json:"yaml"`
	}
	err := c.do("GET", "/v1/tasks/"+slug+"/yaml", nil, &out)
	return out.YAML, err
}

// ValidateTaskYAML returns the per-problem list without persisting anything.
func (c *Client) ValidateTaskYAML(yaml string) ([]string, error) {
	var out struct {
		Errors []string `json:"errors"`
	}
	err := c.do("POST", "/v1/tasks/validate", map[string]string{"yaml": yaml}, &out)
	if err != nil {
		return nil, err
	}
	return out.Errors, nil
}

// Cancel requests cancellation of the slug's active run.
func (c *Client) Cancel(slug string) error {
	return c.do("POST", "/v1/tasks/"+slug+"/cancel", nil, nil)
}

// TaskRuns hits GET /v1/tasks/{slug}/runs.
func (c *Client) TaskRuns(slug string, limit int) ([]Run, error) {
	var out RunList
	path := "/v1/tasks/" + slug + "/runs"
	if limit > 0 {
		path += "?limit=" + fmt.Sprint(limit)
	}
	err := c.do("GET", path, nil, &out)
	return out.Runs, err
}

// Run hits GET /v1/runs/{run_id}.
func (c *Client) Run(runID string) (Run, error) {
	var out Run
	err := c.do("GET", "/v1/runs/"+runID, nil, &out)
	return out, err
}

// Schedules (SPEC-09).
func (c *Client) ListSchedules() ([]Schedule, error) {
	var out []Schedule
	err := c.do("GET", "/v1/schedules", nil, &out)
	return out, err
}

// CreateSchedule posts a new schedule and returns it.
func (c *Client) CreateSchedule(s Schedule) (Schedule, error) {
	var out Schedule
	body := scheduleDTO{
		Slug: s.Slug, TaskSlug: s.TaskSlug, Kind: s.Kind,
		Cron: s.Cron, RunAt: s.RunAt, MissedPolicy: s.MissedPolicy,
	}
	err := c.do("POST", "/v1/schedules", body, &out)
	return out, err
}

// UpdateSchedule replaces a schedule.
func (c *Client) UpdateSchedule(id string, s Schedule) (Schedule, error) {
	var out Schedule
	body := scheduleDTO{
		Slug: s.Slug, TaskSlug: s.TaskSlug, Kind: s.Kind,
		Cron: s.Cron, RunAt: s.RunAt, MissedPolicy: s.MissedPolicy,
	}
	err := c.do("PUT", "/v1/schedules/"+id, body, &out)
	return out, err
}

// DeleteSchedule removes a schedule (run history is kept).
func (c *Client) DeleteSchedule(id string) error {
	return c.do("DELETE", "/v1/schedules/"+id, nil, nil)
}

// EnableSchedule / DisableSchedule flip a schedule's enabled state.
func (c *Client) EnableSchedule(id string) error {
	return c.do("POST", "/v1/schedules/"+id+"/enable", nil, nil)
}
func (c *Client) DisableSchedule(id string) error {
	return c.do("POST", "/v1/schedules/"+id+"/disable", nil, nil)
}

// Secrets (SPEC-11).
func (c *Client) ListSecrets() ([]string, error) {
	var out struct {
		Keys []string `json:"keys"`
	}
	err := c.do("GET", "/v1/secrets", nil, &out)
	return out.Keys, err
}

// SetSecret upserts a secret value (values are never readable back).
func (c *Client) SetSecret(key, value string) error {
	return c.do("PUT", "/v1/secrets/"+key, map[string]string{"value": value}, nil)
}

// DeleteSecret removes a secret; idempotent.
func (c *Client) DeleteSecret(key string) error {
	return c.do("DELETE", "/v1/secrets/"+key, nil, nil)
}

// Shutdown asks the daemon to stop gracefully.
func (c *Client) Shutdown() error {
	return c.do("POST", "/v1/daemon/shutdown", nil, nil)
}
