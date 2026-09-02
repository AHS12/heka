package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

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
	cmd := &cobra.Command{
		Use:   "schedules",
		Short: "Manage schedules",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("specify: heka schedules list|reconcile|missed")
		},
	}
	cmd.AddCommand(a.schedulesListCmd(), a.schedulesReconcileCmd(), a.schedulesMissedCmd())
	return cmd
}

// schedulesListCmd prints a tab-aligned summary of every schedule.
func (a *App) schedulesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List schedules",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := a.client.ListSchedules()
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(rows)
				return nil
			}
			w := tabwriter.NewWriter(a.stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "SLUG\tTASK\tKIND\tRULE\tPOLICY\tENABLED\tLAST\tNEXT")
			for _, s := range rows {
				rule := s.Cron
				if s.Kind != "recurring" {
					rule = s.RunAt
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					s.Slug, s.TaskSlug, s.Kind, rule,
					defaultStr(s.MissedPolicy, "skip"),
					yesNo(s.Enabled),
					defaultStr(s.LastStatus, "—"),
					defaultStr(s.NextRunAt, "—"))
			}
			return w.Flush()
		},
	}
}

// schedulesReconcileCmd asks the daemon to fire any missed recurring schedule
// runs immediately (manual override of the periodic watchdog).
func (a *App) schedulesReconcileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reconcile",
		Short: "Fire missed schedule runs immediately",
		Long: "Recurring schedules that should have run while the daemon was up\n" +
			"but idle (PC sleep, backgrounded, clock drift) are evaluated again.\n" +
			"Schedules with missed_policy=run_now fire now; missed_policy=skip\n" +
			"records a 'missed' run row instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.client.ReconcileSchedules(); err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{"ok": true, "action": "schedules_reconcile"})
				return nil
			}
			fmt.Fprintln(a.stdout,
				"schedules reconciled — what was caught up is in Logs → System, history in `heka schedules missed`")
			return nil
		},
	}
}

// schedulesMissedCmd lists schedule runs the daemon recorded as missed or
// skipped, for debugging why a task didn't run when expected.
func (a *App) schedulesMissedCmd() *cobra.Command {
	var since string
	var statusFilter string
	var taskFilter string
	var limit int
	cmd := &cobra.Command{
		Use:   "missed",
		Short: "List missed and skipped schedule runs",
		Long: "Reads the run history filtered to status=missed,skipped by default\n" +
			"so you can see which schedules didn't fire (PC was off, sleep, overlap)\n" +
			"and when.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dur, err := time.ParseDuration(since)
			if err != nil {
				return fmt.Errorf("--since: %w", err)
			}
			if limit <= 0 {
				limit = 50
			}

			// Build schedule-id → slug map for friendly output.
			scheds, err := a.client.ListSchedules()
			if err != nil {
				return err
			}
			slugByID := make(map[string]string, len(scheds))
			for _, s := range scheds {
				slugByID[s.ID] = s.Slug
			}

			from := time.Now().Add(-dur).UTC().Format(time.RFC3339)
			result, err := a.client.ListRuns(ipc.RunFilters{
				Task:   taskFilter,
				Status: statusFilter,
				From:   from,
				Order:  "desc",
				Limit:  limit,
			})
			if err != nil {
				return err
			}

			if a.json {
				a.printJSON(map[string]any{
					"since":  from,
					"status": statusFilter,
					"total":  result.Total,
					"runs":   result.Runs,
				})
				return nil
			}

			if len(result.Runs) == 0 {
				fmt.Fprintf(a.stdout, "no missed/skipped schedule runs in the last %s.\n", dur)
				return nil
			}

			w := tabwriter.NewWriter(a.stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "WHEN\tTASK\tSCHEDULE\tSTATUS\tTRIGGER\tRUN_ID")
			for _, r := range result.Runs {
				when := r.StartedAt
				if when == "" {
					when = r.FinishedAt
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					when,
					r.TaskSlug,
					defaultStr(slugByID[r.ScheduleID], "—"),
					r.Status,
					r.Trigger,
					shortRunID(r.RunID))
			}
			_ = w.Flush()
			fmt.Fprintf(a.stdout, "\n%d row(s), %d total in window (filter: status=%s, since=%s)\n",
				len(result.Runs), result.Total, statusFilter, dur)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "168h", "lookback window, e.g. 24h, 7d, 30m")
	cmd.Flags().StringVar(&statusFilter, "status", "missed,skipped", "comma-separated status filter")
	cmd.Flags().StringVar(&taskFilter, "task", "", "filter by task slug")
	cmd.Flags().IntVar(&limit, "limit", 50, "max rows to display")
	return cmd
}

func shortRunID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "…"
}

func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
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
