// Window state persistence: the GUI remembers its size and position across
// launches. The state file lives in the daemon's data directory (resolved by
// main.go through config.LoadDefault) but is pure GUI-local state — the
// daemon never reads or writes it.
package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// MinWindowWidth/MinWindowHeight mirror the shell's Wails MinWidth/MinHeight
// (main.go). Saved geometry below them is clamped up.
const (
	MinWindowWidth  = 800
	MinWindowHeight = 560
)

// windowStateFile is the name of the state file inside the data directory.
const windowStateFile = "window-state.json"

// WindowState is the persisted GUI window geometry. Normal bounds are kept
// separate from the maximized flag so that un-maximizing next session
// restores the real pre-maximize size and position.
type WindowState struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized"`
}

// ErrNoWindowState reports that there is nothing to restore: the file is
// missing, unreadable, corrupt, or holds only zero geometry.
var ErrNoWindowState = errors.New("no window state")

// WindowStatePath returns the state file path inside a data directory.
func WindowStatePath(dataDir string) string {
	return filepath.Join(dataDir, windowStateFile)
}

// LoadWindowState reads the saved geometry, sanitized. Use errors.Is(err,
// ErrNoWindowState) to detect "nothing saved" and fall back to defaults.
func LoadWindowState(path string) (WindowState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return WindowState{}, ErrNoWindowState
	}
	var ws WindowState
	if err := json.Unmarshal(raw, &ws); err != nil {
		return WindowState{}, ErrNoWindowState
	}
	return ws.sanitized()
}

// sanitized clamps degenerate values: all-zero geometry means "nothing
// saved" (e.g. a close-while-maximized on first run), anything below the
// shell minimum is raised to it.
func (ws WindowState) sanitized() (WindowState, error) {
	if ws.Width <= 0 && ws.Height <= 0 {
		return WindowState{}, ErrNoWindowState
	}
	if ws.Width < MinWindowWidth {
		ws.Width = MinWindowWidth
	}
	if ws.Height < MinWindowHeight {
		ws.Height = MinWindowHeight
	}
	return ws, nil
}

// SaveWindowState merges the reported geometry into the state file. When the
// window is maximized at close, only the flag flips — the saved normal
// bounds stay as-is so an un-maximize next launch restores the real previous
// geometry. Best effort: callers may ignore the error.
func SaveWindowState(path string, ws WindowState) error {
	if ws.Maximized {
		if prev, err := LoadWindowState(path); err == nil {
			prev.Maximized = true
			ws = prev
		} else {
			ws = WindowState{Maximized: true}
		}
	}
	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// bounds is an axis-aligned rectangle in screen coordinates.
type bounds struct{ left, top, right, bottom int }

// offScreen reports whether a window rectangle lies entirely outside the
// virtual desktop (all monitors) — e.g. after unplugging a display. The
// check is skipped off-Windows and whenever the OS metrics are unavailable.
func offScreen(x, y, w, h int) bool {
	vb, ok := virtualDesktopBounds()
	return offScreenIn(vb, ok, x, y, w, h)
}

// offScreenIn is the testable core of offScreen: a generous slack absorbs
// DPI rounding differences between the saved geometry and OS monitor bounds.
func offScreenIn(vb bounds, valid bool, x, y, w, h int) bool {
	if !valid || w <= 0 || h <= 0 {
		return false
	}
	const slack = 96
	return x+w < vb.left-slack ||
		x > vb.right+slack ||
		y+h < vb.top-slack ||
		y > vb.bottom+slack
}
