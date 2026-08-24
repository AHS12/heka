// Package config resolves Heka's runtime paths and settings (SPEC-02).
//
// Precedence (lowest to highest): platform defaults < config file <
// environment overrides. The package has no side effects — it never creates
// directories; the daemon does that during startup wiring.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultLogRetentionDays is how long run history is kept when not
	// configured otherwise.
	DefaultLogRetentionDays = 90
	// DefaultMaxOutputBytes caps a single run's captured stdout/stderr.
	DefaultMaxOutputBytes = 1 << 20 // 1 MiB

	configFileName = "config.yaml"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	DataDir          string // sqlite db, production logs
	TasksDir         string // canonical YAML task files
	SocketDir        string // POSIX IPC unix-socket dir; empty on Windows (named pipes)
	LogRetentionDays int
	MaxOutputBytes   int64
}

// fileConfig is the schema of the optional YAML config file. Pointers
// distinguish "absent" from zero values.
type fileConfig struct {
	Version          *int   `yaml:"version"`
	DataDir          string `yaml:"data_dir"`
	TasksDir         string `yaml:"tasks_dir"`
	LogRetentionDays *int   `yaml:"log_retention_days"`
	MaxOutputBytes   *int64 `yaml:"max_output_bytes"`
}

// Load resolves configuration from an environment map and a home directory.
// The env map replaces os.Environ so tests can exercise every platform's
// defaults on any machine. Production uses LoadDefault.
func Load(env map[string]string, home string) (Config, error) {
	cfg := defaults(env, home)

	// Locate the config file: explicit override, else <data>/config.yaml
	// where "data" accounts for the env-level data-dir override already.
	cfgPath := env["HEKA_CONFIG"]
	if cfgPath == "" {
		cfgPath = filepath.Join(cfg.DataDir, configFileName)
	}
	data, err := os.ReadFile(cfgPath)
	switch {
	case err == nil:
		if err := applyFile(&cfg, data); err != nil {
			return Config{}, fmt.Errorf("config file %s: %w", cfgPath, err)
		}
	case !os.IsNotExist(err):
		return Config{}, fmt.Errorf("read config file %s: %w", cfgPath, err)
	}

	// Env overrides beat the config file (SPEC-02 §2).
	if v := env["HEKA_DATA_DIR"]; v != "" {
		cfg.DataDir = v
	}
	if v := env["HEKA_TASKS_DIR"]; v != "" {
		cfg.TasksDir = v
	}
	cfg.SocketDir = socketDir(env, cfg.DataDir, runtime.GOOS)

	if err := cfg.absolutize(); err != nil {
		return Config{}, err
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadDefault loads from the real process environment and the current user's
// home directory.
func LoadDefault() (Config, error) {
	return Load(envMap(os.Environ()), userHome())
}

// defaults computes the platform default paths. HEKA_HOME shifts the base
// data dir (both default dirs follow it). Only defaults live here — the
// config file and specific env vars override later.
func defaults(env map[string]string, home string) Config {
	dataDir := env["HEKA_HOME"]
	if dataDir == "" {
		dataDir = defaultDataDir(env, home, runtime.GOOS)
	}
	return Config{
		DataDir:          dataDir,
		TasksDir:         filepath.Join(dataDir, "tasks"),
		LogRetentionDays: DefaultLogRetentionDays,
		MaxOutputBytes:   DefaultMaxOutputBytes,
	}
}

// defaultDataDir is the per-platform base directory. The goos parameter is
// injectable so both platform tables are testable on any machine.
func defaultDataDir(env map[string]string, home, goos string) string {
	if goos == "windows" {
		if v := env["LOCALAPPDATA"]; v != "" {
			return filepath.Join(v, "heka")
		}
		if v := env["USERPROFILE"]; v != "" {
			return filepath.Join(v, "AppData", "Local", "heka")
		}
		return filepath.Join(home, "AppData", "Local", "heka")
	}
	if v := env["XDG_DATA_HOME"]; v != "" {
		return filepath.Join(v, "heka")
	}
	return filepath.Join(home, ".local", "share", "heka")
}

// socketDir is the POSIX IPC socket directory; Windows uses named pipes and
// gets an empty value. goos is injectable so both behaviors are testable.
func socketDir(env map[string]string, dataDir, goos string) string {
	if goos == "windows" {
		return ""
	}
	if v := env["XDG_RUNTIME_DIR"]; v != "" {
		return v
	}
	return dataDir
}

// applyFile overlays a config file onto the defaults. The strict decoder
// rejects unknown keys (same discipline as task YAML in SPEC-04).
func applyFile(cfg *Config, data []byte) error {
	var fc fileConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&fc); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	if fc.Version != nil && *fc.Version != 1 {
		return fmt.Errorf("unsupported config version %d (want 1)", *fc.Version)
	}
	if fc.DataDir != "" {
		cfg.DataDir = fc.DataDir
	}
	if fc.TasksDir != "" {
		cfg.TasksDir = fc.TasksDir
	}
	if fc.LogRetentionDays != nil {
		cfg.LogRetentionDays = *fc.LogRetentionDays
	}
	if fc.MaxOutputBytes != nil {
		cfg.MaxOutputBytes = *fc.MaxOutputBytes
	}
	return nil
}

func (cfg *Config) absolutize() error {
	for name, p := range map[string]string{"data_dir": cfg.DataDir, "tasks_dir": cfg.TasksDir} {
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if name == "data_dir" {
			cfg.DataDir = abs
		} else {
			cfg.TasksDir = abs
		}
	}
	return nil
}

func (cfg Config) validate() error {
	if cfg.DataDir == "" {
		return fmt.Errorf("data_dir: must not be empty")
	}
	if cfg.TasksDir == "" {
		return fmt.Errorf("tasks_dir: must not be empty")
	}
	if cfg.LogRetentionDays <= 0 {
		return fmt.Errorf("log_retention_days: must be > 0 (got %d)", cfg.LogRetentionDays)
	}
	if cfg.MaxOutputBytes <= 0 {
		return fmt.Errorf("max_output_bytes: must be > 0 (got %d)", cfg.MaxOutputBytes)
	}
	return nil
}

func envMap(pairs []string) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		for i := 0; i < len(pair); i++ {
			if pair[i] == '=' {
				m[pair[:i]] = pair[i+1:]
				break
			}
		}
	}
	return m
}

func userHome() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}
