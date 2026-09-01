package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"heka/internal/ipc"
)

func (a *App) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tasks, err := a.client.ListTasks()
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(tasks)
				return nil
			}
			w := tabwriter.NewWriter(a.stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "SLUG\tNAME\tTYPE\tRUNTIME\tENABLED")
			for _, t := range tasks {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n", t.Slug, t.Name, t.Type, t.Runtime, yesNo(t.Enabled))
			}
			return w.Flush()
		},
	}
}

func (a *App) runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <slug>",
		Short: "Run a task now",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			resp, err := a.client.RunTask(slug, "cli")
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{
					"success": true,
					"slug":    slug,
					"run_id":  resp.GroupID, // client-facing logical run (SPEC-08 §3)
					"status":  resp.Status,
				})
				return nil
			}
			fmt.Fprintf(a.stdout, "%s (%s)\n", resp.GroupID, resp.Status)
			return nil
		},
	}
}

func (a *App) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <slug>",
		Short: "Show task status and last run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			detail, err := a.client.GetTask(slug)
			if err != nil {
				return err
			}
			runs, err := a.client.TaskRuns(slug, 1)
			if err != nil {
				return err
			}
			var last *ipc.Run
			if len(runs) > 0 {
				last = &runs[0]
			}
			if a.json {
				m := map[string]any{
					"success": true,
					"slug":    slug,
					"enabled": detail.Enabled,
				}
				if last != nil {
					m["last_run"] = last
				}
				a.printJSON(m)
				return nil
			}
			fmt.Fprintf(a.stdout, "slug:     %s\n", slug)
			fmt.Fprintf(a.stdout, "enabled:  %s\n", yesNo(detail.Enabled))
			if last == nil {
				fmt.Fprintln(a.stdout, "last run: none yet")
				return nil
			}
			fmt.Fprintf(a.stdout, "status:   %s\n", last.Status)
			if last.FinishedAt != "" {
				fmt.Fprintf(a.stdout, "finished: %s\n", last.FinishedAt)
			}
			return nil
		},
	}
}

func (a *App) logsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <slug>",
		Short: "Show the latest run's output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			runs, err := a.client.TaskRuns(slug, 1)
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				return fmt.Errorf("no runs for %q yet", slug)
			}
			run, err := a.client.Run(runs[0].RunID)
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{"success": true, "slug": slug, "run": run})
				return nil
			}
			fmt.Fprintf(a.stdout, "run_id:    %s\n", run.RunID)
			fmt.Fprintf(a.stdout, "status:    %s\n", run.Status)
			fmt.Fprintf(a.stdout, "exit code: %d\n", run.ExitCode)
			fmt.Fprintf(a.stdout, "duration:  %s\n", humanDuration(run.DurationMs))
			fmt.Fprintln(a.stdout)
			fmt.Fprintln(a.stdout, "STDOUT")
			fmt.Fprintln(a.stdout, "------")
			fmt.Fprint(a.stdout, run.Stdout)
			if run.Stderr != "" {
				fmt.Fprintln(a.stdout)
				fmt.Fprintln(a.stdout, "STDERR")
				fmt.Fprintln(a.stdout, "------")
				fmt.Fprint(a.stdout, run.Stderr)
			}
			return nil
		},
	}
}

func (a *App) enableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <slug>",
		Short: "Enable a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if err := a.client.Enable(slug); err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{"success": true, "slug": slug, "enabled": true})
				return nil
			}
			fmt.Fprintf(a.stdout, "heka: %s enabled\n", slug)
			return nil
		},
	}
}

func (a *App) disableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <slug>",
		Short: "Disable a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if err := a.client.Disable(slug); err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{"success": true, "slug": slug, "enabled": false})
				return nil
			}
			fmt.Fprintf(a.stdout, "heka: %s disabled\n", slug)
			return nil
		},
	}
}

func (a *App) schedulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schedules",
		Short: "List schedules (arrives in SPEC-09)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.json {
				a.printJSON(map[string]any{
					"success": false,
					"error":   map[string]string{"code": "not_implemented", "message": "schedules not available yet"},
				})
				return nil
			}
			fmt.Fprintln(a.stdout, "schedules not available yet (arrives in SPEC-09)")
			return nil
		},
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func humanDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}
