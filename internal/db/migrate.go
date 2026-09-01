package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const migrationTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
)`

// migration is a single embedded, immutable migration file.
type migration struct {
	version int
	body    string
}

// loadMigrations reads migrations/*.sql (sorted by name) and parses the
// leading version number from each filename (e.g. 0001_init.sql → 1).
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	var migrations []migration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return nil, fmt.Errorf("migration %s: not a version-prefixed filename", name)
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migration{version: version, body: string(body)})
	}
	return migrations, nil
}

// migrate applies every unapplied migration in version order, each inside its
// own transaction. Re-opening an up-to-date database is a no-op.
func migrate(d *DB) error {
	if _, err := d.sql.Exec(migrationTable); err != nil {
		return err
	}
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, m := range migrations {
		var applied int
		err := d.sql.QueryRow(
			`SELECT version FROM schema_migrations WHERE version = ?`, m.version,
		).Scan(&applied)
		if err == sql.ErrNoRows {
			if err := applyMigration(d, m); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(d *DB, m migration) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(m.body); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migration %04d: %w", m.version, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		m.version, Now(),
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
