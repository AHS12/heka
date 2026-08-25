# Go Rules

## Dependencies & Imports
- Import stdlib first, then external, then internal packages — separated by blank lines
- Never import `internal/` packages from outside the module root
- Use `gopkg.in/yaml.v3` for YAML, not `encoding/json` for task serialization

## Error Handling
- IPC errors use `{"code": "…", "message": "…"}` JSON envelope format
- Wrap errors with `fmt.Errorf("context: %w", err)` — never silently swallow errors
- Map domain errors to IPC error codes at the handler boundary, not deep in business logic

## Concurrency
- Run `go test -race` on concurrency-sensitive packages (notify, executor, ipc, daemon)
- Guard shared state with mutexes — goroutine delivery order is nondeterministic
- Use `sync.WaitGroup` for goroutine coordination, not `time.Sleep`

## Platform Code
- Per-OS files: `_windows.go` / `_unix.go` with build tags
- Process group kills: `taskkill /T` (Windows) vs `kill(-pid, sig)` (POSIX)
- Named pipes (Windows) vs unix sockets (POSIX) — never hardcode paths

## Testing
- Mock at interface boundaries, not internal packages
- Use real SQLite (in-memory or temp file) for DB tests
- Use real named pipe/socket for IPC tests
- Use real process spawning for executor tests

## Code Quality
- Remove placeholder/dead code before finishing features
- Single implementation: shared logic lives in one place, used everywhere
- Consumer-side interfaces for dependency injection
- No `var _ = X` placeholder stubs
