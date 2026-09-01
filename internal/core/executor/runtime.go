package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"heka/internal/core/task"
)

// buildArgv resolves the OS-level argument vector (SPEC-05 §3). Binary tasks
// run the command verbatim; script tasks prepend the runtime interpreter.
func buildArgv(t *task.Task, resolved task.ResolvedPaths) ([]string, error) {
	if t.Type == "binary" {
		return append([]string{resolved.Executable}, t.Args...), nil
	}
	switch t.Runtime {
	case "powershell":
		if runtime.GOOS == "windows" {
			return append([]string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", resolved.Executable}, t.Args...), nil
		}
		return append([]string{"pwsh", "-NoProfile", "-File", resolved.Executable}, t.Args...), nil
	case "python":
		if runtime.GOOS == "windows" {
			return append([]string{"python", resolved.Executable}, t.Args...), nil
		}
		return append([]string{"python3", resolved.Executable}, t.Args...), nil
	case "node":
		return append([]string{"node", resolved.Executable}, t.Args...), nil
	case "bash":
		if runtime.GOOS == "windows" {
			return nil, fmt.Errorf("bash tasks are not supported on windows")
		}
		return append([]string{"bash", resolved.Executable}, t.Args...), nil
	default: // "custom" — the script is an executable itself.
		return append([]string{resolved.Executable}, t.Args...), nil
	}
}

// buildEnv merges the daemon environment with the task's environment, with
// ${VAR} refs resolved through e.env. Task values win over inherited ones;
// unresolvable refs fail the attempt with a clear message (PRD §12-style).
func buildEnv(e *Executor, t *task.Task) ([]string, error) {
	overrides := map[string]string{}
	var sorted []string
	for key, raw := range t.Environment {
		resolved, err := task.ResolveValue(raw, e.env)
		if err != nil {
			return nil, fmt.Errorf("environment.%s: %w", key, err)
		}
		sorted = append(sorted, key)
		overrides[key] = resolved
	}
	sort.Strings(sorted)

	env := os.Environ()
	seen := map[string]bool{}
	for _, pair := range env {
		for i := 0; i < len(pair); i++ {
			if pair[i] == '=' {
				seen[pair[:i]] = true
				break
			}
		}
	}
	for _, key := range sorted {
		seen[key] = true
		env = append(env, key+"="+overrides[key])
	}
	return env, nil
}

// checkExecutable gives the friendly PRD §12 error for a missing runtime
// before spawn. Direct executables resolve through os/exec at Start time.
func checkExecutable(file string) error {
	if _, err := os.Stat(file); err != nil {
		return fmt.Errorf("%s was not found on this system", filepath.Base(file))
	}
	return nil
}
