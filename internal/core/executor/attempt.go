package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oklog/ulid/v2"

	"heka/internal/core/task"
	"heka/internal/db"
)

// taskkillCmd is the Windows tree killer seam (CREATE_NO_WINDOW so cancel
// never flashes a console). Defined on Windows only; nil elsewhere.
var taskkillCmd func(args ...string) *exec.Cmd

const truncationMarker = "\n…[truncated]\n"

// attemptResult is what one attempt produced; runGroup decides about retries.
type attemptResult struct {
	status   string // success|failed|timed_out|cancelled
	duration time.Duration
	exitCode int
	err      error // group-level error (DB failure), not the task's own result
}

// runAttempt spawns the process once, captures bounded output, enforces the
// timeout, honors cancellation, and persists one runs row (SPEC-05 §4). ga
// tees output into per-run artifacts when configured (nil = DB-only).
func (e *Executor) runAttempt(ctx context.Context, t *task.Task, opt Options, trigger, groupID string, attempt int, ga *groupArtifacts) attemptResult {
	resolved := t.Resolve(opt.BaseDir)
	argv, err := buildArgv(t, resolved)
	if err != nil {
		return e.recordFailed(t, opt, trigger, groupID, attempt, ga, err.Error())
	}
	env, err := buildEnv(e, t)
	if err != nil {
		return e.recordFailed(t, opt, trigger, groupID, attempt, ga, err.Error())
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = resolved.WorkingDir
	cmd.Env = env
	setProcessGroup(cmd) // Windows: CREATE_NO_WINDOW + new process group (SPEC-05 §4)

	stdout := newCappedWriter(e.maxOutput)
	stderr := newCappedWriter(e.maxOutput)
	cmd.Stdout, cmd.Stderr = ga.tee(stdout, stderr)

	if err := verifyExecutable(argv[0], t); err != nil {
		return e.recordFailed(t, opt, trigger, groupID, attempt, ga, err.Error())
	}
	if err := cmd.Start(); err != nil {
		return e.recordFailed(t, opt, trigger, groupID, attempt, ga, err.Error())
	}

	runID := ulid.Make().String()
	started := db.Now()
	startedWall := time.Now()
	row := db.Run{
		RunID: runID, GroupID: groupID, Attempt: attempt,
		TaskSlug: t.Slug, ScheduleID: opt.ScheduleID, Trigger: trigger,
		Status: "running", StartedAt: &started, PID: &cmd.Process.Pid,
		CreatedAt: started,
	}
	if err := e.db.Runs().Create(row); err != nil {
		return attemptResult{err: fmt.Errorf("persist run start: %w", err)}
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	// attemptCtx cancels when the attempt ends, so the cancel watcher below
	// never leaks; group cancellation (Handle.Cancel / ctx) shows up through it.
	attemptCtx, stopWatcher := context.WithCancel(ctx)
	defer stopWatcher()

	var (
		mu       sync.Mutex
		canceled bool
		timedOut bool
	)

	// Timeout: graceful at deadline, escalate after the grace period.
	if t.Timeout > 0 {
		deadline := time.Duration(t.Timeout) * time.Second
		time.AfterFunc(deadline, func() {
			mu.Lock()
			timedOut = true
			mu.Unlock()
			e.terminate(cmd, false)
		})
		time.AfterFunc(deadline+e.grace, func() {
			mu.Lock()
			wasTimeout := timedOut
			mu.Unlock()
			if wasTimeout {
				// Still alive past grace → force kill.
				e.terminate(cmd, true)
			}
		})
	}

	select {
	case <-waitCh:
		// Process exited on its own (or due to a requested termination).
	case <-attemptCtx.Done():
		// Cancellation requested: mark it, terminate, and give the graceful
		// signal a grace window before escalating to force.
		mu.Lock()
		canceled = true
		mu.Unlock()
		e.terminate(cmd, false)
		select {
		case <-waitCh:
		case <-time.After(e.grace):
			e.terminate(cmd, true)
			<-waitCh
		}
	}

	finished := db.Now()
	durationMs := time.Since(startedWall).Milliseconds()
	exitCode := cmd.ProcessState.ExitCode()

	mu.Lock()
	status := runStatus(canceled, timedOut, exitCode)
	mu.Unlock()

	row.Status = status
	row.FinishedAt = &finished
	row.DurationMs = &durationMs
	row.ExitCode = &exitCode
	row.Stdout = stdout.String()
	row.Stderr = stderr.String()
	if err := e.db.Runs().Update(row); err != nil {
		return attemptResult{err: fmt.Errorf("persist run finish: %w", err)}
	}

	return attemptResult{status: status, duration: time.Since(startedWall), exitCode: exitCode}
}

// runStatus applies the SPEC-05 precedence: cancelled > timed_out > exit code.
func runStatus(canceled, timedOut bool, exitCode int) string {
	switch {
	case canceled:
		return "cancelled"
	case timedOut:
		return "timed_out"
	case exitCode != 0:
		return "failed"
	default:
		return "success"
	}
}

// recordFailed persists an attempt that never spawned (validation/env/exec
// errors) with the message as its stderr (PRD §12-style surfaced output). The
// message also lands in the group's stderr.log when artifacts are on.
func (e *Executor) recordFailed(t *task.Task, opt Options, trigger, groupID string, attempt int, ga *groupArtifacts, message string) attemptResult {
	if ga != nil {
		_, _ = ga.stderr.WriteString(message + "\n")
	}
	now := db.Now()
	row := db.Run{
		RunID: ulid.Make().String(), GroupID: groupID, Attempt: attempt,
		TaskSlug: t.Slug, ScheduleID: opt.ScheduleID, Trigger: trigger,
		Status: "failed", StartedAt: &now, FinishedAt: &now,
		Stderr: message, CreatedAt: now,
	}
	if err := e.db.Runs().Create(row); err != nil {
		return attemptResult{err: fmt.Errorf("persist failed start: %w", err)}
	}
	return attemptResult{status: "failed"}
}

// verifyExecutable produces the PRD §12 "not found on this system" error:
// script runtimes are resolved on PATH; direct executables (binary/custom)
// must exist on disk.
func verifyExecutable(arg string, t *task.Task) error {
	if t.Type == "script" && t.Runtime != "custom" {
		if _, err := exec.LookPath(arg); err != nil {
			return fmt.Errorf("%s was not found on this system", arg)
		}
		return nil
	}
	return checkExecutable(arg)
}

// terminate asks the process tree to stop: graceful first, forced on request.
// Windows kills the tree via taskkill /T; POSIX signals the whole process
// group (SPEC-05 §4: grandchildren must not survive).
func (e *Executor) terminate(cmd *exec.Cmd, force bool) {
	if cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if runtime.GOOS == "windows" {
		// The taskkill seam stays nil on non-Windows builds, so guard here;
		// the platform init installs it for real processes.
		if taskkillCmd != nil {
			args := []string{"/PID", strconv.Itoa(pid), "/T"}
			if force {
				args = append(args, "/F")
			}
			_ = taskkillCmd(args...).Run()
		}
		return
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	signalGroup(pid, sig)
}

// cappedWriter captures output into a bounded buffer and marks truncation
// (master spec: no unbounded growth).
type cappedWriter struct {
	buf       bytes.Buffer
	cap       int64
	truncated bool
}

func newCappedWriter(capacity int64) *cappedWriter {
	return &cappedWriter{cap: capacity}
}

// Write keeps the first cap bytes and discards (but counts) the rest.
func (w *cappedWriter) Write(p []byte) (int, error) {
	room := w.cap - int64(w.buf.Len())
	if room > 0 {
		if int64(len(p)) <= room {
			w.buf.Write(p)
		} else {
			w.buf.Write(p[:room])
			w.truncated = true
		}
	} else {
		w.truncated = true
	}
	return len(p), nil // report the full write so io.Copy never errors
}

// String returns the captured output with a truncation marker when capped.
func (w *cappedWriter) String() string {
	if w.truncated {
		out := w.buf.String()
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		return out + truncationMarker
	}
	return w.buf.String()
}
