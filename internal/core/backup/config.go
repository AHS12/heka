// Package backup implements Heka's archive backup/restore: an AES-256
// encrypted zip containing a consistent SQLite snapshot, the vault key, the
// canonical task YAML files, and the optional user config — plus an S3-
// compatible upload destination (AWS S3, Cloudflare R2, MinIO, B2).
//
// The daemon creates archives while running (SQLite online snapshot via
// VACUUM INTO). Restore is a maintenance operation performed only while the
// daemon is stopped; the GUI and CLI share Restore here.
package backup

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// KV keys holding the backup configuration and scheduler cursor.
const (
	KVConfig  = "backup_config"
	KVNextRun = "backup_next_run"
)

// Vault secret names used by the backup feature. They follow the same
// env-identifier rule as every vault key (SPEC-11 §2) and are managed
// through the existing secrets API — the backup layer only resolves them.
const (
	SecretPassphrase       = "BACKUP_PASSPHRASE"
	SecretS3AccessKeyID    = "BACKUP_S3_ACCESS_KEY_ID"
	SecretS3SecretAccessKey = "BACKUP_S3_SECRET_ACCESS_KEY"
)

// Schedule cadence kinds.
const (
	ScheduleOff      = "off"
	ScheduleInterval = "interval"
	ScheduleDaily    = "daily"
	ScheduleWeekly   = "weekly"
	ScheduleMonthly  = "monthly"
)

// MaxIntervalHours bounds the interval cadence: one year.
const MaxIntervalHours = 8760

// ScheduleSpec drives automatic backups (all local time):
//   - interval: every EveryHours (1–8760, i.e. hourly up to yearly)
//   - daily:    at AtTime ("HH:MM")
//   - weekly:   on Weekday (0=Sunday…6=Saturday) at AtTime
//   - monthly:  on DayOfMonth (1–28, clamped short of month lengths that
//     vary) at AtTime
//
// Off disables the loop.
type ScheduleSpec struct {
	Kind       string `json:"kind"`
	EveryHours int    `json:"every_hours,omitempty"`  // interval
	AtTime     string `json:"at_time,omitempty"`      // daily/weekly/monthly: "HH:MM"
	Weekday    int    `json:"weekday,omitempty"`      // weekly
	DayOfMonth int    `json:"day_of_month,omitempty"` // monthly
}

// S3Config identifies an S3-compatible destination. Credentials are never
// part of the config — they live in the vault (SecretS3* names). An empty
// Bucket means "no remote destination".
type S3Config struct {
	Endpoint string `json:"endpoint,omitempty"` // host[:port]; scheme comes from UseSSL
	Region   string `json:"region,omitempty"`   // e.g. "auto" for R2; default "us-east-1"
	Bucket   string `json:"bucket,omitempty"`
	Prefix   string `json:"prefix,omitempty"` // key prefix inside the bucket
	UseSSL   bool   `json:"use_ssl,omitempty"`
	KeepLast int    `json:"keep_last,omitempty"` // remote retention, ≥1 when set
}

// Includes selects optional archive contents.
type Includes struct {
	RunHistory bool `json:"run_history"`
	Artifacts  bool `json:"artifacts"`
}

// Config is the full backup configuration persisted in KV as JSON.
type Config struct {
	Schedule      ScheduleSpec `json:"schedule"`
	LocalDir      string       `json:"local_dir,omitempty"` // "" = no local copies
	KeepLastLocal int          `json:"keep_last_local,omitempty"`
	S3            S3Config     `json:"s3"`
	Includes      Includes     `json:"includes"`
}

// Default returns the configuration used before the user saves anything:
// automatic backups off, local copies into <DataDir>/backups when triggered.
func Default(dataDir string) Config {
	return Config{
		Schedule:      ScheduleSpec{Kind: ScheduleOff},
		LocalDir:      "", // manager falls back to <DataDir>/backups
		KeepLastLocal: 5,
		Includes:      Includes{RunHistory: true},
	}
}

// S3Enabled reports whether an S3 destination is configured.
func (c Config) S3Enabled() bool { return c.S3.Bucket != "" && c.S3.Endpoint != "" }

var dailyRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

// parseHHMM decodes a validated "HH:MM" local-time string.
func parseHHMM(s string) (hour, minute int, ok bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil {
		return 0, 0, false
	}
	return h, m, true
}

// Validate checks the config so a bad value can never silently disable
// backups or break the loop.
func (c Config) Validate() error {
	switch c.Schedule.Kind {
	case ScheduleOff:
	case ScheduleInterval:
		if c.Schedule.EveryHours < 1 || c.Schedule.EveryHours > MaxIntervalHours {
			return fmt.Errorf("every_hours must be between 1 and %d (got %d)", MaxIntervalHours, c.Schedule.EveryHours)
		}
	case ScheduleDaily:
		if !dailyRe.MatchString(c.Schedule.AtTime) {
			return fmt.Errorf("at_time must be HH:MM in 24-hour local time (got %q)", c.Schedule.AtTime)
		}
	case ScheduleWeekly:
		if c.Schedule.Weekday < 0 || c.Schedule.Weekday > 6 {
			return fmt.Errorf("weekday must be 0 (Sunday) through 6 (Saturday) (got %d)", c.Schedule.Weekday)
		}
		if !dailyRe.MatchString(c.Schedule.AtTime) {
			return fmt.Errorf("at_time must be HH:MM in 24-hour local time (got %q)", c.Schedule.AtTime)
		}
	case ScheduleMonthly:
		if c.Schedule.DayOfMonth < 1 || c.Schedule.DayOfMonth > 28 {
			return fmt.Errorf("day_of_month must be between 1 and 28 (got %d)", c.Schedule.DayOfMonth)
		}
		if !dailyRe.MatchString(c.Schedule.AtTime) {
			return fmt.Errorf("at_time must be HH:MM in 24-hour local time (got %q)", c.Schedule.AtTime)
		}
	default:
		return fmt.Errorf("schedule kind must be off, interval, daily, weekly, or monthly (got %q)", c.Schedule.Kind)
	}
	if c.LocalDir != "" && c.KeepLastLocal < 1 {
		return fmt.Errorf("keep_last_local must be at least 1")
	}
	if c.S3.Bucket != "" || c.S3.Endpoint != "" {
		if err := c.S3.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s S3Config) validate() error {
	if s.Endpoint == "" {
		return fmt.Errorf("s3: endpoint is required when a bucket is set")
	}
	if s.Bucket == "" {
		return fmt.Errorf("s3: bucket is required when an endpoint is set")
	}
	u, err := url.Parse("http://" + s.Endpoint) // scheme is carried by UseSSL
	if err != nil || u.Host != s.Endpoint {
		return fmt.Errorf("s3: endpoint must be host[:port], without scheme (got %q)", s.Endpoint)
	}
	if s.KeepLast < 0 {
		return fmt.Errorf("s3: keep_last must not be negative")
	}
	return nil
}

// NextRun computes the next activation strictly after `after` (local time).
// The zero time means "schedule off".
func (s ScheduleSpec) NextRun(after time.Time) time.Time {
	switch s.Kind {
	case ScheduleInterval:
		return after.Add(time.Duration(s.EveryHours) * time.Hour)
	case ScheduleDaily, ScheduleWeekly, ScheduleMonthly:
		h, m := 3, 0 // fallback for pre-validation configs; keeps the loop sane
		if ph, pm, ok := parseHHMM(s.AtTime); ok {
			h, m = ph, pm
		}
		switch s.Kind {
		case ScheduleDaily:
			next := time.Date(after.Year(), after.Month(), after.Day(), h, m, 0, 0, time.Local)
			if !next.After(after) {
				next = next.AddDate(0, 0, 1)
			}
			return next
		case ScheduleWeekly:
			weekday := s.Weekday
			if weekday < 0 || weekday > 6 {
				weekday = 0
			}
			// Days until the next matching weekday (0 = today).
			delta := (weekday - int(after.Weekday()) + 7) % 7
			next := time.Date(after.Year(), after.Month(), after.Day()+delta, h, m, 0, 0, time.Local)
			if !next.After(after) {
				next = next.AddDate(0, 0, 7)
			}
			return next
		default: // monthly
			day := s.DayOfMonth
			if day < 1 || day > 28 {
				day = 1
			}
			next := time.Date(after.Year(), after.Month(), day, h, m, 0, 0, time.Local)
			if !next.After(after) {
				// Day ≤ 28 exists in every month; time.Date normalizes Dec→Jan.
				next = time.Date(after.Year(), after.Month()+1, day, h, m, 0, 0, time.Local)
			}
			return next
		}
	default:
		return time.Time{}
	}
}

// Encode serializes the config for its KV row.
func (c Config) Encode() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ParseConfig decodes a KV-stored config. An empty string yields Default.
func ParseConfig(s, dataDir string) (Config, error) {
	if s == "" {
		return Default(dataDir), nil
	}
	var c Config
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return Config{}, fmt.Errorf("backup config: %w", err)
	}
	if c.Schedule.Kind == "" {
		c.Schedule.Kind = ScheduleOff
	}
	if c.KeepLastLocal == 0 {
		c.KeepLastLocal = 5
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// GeneratePassphrase returns a random passphrase for archive encryption:
// 32 bytes of CSPRNG entropy, base64url (43 chars, no padding).
func GeneratePassphrase() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate passphrase: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
