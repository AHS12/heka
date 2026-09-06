package cli

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/spf13/cobra"

	"heka/internal/app"
)

// versionRE is the semver shape accepted by `heka dev update-from`.
var versionRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// devCmd implements `heka dev …` — development helpers that drop a one-shot
// trigger file for the GUI (What's New dialog, onboarding tour). Hidden from
// --help but kept in release builds so installed copies can be QA'd. Pure
// local file writes: the daemon is not involved.
func (a *App) devCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "dev",
		Short:  "Development helpers for the GUI",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("specify: heka dev whats-new|tour|reset|update-from")
		},
	}

	write := func(t app.DevTrigger) error {
		if err := app.WriteDevTrigger(app.OnboardingTriggerPath(a.cfg.DataDir), t); err != nil {
			return err
		}
		if a.json {
			a.printJSON(map[string]any{"ok": true, "trigger": t.Trigger, "version": t.Version})
			return nil
		}
		fmt.Fprintf(a.stdout, "Triggered %s — the running GUI picks it up within a few seconds (otherwise at next launch).\n", t.Trigger)
		return nil
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "whats-new",
			Short: "Show the What's New dialog in the GUI",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return write(app.DevTrigger{Trigger: "whats-new"})
			},
		},
		&cobra.Command{
			Use:   "tour",
			Short: "Start the guided app tour in the GUI",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return write(app.DevTrigger{Trigger: "tour"})
			},
		},
		&cobra.Command{
			Use:   "reset",
			Short: "Clear the GUI's tour and What's New state (fresh-install experience)",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return write(app.DevTrigger{Trigger: "reset"})
			},
		},
		&cobra.Command{
			Use:   "update-from <version>",
			Short: "Pretend the GUI last ran <version> (test the update flow)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if !versionRE.MatchString(args[0]) {
					return fmt.Errorf("invalid version %q (want e.g. 0.8.1)", args[0])
				}
				return write(app.DevTrigger{Trigger: "update-from", Version: args[0]})
			},
		},
	)
	return cmd
}
