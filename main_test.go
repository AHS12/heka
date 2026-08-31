package main

import "testing"

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want mode
	}{
		{"no args -> gui", nil, modeGUI},
		{"gui", []string{"gui"}, modeGUI},
		{"daemon", []string{"daemon"}, modeDaemon},
		// SPEC-08: daemon control moved into the cobra CLI tree.
		{"daemon start", []string{"daemon", "start"}, modeCLI},
		{"daemon status", []string{"daemon", "status"}, modeCLI},
		{"cli list", []string{"list"}, modeCLI},
		{"cli run with args", []string{"run", "daily-research"}, modeCLI},
		{"help command", []string{"help"}, modeCLI},
		{"help short", []string{"-h"}, modeCLI},
		{"help long", []string{"--help"}, modeCLI},
		{"version short", []string{"-v"}, modeCLI},
		{"version long", []string{"--version"}, modeCLI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveMode(tt.args); got != tt.want {
				t.Fatalf("resolveMode(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
