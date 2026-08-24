# SPEC-02 — Config & Paths

Status: **Draft** · Depends on: SPEC-01 · Master spec: §4 (paths table)

## Goal

Introduce the `internal/config` package that resolves all of Heka's runtime paths and small settings, with a clean precedence chain. Every later spec (DB, tasks, IPC, CLI) reads its locations from this package instead of inventing its own. Nothing here creates directories or has side effects — the daemon does that in SPEC-06.

## Scope

**In:**
- `internal/config`: settings model, resolution, validation
- Env overrides: `HEKA_HOME`, `HEKA_DATA_DIR`, `HEKA_TASKS_DIR`, `HEKA_CONFIG`
- Optional YAML config file with a small known-settings set
- Testable resolution (injected env/home, not real `os.Environ`)
- Wire resolved paths into the daemon stub startup line

**Out:** CLI flags (arrive with SPEC-08; SPEC-02 covers env + config file), settings UI (SPEC-16), directory creation, logging package, IPC listeners (SPEC-07).

## 1. Settings model

```go
package config

type Config struct {
    DataDir          string // sqlite db, production logs
    TasksDir         string // canonical YAML task files
    SocketDir        string // POSIX IPC unix-socket dir; empty on Windows (named pipes)
    LogRetentionDays int    // default 90
    MaxOutputBytes   int64  // per-run stdout/stderr cap, default 1 MiB
}
```

Config file keys mirror the fields: `data_dir`, `tasks_dir`, `log_retention_days`, `max_output_bytes`. An optional `version: 1` key must equal 1 if present. Unknown keys and wrong types are rejected with a clear error (same discipline as the task YAML validator in SPEC-04).

## 2. Precedence

```text
defaults  <  config file  <  environment  <  CLI flags (SPEC-08)
```

1. Platform defaults (table below).
2. Optional config file.
3. Env overrides win over the config file.
4. Flags are future; the seam is a small `type Overrides struct` so SPEC-08 just fills it.

### Default paths (master spec §4)

| | Windows | Linux/macOS |
|---|---|---|
| Data dir | `%LOCALAPPDATA%\heka` | `~/.local/share/heka` (`$XDG_DATA_HOME` respected) |
| Tasks dir | `<data>\tasks` | `<data>/tasks` |
| IPC socket dir | n/a (named pipe) | `$XDG_RUNTIME_DIR` if set, else data dir |
| Config file | `<data>\config.yaml` | `<data>/config.yaml` |

## 3. Env vars

| Var | Meaning |
|---|---|
| `HEKA_HOME` | Override the base data dir (defaults of both dirs shift under it) |
| `HEKA_DATA_DIR` | Explicit data dir (beats `HEKA_HOME` + config file) |
| `HEKA_TASKS_DIR` | Explicit tasks dir |
| `HEKA_CONFIG` | Path to config file; unset → `<data>/config.yaml` |

`HEKA_HOME` + `HEKA_DATA_DIR` both set → `DATA_DIR` wins (it is more specific); no error.

## 4. Testability

`Load` does **not** read `os.Environ` directly — it takes an env map and a `HomeDir` hint:

```go
func Load(env map[string]string, home string) (Config, error)
```

Production passes `envMap(os.Environ())`, `userHome()`. Tests therefore exercise Windows and POSIX resolution on any machine.

## 5. Validation

- `log_retention_days > 0`, `max_output_bytes > 0` (config file values included).
- Config file must parse as YAML; no silent partial loads — a config error is a startup error.

## 6. Wiring

`main.go::runDaemon` prints the resolved settings instead of a bare version line:

```text
heka daemon v0.1.0 starting (foreground)
  data dir:  C:\Users\me\AppData\Local\heka
  tasks dir: C:\Users\me\AppData\Local\heka\tasks
```

Config errors exit non-zero with the message on stderr.

## 7. Files

```text
internal/config/config.go      # model, defaults, precedence, loading, validation
internal/config/config_test.go # platform resolution, precedence, invalid input
main.go                        # runDaemon prints resolved config
```

## 8. Acceptance criteria

1. Tests pass on both platform path tables (via injected env/home), cover precedence (defaults < file < env), env var combos, and invalid config errors.
2. `heka daemon` startup line shows the real resolved dirs; exits non-zero with a clear message on a broken config file.
3. `HEKA_DATA_DIR`, `HEKA_TASKS_DIR`, and `HEKA_HOME` demonstrably change the resolved paths.
4. A config file at `HEKA_CONFIG` overrides defaults; env still wins over the file.
5. `make check` stays green.

## DoD

1. Spec approved by user.
2. Criteria verified, `make check` green.
3. No filesystem writes from the package (`Ensure()` deferred to SPEC-06 daemon wiring).