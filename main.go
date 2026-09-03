// Heka — a local task runner and scheduler for programmers (SPEC-01).
//
// main.go is the single entry point for the whole application. It dispatches
// to one of three modes based on the command-line arguments (master spec §3,
// SPEC-08 §1):
//
//	heka                  → GUI (Wails window)
//	heka gui              → GUI
//	heka daemon           → daemon (foreground)
//	heka <command>        → CLI client (cobra tree, incl. daemon start|stop|status)
//
// The CLI tree is the help owner; "heka --help" flows through cobra.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"heka/internal/app"
	"heka/internal/cli"
	"heka/internal/config"
	"heka/internal/daemon"
)

const appName = "Heka"

const (
	defaultWinWidth  = 1310
	defaultWinHeight = 940
)

var appVersion = "0.8.0"

//go:embed all:frontend/dist
var assets embed.FS

// mode identifies which part of the binary is being invoked.
type mode int

const (
	modeGUI    mode = iota // heka, heka gui
	modeDaemon             // heka daemon
	modeCLI                // heka <command> | help | --help (cobra owns the rest)
)

// resolveMode maps command-line arguments to a mode. It is a pure function so
// it can be unit-tested without running the application (SPEC-01).
func resolveMode(args []string) mode {
	if len(args) == 0 {
		return modeGUI
	}
	switch args[0] {
	case "gui":
		return modeGUI
	case "daemon":
		if len(args) == 1 {
			return modeDaemon
		}
		return modeCLI // daemon start|stop|status live in the cobra tree
	}
	return modeCLI
}

func main() {
	switch resolveMode(os.Args[1:]) {
	case modeGUI:
		runGUI()
	case modeDaemon:
		runDaemon()
	case modeCLI:
		if err := cli.RunWithVersion(os.Args[1:], appVersion); err != nil {
			os.Exit(1)
		}
	}
}

// runGUI starts the Wails desktop window (SPEC-01 §3). The single-instance
// guard runs first: if another GUI is already open, tell the user and exit
// without creating a second window.
func runGUI() {
	if !app.TryLockGUI() {
		app.ShowGUIAlreadyRunning()
		return
	}
	defer app.UnlockGUI()

	// Restore the last window size up front (no resize flicker); position
	// and the maximized flag are re-applied in App.Startup because Wails v2
	// options carry no X/Y.
	width, height := defaultWinWidth, defaultWinHeight
	a := app.NewApp(appName, appVersion)
	if cfg, err := config.LoadDefault(); err == nil {
		statePath := app.WindowStatePath(cfg.DataDir)
		a.SetWindowStatePath(statePath)
		if ws, err := app.LoadWindowState(statePath); err == nil {
			width, height = ws.Width, ws.Height
		}
	}
	err := wails.Run(&options.App{
		Title:     appName,
		Width:     width,
		Height:    height,
		MinWidth:  app.MinWindowWidth,
		MinHeight: app.MinWindowHeight,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     a.Startup,
		OnBeforeClose: a.BeforeClose,
		Bind: []interface{}{
			a,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// runDaemon runs the daemon in the foreground with a system tray (SPEC-15).
// HEKA_NO_TRAY=1 (tests, headless runs) skips the tray and runs the core only.
func runDaemon() {
	cfg, err := config.LoadDefault()
	if err != nil {
		fmt.Fprintln(os.Stderr, "heka:", err)
		os.Exit(1)
	}

	if os.Getenv("HEKA_NO_TRAY") == "1" {
		if err := daemon.Run(cfg, appVersion); err != nil {
			fmt.Fprintln(os.Stderr, "heka:", err)
			os.Exit(1)
		}
		return
	}
	if err := daemon.RunTray(cfg, appVersion); err != nil {
		fmt.Fprintln(os.Stderr, "heka:", err)
		os.Exit(1)
	}
}
