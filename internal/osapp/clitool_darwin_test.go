//go:build darwin

package osapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileForShell(t *testing.T) {
	home, _ := os.UserHomeDir()
	cases := []struct {
		shell string
		file  string
		fish  bool
	}{
		{"/bin/zsh", filepath.Join(home, ".zshrc"), false},
		{"/bin/bash", filepath.Join(home, ".bashrc"), false},
		{"/usr/local/bin/fish", "", true},
		{"/opt/homebrew/bin/fish", "", true},
		{"", filepath.Join(home, ".zshrc"), false}, // unknown → zsh fallback
	}
	for _, c := range cases {
		got := profileForShell(c.shell)
		if got.file != c.file || got.fish != c.fish {
			t.Errorf("profileForShell(%q) = {%q %v}, want {%q %v}", c.shell, got.file, got.fish, c.file, c.fish)
		}
	}
}

func TestAppendPathLineIdempotent(t *testing.T) {
	profile := filepath.Join(t.TempDir(), ".zshrc")
	if err := appendPathLine(profile); err != nil {
		t.Fatalf("append: %v", err)
	}
	first, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), hekaPathMarker) {
		t.Errorf("first write missing marker:\n%s", first)
	}
	if err := appendPathLine(profile); err != nil {
		t.Fatalf("re-append: %v", err)
	}
	second, _ := os.ReadFile(profile)
	if string(second) != string(first) {
		t.Errorf("re-append changed the file:\nbefore: %q\nafter:  %q", first, second)
	}
}

func TestAppendPathLineAppendsToExisting(t *testing.T) {
	profile := filepath.Join(t.TempDir(), ".zshrc")
	existing := "# my config\nexport EDITOR=vim\n"
	if err := os.WriteFile(profile, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appendPathLine(profile); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, _ := os.ReadFile(profile)
	got := string(data)
	if !strings.HasPrefix(got, existing) {
		t.Errorf("existing content disturbed:\n%q", got)
	}
	if !strings.Contains(got, hekaPathMarker) {
		t.Errorf("marker missing:\n%q", got)
	}
}

func TestRemovePathLine(t *testing.T) {
	profile := filepath.Join(t.TempDir(), ".zshrc")
	content := "export EDITOR=vim\n# Heka CLI\nexport PATH=\"/Users/x/Library/Application Support/Heka/bin:$PATH\"\nexport FOO=1\n"
	if err := os.WriteFile(profile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removePathLine(profile); err != nil {
		t.Fatalf("remove: %v", err)
	}
	data, _ := os.ReadFile(profile)
	got := string(data)
	want := "export EDITOR=vim\nexport FOO=1\n"
	if got != want {
		t.Errorf("removePathLine got %q, want %q", got, want)
	}
	// Removing again is a no-op.
	if err := removePathLine(profile); err != nil {
		t.Fatalf("re-remove: %v", err)
	}
	data2, _ := os.ReadFile(profile)
	if string(data2) != want {
		t.Errorf("re-remove changed content: %q", data2)
	}
}

func TestRemovePathLineMissingFile(t *testing.T) {
	if err := removePathLine(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Errorf("removePathLine on missing file: %v", err)
	}
}

func TestEnableDisableCLI(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	// Redirect HOME so the symlink lands in a temp dir; the PATH writer
	// runs against the real home's .zshrc — acceptable on the dev machine,
	// but keep the write scoped by pointing profileForShell's home at a
	// temp dir via t.Setenv("HOME", ...).
	home := t.TempDir()
	t.Setenv("HOME", home)

	profile, err := EnableCLI()
	if err != nil {
		t.Fatalf("EnableCLI: %v", err)
	}
	if profile == "" {
		t.Fatal("EnableCLI returned empty profile")
	}

	linkOK, pathOK, binDir := CLIToolStatus()
	if !linkOK {
		t.Error("symlink not reported after EnableCLI")
	}
	if !pathOK {
		t.Error("PATH entry not reported after EnableCLI")
	}
	wantBin := filepath.Join(home, "Library", "Application Support", "Heka", "bin")
	if binDir != wantBin {
		t.Errorf("binDir = %q, want %q", binDir, wantBin)
	}
	if target, err := os.Readlink(filepath.Join(binDir, "heka")); err != nil {
		t.Errorf("readlink: %v", err)
	} else if _, err := os.Stat(target); err != nil {
		t.Errorf("symlink target %q is dangling: %v", target, err)
	}

	if err := DisableCLI(); err != nil {
		t.Fatalf("DisableCLI: %v", err)
	}
	linkOK, pathOK, _ = CLIToolStatus()
	if linkOK {
		t.Error("symlink still present after DisableCLI")
	}
	if pathOK {
		t.Error("PATH entry still present after DisableCLI")
	}
}
