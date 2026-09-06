package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"heka/internal/app"
	"heka/internal/config"
)

func newDevTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	a := NewApp(config.Config{DataDir: dir}, nil)
	return a, dir
}

func TestDevCommandsWriteTriggers(t *testing.T) {
	cases := []struct {
		args []string
		want app.DevTrigger
	}{
		{[]string{"dev", "whats-new"}, app.DevTrigger{Trigger: "whats-new"}},
		{[]string{"dev", "tour"}, app.DevTrigger{Trigger: "tour"}},
		{[]string{"dev", "reset"}, app.DevTrigger{Trigger: "reset"}},
		{[]string{"dev", "update-from", "0.8.1"}, app.DevTrigger{Trigger: "update-from", Version: "0.8.1"}},
	}
	for _, tt := range cases {
		a, dir := newDevTestApp(t)
		if err := a.Execute(tt.args); err != nil {
			t.Fatalf("%v: %v", tt.args, err)
		}
		raw, err := os.ReadFile(app.OnboardingTriggerPath(dir))
		if err != nil {
			t.Fatalf("%v: trigger file missing: %v", tt.args, err)
		}
		var got app.DevTrigger
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("%v: bad JSON %q: %v", tt.args, raw, err)
		}
		if got != tt.want {
			t.Fatalf("%v: got %+v, want %+v", tt.args, got, tt.want)
		}
	}
}

func TestDevUpdateFromRejectsBadVersion(t *testing.T) {
	a, dir := newDevTestApp(t)
	if err := a.Execute([]string{"dev", "update-from", "not-a-version"}); err == nil {
		t.Fatal("want error for invalid version")
	}
	if _, err := os.Stat(app.OnboardingTriggerPath(dir)); !os.IsNotExist(err) {
		t.Fatal("trigger file written for invalid version")
	}
}

func TestDevCommandHiddenFromHelp(t *testing.T) {
	a, _ := newDevTestApp(t)
	var buf strings.Builder
	a.stdout = &buf
	if err := a.Execute([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "dev ") || strings.Contains(buf.String(), "dev\n") {
		t.Fatalf("dev command visible in help:\n%s", buf.String())
	}
}
