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
	"os/exec"
	stdruntime "runtime"
	"strings"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"heka/internal/config"
	"heka/internal/core/task"
	"heka/internal/daemon"
	"heka/internal/ipc"
	"heka/internal/osapp"
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
	ListSchedulesFiltered(kind string) ([]ipc.Schedule, error)
	CreateSchedule(s ipc.Schedule) (ipc.Schedule, error)
	UpdateSchedule(id string, s ipc.Schedule) (ipc.Schedule, error)
	DeleteSchedule(id string) error
	EnableSchedule(id string) error
	DisableSchedule(id string) error
	ListRuns(f ipc.RunFilters) (ipc.RunListResult, error)
	Run(runID string) (ipc.Run, error)
	Cancel(slug string) error
	SystemLog(limit int) ([]ipc.DaemonLog, error)
	PauseScheduler() error
	ResumeScheduler() error
	ReconcileSchedules() error
	Stats() (ipc.Stats, error)
	GetSettings() (ipc.SettingsDTO, error)
	UpdateSettings(s ipc.SettingsDTO) error
	PreviewSound(preset string) error
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

// ScheduleDTO is the Wails bridge view of a schedule (SPEC-14 §2).
type ScheduleDTO = ipc.Schedule

// RunDTO is the Wails bridge view of a run (SPEC-14 §4).
type RunDTO = ipc.Run

// RunListResultDTO is the paginated runs response (SPEC-14 §1).
type RunListResultDTO = ipc.RunListResult

// App is the Wails-bound struct. Its exported methods become JS bindings.
type App struct {
	name      string
	version   string
	ctx       context.Context
	statePath string // window geometry file; empty disables persistence

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
	a.fitWindowToScreen()
	a.restoreWindow()
}

// taskbarMargin is the vertical slack reserved for the OS taskbar/dock when
// fitting the window to the screen (Screen.Size covers the full display).
const taskbarMargin = 48

// fitWindowToScreen shrinks the window — preserving its aspect ratio — when
// the requested size does not fit the display it opens on (e.g. the
// 1310×940 default on a 1366×768 laptop). Screen sizes from Wails are in
// the same logical pixel space as WindowGetSize/WindowSetSize, so no DPI
// conversion is needed. macOS and most Linux window managers already clamp
// oversized windows themselves; the check is cheap and harmless there.
func (a *App) fitWindowToScreen() {
	if a.ctx == nil {
		return
	}
	screens, err := wruntime.ScreenGetAll(a.ctx)
	if err != nil || len(screens) == 0 {
		return
	}
	screen := screens[0]
	for i := range screens {
		if screens[i].IsCurrent {
			screen = screens[i]
			break
		}
	}
	w, h := wruntime.WindowGetSize(a.ctx)
	fitW, fitH := fitWithin(w, h, screen.Size.Width, screen.Size.Height-taskbarMargin)
	if fitW != w || fitH != h {
		wruntime.WindowSetSize(a.ctx, fitW, fitH)
	}
}

// SetWindowStatePath points the app at the window-geometry file used to
// restore the last size/position on launch. Must be called before wails.Run;
// empty disables persistence.
func (a *App) SetWindowStatePath(path string) {
	a.statePath = path
}

// restoreWindow re-applies the saved window geometry. Wails v2 options
// cannot express an X/Y position, so the position (and the maximized flag)
// are restored here; the size was already applied through options.App.
func (a *App) restoreWindow() {
	if a.ctx == nil || a.statePath == "" {
		return
	}
	ws, err := LoadWindowState(a.statePath)
	if err != nil {
		return
	}
	if offScreen(ws.X, ws.Y, ws.Width, ws.Height) {
		wruntime.WindowCenter(a.ctx)
	} else {
		wruntime.WindowSetPosition(a.ctx, ws.X, ws.Y)
	}
	if ws.Maximized {
		wruntime.WindowMaximise(a.ctx)
	}
}

// BeforeClose is called by Wails before the window closes. Returning false
// lets the close proceed after the geometry snapshot is written.
func (a *App) BeforeClose(ctx context.Context) bool {
	if a.statePath != "" {
		width, height := wruntime.WindowGetSize(ctx)
		x, y := wruntime.WindowGetPosition(ctx)
		_ = SaveWindowState(a.statePath, WindowState{
			X:         x,
			Y:         y,
			Width:     width,
			Height:    height,
			Maximized: wruntime.WindowIsMaximised(ctx),
		})
	}
	return false
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
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose script",
		Filters: []wruntime.FileFilter{
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
	dir, err := wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
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
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Import task YAML",
		Filters: []wruntime.FileFilter{
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
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Export task",
		DefaultFilename: slug + ".yaml",
		Filters:         []wruntime.FileFilter{{DisplayName: "YAML files", Pattern: "*.yaml"}},
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

// ---- Schedules surface (SPEC-14 §2): CRUD + filter.

func (a *App) ListSchedules(kind string) ([]ScheduleDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return nil, err
	}
	rows, err := client.ListSchedulesFiltered(kind)
	if err != nil {
		return nil, wrapIPCError(err)
	}
	return rows, nil
}

func (a *App) CreateSchedule(slug, taskSlug, kind, cron, runAt, missedPolicy string) (ScheduleDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return ScheduleDTO{}, err
	}
	s, err := client.CreateSchedule(ipc.Schedule{
		Slug:         slug,
		TaskSlug:     taskSlug,
		Kind:         kind,
		Cron:         cron,
		RunAt:        runAt,
		MissedPolicy: missedPolicy,
	})
	if err != nil {
		return ScheduleDTO{}, wrapIPCError(err)
	}
	return s, nil
}

func (a *App) UpdateSchedule(id, slug, taskSlug, kind, cron, runAt, missedPolicy string) (ScheduleDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return ScheduleDTO{}, err
	}
	s, err := client.UpdateSchedule(id, ipc.Schedule{
		Slug:         slug,
		TaskSlug:     taskSlug,
		Kind:         kind,
		Cron:         cron,
		RunAt:        runAt,
		MissedPolicy: missedPolicy,
	})
	if err != nil {
		return ScheduleDTO{}, wrapIPCError(err)
	}
	return s, nil
}

func (a *App) DeleteSchedule(id string) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	if err := client.DeleteSchedule(id); err != nil {
		return wrapIPCError(err)
	}
	return nil
}

func (a *App) EnableSchedule(id string) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	if err := client.EnableSchedule(id); err != nil {
		return wrapIPCError(err)
	}
	return nil
}

func (a *App) DisableSchedule(id string) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	if err := client.DisableSchedule(id); err != nil {
		return wrapIPCError(err)
	}
	return nil
}

// ---- Runs surface (SPEC-14 §1, §4): global listing + detail.

func (a *App) ListRuns(task, status, from, to, q, cursor, order string, limit int) (RunListResultDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return RunListResultDTO{}, err
	}
	result, err := client.ListRuns(ipc.RunFilters{
		Task: task, Status: status, From: from, To: to, Q: q,
		Cursor: cursor, Limit: limit, Order: order,
	})
	if err != nil {
		return RunListResultDTO{}, wrapIPCError(err)
	}
	return result, nil
}

func (a *App) GetRun(runID string) (RunDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return RunDTO{}, err
	}
	run, err := client.Run(runID)
	if err != nil {
		return RunDTO{}, wrapIPCError(err)
	}
	return run, nil
}

func (a *App) CancelRun(slug string) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	if err := client.Cancel(slug); err != nil {
		return wrapIPCError(err)
	}
	return nil
}

// ---- Daemon event log (Logs → System view).

// DaemonLogDTO is the Wails bridge view of a daemon log entry.
type DaemonLogDTO = ipc.DaemonLog

// ListSystemLog returns the daemon's own event log (scheduler reconcile,
// lifecycle, wake detection), newest first.
func (a *App) ListSystemLog(limit int) ([]DaemonLogDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return nil, err
	}
	logs, err := client.SystemLog(limit)
	if err != nil {
		return nil, wrapIPCError(err)
	}
	return logs, nil
}

func (a *App) ListTasksForSchedules() ([]TaskSummaryDTO, error) {
	return a.ListTasks()
}

// ---- Startup / Watchdog passthroughs (SPEC-15 §4) — local OS operations,
// not IPC. The daemon is not involved; these talk to the OS directly.

// StartupEnabled reports whether the daemon is registered to start with the OS.
func (a *App) StartupEnabled() (bool, error) {
	return osapp.NewStartupRegistrar().Enabled()
}

// StartupSet enables or disables OS-level startup registration for the daemon.
func (a *App) StartupSet(on bool) error {
	if on {
		exe, err := osapp.GUIExecutable()
		if err != nil {
			return err
		}
		return osapp.NewStartupRegistrar().Enable(exe)
	}
	return osapp.NewStartupRegistrar().Disable()
}

// WatchdogStatusDTO is the shell's view of the OS watchdog (SPEC-10 §3):
// installed plus the check interval, so the Settings toggle can render
// "Checks every Nm" without guessing.
type WatchdogStatusDTO struct {
	Installed       bool  `json:"installed"`
	IntervalMinutes int64 `json:"interval_minutes"`
}

// WatchdogEnabled reports whether the OS-level watchdog is installed.
func (a *App) WatchdogEnabled() (WatchdogStatusDTO, error) {
	installed, interval, err := osapp.NewInstaller().Status()
	if err != nil {
		return WatchdogStatusDTO{}, err
	}
	// A platform Status() can report 0 while installed (unparseable OS
	// output). Never surface 0m to the Settings page — fall back to the
	// default install interval.
	intervalMinutes := int64(interval.Minutes())
	if installed && intervalMinutes <= 0 {
		intervalMinutes = int64(osapp.DefaultWatchdogInterval.Minutes())
	}
	return WatchdogStatusDTO{
		Installed:       installed,
		IntervalMinutes: intervalMinutes,
	}, nil
}

// WatchdogSet installs or uninstalls the OS-level watchdog.
func (a *App) WatchdogSet(on bool) error {
	if on {
		exe, err := osapp.GUIExecutable()
		if err != nil {
			return err
		}
		return osapp.NewInstaller().Install(osapp.DefaultWatchdogInterval, exe)
	}
	return osapp.NewInstaller().Uninstall()
}

// PauseScheduler pauses the scheduler (SPEC-15 §2).
func (a *App) PauseScheduler() error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	return wrapIPCError(client.PauseScheduler())
}

// ResumeScheduler resumes the scheduler (SPEC-15 §2).
func (a *App) ResumeScheduler() error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	return wrapIPCError(client.ResumeScheduler())
}

// ReconcileSchedules asks the daemon to fire any missed recurring schedule
// runs immediately. The daemon's periodic watchdog does the same every 10
// minutes; this is the manual override.
func (a *App) ReconcileSchedules() error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	return wrapIPCError(client.ReconcileSchedules())
}

// Stats returns dashboard statistics (SPEC-16 §1).
func (a *App) Stats() (ipc.Stats, error) {
	client, err := a.cfgClient()
	if err != nil {
		return ipc.Stats{}, err
	}
	s, err := client.Stats()
	if err != nil {
		return ipc.Stats{}, wrapIPCError(err)
	}
	return s, nil
}

// GetSettings returns daemon settings (SPEC-16 §2).
func (a *App) GetSettings() (ipc.SettingsDTO, error) {
	client, err := a.cfgClient()
	if err != nil {
		return ipc.SettingsDTO{}, err
	}
	s, err := client.GetSettings()
	if err != nil {
		return ipc.SettingsDTO{}, wrapIPCError(err)
	}
	return s, nil
}

// UpdateSettings persists daemon settings (SPEC-16 §2).
func (a *App) UpdateSettings(s ipc.SettingsDTO) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	return wrapIPCError(client.UpdateSettings(s))
}

// PreviewSound plays a sound preview (SPEC-16 §2).
func (a *App) PreviewSound(preset string) error {
	client, err := a.cfgClient()
	if err != nil {
		return err
	}
	return wrapIPCError(client.PreviewSound(preset))
}

// OpenURL opens an external http(s) link in the user's default browser —
// never the app webview (SPEC-12 §1).
func (a *App) OpenURL(url string) error {
	if a.ctx == nil {
		return errors.New("browser unavailable before startup")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("only http(s) URLs can be opened")
	}
	wruntime.BrowserOpenURL(a.ctx, url)
	return nil
}

// OpenDataDir opens the data directory in the OS file manager (SPEC-16 §2).
func (a *App) OpenDataDir() error {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	if cfg == nil {
		c, err := a.loadCfg()
		if err != nil {
			return err
		}
		cfg = &c
	}
	dir := cfg.DataDir
	switch stdruntime.GOOS {
	case "windows":
		return exec.Command("explorer", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

// DataDir returns the data directory path for display in Settings.
func (a *App) DataDir() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg != nil {
		return a.cfg.DataDir
	}
	cfg, err := a.loadCfg()
	if err != nil {
		return ""
	}
	return cfg.DataDir
}

// TasksDir returns the tasks directory path for display in Settings.
func (a *App) TasksDir() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg != nil {
		return a.cfg.TasksDir
	}
	cfg, err := a.loadCfg()
	if err != nil {
		return ""
	}
	return cfg.TasksDir
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
