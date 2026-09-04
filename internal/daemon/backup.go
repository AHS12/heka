package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/oklog/ulid/v2"

	"heka/internal/config"
	"heka/internal/core/backup"
	"heka/internal/db"
	"heka/internal/ipc"
)

const (
	backupLoopInterval  = 30 * time.Second
	backupCatchupGrace  = 2 * backupLoopInterval // tick jitter vs. a real miss
	backupHistoryKeep   = 100
	backupUploadTimeout = 15 * time.Minute
	backupTestTimeout   = 30 * time.Second
)

// BackupManager runs archive backups inside the daemon: the schedule loop,
// manual triggers, config persistence, job history, and destination tests.
// It owns no state of its own beyond the in-flight job — config and the
// next-run cursor live in KV, history in the backups table.
type BackupManager struct {
	cfg      config.Config
	db       *db.DB
	version  string
	resolver backup.CredentialResolver
	notify   func(title, message string) // desktop toast (beeep), best-effort
	now      func() time.Time

	mu          sync.Mutex
	running     bool
	current     *db.BackupJob
	reconcileMu sync.Mutex // serializes schedule passes (loop + reconcile)
}

func newBackupManager(cfg config.Config, database *db.DB, version string, resolver backup.CredentialResolver) *BackupManager {
	return &BackupManager{
		cfg:      cfg,
		db:       database,
		version:  version,
		resolver: resolver,
		notify: func(title, message string) {
			_ = beeep.Notify(title, message, "")
		},
		now: time.Now,
	}
}

// failInterrupted marks jobs left "running" by a previous daemon life as
// failed, so the history never shows a phantom in-flight backup.
func (m *BackupManager) failInterrupted() {
	jobs, err := m.db.Backups().List(backupHistoryKeep)
	if err != nil {
		return
	}
	for _, j := range jobs {
		if j.Status != "running" {
			continue
		}
		j.Status = "failed"
		j.Error = "interrupted by daemon restart"
		_ = m.db.Backups().Update(j)
	}
}

// ---- Config ----------------------------------------------------------------

func (m *BackupManager) loadConfig() (backup.Config, error) {
	raw, _, _ := m.db.KV().Get(backup.KVConfig)
	c, err := backup.ParseConfig(raw, m.cfg.DataDir)
	if err != nil {
		return backup.Default(m.cfg.DataDir), nil // corrupt config: fall back, don't wedge the loop
	}
	return c, nil
}

func (m *BackupManager) getConfig() ipc.BackupConfigDTO {
	c, _ := m.loadConfig()
	dto := backupConfigDTO(c)
	_, dto.PassphraseSet = m.resolver(backup.SecretPassphrase)
	return dto
}

func (m *BackupManager) updateConfig(dto ipc.BackupConfigDTO) error {
	c := configFromDTO(dto)
	if err := c.Validate(); err != nil {
		return err
	}
	enc, err := c.Encode()
	if err != nil {
		return err
	}
	if err := m.db.KV().Set(backup.KVConfig, enc); err != nil {
		return err
	}
	// Recompute the schedule cursor from now so a cadence change takes
	// effect immediately instead of surprising the next tick.
	if next := c.Schedule.NextRun(m.now()); next.IsZero() {
		_ = m.db.KV().Delete(backup.KVNextRun)
	} else {
		_ = m.db.KV().Set(backup.KVNextRun, next.UTC().Format(time.RFC3339))
	}
	m.logf("info", "backup config updated (schedule=%s)", c.Schedule.Kind)
	return nil
}

// ---- Manual trigger --------------------------------------------------------

// runNow starts a backup job unless one is already in flight.
func (m *BackupManager) runNow() (string, error) {
	job, err := m.begin("manual")
	if err != nil {
		return "", err
	}
	if err := m.db.Backups().Insert(*job); err != nil {
		m.finish(job) // allow retry
		return "", fmt.Errorf("backup: record job: %w", err)
	}
	m.logf("info", "manual backup starting")
	go m.execute(job)
	return job.ID, nil
}

// begin atomically claims the single in-flight slot.
func (m *BackupManager) begin(trigger string) (*db.BackupJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil, errors.New("a backup is already running")
	}
	m.running = true
	m.current = &db.BackupJob{
		ID:        ulid.Make().String(),
		Trigger:   trigger,
		Status:    "running",
		StartedAt: db.Now(),
	}
	return m.current, nil
}

// finish releases the slot.
func (m *BackupManager) finish(job *db.BackupJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == job {
		m.current = nil
		m.running = false
	}
}

// ---- Job execution ---------------------------------------------------------

// execute runs the whole pipeline for one claimed job: local archive →
// optional S3 upload → retention pruning → history row → notifications.
func (m *BackupManager) execute(job *db.BackupJob) {
	defer m.finish(job)

	cfg, err := m.loadConfig()
	if err != nil {
		m.failJob(job, nil, err)
		return
	}
	passphrase, _ := m.resolver(backup.SecretPassphrase)
	if passphrase == "" {
		m.failJob(job, nil, fmt.Errorf(
			"no backup passphrase in the vault — set it up in Settings → Backup first"))
		return
	}

	outDir := cfg.LocalDir
	if outDir == "" {
		outDir = filepath.Join(m.cfg.DataDir, "backups")
	}
	name := backup.ArchiveName(m.now())
	localPath := filepath.Join(outDir, name)

	var dests []ipc.BackupDestinationResult
	var size int64

	_, err = backup.Create(backup.CreateOptions{
		DataDir:      m.cfg.DataDir,
		TasksDir:     m.cfg.TasksDir,
		ArtifactsDir: m.cfg.RunArtifactsDir,
		OutPath:      localPath,
		Passphrase:   passphrase,
		AppVersion:   m.version,
		Includes:     cfg.Includes,
		Now:          m.now,
	})
	if err != nil {
		dests = append(dests, ipc.BackupDestinationResult{Type: "local", Path: localPath, Err: err.Error()})
		m.failJob(job, dests, fmt.Errorf("archive failed: %w", err))
		return
	}
	if info, err := os.Stat(localPath); err == nil {
		size = info.Size()
	}
	dests = append(dests, ipc.BackupDestinationResult{Type: "local", OK: true, Path: localPath})

	if cfg.S3Enabled() {
		dests = append(dests, m.upload(cfg, localPath, name, size))
	}

	job.LocalPath = localPath
	job.SizeBytes = &size
	job.DestinationsJSON = marshalDestinations(dests)

	okCount := 0
	for _, d := range dests {
		if d.OK {
			okCount++
		}
	}
	switch {
	case okCount == len(dests):
		job.Status = "success"
	case okCount > 0:
		job.Status = "partial"
		job.Error = "some destinations failed"
	default:
		job.Status = "failed"
		job.Error = "all destinations failed"
	}
	m.complete(job)

	m.pruneLocal(outDir, cfg.KeepLastLocal)
	_ = m.db.Backups().Prune(backupHistoryKeep)

	switch job.Status {
	case "success":
		m.logf("info", "backup completed (%s) — %s", job.Trigger, humanBytes(size))
	case "partial":
		m.logf("warn", "backup partially failed (%s): %s", job.Trigger, destFailures(dests))
		m.toast("Heka backup partially failed", destFailures(dests))
	default:
		m.logf("error", "backup failed (%s): %s", job.Trigger, job.Error)
		m.toast("Heka backup failed", job.Error)
	}
}

// upload pushes the archive to the S3 destination and prunes old objects.
func (m *BackupManager) upload(cfg backup.Config, localPath, name string, size int64) ipc.BackupDestinationResult {
	res := ipc.BackupDestinationResult{Type: "s3", Path: cfg.S3.ObjectKey(name)}
	store, err := cfg.S3.BuildStore(m.resolver)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	ctx, cancel := context.WithTimeout(context.Background(), backupUploadTimeout)
	defer cancel()

	f, err := os.Open(localPath)
	if err != nil {
		res.Err = fmt.Sprintf("open archive: %v", err)
		return res
	}
	defer f.Close()
	if err := store.Put(ctx, cfg.S3.ObjectKey(name), f, size); err != nil {
		res.Err = fmt.Sprintf("upload: %v", err)
		return res
	}
	if cfg.S3.KeepLast > 0 {
		if _, err := backup.PruneRemote(ctx, store, cfg.S3.Prefix, cfg.S3.KeepLast); err != nil {
			m.logf("warn", "backup remote retention: %v", err)
		}
	}
	res.OK = true
	return res
}

// failJob records a terminal failure and notifies.
func (m *BackupManager) failJob(job *db.BackupJob, dests []ipc.BackupDestinationResult, err error) {
	job.Status = "failed"
	job.Error = err.Error()
	if dests != nil {
		job.DestinationsJSON = marshalDestinations(dests)
	}
	m.complete(job)
	m.logf("error", "backup failed (%s): %v", job.Trigger, err)
	m.toast("Heka backup failed", err.Error())
}

// complete persists the terminal row state.
func (m *BackupManager) complete(job *db.BackupJob) {
	now := db.Now()
	job.FinishedAt = &now
	_ = m.db.Backups().Update(*job)
}

// pruneLocal keeps only the newest keep backup archives in outDir.
func (m *BackupManager) pruneLocal(outDir string, keep int) {
	if keep < 1 {
		return
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasPrefix(name, "heka-backup-") && strings.HasSuffix(name, ".zip") {
			names = append(names, name)
		}
	}
	// Stamp-suffixed names sort lexicographically by time.
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names[min(keep, len(names)):] {
		if err := os.Remove(filepath.Join(outDir, name)); err != nil {
			m.logf("warn", "backup local retention: %v", err)
		}
	}
}

// ---- Schedule loop ---------------------------------------------------------

// Loop drives scheduled backups. The cadence cursor lives in KV so a daemon
// restart never double-fires and a missed window (daemon down at the due
// time) runs once at startup.
func (m *BackupManager) Loop(shutdown <-chan struct{}) {
	m.failInterrupted()
	m.tickReason("startup") // startup pass: initialize cursor / catch a missed run
	ticker := time.NewTicker(backupLoopInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.tickReason("loop")
		case <-shutdown:
			return
		}
	}
}

// tick is the loop's periodic pass.
func (m *BackupManager) tick() {
	m.tickReason("loop")
}

// Reconcile runs one catch-up pass alongside the task-schedule reconcile
// (daemon start, wake from sleep, periodic watchdog, manual "Reconcile now").
// Serialized with the loop so a pass can never double-fire a window.
func (m *BackupManager) Reconcile(reason string) {
	m.tickReason(reason)
}

func (m *BackupManager) tickReason(reason string) {
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	m.tickLocked(reason)
}

// tickLocked checks the schedule once and fires when due, advancing the
// cursor before claiming the slot so a long job can't cause double-fire.
// Caller must hold reconcileMu.
func (m *BackupManager) tickLocked(reason string) {
	cfg, err := m.loadConfig()
	if err != nil {
		m.logf("warn", "backup config unreadable: %v", err)
		return
	}
	if cfg.Schedule.Kind == backup.ScheduleOff {
		return
	}
	now := m.now()
	next, ok := m.nextRunCursor()
	if !ok || next.IsZero() {
		first := cfg.Schedule.NextRun(now)
		m.logf("info", "backup schedule (%s) armed — next run %s",
			cfg.Schedule.Kind, first.Local().Format(time.RFC3339))
		m.setCursor(first)
		return
	}
	if next.After(now) {
		return
	}
	// Due — or missed while the daemon was down or asleep. Advance the
	// cursor first, then claim the slot; an in-flight job absorbs the tick.
	m.setCursor(cfg.Schedule.NextRun(now))
	trigger := "scheduled"
	if reason != "loop" || now.Sub(next) > backupCatchupGrace {
		// Fired by reconcile, or so late that the loop itself woke from
		// sleep: this is a catch-up of a missed window, not the cadence.
		trigger = "catch-up"
		m.logf("info", "backup reconcile (%s): scheduled backup window due %s was missed — starting catch-up run",
			reason, next.Local().Format(time.RFC3339))
	}
	job, err := m.begin(trigger)
	if err != nil {
		m.logf("info", "backup tick skipped: %v", err)
		return
	}
	if err := m.db.Backups().Insert(*job); err != nil {
		m.finish(job)
		m.logf("warn", "backup job not recorded: %v", err)
		return
	}
	if trigger == "scheduled" {
		m.logf("info", "scheduled backup starting (schedule=%s)", cfg.Schedule.Kind)
	}
	go m.execute(job)
}

func (m *BackupManager) nextRunCursor() (time.Time, bool) {
	raw, ok, _ := m.db.KV().Get(backup.KVNextRun)
	if !ok || raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func (m *BackupManager) setCursor(next time.Time) {
	if next.IsZero() {
		return
	}
	_ = m.db.KV().Set(backup.KVNextRun, next.UTC().Format(time.RFC3339))
}

// ---- Queries ---------------------------------------------------------------

func (m *BackupManager) status() ipc.BackupStatusDTO {
	out := ipc.BackupStatusDTO{}
	m.mu.Lock()
	out.Running = m.running
	if m.current != nil {
		cur := *m.current
		dto := jobDTO(cur)
		out.Current = &dto
	}
	m.mu.Unlock()
	if last, err := m.db.Backups().Latest(); err == nil {
		if last.Status == "running" && out.Current != nil {
			// The in-flight job is surfaced via Current; don't duplicate it.
		} else {
			dto := jobDTO(last)
			out.Last = &dto
		}
	}
	if next, ok := m.nextRunCursor(); ok && !next.IsZero() {
		out.NextRunAt = next.UTC().Format(time.RFC3339)
	}
	return out
}

func (m *BackupManager) history(limit int) ([]ipc.BackupJobDTO, error) {
	jobs, err := m.db.Backups().List(limit)
	if err != nil {
		return nil, err
	}
	out := make([]ipc.BackupJobDTO, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, jobDTO(j))
	}
	return out, nil
}

// test probes every configured destination.
func (m *BackupManager) test() ipc.BackupTestDTO {
	cfg, _ := m.loadConfig()
	out := ipc.BackupTestDTO{}

	outDir := cfg.LocalDir
	if outDir == "" {
		outDir = filepath.Join(m.cfg.DataDir, "backups")
	}
	if err := probeLocalDir(outDir); err != nil {
		out.Local = &ipc.BackupDestinationResult{Type: "local", Path: outDir, Err: err.Error()}
	} else {
		out.Local = &ipc.BackupDestinationResult{Type: "local", OK: true, Path: outDir}
	}

	if cfg.S3Enabled() {
		ctx, cancel := context.WithTimeout(context.Background(), backupTestTimeout)
		defer cancel()
		if err := cfg.S3.TestConnection(ctx, m.resolver); err != nil {
			out.S3 = &ipc.BackupDestinationResult{Type: "s3", Err: err.Error()}
		} else {
			out.S3 = &ipc.BackupDestinationResult{Type: "s3", OK: true}
		}
	}
	return out
}

func probeLocalDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	probe := filepath.Join(dir, ".heka-probe")
	if err := os.WriteFile(probe, []byte("probe"), 0o600); err != nil {
		return err
	}
	return os.Remove(probe)
}

// ---- Plumbing --------------------------------------------------------------

func (m *BackupManager) logf(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "heka: %s\n", msg)
	_ = m.db.Logs().Add(level, "backup", msg)
}

func (m *BackupManager) toast(title, message string) {
	if m.notify != nil {
		m.notify(title, message)
	}
}

// ---- Mapping helpers -------------------------------------------------------

func backupConfigDTO(c backup.Config) ipc.BackupConfigDTO {
	return ipc.BackupConfigDTO{
		Schedule:      c.Schedule,
		LocalDir:      c.LocalDir,
		KeepLastLocal: c.KeepLastLocal,
		S3:            c.S3,
		Includes:      c.Includes,
	}
}

func configFromDTO(dto ipc.BackupConfigDTO) backup.Config {
	return backup.Config{
		Schedule:      dto.Schedule,
		LocalDir:      dto.LocalDir,
		KeepLastLocal: dto.KeepLastLocal,
		S3:            dto.S3,
		Includes:      dto.Includes,
	}
}

func jobDTO(j db.BackupJob) ipc.BackupJobDTO {
	dto := ipc.BackupJobDTO{
		ID:        j.ID,
		Trigger:   j.Trigger,
		Status:    j.Status,
		StartedAt: j.StartedAt,
		LocalPath: j.LocalPath,
		Err:       j.Error,
	}
	if j.FinishedAt != nil {
		dto.FinishedAt = *j.FinishedAt
	}
	if j.SizeBytes != nil {
		dto.SizeBytes = *j.SizeBytes
	}
	if j.DestinationsJSON != "" {
		_ = json.Unmarshal([]byte(j.DestinationsJSON), &dto.Destinations)
	}
	return dto
}

func marshalDestinations(dests []ipc.BackupDestinationResult) string {
	if dests == nil {
		return ""
	}
	b, err := json.Marshal(dests)
	if err != nil {
		return ""
	}
	return string(b)
}

func destFailures(dests []ipc.BackupDestinationResult) string {
	var parts []string
	for _, d := range dests {
		if !d.OK {
			parts = append(parts, d.Type+": "+d.Err)
		}
	}
	return strings.Join(parts, "; ")
}

// humanBytes renders a byte count for logs and toasts.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
