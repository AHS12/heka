package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"heka/internal/daemon"
	"heka/internal/ipc"
	"heka/internal/osapp"
)

func (a *App) daemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the background daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("specify: heka daemon start|stop|status")
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "start",
			Short: "Start the daemon in the background",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				// Diagnostic: write a marker so we can tell whether "daemon start"
				// is actually reached when launched from the registry at boot.
				if exe, err := os.Executable(); err == nil {
					marker := filepath.Join(filepath.Dir(exe), "daemon-start-cli.trace")
					_ = os.WriteFile(marker, []byte(fmt.Sprintf("daemon start called at %s from pid %d exe %s\n",
						time.Now().Format(time.RFC3339Nano), os.Getpid(), exe)), 0o600)
				}
				if err := a.startDaemon(a.cfg); err != nil {
					return err
				}
				if a.json {
					a.printJSON(map[string]any{"ok": true, "action": "daemon_start"})
					return nil
				}
				fmt.Fprintln(a.stdout, "heka daemon started")
				return nil
			},
		},
		&cobra.Command{
			Use:   "stop",
			Short: "Stop the daemon gracefully",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := daemon.Stop(a.cfg); err != nil {
					return err
				}
				if a.json {
					a.printJSON(map[string]any{"ok": true, "action": "daemon_stop"})
					return nil
				}
				fmt.Fprintln(a.stdout, "heka daemon stopped")
				return nil
			},
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show daemon health",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				h, err := daemon.Status(a.cfg)
				if err != nil {
					return ipc.ErrDaemonNotRunning
				}
				if a.json {
					a.printJSON(map[string]any{"daemon": "running", "health": h})
					return nil
				}
				fmt.Fprintf(a.stdout, "Heka daemon: running\n")
				fmt.Fprintf(a.stdout, "version:    %s\n", h.Version)
				fmt.Fprintf(a.stdout, "uptime:     %s\n", humanDuration(h.UptimeSeconds*1000))
				fmt.Fprintf(a.stdout, "core:       %s\n", h.Core)
				fmt.Fprintf(a.stdout, "scheduler:  %s\n", h.Scheduler)
				if !h.LastHeartbeat.IsZero() {
					fmt.Fprintf(a.stdout, "heartbeat:  %s ago\n",
						time.Since(h.LastHeartbeat).Truncate(time.Second))
				}
				return nil
			},
		},
	)
	cmd.AddCommand(a.watchCmd(), a.watchdogCmd(), a.startupCmd())
	return cmd
}

// watchCmd implements `heka daemon watch [--once]` (SPEC-10 §1): the command
// the OS entries call, and a manual foreground loop.
func (a *App) watchCmd() *cobra.Command {
	var once bool
	w := &cobra.Command{
		Use:   "watch",
		Short: "Check the daemon and restart it if down (watchdog)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if once {
				return osapp.WatchOnce(a.cfg, a.startDaemon)
			}
			// Foreground loop for manual testing / container-style runs.
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				if err := osapp.WatchOnce(a.cfg, a.startDaemon); err != nil {
					if a.json {
						a.printJSON(map[string]any{"ok": false, "error": err.Error()})
					} else {
						fmt.Fprintf(a.stderr, "heka: watch: %v\n", err)
					}
				}
				<-ticker.C
			}
		},
	}
	w.Flags().BoolVar(&once, "once", false, "run a single check, then exit")
	return w
}

// watchdogCmd manages the OS-level watchdog entry (SPEC-10 §3).
func (a *App) watchdogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watchdog",
		Short: "Manage the OS-level watchdog entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("specify: heka daemon watchdog install|uninstall|status")
		},
	}

	var intervalMinutes int
	install := &cobra.Command{
		Use:   "install",
		Short: "Register the watchdog with the operating system",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			interval := time.Duration(intervalMinutes) * time.Minute
			if interval <= 0 {
				interval = osapp.DefaultWatchdogInterval
			}
			if err := osapp.NewInstaller().Install(interval, exe); err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{
					"ok": true, "action": "watchdog_install",
					"interval_minutes": int(interval.Minutes()),
				})
				return nil
			}
			fmt.Fprintf(a.stdout, "watchdog installed (every %dm)\n", int(interval.Minutes()))
			return nil
		},
	}
	install.Flags().IntVar(&intervalMinutes, "interval", 0, "check interval in minutes (default 5)")

	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the watchdog OS entry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := osapp.NewInstaller().Uninstall(); err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{"ok": true, "action": "watchdog_uninstall"})
				return nil
			}
			fmt.Fprintln(a.stdout, "watchdog removed")
			return nil
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Show watchdog installation state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, interval, err := osapp.NewInstaller().Status()
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{
					"installed": installed,
					"interval_minutes": func() int {
						if installed {
							return int(interval.Minutes())
						}
						return 0
					}(),
				})
				return nil
			}
			if installed {
				fmt.Fprintf(a.stdout, "watchdog: installed (every %dm)\n", int(interval.Minutes()))
			} else {
				fmt.Fprintln(a.stdout, "watchdog: not installed")
			}
			return nil
		},
	}
	cmd.AddCommand(install, uninstall, status)
	return cmd
}

// startupCmd manages OS-level startup registration (SPEC-15 §3).
func (a *App) startupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "startup",
		Short: "Manage OS startup registration (user-level, no admin)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("specify: heka daemon startup on|off|status")
		},
	}

	on := &cobra.Command{
		Use:   "on",
		Short: "Register the daemon to start with the OS",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if err := osapp.NewStartupRegistrar().Enable(exe); err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{"ok": true, "action": "startup_on"})
				return nil
			}
			fmt.Fprintln(a.stdout, "startup registration enabled")
			return nil
		},
	}

	off := &cobra.Command{
		Use:   "off",
		Short: "Remove OS startup registration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := osapp.NewStartupRegistrar().Disable(); err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{"ok": true, "action": "startup_off"})
				return nil
			}
			fmt.Fprintln(a.stdout, "startup registration removed")
			return nil
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Show OS startup registration state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			enabled, err := osapp.NewStartupRegistrar().Enabled()
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{"enabled": enabled})
				return nil
			}
			if enabled {
				fmt.Fprintln(a.stdout, "startup: enabled")
			} else {
				fmt.Fprintln(a.stdout, "startup: not enabled")
			}
			return nil
		},
	}

	cmd.AddCommand(on, off, status)
	return cmd
}
