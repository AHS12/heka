package task

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadError pairs a file with its load/validation error. Scan never aborts on
// one bad file (SPEC-04 §4).
type LoadError struct {
	File string
	Err  error
}

func (e LoadError) Error() string {
	return fmt.Sprintf("%s: %v", e.File, e.Err)
}

// Scan loads every *.yaml in dir, sorted by name. Tasks are returned only if
// they parse, validate, and match their filename; duplicate slugs exclude all
// involved files (reported as load errors).
func Scan(dir string) ([]Task, []LoadError) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []LoadError{{File: dir, Err: err}}
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)

	var loaded []Task
	var loadErrs []LoadError
	for _, f := range files {
		t, err := LoadFile(f)
		if err != nil {
			loadErrs = append(loadErrs, LoadError{File: f, Err: err})
			continue
		}
		loaded = append(loaded, t)
	}
	return dedupe(loaded, dir, loadErrs)
}

// dedupe excludes tasks whose slug is defined by more than one successfully
// loaded task, reporting all involved entries as load errors. With the strict
// filename rule this is defense-in-depth (two files can't both match the same
// slug), not a normal path.
func dedupe(loaded []Task, dir string, loadErrs []LoadError) ([]Task, []LoadError) {
	slugs := map[string]int{}
	for _, t := range loaded {
		slugs[t.Slug]++
	}
	var tasks []Task
	for _, t := range loaded {
		if slugs[t.Slug] > 1 {
			loadErrs = append(loadErrs, LoadError{
				File: filepath.Join(dir, t.Slug+".yaml"),
				Err:  fmt.Errorf("duplicate slug %q: multiple task files define it", t.Slug),
			})
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, loadErrs
}

// LoadFile parses and validates a single task file, enforcing the filename
// rule: <slug>.yaml must match the task's slug (SPEC-04 §4).
func LoadFile(path string) (Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Task{}, err
	}
	t, err := Parse(data)
	if err != nil {
		return Task{}, err
	}
	base := filepath.Base(path)
	want := strings.TrimSuffix(base, filepath.Ext(base))
	if t.Slug != want {
		return Task{}, LoadError{
			File: path,
			Err:  fmt.Errorf("slug %q does not match filename %q", t.Slug, base),
		}
	}
	return t, nil
}

// Save writes the canonical YAML atomically (temp + rename) at
// <dir>/<slug>.yaml. It validates before writing.
func Save(dir string, t Task) error {
	out, err := Export(t)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create tasks dir: %w", err)
	}
	path := filepath.Join(dir, t.Slug+".yaml")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Delete removes <dir>/<slug>.yaml.
func Delete(dir, slug string) error {
	path := filepath.Join(dir, slug+".yaml")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete %s: %w", path, err)
	}
	return nil
}

// ResolvedPaths are the executor's absolute paths, resolved against a base
// directory (the task file's dir, SPEC-04 §3).
type ResolvedPaths struct {
	WorkingDir string
	Executable string // script (script tasks) or command (binary tasks)
}

// Resolve applies the path rule from master spec §26: relative paths resolve
// against baseDir (the YAML file's directory).
func (t Task) Resolve(baseDir string) ResolvedPaths {
	wd := t.WorkingDirectory
	switch {
	case wd == "":
		wd = baseDir
	case !filepath.IsAbs(wd):
		wd = filepath.Join(baseDir, wd)
	}
	exec := t.Script
	if t.Type == "binary" {
		exec = t.Command
	}
	if exec != "" && !filepath.IsAbs(exec) {
		exec = filepath.Join(baseDir, exec)
	}
	return ResolvedPaths{
		WorkingDir: filepath.Clean(wd),
		Executable: filepath.Clean(exec),
	}
}
