package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-state.json")
	if err := SaveWindowState(path, WindowState{X: 120, Y: 64, Width: 1400, Height: 900}); err != nil {
		t.Fatalf("SaveWindowState: %v", err)
	}
	ws, err := LoadWindowState(path)
	if err != nil {
		t.Fatalf("LoadWindowState: %v", err)
	}
	if ws.X != 120 || ws.Y != 64 || ws.Width != 1400 || ws.Height != 900 || ws.Maximized {
		t.Fatalf("unexpected state: %+v", ws)
	}
}

func TestLoadWindowStateMissingFile(t *testing.T) {
	if _, err := LoadWindowState(filepath.Join(t.TempDir(), "window-state.json")); !errors.Is(err, ErrNoWindowState) {
		t.Fatalf("want ErrNoWindowState, got %v", err)
	}
}

func TestLoadWindowStateCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWindowState(path); !errors.Is(err, ErrNoWindowState) {
		t.Fatalf("want ErrNoWindowState, got %v", err)
	}
}

func TestWindowStateSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-state.json")
	if err := SaveWindowState(path, WindowState{X: 5, Y: 5, Width: 400, Height: 300}); err != nil {
		t.Fatal(err)
	}
	ws, err := LoadWindowState(path)
	if err != nil {
		t.Fatalf("small-but-valid geometry should load: %v", err)
	}
	if ws.Width != MinWindowWidth || ws.Height != MinWindowHeight {
		t.Fatalf("want clamped to minimums, got %+v", ws)
	}

	if err := SaveWindowState(path, WindowState{}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWindowState(path); !errors.Is(err, ErrNoWindowState) {
		t.Fatalf("zero geometry should mean nothing to restore, got %v", err)
	}
}

func TestSaveWindowStateMaximizedKeepsNormalGeometry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-state.json")
	if err := SaveWindowState(path, WindowState{X: 10, Y: 20, Width: 1200, Height: 800}); err != nil {
		t.Fatal(err)
	}
	// While maximized the runtime reports the maximized rect; the saved
	// normal bounds must survive so an un-maximize restores them.
	if err := SaveWindowState(path, WindowState{X: -8, Y: -8, Width: 1920, Height: 1080, Maximized: true}); err != nil {
		t.Fatal(err)
	}
	ws, err := LoadWindowState(path)
	if err != nil {
		t.Fatalf("LoadWindowState: %v", err)
	}
	if ws.X != 10 || ws.Y != 20 || ws.Width != 1200 || ws.Height != 800 || !ws.Maximized {
		t.Fatalf("normal geometry not preserved: %+v", ws)
	}
}

func TestSaveWindowStateMaximizedFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-state.json")
	if err := SaveWindowState(path, WindowState{X: -8, Y: -8, Width: 1920, Height: 1080, Maximized: true}); err != nil {
		t.Fatal(err)
	}
	// Only the flag is known — next launch uses default bounds + maximize.
	if _, err := LoadWindowState(path); !errors.Is(err, ErrNoWindowState) {
		t.Fatalf("want ErrNoWindowState, got %v", err)
	}
}

func TestSaveWindowStateCreatesMissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "window-state.json")
	if err := SaveWindowState(path, WindowState{Width: 1000, Height: 700}); err != nil {
		t.Fatalf("SaveWindowState: %v", err)
	}
	if _, err := LoadWindowState(path); err != nil {
		t.Fatalf("LoadWindowState: %v", err)
	}
}

func TestFitWithin(t *testing.T) {
	cases := []struct {
		name             string
		w, h, maxW, maxH int
		wantW, wantH     int
	}{
		{"fits unchanged", 1310, 940, 1920, 1032, 1310, 940},
		{"exactly fits", 1310, 940, 1310, 940, 1310, 940},
		{"height-bound laptop", 1310, 940, 1366, 720, 1003, 720},
		{"width-bound narrow screen", 1310, 940, 1024, 900, 1024, 735},
		{"scaled down on both axes", 2000, 1000, 1000, 500, 1000, 500},
		{"degenerate window", 0, 0, 1920, 1032, 0, 0},
		{"degenerate screen", 1310, 940, 0, 0, 1310, 940},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := fitWithin(tc.w, tc.h, tc.maxW, tc.maxH)
			if gotW != tc.wantW || gotH != tc.wantH {
				t.Fatalf("fitWithin(%d,%d,%d,%d) = (%d,%d), want (%d,%d)",
					tc.w, tc.h, tc.maxW, tc.maxH, gotW, gotH, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestOffScreenIn(t *testing.T) {
	vb := bounds{left: 0, top: 0, right: 3840, bottom: 1080}
	cases := []struct {
		name       string
		valid      bool
		x, y, w, h int
		want       bool
	}{
		{"on screen", true, 100, 100, 1200, 800, false},
		{"second monitor right of primary", true, 2500, 50, 1200, 800, false},
		{"entirely right of all monitors", true, 5000, 50, 1200, 800, true},
		{"entirely left of all monitors", true, -2000, 50, 1200, 800, true},
		{"entirely below all monitors", true, 100, 2000, 1200, 800, true},
		{"within slack of the edge", true, -60, -60, 1200, 800, false},
		{"no monitor metrics", false, 5000, 5000, 1200, 800, false},
		{"degenerate size", true, 5000, 5000, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := offScreenIn(vb, tc.valid, tc.x, tc.y, tc.w, tc.h); got != tc.want {
				t.Fatalf("offScreenIn = %v, want %v", got, tc.want)
			}
		})
	}
}
