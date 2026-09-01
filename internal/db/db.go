// Package db owns Heka's SQLite layer (SPEC-03). Only the daemon opens this
// database (PRD §4) — the GUI and CLI go through IPC.
package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, no CGO (master spec D4)
)

// Now returns the current UTC time in the storage format: RFC 3339 TEXT
// (master spec §4). Callers must never store local times.
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// DB wraps the underlying connection pool. Stores are named methods on it.
type DB struct {
	sql *sql.DB
	key []byte // secrets at-rest key (loaded per Open; decrypt fallback = legacy plaintext)
}

// Open creates the data directory if needed, opens <data>/heka.db with the
// SPEC-03 pragmas, and applies pending migrations. The file is created on
// first use, so the restrictive permission is applied after migrating.
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", dataDir, err)
	}
	key, err := loadSecretKey(dataDir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "heka.db")
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
		filepath.ToSlash(path),
	)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	d := &DB{sql: sqlDB, key: key}
	if err := migrate(d); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("chmod %s: %w", path, err)
		}
	}
	return d, nil
}

// Close closes the underlying pool.
func (d *DB) Close() error {
	return d.sql.Close()
}

// loadSecretKey loads or creates the per-install key that encrypts the
// secrets vault at rest (SPEC-11 §2 security). The key file sits next to the
// database with 0600 permissions. This is local-machine protection: a copy of
// the DB without the key file yields ciphertext only.
func loadSecretKey(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, "secret.key")
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate secret key: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("write secret key: %w", err)
	}
	return key, nil
}

// encryptSecret seals a vault value with AES-256-GCM; the blob is
// base64(nonce || ciphertext) so no plaintext prefix is recoverable.
func (d *DB) encryptSecret(plain string) (string, error) {
	block, err := aes.NewCipher(d.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plain), nil)
	return base64.RawStdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

// decryptSecret opens a vault value. Values that were stored before
// encryption (legacy plaintext) or under a different key are returned
// unchanged — the vault degrades to readable rather than breaking.
func (d *DB) decryptSecret(stored string) (string, bool) {
	blob, err := base64.RawStdEncoding.DecodeString(stored)
	if err != nil {
		return stored, false
	}
	block, err := aes.NewCipher(d.key)
	if err != nil {
		return stored, false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(blob) < gcm.NonceSize()+gcm.Overhead() {
		return stored, false
	}
	plain, err := gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], nil)
	if err != nil {
		return stored, false
	}
	return string(plain), true
}