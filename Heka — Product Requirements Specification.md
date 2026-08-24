# Heka

**A local task runner and scheduler for programmers, built for AI agents.**

# Heka — Product Requirements Specification

## 1. Product Overview

Build a lightweight, local-first desktop application for developers to define, execute, schedule, and monitor scripts and native executables.

Heka is primarily a **persistent background daemon with a GUI management console and CLI clients**.

The GUI is not the runtime. Closing the GUI must never stop the scheduler or currently running tasks.

The application is intended to sit between the operating system and developer/AI-agent workflows.

### Primary Goals

- Run scripts and native binaries.
- Run tasks manually or on a schedule.
- Support recurring schedules and one-time jobs.
- Register schedules with the operating system where appropriate.
- Run continuously in the background/tray.
- Start automatically with the operating system.
- Expose a CLI so external agents and automation systems can trigger tasks by slug.
- Provide a local IPC API shared by the GUI, CLI, and future agent integrations.
- Store execution history and logs.
- Provide a modern, polished desktop GUI.
- Provide both a visual task editor and a YAML editor.
- Keep YAML as the canonical, portable task definition.
- Keep the application substantially simpler than n8n/Windmill.
- Be local-first and usable without an internet connection.

### Non-Goal

Heka is **not** intended to be a general-purpose visual workflow automation platform.

---

# 2. Product Architecture

## 2.1 High-Level Architecture

Heka consists of three primary interfaces around one persistent background daemon:

```text
                         Heka Daemon
                      ┌─────────────────┐
                      │ Scheduler       │
                      │ Task Engine     │
                      │ Executor        │
                      │ Process Manager │
                      │ SQLite          │
                      │ Logs            │
                      │ Notifications   │
                      │ Recovery        │
                      └────────┬────────┘
                               │
                         Local IPC / API
                    ┌──────────┼──────────┐
                    │          │          │
                  GUI        CLI       Agent
                (Wails)    (terminal)  (future)
````

The daemon is the **single authoritative runtime**.

The GUI, CLI, and future agent integrations must not implement separate schedulers or task execution engines.

They are clients of the daemon.

## 2.2 Daemon

The daemon owns:

* scheduling
* task execution
* process lifecycle
* SQLite access
* execution state
* logs
* recovery
* notifications
* OS scheduler registration/reconciliation

The daemon must be able to run indefinitely without the GUI being open.

A user should be able to:

1. Create a task.
2. Create a schedule.
3. Close the GUI.
4. Leave the computer running.
5. Have Heka continue executing the task.

## 2.3 GUI

The GUI is a management client.

It is responsible for:

* creating and editing tasks
* creating schedules
* viewing task state
* viewing execution history
* viewing logs
* configuring notifications
* configuring startup/background behavior
* manually triggering tasks
* importing/exporting YAML

The GUI must not own the scheduler.

Closing the GUI must have no effect on scheduled or currently running tasks.

## 2.4 CLI

The CLI is another client of the daemon.

Examples:

```bash
heka list
heka run daily-research
heka status daily-research
heka logs daily-research
heka enable daily-research
heka disable daily-research
heka schedules
```

The CLI must not start an independent scheduler.

CLI commands communicate with the existing daemon.

## 2.5 Future Agent Integration

Heka should be designed so AI agents can use the same capabilities exposed to the CLI.

For example:

```text
Hermes
   │
   │ local IPC/API
   ▼
Heka Daemon
   │
   ├── run task
   ├── inspect status
   ├── retrieve logs
   └── manage schedules
```

Heka must be **agent-friendly but agent-agnostic**.

It must not depend specifically on Hermes or any particular AI model.

---

# 3. Local IPC

GUI and CLI communication with the daemon should use local IPC rather than a network listener by default.

### Recommended transports

* Windows: Named Pipes
* Linux: Unix Domain Sockets
* macOS: Unix Domain Sockets

The IPC layer should expose a clean, versioned request/response API.

Example logical operations:

```text
GET    /health

GET    /tasks
GET    /tasks/:slug
POST   /tasks
PUT    /tasks/:slug
DELETE /tasks/:slug

POST   /tasks/:slug/run
POST   /tasks/:slug/enable
POST   /tasks/:slug/disable

GET    /tasks/:slug/logs
GET    /tasks/:slug/runs

GET    /schedules
POST   /schedules
DELETE /schedules/:id
```

These are logical API operations.

The transport does not need to be HTTP.

The protocol should be versioned so future clients can remain compatible with newer daemon versions.

## 3.1 Daemon Unavailable

If the GUI or CLI cannot connect to the daemon, it should clearly report that the daemon is unavailable.

Example:

```text
Heka daemon is not running.

Start the daemon:
heka daemon start
```

Where appropriate, the CLI may automatically start the daemon.

The CLI must never create a temporary independent scheduler just to execute a command.

---

# 4. Database Ownership

Only the daemon directly owns Heka's runtime database.

```text
GUI ───► IPC ───► Daemon ───► SQLite
CLI ───► IPC ───► Daemon ───► SQLite
Agent ─► IPC ───► Daemon ───► SQLite
```

The GUI and CLI must not directly modify the SQLite database.

This prevents multiple clients from implementing conflicting state or scheduler behavior.

The database stores:

* task metadata
* schedule metadata
* execution history
* execution status
* exit codes
* timestamps
* logs/metadata
* scheduler state
* failure state
* health/recovery information

Task configuration itself must remain representable as the canonical YAML format.

---

# 5. Process Model

For v1, Heka may use a single executable with multiple modes:

```bash
heka daemon
heka gui
heka <command>
```

Conceptually:

```text
heka.exe
   ├── daemon mode
   ├── GUI mode
   └── CLI mode
```

These are separate processes even if they share the same executable.

Only daemon mode owns:

* scheduler
* task execution
* process management
* runtime state

The GUI and CLI connect to the daemon.

---

# 6. Startup and Background Operation

When enabled, Heka should register the daemon for automatic startup using the native OS mechanism.

At system startup:

```text
Operating System
       │
       ▼
Heka Daemon
       │
       ├── load database
       ├── verify scheduler state
       ├── reconcile OS schedules
       ├── detect missed runs
       └── resume normal scheduling
```

The daemon must not depend on the GUI starting first.

The GUI may remain completely closed while Heka continues operating.

---

# 7. Reliability and Recovery

Heka should provide layered reliability.

1. Persistent daemon independent of the GUI.
2. Native OS startup registration.
3. Native OS scheduler integration where practical.
4. Persistent execution state in SQLite.
5. Scheduler reconciliation after restart.
6. Configurable missed-run handling.
7. Failure recording.
8. Native desktop failure notifications.
9. Individual task failures must not crash the scheduler.

## 7.1 Missed Runs

When the daemon starts after being unavailable, it should determine whether scheduled executions were missed.

For v1, support:

```text
Skip missed run
Run once immediately
```

Future versions may support more advanced policies.

## 7.2 Health State

The GUI should expose daemon and scheduler health.

Example:

```text
Heka

● Daemon: Healthy
● Scheduler: Running

Last heartbeat:
12 seconds ago

Next task:
Daily Research
08:00

Last execution:
Daily Research
✓ Success
```

Each schedule should expose:

```text
Status
Last Run
Next Run
Failures
Missed Runs
```

## 7.3 Failure Notifications

Heka should support native desktop notifications for:

* task failure
* task timeout
* repeated failure
* daemon failure
* scheduler failure

Notifications must not depend on an AI agent being online.

External notification services such as Telegram or email should not be required by the core.

Users can implement external notifications through their own scripts or future Heka integrations.

---

# 8. Technology Stack

## Desktop Shell

Use:

**Wails**

## Frontend

Use:

* React
* TypeScript
* HeroUI
* Tailwind CSS

The UI should feel like a modern native desktop application rather than a web dashboard.

## Backend

Use **Go through Wails**.

Responsibilities:

* task execution
* process management
* scheduler
* filesystem operations
* configuration
* SQLite access
* logging
* tray integration
* startup registration
* CLI integration
* background daemon/service lifecycle
* OS-level scheduling and startup integration
* local IPC

## Database

Use:

**SQLite**

Do not introduce a separate database server.

---

# 9. Core Concepts

Heka has two primary user concepts:

1. Task
2. Schedule

---

## 9.1 Task

A Task is an executable unit.

A task can execute:

1. A script.
2. A native executable/binary.

Examples:

* PowerShell script
* Python script
* Bash script
* Node.js script
* `.exe`
* compiled Go/Rust application
* any other executable supported by the operating system

Heka does not need to understand the internal logic of the program.

It executes the configured command and captures its result.

---

## 9.2 Schedule

A Schedule associates a task with an execution rule.

Examples:

* Every 5 minutes
* Every hour
* Every day at 08:00
* Every Monday at 09:00
* Cron expression

A schedule must have a unique human-readable slug.

Example:

```text
daily-ai-research
```

The slug is used by the CLI and API.

---

# 10. Task Definition

Tasks should be represented by YAML files.

The **YAML task definition is the canonical, portable representation of a Heka task**.

Example:

```yaml
version: 1

name: Daily AI Research
slug: daily-ai-research

type: script

runtime: powershell

script: ./scripts/daily-ai-research.ps1

working_directory: ./scripts

environment:
  OPENROUTER_API_KEY: ${OPENROUTER_API_KEY}

timeout: 300

capture_output: true
```

Binary example:

```yaml
version: 1

name: Database Backup
slug: database-backup

type: binary

command: ./bin/backup.exe

args:
  - "--database"
  - "main"

working_directory: ./bin

timeout: 600

capture_output: true
```

The exact YAML schema must be versioned.

Every task file must contain:

```yaml
version: 1
```

---

# 11. Visual Task Editor

Users must not be required to write YAML.

Heka should provide a visual form editor that maps directly to the YAML schema.

The two interfaces are:

```text
[ Form ]    [ YAML ]
```

Both operate on the same task definition.

Architecture:

```text
Visual Form
     │
     ▼
YAML Definition
     │
     ▼
Heka Core
```

The editor must support bidirectional conversion:

```text
Form → YAML
YAML → Form
```

A valid YAML definition must be renderable back into the visual editor.

## 11.1 Visual Form

The visual editor should expose common task properties through structured controls.

Example:

```text
Name:        Daily Research
Slug:        daily-research

Type:        Script
Runtime:     PowerShell
Script:      ./scripts/research.ps1

Arguments:
    --output
    ./results

Timeout:
    5 minutes

Schedule:
    Every day
    08:00
```

The visual editor should support at minimum:

* task name
* slug
* task type
* executable/script selection
* runtime/interpreter selection
* arguments
* environment variables
* working directory
* timeout
* retry policy
* schedule
* enabled/disabled state
* notification settings
* missed-run policy

## 11.2 YAML Editor

Advanced users can switch to a YAML editor and directly modify the canonical task definition.

Changes must be validated before being applied.

The YAML editor should provide useful syntax and schema errors.

Example:

```text
Invalid task definition

timeout:
  must be a positive integer
```

## 11.3 Schema Validation

Validation should detect:

* malformed YAML
* unknown fields
* missing required fields
* invalid field types
* invalid enum values
* invalid schedules
* invalid timeout values
* invalid retry values

The visual editor must not silently discard fields from a valid YAML definition.

If an advanced field cannot yet be represented visually, Heka should preserve it where possible and clearly identify it as an advanced/unsupported visual field.

## 11.4 Source of Truth

Heka must not maintain a separate hidden task representation that becomes the real source of truth.

The YAML schema is the canonical task configuration format.

The database may store:

* execution history
* logs
* runtime state
* scheduler state
* metadata

but task configuration must remain representable as YAML.

This makes tasks:

* portable
* inspectable
* version-control friendly
* easy for programmers to edit
* easy for AI agents to generate
* easy to import/export

---

# 12. Script Tasks

The application must allow users to select a script file.

The user specifies the runtime.

Examples:

```text
PowerShell
Python
Node.js
Bash
Custom command
```

Heka should not bundle these runtimes.

Instead, it executes the user's installed runtime.

Examples:

```text
powershell.exe -File script.ps1
```

```text
python script.py
```

```text
node script.js
```

The application should detect whether the configured runtime exists.

If unavailable, display a clear error.

Example:

> Python was not found on this system.

---

# 13. Binary Tasks

Allow users to select a native executable.

Windows:

```text
.exe
```

Linux/macOS:

Use native executable files with appropriate executable permissions.

The application should support:

* arguments
* environment variables
* working directory
* timeout
* stdout capture
* stderr capture

Heka should not assume that an executable is safe.

Display a confirmation when a new executable is added.

---

# 14. Environment Variables and Secrets

Tasks may define environment variables.

Support references such as:

```yaml
environment:
  OPENROUTER_API_KEY: ${OPENROUTER_API_KEY}
```

Do not store secrets directly inside task YAML where avoidable.

Provide a local application-level environment/secret store.

The GUI should display secret values as masked.

Example:

```text
OPENROUTER_API_KEY
••••••••••••••••
```

---

# 15. Manual Task Execution

Every task must have:

**Run Now**

The user can execute any task manually.

Display:

* start time
* elapsed time
* current status
* stdout
* stderr
* exit code

Statuses:

```text
Queued
Running
Success
Failed
Timed Out
Cancelled
```

---

# 16. One-Time Jobs

Provide a concept for one-time task execution.

A user can:

1. Select a task.
2. Select a date/time.
3. Create a one-time job.

Example:

```text
Run "database-backup"

Tomorrow
02:00

[Schedule]
```

The job should appear in the Jobs view.

Once completed, it becomes historical rather than remaining as an active recurring schedule.

---

# 17. Recurring Schedules

Allow recurring schedules.

Minimum supported options:

* Every N minutes
* Every N hours
* Daily
* Weekly
* Monthly
* Cron expression

Example:

```text
Task:
daily-ai-research

Schedule:
Every day
08:00
```

Each schedule must have:

* ID
* slug
* task reference
* schedule expression
* enabled/disabled state
* next execution time
* last execution time

---

# 18. Schedule Slugs

Every schedule must have a unique slug.

Examples:

```text
daily-ai-research
github-trending
backup-database
telegram-report
```

The CLI uses the slug.

Example:

```bash
heka run daily-ai-research
```

The CLI must not require the GUI to be open.

The daemon handles the request.

---

# 19. CLI

Provide a dedicated CLI.

Examples:

```bash
heka list
heka run daily-ai-research
heka status daily-ai-research
heka logs daily-ai-research
heka enable daily-ai-research
heka disable daily-ai-research
```

Recommended commands:

```text
heka list
heka run <slug>
heka status <slug>
heka logs <slug>
heka schedules
heka enable <slug>
heka disable <slug>

heka daemon status
heka daemon start
heka daemon stop
```

CLI output should support:

1. Human-readable output.
2. JSON output.

Example:

```bash
heka run daily-ai-research --json
```

Output:

```json
{
  "success": true,
  "slug": "daily-ai-research",
  "run_id": "01J...",
  "status": "queued"
}
```

JSON mode is important for AI agents.

---

# 20. Agent-Friendly CLI

The CLI should be designed so an AI agent can safely interact with Heka.

Example:

```bash
heka run daily-ai-research --json
```

```json
{
  "success": true,
  "run_id": "abc123",
  "status": "queued"
}
```

Then:

```bash
heka status daily-ai-research --json
```

The agent can inspect the result.

The agent should also be able to retrieve logs:

```bash
heka logs daily-ai-research --json
```

The CLI should avoid output that is difficult for an agent to parse.

---

# 21. Process Execution

Use the Go process execution APIs.

The application should:

1. Spawn the configured process.
2. Capture stdout.
3. Capture stderr.
4. Track process ID.
5. Track start time.
6. Track end time.
7. Track exit code.
8. Enforce timeout.
9. Allow cancellation.
10. Store execution metadata.

Example:

```text
Task
 ↓
Process
 ↓
stdout ──┐
stderr ──┼──► Heka
exit code┘
 ↓
Execution Record
```

Heka should execute arbitrary developer commands without needing to understand their internal behavior.

For example:

```bash
curl https://example.com/api
```

is a valid task operation.

Heka simply executes the process and records the result.

---

# 22. Shell vs Direct Execution

Prefer direct process execution rather than unnecessary shell interpretation.

For example, prefer:

```text
python script.py
```

over:

```text
cmd.exe /c "python script.py"
```

unless shell behavior is explicitly requested.

This reduces command injection and quoting problems.

A future task option may explicitly request shell execution.

---

# 23. Concurrency

Different tasks may execute concurrently.

Example:

```text
Task A ──────────────► Running
Task B ─────► Running
Task C ──────────────► Running
```

The scheduler must not block the entire system while one task runs.

However, the same task should **not overlap with itself by default**.

Example:

```text
Daily Research
│
├── 08:00 → Running
│
└── 08:05 → skipped/queued because previous run is active
```

The behavior should be configurable in a future version.

---

# 24. Process Cancellation

Users should be able to cancel a running task.

The application should attempt graceful termination first.

If the process does not terminate, the application may forcefully terminate it.

The execution should be recorded as:

```text
Cancelled
```

---

# 25. Timeouts

Tasks may define a timeout.

Example:

```yaml
timeout: 300
```

Timeout is specified in seconds.

When a timeout occurs:

1. Attempt graceful termination.
2. Force termination if necessary.
3. Record the execution as `Timed Out`.
4. Capture available output.
5. Trigger configured notifications.

---

# 26. Working Directory

Tasks may define a working directory.

Example:

```yaml
working_directory: ./scripts
```

Relative paths should be resolved relative to the task/project context according to a clearly documented rule.

Absolute paths should be supported where permitted by the operating system.

---

# 27. Logs

Store execution logs.

Each execution should have:

```text
run_id
task
start_time
end_time
duration
status
exit_code
stdout
stderr
```

The GUI should provide a log viewer.

Example:

```text
Daily AI Research
────────────────────────────

08:00:01 Starting
08:00:02 Fetching data...
08:00:05 Processing...
08:00:12 Complete

Exit code: 0
Duration: 11.2s
```

The CLI should provide access to the same logs.

---

# 28. Execution History

Provide an execution history view.

Example:

```text
Daily AI Research

Today 08:00    ✓ Success     12s
Yesterday      ✓ Success     10s
Aug 22         ✗ Failed       3s
Aug 21         ✓ Success      9s
```

Allow users to inspect individual runs.

---

# 29. Failure Handling

A failed task must not crash the scheduler.

Example:

```text
Task A → FAILED
Task B → SUCCESS
Task C → SUCCESS
```

The scheduler continues.

Store failure details.

For v1, support:

```text
No retry
Retry N times
Retry after N seconds
```

Do not implement complex workflow retry logic unless required.

---

# 30. Notifications

Optional native desktop notifications:

```text
Task completed
Task failed
Task timed out
```

Allow per-task notification configuration.

Two channel types are first-class in the MVP:

1. **Native desktop notifications.**
2. **Outgoing webhooks**: Slack, Discord, Pumble (Slack-compatible), Telegram (Bot API), and a generic HTTP endpoint.

A per-task `notify_on` setting enables `success` / `failure` / `timeout` notifications; an empty setting disables all channels for that task.

Webhook destinations may reference values in Heka's local secret store, so tokens and URLs never have to appear in task YAML.

Delivery is asynchronous and best-effort: a failed webhook call is logged by the daemon and never affects the task's execution status or retries.

Users can still implement fully custom external integrations in their own scripts; the built-in channels just remove the boilerplate.

---

# 31. Security

The application executes arbitrary code.

Treat every task as trusted code.

Important protections:

* show a clear warning when adding a new executable/script
* never silently execute an unknown downloaded binary
* avoid unnecessary shell interpretation
* do not expose the local API to the network by default
* protect secrets
* validate task paths
* prevent accidental command injection in GUI-generated arguments
* use operating-system access controls for the local IPC endpoint

The default installation must be local-only.

---

# 32. Local API

The primary local integration mechanism is IPC.

The daemon should expose a clean internal API that can later support an optional local HTTP API.

Potential future endpoints:

```text
GET  /api/tasks
POST /api/tasks/{slug}/run
GET  /api/tasks/{slug}/status
GET  /api/tasks/{slug}/runs
GET  /api/runs/{run_id}
```

Do not expose an unauthenticated network API by default.

If network access is introduced in a future version, it must have explicit authentication and authorization.

---

# 33. Graph/Workflow Support

Graphs are **not required for v1**.

Do not build a visual workflow editor.

If workflow visualization is eventually needed, use:

**XYFlow**

Possible future representation:

```text
Fetch Data
     ↓
Process
     ↓
Save
     ↓
Notify
```

However, v1 should remain focused on executing standalone tasks.

A task can execute an arbitrary script, which already allows users to implement complex logic without requiring a visual workflow engine.

---

# 34. Non-Goals

Do NOT turn the first version into:

* an n8n clone
* a Windmill clone
* a full RPA platform
* a visual programming environment
* a SaaS automation service
* a cloud workflow platform
* an LLM agent framework
* a secrets management platform
* a container orchestration platform

The product should remain a:

> **Local developer task runner and scheduler.**

---

# 35. Suggested Project Structure

```text
heka/
│
├── app/
│   ├── main.go
│   ├── core/
│   │   ├── tasks/
│   │   ├── scheduler/
│   │   ├── executor/
│   │   ├── database/
│   │   ├── recovery/
│   │   ├── notifications/
│   │   ├── ipc/
│   │   └── os/
│   │
│   ├── daemon/
│   ├── cli/
│   └── go.mod
│
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   ├── hooks/
│   │   ├── lib/
│   │   ├── types/
│   │   └── App.tsx
│   │
│   ├── package.json
│   └── ...
│
├── tasks/
│   └── example.yaml
│
└── README.md
```

---

# 36. MVP Acceptance Criteria

The first usable version is complete when a user can:

1. Install Heka.
2. Start the Heka daemon.
3. Launch the GUI.
4. Create a script task through the visual form.
5. Edit the same task as YAML.
6. Import a YAML task.
7. Export a task as YAML.
8. Select a PowerShell/Python/etc. script.
9. Run it manually.
10. See stdout/stderr.
11. See the exit code.
12. Create a recurring schedule.
13. Create a one-time job.
14. Close the GUI while the daemon continues running.
15. See Heka in the system tray.
16. Enable OS startup.
17. Trigger a task through CLI using its slug.
18. Query task status through CLI.
19. View execution history.
20. View failed execution details.
21. Run a native executable.
22. Run multiple different tasks concurrently.
23. Prevent overlapping executions of the same task by default.
24. Recover scheduling after daemon/system restart.
25. Detect missed scheduled executions.
26. Send a native notification when a configured task fails.
27. Operate entirely locally without requiring a cloud account.
28. Allow an AI agent to trigger a task and retrieve structured JSON results.

---

# 37. Design Principle

The most important architectural principle is:

> **Heka schedules and executes. Scripts decide what the work actually does.**

The application should remain intentionally generic.

For example, Heka should not know what "OpenRouter research" means.

It only knows:

```text
Task:
openrouter-research

Executable:
powershell.exe

Script:
./scripts/openrouter-research.ps1

Schedule:
Every 6 hours
```

The script handles the actual work.

This makes Heka useful for:

* developers
* system administration
* backups
* data collection
* local AI workflows
* CI-like jobs
* monitoring
* personal automation
* AI-agent tool execution

without requiring specialized integrations for each use case.

