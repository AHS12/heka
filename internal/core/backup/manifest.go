package backup

import (
	"encoding/json"
	"fmt"
	"io"
)

// FormatVersion is the archive layout version this binary writes and the
// maximum version it restores. A bump means reader changes; writers stay
// backward-compatible by keeping the highest understood reader version.
const FormatVersion = 1

// Counts describes the archive contents at creation time.
type Counts struct {
	Tasks     int `json:"tasks"`
	Schedules int `json:"schedules"`
	Secrets   int `json:"secrets"`
	Runs      int `json:"runs"`
}

// Checksums are sha256 hex digests of the extracted db and vault key, so
// restore can detect a truncated or tampered archive before overwriting
// anything.
type Checksums struct {
	DB       string `json:"db"`
	SecretKey string `json:"secret_key"`
}

// Manifest is the archive's self-description, stored as manifest.json.
type Manifest struct {
	FormatVersion int      `json:"format_version"`
	AppVersion    string   `json:"app_version,omitempty"`
	CreatedAt     string   `json:"created_at"` // UTC RFC 3339
	Hostname      string   `json:"hostname,omitempty"`
	OS            string   `json:"os,omitempty"`
	Arch          string   `json:"arch,omitempty"`
	SchemaVersion int      `json:"schema_version"`
	Counts        Counts   `json:"counts"`
	Includes      Includes `json:"includes"`
	HasConfig     bool     `json:"has_config"`
	HasArtifacts  bool     `json:"has_artifacts"`
	Checksums     Checksums `json:"checksums"`
}

// decodeManifest parses and sanity-checks a decrypted manifest.json.
func decodeManifest(r io.Reader) (Manifest, error) {
	data, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return Manifest{}, ErrWrongPassphrase
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("backup: manifest unreadable: %w", err)
	}
	if m.FormatVersion <= 0 || m.FormatVersion > FormatVersion {
		return Manifest{}, fmt.Errorf("backup: unsupported archive format version %d (this build reads %d)", m.FormatVersion, FormatVersion)
	}
	return m, nil
}
