package daemon

import (
	"os"
	"path/filepath"

	"heka/internal/core/task"
	"heka/internal/db"
)

// taskFS is the ipc.TaskFilesystem backing task CRUD (SPEC-13 §1): strict
// parse/validate in, atomic canonical YAML write, raw text load, delete.
type taskFS struct {
	dir string
}

func (f taskFS) Parse(yaml []byte) (task.Task, error) {
	return task.Parse(yaml)
}

func (f taskFS) Write(t task.Task) error {
	return task.Save(f.dir, t)
}

func (f taskFS) Load(slug string) (string, error) {
	path := filepath.Join(f.dir, slug+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", db.ErrNotFound
		}
		return "", err
	}
	return string(data), nil
}

func (f taskFS) Remove(slug string) error {
	err := task.Delete(f.dir, slug)
	if err != nil && os.IsNotExist(err) {
		return db.ErrNotFound
	}
	return err
}
