// Heka — a local task runner and scheduler for programmers (SPEC-01).
//
// main.go is the single entry point for the whole application. It dispatches
// to one of four modes based on the command-line arguments (master spec §3):
//
//	heka                  → GUI (Wails window)
//	heka gui              → GUI
//	heka daemon           → daemon (foreground)
//	heka daemon <cmd>     → daemon control
//	heka <command>        → CLI client
package main

import (
	"embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"heka/internal/app"
	"heka/internal/cli"
	"heka/internal/config"
)

const (
	appName    = "Heka"
	appVersion = "0.1.0"
)

//go:embed all:frontend/dist
var assets embed.FS

// mode identifies which part of the binary is being invoked.
type mode int

const (
	modeGUI           mode = iota // heka, heka gui
	modeDaemon                    // heka daemon
	modeDaemonControl             // heka daemon <start|stop|status>
	modeCLI                       // heka <command>
	modeHelp                      // heka help | -h | --help
)

// resolveMode maps command-line arguments to a mode. It is a pure function so
// it can be unit-tested without running the application.
func resolveMode(args []string) (mode, string) {
	if len(args) == 0 {
		return modeGUI, ""
	}
	switch args[0] {
	case "gui":
		return modeGUI, ""
	case "daemon":
		if len(args) == 1 {
			return modeDaemon, ""
		}
		return modeDaemonControl, args[1]
	case "help", "-h", "--help":
		return modeHelp, ""
	}
	return modeCLI, args[0]
}

func main() {
	m, arg := resolveMode(os.Args[1:])
	switch m {
	case modeGUI:
		runGUI()
	case modeDaemon:
		runDaemon()
	case modeDaemonControl:
		runDaemonControl(arg)
	case modeCLI:
		cli.Stub(arg)
	case modeHelp:
		usage()
	}
}

// runGUI starts the Wails desktop window (SPEC-01 §3).
func runGUI() {
	a := app.NewApp(appName, appVersion)
	err := wails.Run(&options.App{
		Title:  appName,
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: a.Startup,
		Bind: []interface{}{
			a,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// runDaemon is a stub until SPEC-06. It runs in the foreground, reports the
// resolved configuration (SPEC-02), and blocks until interrupted.
func runDaemon() {
	cfg, err := config.LoadDefault()
	if err != nil {
		fmt.Fprintln(os.Stderr, "heka:", err)
		os.Exit(1)
	}
	fmt.Printf("heka daemon v%s starting (foreground)\n", appVersion)
	fmt.Printf("  data dir:  %s\n", cfg.DataDir)
	fmt.Printf("  tasks dir: %s\n", cfg.TasksDir)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("heka daemon stopped")
}

// runDaemonControl is a stub until SPEC-06.
func runDaemonControl(cmd string) {
	fmt.Fprintf(os.Stderr, "heka: daemon control %q arrives in SPEC-06\n", cmd)
	os.Exit(1)
}

// usage prints the command-line help text.
func usage() {
	fmt.Println(`Heka — a local task runner and scheduler for programmers.

Usage:
  heka                     Start the desktop GUI.
  heka gui                 Start the desktop GUI.
  heka daemon              Run the daemon in the foreground.
  heka daemon <command>    Manage the background daemon (SPEC-06).
  heka <command> [flags]   Run a CLI command (SPEC-08).`)
}
