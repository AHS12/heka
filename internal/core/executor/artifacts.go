package executor

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// Per-run artifact files (config run_artifacts_dir): every group gets
//   <root>/<groupID>/stdout.log  — appended across attempts
//   <root>/<groupID>/stderr.log  — appended across attempts
//   <root>/<groupID>/run.json    — manifest (group, slug, trigger, attempts)
// DB capture always happens; these files are an additive, navigable view and
// are best-effort (a filesystem failure never fails the run).

// attemptMeta is one entry in the run manifest.
type attemptMeta struct {
	Attempt    int   `json:"attempt"`
	Status     string `json:"status"`
	DurationMs int64 `json:"duration_ms,omitempty"`
	ExitCode   int   `json:"exit_code,omitempty"`
}

// runManifest is written when the group finishes.
type runManifest struct {
	GroupID    string        `json:"group_id"`
	TaskSlug   string        `json:"task_slug"`
	Trigger    string        `json:"trigger"`
	Attempts   []attemptMeta `json:"attempts"`
	StartedAt  string        `json:"started_at"`
	FinishedAt string        `json:"finished_at"`
}

// groupArtifacts holds the open append streams for one group.
type groupArtifacts struct {
	stdout *os.File
	stderr *os.File
}

// openGroupArtifacts creates the group folder and both log streams under the
// default artifacts root. Returns nil on any failure so the run continues
// with DB-only capture.
func (e *Executor) openGroupArtifacts(groupID string) *groupArtifacts {
	return e.openGroupArtifactsAt(e.artifactsDir, groupID)
}

// openGroupArtifactsAt creates the group folder and both log streams at the
// specified root directory.
func (e *Executor) openGroupArtifactsAt(root, groupID string) *groupArtifacts {
	if root == "" {
		return nil
	}
	dir := filepath.Join(root, groupID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	so, err := os.OpenFile(filepath.Join(dir, "stdout.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil
	}
	se, err := os.OpenFile(filepath.Join(dir, "stderr.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		so.Close()
		return nil
	}
	return &groupArtifacts{stdout: so, stderr: se}
}

// teed writers mix the DB-bound capped buffers with the artifact files.
func (ga *groupArtifacts) tee(stdout, stderr io.Writer) (io.Writer, io.Writer) {
	if ga == nil {
		return stdout, stderr
	}
	return io.MultiWriter(stdout, ga.stdout), io.MultiWriter(stderr, ga.stderr)
}

// writeManifest persists run.json atomically.
func writeManifest(m runManifest, root, groupID string) error {
	dir := filepath.Join(root, groupID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "run.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}