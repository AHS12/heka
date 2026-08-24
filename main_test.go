package main

import "testing"

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want mode
		arg  string
	}{
		{"no args -> gui", nil, modeGUI, ""},
		{"gui", []string{"gui"}, modeGUI, ""},
		{"daemon", []string{"daemon"}, modeDaemon, ""},
		{"daemon start", []string{"daemon", "start"}, modeDaemonControl, "start"},
		{"daemon status", []string{"daemon", "status"}, modeDaemonControl, "status"},
		{"cli list", []string{"list"}, modeCLI, "list"},
		{"cli run with args", []string{"run", "daily-research"}, modeCLI, "run"},
		{"help short", []string{"-h"}, modeHelp, ""},
		{"help long", []string{"--help"}, modeHelp, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, arg := resolveMode(tt.args)
			if got != tt.want || arg != tt.arg {
				t.Fatalf("resolveMode(%v) = (%v, %q), want (%v, %q)",
					tt.args, got, arg, tt.want, tt.arg)
			}
		})
	}
}
