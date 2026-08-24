package config

import (
	"os"
	"path/filepath"
	"testing"
)

const winLocalAppData = `C:\Users\alice\AppData\Local`

func TestDefaultDataDirWindows(t *testing.T) {
	for name, tt := range map[string]struct {
		env  map[string]string
		want string
	}{
		"localappdata":         {map[string]string{"LOCALAPPDATA": winLocalAppData}, `C:\Users\alice\AppData\Local\heka`},
		"userprofile fallback": {map[string]string{"USERPROFILE": `C:\Users\alice`}, `C:\Users\alice\AppData\Local\heka`},
		"home fallback":        {map[string]string{}, `C:\Users\alice\AppData\Local\heka`},
	} {
		t.Run(name, func(t *testing.T) {
			if got := defaultDataDir(tt.env, `C:\Users\alice`, "windows"); got != tt.want {
				t.Fatalf("defaultDataDir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultDataDirPosix(t *testing.T) {
	for name, tt := range map[string]struct {
		env  map[string]string
		want string
	}{
		"xdg data home": {map[string]string{"XDG_DATA_HOME": "/home/alice/.local/share"}, clean("/home/alice/.local/share/heka")},
		"home fallback": {map[string]string{}, clean("/home/alice/.local/share/heka")},
	} {
		t.Run(name, func(t *testing.T) {
			if got := defaultDataDir(tt.env, "/home/alice", "linux"); got != tt.want {
				t.Fatalf("defaultDataDir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(map[string]string{"LOCALAPPDATA": winLocalAppData}, `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != `C:\Users\alice\AppData\Local\heka` {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
	if cfg.TasksDir != `C:\Users\alice\AppData\Local\heka\tasks` {
		t.Errorf("TasksDir = %q", cfg.TasksDir)
	}
	if cfg.LogRetentionDays != 90 {
		t.Errorf("LogRetentionDays = %d, want 90", cfg.LogRetentionDays)
	}
	if cfg.MaxOutputBytes != 1<<20 {
		t.Errorf("MaxOutputBytes = %d, want 1 MiB", cfg.MaxOutputBytes)
	}
	if cfg.SocketDir != "" {
		t.Errorf("SocketDir = %q on windows, want \"\"", cfg.SocketDir)
	}
}

func TestSocketDirBehavior(t *testing.T) {
	// socketDir's goos parameter keeps both platform behaviors testable on
	// any host (SPEC-02 §4).
	posixDefaults := clean("/home/alice/.local/share/heka")
	if got := socketDir(map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, posixDefaults, "linux"); got != "/run/user/1000" {
		t.Errorf("socketDir with XDG_RUNTIME_DIR = %q", got)
	}
	if got := socketDir(nil, posixDefaults, "linux"); got != posixDefaults {
		t.Errorf("socketDir fallback = %q, want data dir", got)
	}
	if got := socketDir(map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, `C:\data`, "windows"); got != "" {
		t.Errorf("socketDir on windows = %q, want \"\"", got)
	}
}

func TestHEKAHOMEShiftsBase(t *testing.T) {
	cfg, err := Load(map[string]string{
		"LOCALAPPDATA": winLocalAppData,
		"HEKA_HOME":    `D:\heka`,
	}, `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != `D:\heka` {
		t.Errorf("DataDir = %q, want D:\\heka", cfg.DataDir)
	}
	if cfg.TasksDir != `D:\heka\tasks` {
		t.Errorf("TasksDir = %q, want D:\\heka\\tasks", cfg.TasksDir)
	}
}

func TestEnvOverridesBeatEverything(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", "data_dir: /from/file\n")
	cfg, err := Load(map[string]string{
		"HEKA_CONFIG":    filepath.Join(dir, "config.yaml"),
		"HEKA_DATA_DIR":  filepath.Join(dir, "data"),
		"HEKA_TASKS_DIR": filepath.Join(dir, "tasks"),
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != filepath.Join(dir, "data") {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
	if cfg.TasksDir != filepath.Join(dir, "tasks") {
		t.Errorf("TasksDir = %q", cfg.TasksDir)
	}
}

func TestConfigFileOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "config.yaml", `
data_dir: /cfg/data
tasks_dir: /cfg/tasks
log_retention_days: 7
max_output_bytes: 4096
`)
	cfg, err := Load(map[string]string{"HEKA_CONFIG": filepath.Join(dir, "config.yaml")}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != absClean(t, "/cfg/data") {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
	if cfg.LogRetentionDays != 7 {
		t.Errorf("LogRetentionDays = %d, want 7", cfg.LogRetentionDays)
	}
	if cfg.MaxOutputBytes != 4096 {
		t.Errorf("MaxOutputBytes = %d, want 4096", cfg.MaxOutputBytes)
	}
}

func TestConfigFileDefaultLocation(t *testing.T) {
	// Without HEKA_CONFIG, the file lives at <data>/config.yaml — which must
	// follow the env-level data dir override.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "heka")
	writeConfig(t, dir, filepath.Join("heka", "config.yaml"), "tasks_dir: /cfg/tasks\n")
	cfg, err := Load(map[string]string{
		"LOCALAPPDATA":  dir,
		"HEKA_DATA_DIR": dataDir,
	}, `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TasksDir != absClean(t, "/cfg/tasks") {
		t.Errorf("TasksDir = %q, want config file value", cfg.TasksDir)
	}
}

func TestConfigFileErrors(t *testing.T) {
	for name, body := range map[string]string{
		"unknown key": "unknown_key: true\n",
		"bad version": "version: 2\n",
		"bad yaml":    "data_dir: [unclosed\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "config.yaml", body)
			if _, err := Load(map[string]string{"HEKA_CONFIG": filepath.Join(dir, "config.yaml")}, dir); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestValidation(t *testing.T) {
	for name, body := range map[string]string{
		"retention zero":  "log_retention_days: 0\n",
		"output negative": "max_output_bytes: -1\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "config.yaml", body)
			if _, err := Load(map[string]string{"HEKA_CONFIG": filepath.Join(dir, "config.yaml")}, dir); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestMissingConfigFileIsDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(map[string]string{"LOCALAPPDATA": dir}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != filepath.Join(dir, "heka") {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
}

func writeConfig(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// clean normalizes a "/" path literal to the host platform's separators.
func clean(p string) string {
	return filepath.Clean(filepath.FromSlash(p))
}

// absClean mirrors Load's absolutize step for "/" path literals.
func absClean(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(filepath.FromSlash(p))
	if err != nil {
		t.Fatal(err)
	}
	return a
}