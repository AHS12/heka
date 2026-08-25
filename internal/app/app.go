// Package app provides the Wails-bound application struct exposed to the
// frontend (SPEC-01 §3, SPEC-12 §1). These methods are the bridge: the JS
// shell calls them instead of ipc.Client directly because browser JS cannot
// open named pipes (SPEC-12 §1).
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"heka/internal/config"
	"heka/internal/core/task"
	"heka/internal/daemon"
	"heka/internal/ipc"
)

// Info is the static application information shown by the frontend shell.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Daemon  string `json:"daemon"`
}

// HealthDTO is the shell's view of daemon health (SPEC-12 §1). It mirrors
// ipc.Health's JSON shape, minus fields the shell does not consume yet.
type HealthDTO struct {
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Core          string `json:"core"`
	Scheduler     string `json:"scheduler"`
}

// ipcCaller is the slice of the IPC client the shell uses; tests stub it.
type ipcCaller interface {
	Health() (ipc.Health, error)
	ListTasks() ([]ipc.TaskSummary, error)
	GetTask(slug string) (ipc.TaskDetail, error)
	CreateTask(yaml string) (ipc.TaskDetail, error)
	UpdateTask(slug, yaml string) (ipc.TaskDetail, error)
	DeleteTask(slug string) error
	TaskYAML(slug string) (string, error)
	ValidateTaskYAML(yaml string) ([]string, error)
	ParseTask(yaml string) (ipc.TaskDetail, error)
	RunTask(slug, trigger string) (ipc.RunResponse, error)
	SetTaskEnabled(slug string, enabled bool) error
	SetSecret(key, value string) error
	ListSecrets() ([]string, error)
	DeleteSecret(key string) error
}

// ErrDialogCanceled is the sentinel for a user canceling an open/save dialog;
// the frontend API layer maps it to code "canceled" and silently ignores it.
var ErrDialogCanceled = errors.New("dialog canceled")

// ---- Task DTOs (SPEC-13 §2) — owned here so the JS side has one shape.
type TaskSummaryDTO struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Runtime    string `json:"runtime"`
	Enabled    bool   `json:"enabled"`
	UpdatedAt  string `json:"updated_at"`
	LastStatus string `json:"last_status,omitempty"`
	LastRunAt  string `json:"last_run_at,omitempty"`
}

type TaskDTO struct {
	Enabled   bool      `json:"enabled"`
	UpdatedAt string    `json:"updated_at"`
	Task      task.Task `json:"task"`
}

type RunResponseDTO struct {
	GroupID string `json:"group_id"`
	Status  string `json:"status"`
}

// App is the Wails-bound struct. Its exported methods become JS bindings.
type App struct {
	name    string
	version string
	ctx     context.Context

	mu      sync.Mutex
	cfg     *config.Config
	client  ipcCaller
	start   func(config.Config) error     // seam: daemon.Start
	loadCfg func() (config.Config, error) // seam: config.LoadDefault
}

// NewApp creates the application struct with production seams.
func NewApp(name, version string) *App {
	return &App{
		name:    name,
		version: version,
		start:   daemon.Start,
		loadCfg: config.LoadDefault,
	}
}

// Startup is called by Wails when the window starts up.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// AppInfo returns static information about the running application.
func (a *App) AppInfo() Info {
	return Info{
		Name:    a.name,
		Version: a.version,
		Daemon:  "not-running",
	}
}

// Health polls the daemon through the IPC client and maps the result to the
// shell's DTO. Envelope codes are preserved across the Wails bridge (the JS
// API layer parses the "code: message" prefix back into typed errors).
func (a *App) Health() (HealthDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return HealthDTO{}, err
	}
	h, err := client.Health()
	if err != nil {
		return HealthDTO{}, wrapIPCError(err)
	}
	return HealthDTO{
		Version:       h.Version,
		UptimeSeconds: h.UptimeSeconds,
		Core:          h.Core,
		Scheduler:     h.Scheduler,
	}, nil
}

// DaemonStatus reports "running" or "not-running" (SPEC-12 §1): the shell's
// pill and banner branch on exactly these two values.
func (a *App) DaemonStatus() (string, error) {
	client, err := a.cfgClient()
	if err != nil {
		return "", err
	}
	if _, err := client.Health(); err != nil {
		if errors.Is(err, ipc.ErrDaemonNotRunning) {
			return "not-running", nil
		}
		return "", wrapIPCError(err)
	}
	return "running", nil
}

// StartDaemon spawns the daemon detached — the exact code the CLI uses
// (SPEC-08) — so the shell and the shell's own console stay in sync.
func (a *App) StartDaemon() error {
	cfg, err := a.loadCfg()
	if err != nil {
		return err
	}
	return a.start(cfg)
}

// ---- Task surface (SPEC-13 §2): thin passthroughs over the IPC client.

func (a *App) ListTasks() ([]TaskSummaryDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return nil, err
	}
	rows, err := client.ListTasks()
	if err != nil {
		return nil, wrapIPCError(err)
	}
	out := make([]TaskSummaryDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, TaskSummaryDTO{
			Slug: r.Slug, Name: r.Name, Type: r.Type, Runtime: r.Runtime,
			Enabled: r.Enabled, UpdatedAt: r.UpdatedAt,
			LastStatus: r.LastStatus, LastRunAt: r.LastRunAt,
		})
	}
	return out, nil
}

func (a *App) GetTask(slug string) (TaskDTO, error) {
	dto, err := a.taskCall(func(client ipcCaller) (ipc.TaskDetail, error) {
		return client.GetTask(slug)
	})
	return TaskDTO(dto), err
}

func (a *App) CreateTask(yaml string) (TaskDTO, error) {
	dto, err := a.taskCall(func(client ipcCaller) (ipc.TaskDetail, error) {
		return client.CreateTask(yaml)
	})
	return TaskDTO(dto), err
}

func (a *App) UpdateTask(slug, yaml string) (TaskDTO, error) {
	dto, err := a.taskCall(func(client ipcCaller) (ipc.TaskDetail, error) {
		return client.UpdateTask(slug, yaml)
	})
	return TaskDTO(dto), err
}

// taskCall runs one CRUD call against the client and maps envelope codes.
func (a *App) taskCall(fn func(ipcCaller) (ipc.TaskDetail, error)) (TaskDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return TaskDTO{}, err
	}
	dto, err := fn(client)
	if err != nil {
		return TaskDTO{}, wrapIPCError(err)
	}
	return TaskDTO(dto), nil
}

func (a *App) DeleteTask(slug string) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	if err := client.DeleteTask(slug); err != nil {
		return wrapIPCError(err)
	}
	return nil
}

// GetTaskYAML returns the raw canonical YAML for the editor's YAML tab.
func (a *App) GetTaskYAML(slug string) (string, error) {
	client, err := a.cfgClient()
	if err != nil {
		return "", err
	}
	text, err := client.TaskYAML(slug)
	if err != nil {
		return "", wrapIPCError(err)
	}
	return text, nil
}

// ValidateTaskYAML returns the per-problem list for the Form↔YAML handoff.
func (a *App) ValidateTaskYAML(yaml string) ([]string, error) {
	client, err := a.cfgClient()
	if err != nil {
		return nil, err
	}
	errs, err := client.ValidateTaskYAML(yaml)
	if err != nil {
		return nil, wrapIPCError(err)
	}
	return errs, nil
}

// ParseTaskYAML parses YAML into a TaskDTO without persisting — rebuilding
// the Form tab from YAML-tab edits (SPEC-13 §4).
func (a *App) ParseTaskYAML(yaml string) (TaskDTO, error) {
	dto, err := a.taskCall(func(client ipcCaller) (ipc.TaskDetail, error) {
		return client.ParseTask(yaml)
	})
	return TaskDTO(dto), err
}

// RunTask fires a manual run; callers toast the group_id (SPEC-13 §3).
func (a *App) RunTask(slug, trigger string) (RunResponseDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return RunResponseDTO{}, err
	}
	resp, err := client.RunTask(slug, trigger)
	if err != nil {
		return RunResponseDTO{}, wrapIPCError(err)
	}
	return RunResponseDTO{GroupID: resp.GroupID, Status: resp.Status}, nil
}

// SetTaskEnabled flips the index row's enabled state.
func (a *App) SetTaskEnabled(slug string, enabled bool) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	if err := client.SetTaskEnabled(slug, enabled); err != nil {
		return wrapIPCError(err)
	}
	return nil
}

// ---- Secrets vault (SPEC-11 surface for the Settings page): names are all
// the UI ever sees; values go in (encrypted at rest) and never come back out.

func (a *App) ListSecrets() ([]string, error) {
	client, err := a.cfgClient()
	if err != nil {
		return nil, err
	}
	keys, err := client.ListSecrets()
	if err != nil {
		return nil, wrapIPCError(err)
	}
	return keys, nil
}

func (a *App) SetSecret(key, value string) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	if err := client.SetSecret(key, value); err != nil {
		return wrapIPCError(err)
	}
	return nil
}

func (a *App) DeleteSecret(key string) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	if err := client.DeleteSecret(key); err != nil {
		return wrapIPCError(err)
	}
	return nil
}

// PickScriptFile opens a file picker and returns the chosen path for the
// editor's Script field (SPEC-13 §4). Paths cross the bridge as text — the
// daemon still owns file *content* access.
func (a *App) PickScriptFile() (string, error) {
	if a.ctx == nil {
		return "", errors.New("dialog unavailable before startup")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose script",
		Filters: []runtime.FileFilter{
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", ErrDialogCanceled
	}
	return path, nil
}

// PickWorkingDir opens a folder picker for the editor's Working directory.
func (a *App) PickWorkingDir() (string, error) {
	if a.ctx == nil {
		return "", errors.New("dialog unavailable before startup")
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose working directory",
	})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", ErrDialogCanceled
	}
	return dir, nil
}

// ImportTaskFromFile opens a picker, reads the file on the daemon side, and
// creates the task. File paths never reach JS (PRD §2).
func (a *App) ImportTaskFromFile() (TaskDTO, error) {
	if a.ctx == nil {
		return TaskDTO{}, errors.New("dialog unavailable before startup")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import task YAML",
		Filters: []runtime.FileFilter{
			{DisplayName: "YAML files", Pattern: "*.yaml"},
		},
	})
	if err != nil {
		return TaskDTO{}, err
	}
	if path == "" {
		return TaskDTO{}, ErrDialogCanceled
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("read picked file: %w", err)
	}
	return a.CreateTask(string(data))
}

// ExportTaskYAML saves the task's canonical YAML to a user-picked path
// (daemon-side write of a user-chosen path, SPEC-13 §5).
func (a *App) ExportTaskYAML(slug string) error {
	if a.ctx == nil {
		return errors.New("dialog unavailable before startup")
	}
	if _, err := a.cfgClient(); err != nil {
		return err
	}
	text, err := a.GetTaskYAML(slug)
	if err != nil {
		return err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export task",
		DefaultFilename: slug + ".yaml",
		Filters:         []runtime.FileFilter{{DisplayName: "YAML files", Pattern: "*.yaml"}},
	})
	if err != nil {
		return err
	}
	if path == "" {
		return ErrDialogCanceled
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return fmt.Errorf("write export: %w", err)
	}
	return nil
}

// cfgClient lazily resolves the configuration and IPC client. Cached so
// repeated 5 s polls never re-read config.
func (a *App) cfgClient() (ipcCaller, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil {
		return a.client, nil
	}
	cfg, err := a.loadCfg()
	if err != nil {
		return nil, err
	}
	a.cfg = &cfg
	a.client = ipc.NewClient(cfg)
	return a.client, nil
}

// wrapIPCError keeps the envelope's code across the Wails bridge. Wails only
// sends the error string to JS, so the code is folded in as "code: message"
// — parseable by the frontend API layer (SPEC-12 §3).
func wrapIPCError(err error) error {
	if err == nil {
		return nil
	}
	var e *ipc.Error
	if errors.As(err, &e) {
		return fmt.Errorf("%s: %s", e.Code, e.Message)
	}
	return err
}
