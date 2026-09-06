package app

import (
	"os"
	"path/filepath"
	"testing"

	"heka/internal/config"
)

func TestChangelogBinding(t *testing.T) {
	a := NewApp("Heka", "0.1.0", "# Changelog\n\n## [0.1.0]")
	if a.Changelog() != "# Changelog\n\n## [0.1.0]" {
		t.Fatalf("Changelog() = %q", a.Changelog())
	}
}

func TestTakeDevTriggerFileConsumesPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trigger.json")
	if err := WriteDevTrigger(path, DevTrigger{Trigger: "whats-new"}); err != nil {
		t.Fatal(err)
	}
	got := TakeDevTriggerFile(path)
	if got.Trigger != "whats-new" || got.Version != "" {
		t.Fatalf("got = %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("trigger file still present: %v", err)
	}
	if _, err := os.Stat(path + ".consuming"); !os.IsNotExist(err) {
		t.Fatalf("leftover .consuming file: %v", err)
	}
	if second := TakeDevTriggerFile(path); second != (DevTrigger{}) {
		t.Fatalf("second read = %+v, want empty", second)
	}
}

func TestTakeDevTriggerFileVersioned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trigger.json")
	if err := WriteDevTrigger(path, DevTrigger{Trigger: "update-from", Version: "0.8.1"}); err != nil {
		t.Fatal(err)
	}
	got := TakeDevTriggerFile(path)
	if got.Trigger != "update-from" || got.Version != "0.8.1" {
		t.Fatalf("got = %+v", got)
	}
}

func TestTakeDevTriggerFileMissing(t *testing.T) {
	if got := TakeDevTriggerFile(filepath.Join(t.TempDir(), "absent.json")); got != (DevTrigger{}) {
		t.Fatalf("got = %+v, want empty", got)
	}
}

func TestTakeDevTriggerFileMalformedAndUnknown(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := TakeDevTriggerFile(path); got != (DevTrigger{}) {
		t.Fatalf("malformed: got = %+v, want empty", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("malformed file was not consumed")
	}

	path = filepath.Join(dir, "unknown.json")
	if err := WriteDevTrigger(path, DevTrigger{Trigger: "launch-missiles"}); err != nil {
		t.Fatal(err)
	}
	if got := TakeDevTriggerFile(path); got != (DevTrigger{}) {
		t.Fatalf("unknown trigger: got = %+v, want empty", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unknown-trigger file was not consumed")
	}
}

func TestAppTakeDevTriggerUsesDataDir(t *testing.T) {
	dir := t.TempDir()
	a := NewApp("Heka", "0.1.0", "")
	a.cfg = &config.Config{DataDir: dir}
	if err := WriteDevTrigger(OnboardingTriggerPath(dir), DevTrigger{Trigger: "tour"}); err != nil {
		t.Fatal(err)
	}
	if got := a.TakeDevTrigger(); got.Trigger != "tour" {
		t.Fatalf("got = %+v, want tour", got)
	}
}
