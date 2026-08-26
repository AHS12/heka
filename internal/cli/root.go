// Package cli implements the Heka command-line client (SPEC-08). The CLI is
// a pure client: every command goes through the IPC API — it never executes
// tasks or touches the database itself (PRD §13).
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"heka/internal/config"
	"heka/internal/daemon"
	"heka/internal/ipc"
)

// APIClient is the consumer-side IPC seam (SPEC-08 §4) so command tests use a
// stub instead of a live daemon. Production value: ipc.NewClient(cfg).
type APIClient interface {
	ListTasks() ([]ipc.TaskSummary, error)
	GetTask(slug string) (ipc.TaskDetail, error)
	RunTask(slug, trigger string) (ipc.RunResponse, error)
	Enable(slug string) error
	Disable(slug string) error
	TaskRuns(slug string, limit int) ([]ipc.Run, error)
	Run(runID string) (ipc.Run, error)
}

// App wires one command invocation. stdout/stderr are injectable for tests.
type App struct {
	cfg         config.Config
	client      APIClient
	json        bool
	startDaemon func(config.Config) error // seam: daemon.Start

	stdout io.Writer
	stderr io.Writer
	root   *cobra.Command
}

// Run executes CLI args against the default configuration (used by main).
func Run(args []string) error {
	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}
	return NewApp(cfg, ipc.NewClient(cfg)).RunErr(args)
}

// RunWithVersion is like Run but sets the version string for --version.
func RunWithVersion(args []string, version string) error {
	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}
	a := NewApp(cfg, ipc.NewClient(cfg))
	a.root.Version = version
	return a.RunErr(args)
}

// RunErr executes the tree and reports command errors in the current output
// mode. Package Run and the tests both go through here, so error rendering is
// covered either way.
func (a *App) RunErr(args []string) error {
	if err := a.Execute(args); err != nil {
		a.reportError(err)
		return err
	}
	return nil
}

// NewApp builds the command tree. Exporting it keeps tests honest about
// construction.
func NewApp(cfg config.Config, client APIClient) *App {
	a := &App{cfg: cfg, client: client, stdout: os.Stdout, stderr: os.Stderr, startDaemon: daemon.Start}
	root := &cobra.Command{
		Use:           "heka",
		Short:         "Heka — a local task runner and scheduler",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("specify a command (try 'heka --help')")
		},
	}
	root.PersistentFlags().BoolVar(&a.json, "json", false, "emit machine-readable JSON (for agents)")
	root.AddCommand(
		a.listCmd(), a.runCmd(), a.statusCmd(), a.logsCmd(),
		a.enableCmd(), a.disableCmd(), a.schedulesCmd(),
		a.daemonCmd(),
	)
	a.root = root
	return a
}

// Execute runs the tree with the given args.
func (a *App) Execute(args []string) error {
	a.root.SetArgs(args)
	return a.root.Execute()
}

// reportError renders an error in the current output mode: JSON errors go to
// stdout (agents parse one stream), human errors to stderr (SPEC-08 §3).
func (a *App) reportError(err error) {
	code, message := "internal", err.Error()
	var ipcErr *ipc.Error
	if errors.As(err, &ipcErr) {
		code, message = ipcErr.Code, ipcErr.Message
	} else if errors.Is(err, ipc.ErrDaemonNotRunning) {
		code = "daemon_not_running"
		message = "heka daemon is not running."
	}
	if a.json {
		a.printJSON(map[string]any{"error": map[string]string{"code": code, "message": message}})
		return
	}
	fmt.Fprintln(a.stderr, "heka:", message)
	if code == "daemon_not_running" {
		fmt.Fprintln(a.stderr, "Start the daemon:")
		fmt.Fprintln(a.stderr, "  heka daemon start")
	}
}

// printJSON writes a JSON value to stdout (the stable agent surface).
func (a *App) printJSON(v any) {
	_ = json.NewEncoder(a.stdout).Encode(v)
}
