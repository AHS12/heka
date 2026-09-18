//go:build darwin

package osapp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CLI-on-PATH wiring, Herd-style (SPEC-18 §3.1): a stable symlink in
// ~/Library/Application Support/Heka/bin → the app bundle's binary, plus one
// idempotent PATH line in the user's shell profile. No sudo, no brew.

// pathMarker identifies Heka's PATH lines — enable/disable/status all match
// on this substring so re-runs never duplicate the entry.
const hekaPathMarker = "Application Support/Heka/bin"

// CLIBinDir is the directory that lands on PATH.
func CLIBinDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Heka", "bin")
}

// cliSymlinkTarget picks what the symlink points at: the installed app
// bundle when present (follows app updates with no reinstall), else the
// running executable (dev runs, non-standard install locations).
func cliSymlinkTarget() string {
	if _, err := os.Stat("/Applications/Heka.app/Contents/MacOS/heka"); err == nil {
		return "/Applications/Heka.app/Contents/MacOS/heka"
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "/Applications/Heka.app/Contents/MacOS/heka"
}

// cliProfile describes where the PATH line goes for the invoking shell.
type cliProfile struct {
	file string // rc file the PATH line lives in ("" for fish)
	fish bool
}

// profileForShell maps $SHELL to its rc file. Unknown shells fall back to
// zsh — the macOS default.
func profileForShell(shell string) cliProfile {
	base := filepath.Base(shell)
	home, _ := os.UserHomeDir()
	switch base {
	case "bash":
		return cliProfile{file: filepath.Join(home, ".bashrc")}
	case "fish":
		return cliProfile{fish: true}
	default: // zsh or anything else
		return cliProfile{file: filepath.Join(home, ".zshrc")}
	}
}

// currentProfile returns the profile for the process's login shell.
func currentProfile() cliProfile {
	return profileForShell(os.Getenv("SHELL"))
}

// profileLine reports whether the profile already carries Heka's PATH entry.
func profileLine(p cliProfile) (bool, error) {
	if p.fish {
		return fishPathPresent(), nil
	}
	data, err := os.ReadFile(p.file)
	if err != nil {
		return false, nil // no rc file yet — not present
	}
	return strings.Contains(string(data), hekaPathMarker), nil
}

func fishPathPresent() bool {
	fish, err := exec.LookPath("fish")
	if err != nil {
		return false
	}
	dir := CLIBinDir()
	cmd := exec.Command(fish, "-c", fmt.Sprintf(`string match -q -- %q $fish_user_paths`, dir))
	return cmd.Run() == nil
}

// appendPathLine adds the marker-commented PATH export to an rc file,
// skipping files that already carry it (idempotent across re-runs).
func appendPathLine(profile string) error {
	if data, err := os.ReadFile(profile); err == nil && strings.Contains(string(data), hekaPathMarker) {
		return nil
	}
	entry := fmt.Sprintf("\n# Heka CLI\nexport PATH=%q:$PATH\n", CLIBinDir())
	f, err := os.OpenFile(profile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}

// removePathLine strips every line that references Heka's bin dir (both the
// marker comment and the export). Never touches anything else.
func removePathLine(profile string) error {
	data, err := os.ReadFile(profile)
	if err != nil {
		return nil // no file — already clean
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, hekaPathMarker) || strings.TrimSpace(line) == "# Heka CLI" {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.Join(kept, "\n")
	if out == string(data) {
		return nil
	}
	return os.WriteFile(profile, []byte(out), 0o600)
}

func fishAddPath() error {
	fish, err := exec.LookPath("fish")
	if err != nil {
		return fmt.Errorf("fish not found: %w", err)
	}
	dir := CLIBinDir()
	cmd := exec.Command(fish, "-c", fmt.Sprintf(`fish_add_path -U %q`, dir))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("fish_add_path: %w: %s", err, out)
	}
	return nil
}

func fishRemovePath() error {
	fish, err := exec.LookPath("fish")
	if err != nil {
		return nil // nothing to remove without fish
	}
	dir := CLIBinDir()
	cmd := exec.Command(fish, "-c", fmt.Sprintf(`set -U fish_user_paths (string match -v -- %q $fish_user_paths)`, dir))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("fish_remove_path: %w: %s", err, out)
	}
	return nil
}

// EnableCLI installs the symlink and wires the PATH. Returns the profile
// path (or "fish") so the UI can show what changed.
func EnableCLI() (string, error) {
	binDir := CLIBinDir()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	link := filepath.Join(binDir, "heka")
	target := cliSymlinkTarget()
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("replace heka link: %w", err)
	}
	if err := os.Symlink(target, link); err != nil {
		return "", fmt.Errorf("symlink heka: %w", err)
	}
	p := currentProfile()
	if p.fish {
		if err := fishAddPath(); err != nil {
			return "", err
		}
		return "fish (universal PATH)", nil
	}
	if err := appendPathLine(p.file); err != nil {
		return "", err
	}
	return p.file, nil
}

// DisableCLI removes the PATH wiring and the symlink (only when it points at
// a Heka binary — a user-made link is left alone).
func DisableCLI() error {
	p := currentProfile()
	if p.fish {
		if err := fishRemovePath(); err != nil {
			return err
		}
	} else if err := removePathLine(p.file); err != nil {
		return err
	}
	link := filepath.Join(CLIBinDir(), "heka")
	if target, err := os.Readlink(link); err == nil &&
		(strings.Contains(target, "Heka") || target == cliSymlinkTarget()) {
		if err := os.Remove(link); err != nil {
			return err
		}
	}
	// Leave the (empty) bin dir in place: PATH lines pointing at a removed
	// directory are harmless, and re-enabling recreates everything.
	return nil
}

// CLIToolStatus reports the CLI-on-PATH state for the Settings surface.
func CLIToolStatus() (linkOK bool, pathOK bool, binDir string) {
	binDir = CLIBinDir()
	link := filepath.Join(binDir, "heka")
	if target, err := os.Readlink(link); err == nil {
		linkOK = true
		_ = target
		if _, err := os.Stat(link); err != nil {
			linkOK = false // dangling
		}
	}
	p := currentProfile()
	pathOK, _ = profileLine(p)
	return linkOK, pathOK, binDir
}

// CLIToolSupported is true on macOS only (SPEC-18 §3.1).
func CLIToolSupported() bool { return true }
