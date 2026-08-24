// Package app provides the Wails-bound application struct exposed to the
// frontend (SPEC-01 §3).
package app

import "context"

// Info is the static application information shown by the frontend shell.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Daemon  string `json:"daemon"`
}

// App is the Wails-bound struct. Its exported methods become JS bindings.
type App struct {
	name    string
	version string
	ctx     context.Context
}

// NewApp creates the application struct.
func NewApp(name, version string) *App {
	return &App{name: name, version: version}
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
