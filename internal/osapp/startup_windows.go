//go:build windows

package osapp

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const startupValueName = "Heka"

// registryStartupRegistrar reads/writes HKCU\...\Run for user-level startup
// (SPEC-15 §3). The registry path is user-scoped — no elevation required.
type registryStartupRegistrar struct{}

func newStartupRegistrar() StartupRegistrar { return &registryStartupRegistrar{} }

// startupPointsAtImpl reports whether the startup Run value references the
// given binary path. A Run value pointing elsewhere (old install dir) must be
// rewritten on upgrade.
func startupPointsAtImpl(exe string) bool {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.READ,
	)
	if err != nil {
		return false
	}
	defer key.Close()
	val, _, err := key.GetStringValue(startupValueName)
	if err != nil {
		return false
	}
	return strings.Contains(val, exe)
}

func (r *registryStartupRegistrar) Enable(exePath string) error {
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()
	// daemon start already detaches with CREATE_NO_WINDOW (control_windows.go).
	cmd := fmt.Sprintf(`"%s" daemon start`, exePath)
	return key.SetStringValue(startupValueName, cmd)
}

func (r *registryStartupRegistrar) Disable() error {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()
	// DeleteValue returns nil if the value doesn't exist.
	_ = key.DeleteValue(startupValueName)
	return nil
}

func (r *registryStartupRegistrar) Enabled() (bool, error) {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.READ,
	)
	if err != nil {
		return false, nil // key doesn't exist → not enabled
	}
	defer key.Close()
	val, _, err := key.GetStringValue(startupValueName)
	if err != nil {
		return false, nil
	}
	return val != "", nil
}
