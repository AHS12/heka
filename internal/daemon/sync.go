package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/oklog/ulid/v2"

	"heka/internal/core/task"
	"heka/internal/db"
)

// syncTasks scans the tasks directory and reconciles the index (SPEC-06 §5):
//
//   - new/changed YAML → upsert index row (id/created_at/enabled preserved)
//   - `enabled` is daemon-side state and is never clobbered by a re-scan
//   - deleted files → index row removed (only for rows under this tasks dir)
//   - broken YAML → skipped from the index, reported, daemon stays up
func (d *Daemon) syncTasks() {
	tasks, loadErrs := task.Scan(d.cfg.TasksDir)
	for _, le := range loadErrs {
		// No logger yet; a console daemon reports per-file errors to stderr.
		fmt.Fprintf(os.Stderr, "heka: %v\n", le)
	}

	present := map[string]bool{}
	for _, t := range tasks {
		present[t.Slug] = true
		// The index stores the full canonical task JSON so IPC handlers can
		// execute it directly (SPEC-07 §3).
		parsed, err := json.Marshal(t)
		if err != nil {
			continue
		}
		row := db.Task{
			ID:         ulid.Make().String(),
			Slug:       t.Slug,
			Name:       t.Name,
			YAMLPath:   filepath.Join(d.cfg.TasksDir, t.Slug+".yaml"),
			ParsedJSON: string(parsed),
			Enabled:    true,
			CreatedAt:  db.Now(),
			UpdatedAt:  db.Now(),
		}
		if existing, err := d.db.Tasks().Get(t.Slug); err == nil {
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			row.Enabled = existing.Enabled // never clobber daemon state
		}
		if err := d.db.Tasks().Save(row); err != nil {
			fmt.Fprintf(os.Stderr, "heka: index %s: %v\n", t.Slug, err)
		}
	}

	existing, err := d.db.Tasks().List()
	if err != nil {
		return
	}
	for _, row := range existing {
		if filepath.Dir(row.YAMLPath) == filepath.Clean(d.cfg.TasksDir) && !present[row.Slug] {
			if err := d.db.Tasks().Delete(row.Slug); err != nil {
				fmt.Fprintf(os.Stderr, "heka: drop index %s: %v\n", row.Slug, err)
			}
		}
	}
}
