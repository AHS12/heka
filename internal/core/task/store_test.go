package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const helloYAML = `
version: 1
name: Hello
slug: hello
script: hello.sh
type: script
`

func TestSaveLoadScanDelete(t *testing.T) {
	dir := t.TempDir()
	original, err := Parse([]byte(helloYAML))
	if err != nil {
		t.Fatal(err)
	}

	if err := Save(dir, original); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hello.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}

	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Hello" || loaded.Slug != "hello" {
		t.Fatalf("loaded: %+v", loaded)
	}

	tasks, loadErrs := Scan(dir)
	if len(loadErrs) != 0 {
		t.Fatalf("scan errors: %v", loadErrs)
	}
	if len(tasks) != 1 || tasks[0].Slug != "hello" {
		t.Fatalf("scan: %+v", tasks)
	}

	if err := Delete(dir, "hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file still exists after delete")
	}
	if err := Delete(dir, "hello"); err == nil {
		t.Fatal("delete of missing file should error")
	}
}

func TestScanSurvivesBrokenFile(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, mustParse(t, helloYAML)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("version: 1\nslug: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, loadErrs := Scan(dir)
	if len(tasks) != 1 || tasks[0].Slug != "hello" {
		t.Fatalf("healthy task lost: %+v (errors %v)", tasks, loadErrs)
	}
	if len(loadErrs) != 1 || !strings.Contains(loadErrs[0].Error(), "broken.yaml") {
		t.Fatalf("errors = %v, want one for broken.yaml", loadErrs)
	}
}

func TestDedupe(t *testing.T) {
	// The filename rule makes same-slug files impossible through LoadFile, so
	// dedupe is exercised directly as defense-in-depth.
	dir := t.TempDir()
	a := mustParse(t, helloYAML)
	b := a
	b.Name = "Hello 2"
	b.Script = "hello2.sh"
	tasks, loadErrs := dedupe([]Task{a, b}, dir, nil)
	if len(tasks) != 0 {
		t.Fatalf("duplicate-slug tasks must be excluded, got %+v", tasks)
	}
	if len(loadErrs) != 2 {
		t.Fatalf("want 2 duplicate-slug errors, got %v", loadErrs)
	}
	// Single slugs pass through untouched.
	tasks, loadErrs = dedupe([]Task{a}, dir, nil)
	if len(tasks) != 1 || len(loadErrs) != 0 {
		t.Fatalf("unique slug mishandled: tasks=%v errs=%v", tasks, loadErrs)
	}
}

func TestLoadFileSlugMismatch(t *testing.T) {
	dir := t.TempDir()
	// Filename wrong for the slug inside.
	bad := strings.Replace(helloYAML, "slug: hello", "slug: different", 1)
	path := filepath.Join(dir, "hello.yaml")
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected slug/filename mismatch error")
	}
}

func TestScanMissingDir(t *testing.T) {
	tasks, loadErrs := Scan(filepath.Join(t.TempDir(), "nope"))
	if len(tasks) != 0 || len(loadErrs) != 0 {
		t.Fatalf("missing dir: tasks=%v errs=%v, want both empty", tasks, loadErrs)
	}
}

func TestResolvePaths(t *testing.T) {
	task, err := Parse([]byte(binaryYAML))
	if err != nil {
		t.Fatal(err)
	}
	r := task.Resolve(t.TempDir())
	if !strings.HasSuffix(r.Executable, "bin"+string(filepath.Separator)+"backup.exe") &&
		!strings.HasSuffix(r.Executable, "bin/backup.exe") {
		t.Fatalf("executable = %q", r.Executable)
	}
	// No working directory → base dir.
	if r.WorkingDir == "" {
		t.Fatal("working dir empty")
	}
	// Script tasks resolve the script.
	s, _ := Parse([]byte(helloYAML))
	rs := s.Resolve(t.TempDir())
	if !strings.HasSuffix(rs.Executable, "hello.sh") {
		t.Fatalf("script executable = %q", rs.Executable)
	}
}

func mustParse(t *testing.T, src string) Task {
	t.Helper()
	task, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return task
}
