package backup

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	yzip "github.com/yeka/zip"
)

// RestoreOptions configures one restore. Restore is a maintenance operation:
// the caller must ensure the daemon is stopped (the DaemonRunning seam makes
// that a hard check rather than a convention).
type RestoreOptions struct {
	ZipPath          string
	Passphrase       string
	DataDir          string // heka.db + secret.key + config.yaml live here
	TasksDir         string // canonical task YAML files
	ArtifactsDir     string // per-run output folders
	IncludeConfig    bool   // replace config.yaml with the archived one
	IncludeArtifacts bool   // replace run artifact files (when present in archive)
	CurrentSchema    int    // max migration version this binary understands
	DaemonRunning    func() bool
	Now              func() time.Time
}

// RestoreResult summarizes what was replaced.
type RestoreResult struct {
	Manifest          Manifest
	SafetyBackupPath  string
	RestoredConfig    bool
	RestoredArtifacts bool
}

// maxExtractBytes bounds cumulative extraction (zip-bomb guard).
const maxExtractBytes = 16 << 30 // 16 GiB

// Restore replaces the live data with the archive's contents. Everything is
// extracted and verified to a staging dir first; a safety backup of the
// current state is taken before anything is overwritten.
func Restore(o RestoreOptions) (RestoreResult, error) {
	if o.Passphrase == "" {
		return RestoreResult{}, ErrWrongPassphrase
	}
	if o.DaemonRunning != nil && o.DaemonRunning() {
		return RestoreResult{}, ErrDaemonRunning
	}
	if o.Now == nil {
		o.Now = time.Now
	}

	manifest, err := Inspect(o.ZipPath, o.Passphrase)
	if err != nil {
		return RestoreResult{}, err
	}
	if manifest.SchemaVersion > o.CurrentSchema {
		return RestoreResult{}, fmt.Errorf("%w (archive schema %d > this binary's %d)",
			ErrBackupTooNew, manifest.SchemaVersion, o.CurrentSchema)
	}

	if err := os.MkdirAll(o.DataDir, 0o700); err != nil {
		return RestoreResult{}, fmt.Errorf("backup: data dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(o.DataDir, "heka-restore-*")
	if err != nil {
		return RestoreResult{}, fmt.Errorf("backup: staging dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// --- Extract + verify before touching anything live.
	if err := extractAll(o.ZipPath, o.Passphrase, tmpDir); err != nil {
		return RestoreResult{}, err
	}
	extractedDB := filepath.Join(tmpDir, dbEntry)
	extractedKey := filepath.Join(tmpDir, keyEntry)
	if err := verifyChecksum(extractedDB, manifest.Checksums.DB, "db"); err != nil {
		return RestoreResult{}, err
	}
	if err := verifyChecksum(extractedKey, manifest.Checksums.SecretKey, "vault key"); err != nil {
		return RestoreResult{}, err
	}
	if manifest.HasDB() {
		if err := checkIntegrity(extractedDB); err != nil {
			return RestoreResult{}, err
		}
	}

	// --- Safety backup of the current state (skipped on a fresh install
	// where neither db nor vault key exists yet).
	result := RestoreResult{Manifest: manifest}
	dbSrc := filepath.Join(o.DataDir, "heka.db")
	keySrc := filepath.Join(o.DataDir, "secret.key")
	if fileExists(dbSrc) && fileExists(keySrc) {
		safetyDir := filepath.Join(o.DataDir, "backups")
		safetyPath := filepath.Join(safetyDir, "pre-restore-"+o.Now().Format("20060102-150405")+".zip")
		_, err := Create(CreateOptions{
			DataDir:      o.DataDir,
			TasksDir:     o.TasksDir,
			ArtifactsDir: o.ArtifactsDir,
			OutPath:      safetyPath,
			Passphrase:   o.Passphrase,
			Includes:     Includes{RunHistory: true, Artifacts: false},
		})
		if err != nil {
			return RestoreResult{}, fmt.Errorf("backup: safety backup failed (nothing was modified): %w", err)
		}
		result.SafetyBackupPath = safetyPath
	}

	// --- Database (+ vault key) replace atomically as a pair.
	if manifest.HasDB() {
		if err := os.Remove(dbSrc); err != nil && !os.IsNotExist(err) {
			return RestoreResult{}, fmt.Errorf("backup: remove old db: %w", err)
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if err := os.Remove(dbSrc + suffix); err != nil && !os.IsNotExist(err) {
				return RestoreResult{}, fmt.Errorf("backup: remove old db %s: %w", suffix, err)
			}
		}
		if err := os.Rename(extractedDB, dbSrc); err != nil {
			return RestoreResult{}, fmt.Errorf("backup: install db: %w", err)
		}
		if err := os.Rename(extractedKey, keySrc); err != nil {
			return RestoreResult{}, fmt.Errorf("backup: install vault key: %w", err)
		}
		chmod0600(dbSrc)
		chmod0600(keySrc)
	}

	// --- Tasks dir: wipe then rewrite (the YAML files are the source of
	// truth; the daemon re-syncs its index on start).
	if err := replaceDirContents(filepath.Join(tmpDir, tasksPrefix), o.TasksDir, ".yaml"); err != nil {
		return RestoreResult{}, fmt.Errorf("backup: restore tasks: %w", err)
	}

	// --- Optional user config.
	if o.IncludeConfig && manifest.HasConfig {
		if err := copyFile(filepath.Join(tmpDir, "config.yaml"), filepath.Join(o.DataDir, "config.yaml"), 0o600); err != nil {
			return RestoreResult{}, fmt.Errorf("backup: restore config: %w", err)
		}
		result.RestoredConfig = true
	}

	// --- Optional run artifacts.
	if o.IncludeArtifacts && manifest.HasArtifacts {
		if err := replaceDirContents(filepath.Join(tmpDir, artifactsRoot), o.ArtifactsDir, ""); err != nil {
			return RestoreResult{}, fmt.Errorf("backup: restore artifacts: %w", err)
		}
		result.RestoredArtifacts = true
	}

	return result, nil
}

// HasDB reports whether the archive carries a database snapshot. (Format v1
// always does; the flag keeps the restore path honest if that ever changes.)
func (m Manifest) HasDB() bool { return m.Checksums.DB != "" }

// extractAll unpacks every entry into dest, preserving archive paths.
func extractAll(zipPath, passphrase, dest string) error {
	r, err := yzip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("backup: open archive: %w", err)
	}
	defer r.Close()

	var total int64
	for _, f := range r.File {
		if f.IsEncrypted() {
			f.SetPassword(passphrase)
		}
		rc, err := f.Open()
		if err != nil {
			return ErrWrongPassphrase
		}
		name := filepath.FromSlash(f.Name)
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			rc.Close()
			return fmt.Errorf("backup: unsafe archive path %q", f.Name)
		}
		target := filepath.Join(dest, name)
		if f.FileInfo().IsDir() {
			rc.Close()
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			rc.Close()
			return err
		}
		total += int64(f.UncompressedSize64)
		if total > maxExtractBytes {
			rc.Close()
			return fmt.Errorf("backup: archive exceeds the %d GiB extraction limit", maxExtractBytes>>30)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return ErrWrongPassphrase
		}
		out.Close()
		rc.Close()
	}
	return nil
}

func verifyChecksum(path, want, label string) error {
	if want == "" {
		return nil
	}
	got, err := sha256File(path)
	if err != nil {
		return fmt.Errorf("backup: checksum %s: %w", label, err)
	}
	if got != want {
		return fmt.Errorf("backup: %s checksum mismatch — the archive is truncated or was modified", label)
	}
	return nil
}

func checkIntegrity(dbPath string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		return fmt.Errorf("backup: open restored db: %w", err)
	}
	defer db.Close()
	var check string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&check); err != nil {
		return fmt.Errorf("backup: integrity_check: %w", err)
	}
	if check != "ok" {
		return fmt.Errorf("backup: restored db failed integrity_check: %s", check)
	}
	return nil
}

// replaceDirContents clears dst and copies every entry from src. ext filters
// the copied files ("" = all). src may not exist (empty archive section).
func replaceDirContents(src, dst, ext string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	if !dirExists(src) {
		return nil
	}
	return copyTree(src, dst)
}

func chmod0600(path string) {
	if runtime.GOOS != "windows" {
		_ = os.Chmod(path, 0o600)
	}
}
