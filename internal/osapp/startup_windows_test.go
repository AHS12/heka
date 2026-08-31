//go:build windows

package osapp

import "testing"

func TestStartupCommand(t *testing.T) {
	exe := `C:\Program Files\Heka\Heka\Heka.exe`
	want := `"C:\Program Files\Heka\Heka\Heka.exe" daemon`
	if got := startupCommand(exe); got != want {
		t.Fatalf("startupCommand() = %q, want %q", got, want)
	}
}

func TestStartupCommandPointsAt(t *testing.T) {
	exe := `C:\Program Files\Heka\Heka\Heka.exe`
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{"direct daemon", `"C:\Program Files\Heka\Heka\Heka.exe" daemon`, true},
		{"path case differs", `"c:\program files\heka\heka\heka.exe" daemon`, true},
		{"surrounding whitespace", `  "C:\Program Files\Heka\Heka\Heka.exe" daemon  `, true},
		{"legacy wrapper", `"C:\Program Files\Heka\Heka\Heka.exe" daemon start`, false},
		{"stale path", `"C:\Old\Heka.exe" daemon`, false},
		{"missing mode", `"C:\Program Files\Heka\Heka\Heka.exe"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := startupCommandPointsAt(tt.command, exe); got != tt.want {
				t.Fatalf("startupCommandPointsAt(%q, %q) = %v, want %v", tt.command, exe, got, tt.want)
			}
		})
	}
}
