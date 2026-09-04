package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"heka/internal/core/backup"
	"heka/internal/daemon"
	"heka/internal/db"
	"heka/internal/ipc"
)

// backupCmd is `heka backup` — archive jobs through the running daemon
// (create/status/history/test). The daemon owns the SQLite snapshot and the
// schedule; the CLI only talks IPC.
func (a *App) backupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create and inspect encrypted backups",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("specify: heka backup create|status|history|test")
		},
	}
	cmd.AddCommand(a.backupCreateCmd(), a.backupStatusCmd(), a.backupHistoryCmd(), a.backupTestCmd())
	return cmd
}

func (a *App) backupCreateCmd() *cobra.Command {
	var wait bool
	create := &cobra.Command{
		Use:   "create",
		Short: "Run a backup now (async in the daemon)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID, err := a.client.RunBackup()
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(map[string]any{"ok": true, "job_id": jobID})
				return nil
			}
			fmt.Fprintf(a.stdout, "backup started (job %s)\n", jobID)
			if !wait {
				fmt.Fprintln(a.stdout, "watch progress: heka backup status")
				return nil
			}
			deadline := time.Now().Add(10 * time.Minute)
			for {
				time.Sleep(500 * time.Millisecond)
				st, err := a.client.BackupStatus()
				if err != nil {
					return err
				}
				if !st.Running && st.Last != nil && st.Last.ID == jobID {
					a.printJobSummary(*st.Last)
					if st.Last.Status != "success" {
						return errors.New("backup failed")
					}
					return nil
				}
				if time.Now().After(deadline) {
					return errors.New("timed out waiting for the backup to finish")
				}
			}
		},
	}
	create.Flags().BoolVar(&wait, "wait", false, "wait for the backup to finish, then report it")
	return create
}

func (a *App) backupStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current/last backup job and next scheduled run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.client.BackupStatus()
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(st)
				return nil
			}
			if st.Running && st.Current != nil {
				fmt.Fprintf(a.stdout, "running:  job %s (%s) since %s\n",
					st.Current.ID, st.Current.Trigger, st.Current.StartedAt)
			} else {
				fmt.Fprintln(a.stdout, "running:  no")
			}
			if st.Last != nil {
				a.printJobSummary(*st.Last)
			} else {
				fmt.Fprintln(a.stdout, "last:     none")
			}
			if st.NextRunAt != "" {
				fmt.Fprintf(a.stdout, "next:     %s\n", st.NextRunAt)
			}
			return nil
		},
	}
}

func (a *App) backupHistoryCmd() *cobra.Command {
	var limit int
	hist := &cobra.Command{
		Use:   "history",
		Short: "List recent backup jobs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobs, err := a.client.BackupHistory(limit)
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(jobs)
				return nil
			}
			if len(jobs) == 0 {
				fmt.Fprintln(a.stdout, "no backups yet")
				return nil
			}
			for _, j := range jobs {
				fmt.Fprintf(a.stdout, "%-8s %-9s %-20s %s\n",
					j.Status, j.Trigger, j.StartedAt, j.LocalPath)
				if j.Err != "" {
					fmt.Fprintf(a.stdout, "         error: %s\n", j.Err)
				}
			}
			return nil
		},
	}
	hist.Flags().IntVar(&limit, "limit", 20, "max jobs to list")
	return hist
}

func (a *App) backupTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Probe the configured backup destinations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := a.client.TestBackupDestinations()
			if err != nil {
				return err
			}
			if a.json {
				a.printJSON(res)
				return nil
			}
			if res.Local != nil {
				if res.Local.OK {
					fmt.Fprintf(a.stdout, "local: ok (%s)\n", res.Local.Path)
				} else {
					fmt.Fprintf(a.stdout, "local: FAILED (%s)\n", res.Local.Err)
				}
			}
			if res.S3 != nil {
				if res.S3.OK {
					fmt.Fprintln(a.stdout, "s3:    ok")
				} else {
					fmt.Fprintf(a.stdout, "s3:    FAILED (%s)\n", res.S3.Err)
				}
			}
			return nil
		},
	}
}

func (a *App) printJobSummary(j ipc.BackupJobDTO) {
	fmt.Fprintf(a.stdout, "last:     %s (%s, %s)\n", j.Status, j.Trigger, j.StartedAt)
	if j.LocalPath != "" {
		fmt.Fprintf(a.stdout, "archive:  %s\n", j.LocalPath)
	}
	if j.Err != "" {
		fmt.Fprintf(a.stdout, "error:    %s\n", j.Err)
	}
}

// restoreCmd is `heka restore <archive>` — a maintenance operation that
// replaces local data with an archive's contents. Unlike the rest of the CLI
// it touches the filesystem directly: the daemon must be stopped, so there is
// no IPC to talk to.
func (a *App) restoreCmd() *cobra.Command {
	var (
		passphrase       string
		includeConfig    bool
		includeArtifacts bool
		yes              bool
	)
	cmd := &cobra.Command{
		Use:   "restore <archive.zip>",
		Short: "Restore your data from a backup archive (daemon must be stopped)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			archive := args[0]
			if passphrase == "" {
				return errors.New("passphrase is required (--passphrase); backups are encrypted")
			}
			// Hard gate + a helpful hint rather than a raw restore error.
			if _, err := daemon.Status(a.cfg); err == nil {
				return errors.New("the heka daemon is running — stop it first: heka daemon stop")
			}

			manifest, err := backup.Inspect(archive, passphrase)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "archive:  %s\n", archive)
			fmt.Fprintf(a.stdout, "created:  %s (%s)\n", manifest.CreatedAt, manifest.AppVersion)
			fmt.Fprintf(a.stdout, "contents: %d tasks, %d schedules, %d secrets, %d runs\n",
				manifest.Counts.Tasks, manifest.Counts.Schedules, manifest.Counts.Secrets, manifest.Counts.Runs)
			if !yes {
				fmt.Fprintln(a.stdout, "\nThis REPLACES your current data. Re-run with --yes to proceed.")
				return nil
			}

			res, err := backup.Restore(backup.RestoreOptions{
				ZipPath:          archive,
				Passphrase:       passphrase,
				DataDir:          a.cfg.DataDir,
				TasksDir:         a.cfg.TasksDir,
				ArtifactsDir:     a.cfg.RunArtifactsDir,
				IncludeConfig:    includeConfig,
				IncludeArtifacts: includeArtifacts,
				CurrentSchema:    db.MaxMigrationVersion(),
			})
			if err != nil {
				if errors.Is(err, backup.ErrDaemonRunning) {
					return errors.New("the heka daemon is running — stop it first: heka daemon stop")
				}
				return err
			}
			fmt.Fprintf(a.stdout, "restored: %d tasks, %d schedules, %d secrets\n",
				res.Manifest.Counts.Tasks, res.Manifest.Counts.Schedules, res.Manifest.Counts.Secrets)
			if res.SafetyBackupPath != "" {
				fmt.Fprintf(a.stdout, "safety:   previous state saved to %s\n", res.SafetyBackupPath)
			}
			fmt.Fprintln(a.stdout, "start the daemon again: heka daemon start")
			return nil
		},
	}
	cmd.Flags().StringVar(&passphrase, "passphrase", "", "archive passphrase (required)")
	cmd.Flags().BoolVar(&includeConfig, "include-config", true, "also restore config.yaml when present in the archive")
	cmd.Flags().BoolVar(&includeArtifacts, "include-artifacts", false, "also restore run artifact files when present")
	cmd.Flags().BoolVar(&yes, "yes", false, "actually perform the restore (without it, only a preview is shown)")
	return cmd
}
