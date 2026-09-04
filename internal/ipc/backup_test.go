package ipc

import (
	"errors"
	"testing"
)

func TestBackupConfigRoundTrip(t *testing.T) {
	database := openDB(t)
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks: database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(),
		Runner: &fakeRunner{},
		GetBackupConfig: func() BackupConfigDTO {
			return BackupConfigDTO{LocalDir: "/tmp/heka-backups", KeepLastLocal: 5,
				PassphraseSet: true}
		},
		UpdateBackupConfig: func(dto BackupConfigDTO) error {
			if dto.KeepLastLocal < 1 {
				return errors.New("keep_last_local must be at least 1")
			}
			return nil
		},
	})
	client := NewClient(cfg)

	got, err := client.GetBackupConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !got.PassphraseSet || got.LocalDir != "/tmp/heka-backups" {
		t.Fatalf("config = %+v", got)
	}

	// Valid update → ok.
	got.KeepLastLocal = 3
	if err := client.UpdateBackupConfig(got); err != nil {
		t.Fatal(err)
	}

	// Invalid update → invalid_backup_config envelope.
	got.KeepLastLocal = 0
	err = client.UpdateBackupConfig(got)
	var e *Error
	if !errors.As(err, &e) || e.Code != "invalid_backup_config" {
		t.Fatalf("want invalid_backup_config, got %v", err)
	}
}

func TestBackupRunBusyMapping(t *testing.T) {
	database := openDB(t)
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks: database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(),
		Runner: &fakeRunner{},
		RunBackup: func() (string, error) {
			return "", errors.New("a backup is already running")
		},
	})
	_, err := NewClient(cfg).RunBackup()
	var e *Error
	if !errors.As(err, &e) || e.Code != "backup_busy" {
		t.Fatalf("want backup_busy, got %v", err)
	}
}

func TestBackupStatusAndHistoryRoundTrip(t *testing.T) {
	database := openDB(t)
	job := BackupJobDTO{
		ID: "job-1", Trigger: "scheduled", Status: "success",
		StartedAt: "2026-09-04T12:00:00Z", FinishedAt: "2026-09-04T12:00:05Z",
		SizeBytes: 1234, LocalPath: "/data/backups/heka-backup.zip",
		Destinations: []BackupDestinationResult{
			{Type: "local", OK: true, Path: "/data/backups/heka-backup.zip"},
			{Type: "s3", OK: false, Err: "bucket missing"},
		},
	}
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks: database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(),
		Runner: &fakeRunner{},
		RunBackup: func() (string, error) { return "job-2", nil },
		BackupStatus: func() BackupStatusDTO {
			last := job
			return BackupStatusDTO{NextRunAt: "2026-09-04T18:00:00Z", Last: &last}
		},
		BackupHistory: func(limit int) ([]BackupJobDTO, error) {
			if limit != 20 {
				t.Fatalf("limit = %d", limit)
			}
			return []BackupJobDTO{job}, nil
		},
	})
	client := NewClient(cfg)

	st, err := client.BackupStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.Running || st.Last == nil || st.Last.ID != "job-1" || st.NextRunAt == "" {
		t.Fatalf("status = %+v", st)
	}

	runID, err := client.RunBackup()
	if err != nil || runID != "job-2" {
		t.Fatalf("run = %q %v", runID, err)
	}

	hist, err := client.BackupHistory(20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 || len(hist[0].Destinations) != 2 || hist[0].Destinations[1].OK {
		t.Fatalf("history = %+v", hist)
	}
}

func TestBackupTestEndpoint(t *testing.T) {
	database := openDB(t)
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks: database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(),
		Runner: &fakeRunner{},
		TestBackupDestinations: func() BackupTestDTO {
			return BackupTestDTO{
				Local: &BackupDestinationResult{Type: "local", OK: true, Path: "/b"},
				S3:    &BackupDestinationResult{Type: "s3", Err: "no route to host"},
			}
		},
	})
	res, err := NewClient(cfg).TestBackupDestinations()
	if err != nil {
		t.Fatal(err)
	}
	if res.Local == nil || !res.Local.OK {
		t.Fatalf("local = %+v", res.Local)
	}
	if res.S3 == nil || res.S3.OK || res.S3.Err == "" {
		t.Fatalf("s3 = %+v", res.S3)
	}
}

func TestSecretsUsageEndpoint(t *testing.T) {
	database := openDB(t)
	seedTask(t, database, "alpha", "Alpha", true)
	cfg := startTestServer(t, Deps{
		Health: func() Health { return Health{Core: "healthy"} },
		Tasks: database.Tasks(), Runs: database.Runs(), Schedules: database.Schedules(),
		Runner: &fakeRunner{},
		SecretsUsage: func() (map[string][]string, error) {
			return map[string][]string{"API_TOKEN": {"alpha"}}, nil
		},
	})
	usage, err := NewClient(cfg).SecretsUsage()
	if err != nil {
		t.Fatal(err)
	}
	if got := usage["API_TOKEN"]; len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("usage = %v", usage)
	}
}
