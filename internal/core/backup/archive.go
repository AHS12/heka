package backup

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	yzip "github.com/yeka/zip"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (same import as internal/db)
)

// Sentinels surfaced to GUI/CLI so they can render precise guidance.
var (
	// ErrWrongPassphrase: the archive could not be decrypted.
	ErrWrongPassphrase = errors.New("backup: wrong passphrase (or the archive is corrupted)")
	// ErrDaemonRunning: restore refused because the daemon owns the live data.
	ErrDaemonRunning = errors.New("backup: the heka daemon is running — stop it before restoring")
	// ErrBackupTooNew: the archive's schema is newer than this binary.
	ErrBackupTooNew = errors.New("backup: archive was created by a newer Heka version — upgrade first")
)

// CreateOptions configures one archive. The DB is snapshotted with SQLite's
// VACUUM INTO, so the daemon can keep serving while the archive is built.
type CreateOptions struct {
	DataDir      string // heka.db + secret.key + config.yaml live here
	TasksDir     string // canonical task YAML files
	ArtifactsDir string // per-run output folders (used when Includes.Artifacts)
	OutPath      string // final archive path (written via .tmp + rename)
	Passphrase   string
	AppVersion   string
	Includes     Includes
	Now          func() time.Time // injectable clock for tests
}

// ArchiveName is the canonical archive file name for a point in time.
func ArchiveName(t time.Time) string {
	return "heka-backup-" + t.Format("20060102-150405") + ".zip"
}

// Create builds an encrypted archive and returns its manifest.
func Create(o CreateOptions) (Manifest, error) {
	if o.Passphrase == "" {
		return Manifest{}, fmt.Errorf("backup: passphrase must not be empty")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	now := o.Now()
	stamp := now.UTC().Format(time.RFC3339)

	if err := os.MkdirAll(filepath.Dir(o.OutPath), 0o700); err != nil {
		return Manifest{}, fmt.Errorf("backup: create output dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(o.DataDir, "heka-bak-*")
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: staging dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	manifest := Manifest{
		FormatVersion: FormatVersion,
		AppVersion:    o.AppVersion,
		CreatedAt:     stamp,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Includes:      o.Includes,
	}
	if host, err := os.Hostname(); err == nil {
		manifest.Hostname = host
	}

	dbSrc := filepath.Join(o.DataDir, "heka.db")
	keySrc := filepath.Join(o.DataDir, "secret.key")
	hasDB := fileExists(dbSrc)

	// --- SQLite snapshot (consistent even while the daemon writes).
	snapDir := filepath.Join(tmpDir, "db")
	if hasDB {
		if err := os.MkdirAll(snapDir, 0o700); err != nil {
			return Manifest{}, fmt.Errorf("backup: staging db dir: %w", err)
		}
		snapPath := filepath.Join(snapDir, "heka.db")
		if err := vacuumInto(dbSrc, snapPath); err != nil {
			return Manifest{}, err
		}
		counts, schema, err := stripAndMeasure(snapPath, o.Includes.RunHistory)
		if err != nil {
			return Manifest{}, err
		}
		manifest.Counts = counts
		manifest.SchemaVersion = schema
	}

	// --- Vault key: without it the DB's secrets are ciphertext forever.
	vaultDir := filepath.Join(tmpDir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("backup: staging vault dir: %w", err)
	}
	keyDst := filepath.Join(vaultDir, "secret.key")
	if err := copyFile(keySrc, keyDst, 0o600); err != nil {
		return Manifest{}, fmt.Errorf("backup: vault key: %w", err)
	}

	// --- Task YAML files (the source of truth for task definitions).
	if err := copyTreeFlat(o.TasksDir, filepath.Join(tmpDir, "tasks"), ".yaml"); err != nil {
		return Manifest{}, fmt.Errorf("backup: tasks: %w", err)
	}

	// --- Optional user config.
	if cfgSrc := filepath.Join(o.DataDir, "config.yaml"); fileExists(cfgSrc) {
		if err := copyFile(cfgSrc, filepath.Join(tmpDir, "config.yaml"), 0o600); err != nil {
			return Manifest{}, fmt.Errorf("backup: config: %w", err)
		}
		manifest.HasConfig = true
	}

	// --- Optional run artifacts (arbitrarily large; off by default).
	if o.Includes.Artifacts && o.ArtifactsDir != "" && dirExists(o.ArtifactsDir) {
		if err := copyTree(o.ArtifactsDir, filepath.Join(tmpDir, "runs")); err != nil {
			return Manifest{}, fmt.Errorf("backup: artifacts: %w", err)
		}
		manifest.HasArtifacts = true
	}

	// --- Checksums over the exact bytes going into the archive.
	dbSum, err := sha256File(filepath.Join(snapDir, "heka.db"))
	if err != nil && !os.IsNotExist(err) {
		return Manifest{}, fmt.Errorf("backup: checksum db: %w", err)
	}
	keySum, err := sha256File(keyDst)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: checksum key: %w", err)
	}
	manifest.Checksums = Checksums{DB: dbSum, SecretKey: keySum}

	if err := writeJSON(filepath.Join(tmpDir, "manifest.json"), manifest); err != nil {
		return Manifest{}, err
	}

	// --- Zip the staging dir, then atomically move into place.
	tmpZip := o.OutPath + ".tmp"
	if err := writeZip(tmpDir, tmpZip, o.Passphrase, now); err != nil {
		os.Remove(tmpZip)
		return Manifest{}, err
	}
	if err := os.Rename(tmpZip, o.OutPath); err != nil {
		os.Remove(tmpZip)
		return Manifest{}, fmt.Errorf("backup: publish archive: %w", err)
	}
	return manifest, nil
}

// Inspect reads (and decrypts) only the manifest, without touching any
// live data. Used by GUI/CLI to preview a restore.
func Inspect(zipPath, passphrase string) (Manifest, error) {
	if passphrase == "" {
		return Manifest{}, ErrWrongPassphrase
	}
	r, err := yzip.OpenReader(zipPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: open archive: %w", err)
	}
	defer r.Close()
	rc, err := openEntry(&r.Reader, "manifest.json", passphrase)
	if err != nil {
		return Manifest{}, err
	}
	defer rc.Close()
	m, err := decodeManifest(rc)
	if err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// ---- SQLite snapshot helpers ----------------------------------------------

// vacuumInto copies dbPath into a fresh, consistent, compacted snapshot at
// snapPath using SQLite's online backup (VACUUM INTO). Works while the
// daemon is running (WAL allows a concurrent read transaction) and while it
// is stopped (a stale -wal is replayed on open).
func vacuumInto(dbPath, snapPath string) error {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", filepath.ToSlash(dbPath))
	live, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("backup: open %s: %w", dbPath, err)
	}
	defer live.Close()
	// VACUUM INTO refuses an existing target; the path is single-quoted SQL.
	literal := strings.ReplaceAll(filepath.ToSlash(snapPath), "'", "''")
	if _, err := live.Exec(fmt.Sprintf("VACUUM INTO '%s'", literal)); err != nil {
		return fmt.Errorf("backup: snapshot %s: %w", dbPath, err)
	}
	return nil
}

// stripAndMeasure drops volatile/diagnostic rows from the snapshot copy and
// returns the post-strip row counts plus the schema version. Mutating the
// snapshot (never the live DB) keeps archives portable across machines.
func stripAndMeasure(snapPath string, keepRuns bool) (Counts, int, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(snapPath))
	if err != nil {
		return Counts{}, 0, fmt.Errorf("backup: open snapshot: %w", err)
	}
	defer db.Close()

	var check string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&check); err != nil {
		return Counts{}, 0, fmt.Errorf("backup: integrity_check: %w", err)
	}
	if check != "ok" {
		return Counts{}, 0, fmt.Errorf("backup: snapshot failed integrity_check: %s", check)
	}

	stmts := []string{
		`DELETE FROM daemon_log`,
		`DELETE FROM kv WHERE key IN ('daemon_pid', 'heartbeat', 'daemon_version')`,
	}
	if !keepRuns {
		stmts = append(stmts, `DELETE FROM runs`)
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			return Counts{}, 0, fmt.Errorf("backup: strip snapshot (%s): %w", q, err)
		}
	}

	var counts Counts
	for _, c := range []struct {
		n *int
		t string
	}{
		{&counts.Tasks, "tasks"}, {&counts.Schedules, "schedules"},
		{&counts.Secrets, "secrets"}, {&counts.Runs, "runs"},
	} {
		if err := db.QueryRow("SELECT COUNT(*) FROM " + c.t).Scan(c.n); err != nil {
			return Counts{}, 0, fmt.Errorf("backup: count %s: %w", c.t, err)
		}
	}
	var schema int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schema); err != nil {
		return Counts{}, 0, fmt.Errorf("backup: schema version: %w", err)
	}
	return counts, schema, nil
}

// ---- Zip plumbing ----------------------------------------------------------

const (
	manifestEntry = "manifest.json"
	dbEntry       = "db/heka.db"
	keyEntry      = "vault/secret.key"
	tasksPrefix   = "tasks/"
	artifactsRoot = "runs"
)

// writeZip packs every file under root into an AES-256 encrypted archive.
func writeZip(root, outPath, passphrase string, modTime time.Time) error {
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("backup: create archive: %w", err)
	}
	defer out.Close()

	zw := yzip.NewWriter(out)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		fh := &yzip.FileHeader{
			Name:   name,
			Method: yzip.Deflate,
		}
		fh.SetModTime(modTime)
		fh.SetEncryptionMethod(yzip.AES256Encryption)
		fh.SetPassword(passphrase)
		dst, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		_, err = io.Copy(dst, src)
		return err
	})
	if err != nil {
		zw.Close()
		return fmt.Errorf("backup: write archive: %w", err)
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("backup: finalize archive: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("backup: close archive: %w", err)
	}
	return nil
}

// openEntry decrypts and opens one archive member by name.
func openEntry(r *yzip.Reader, name, passphrase string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		if f.IsEncrypted() {
			f.SetPassword(passphrase)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, ErrWrongPassphrase
		}
		return rc, nil
	}
	return nil, fmt.Errorf("backup: %s missing from archive", name)
}

// ---- Small file helpers ----------------------------------------------------

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// copyTreeFlat copies every regular file in src (no recursion) into dst.
func copyTreeFlat(src, dst, ext string) error {
	if !dirExists(src) {
		return nil
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || (ext != "" && !strings.HasSuffix(e.Name(), ext)) {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// copyTree recursively copies src into dst (directories and files).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		return copyFile(path, target, 0o600)
	})
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
