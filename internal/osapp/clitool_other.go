//go:build !darwin

package osapp

// CLI-on-PATH wiring is macOS-only (SPEC-18 §3.1); other platforms keep
// their existing install story.

// EnableCLI is not available off-macOS.
func EnableCLI() (string, error) { return "", ErrCLIToolUnsupported }

// DisableCLI is not available off-macOS.
func DisableCLI() error { return ErrCLIToolUnsupported }

// CLIToolStatus reports nothing off-macOS.
func CLIToolStatus() (linkOK bool, pathOK bool, binDir string) { return false, false, "" }

// CLIToolSupported reports platform availability for the Settings surface.
func CLIToolSupported() bool { return false }
