// lib/api.ts (SPEC-12 §3) — typed wrappers over the generated Wails
// bindings. Browser JS cannot open named pipes, so every daemon call goes
// through the Go bridge (internal/app); this module is the single place that
// (a) owns the DTO shapes the shell consumes and (b) translates IPC failures
// into typed errors with a `code` (parseable from the "code: message" prefix
// the Go binding slaps on before Wails flattens it to a string).
import {
  Health,
  DaemonStatus,
  StartDaemon,
  ListTasks,
  GetTask,
  CreateTask,
  UpdateTask,
  DeleteTask,
  GetTaskYAML,
  ValidateTaskYAML,
  ParseTaskYAML,
  RunTask,
  SetTaskEnabled,
  ImportTaskFromFile,
  ExportTaskYAML,
  PickScriptFile,
  PickWorkingDir,
  ListSecrets,
  SetSecret,
  DeleteSecret,
  ListSchedules,
  CreateSchedule,
  UpdateSchedule,
  DeleteSchedule,
  EnableSchedule,
  DisableSchedule,
  ListRuns,
  GetRun,
  CancelRun,
  StartupEnabled,
  StartupSet,
  WatchdogEnabled,
  WatchdogSet,
  PauseScheduler,
  ResumeScheduler,
  ReconcileSchedules,
  Stats,
  GetSettings,
  UpdateSettings,
  PreviewSound,
  DataDir,
  TasksDir,
  OpenDataDir,
} from '@wailsjs/go/app/App'
import type {task} from '@wailsjs/go/models'

// HealthDTO mirrors the Go HealthDTO (internal/app). Fields follow the
// generated model's snake_case keys.
export interface HealthDTO {
  version: string
  uptime_seconds: number
  core: string
  scheduler: string
}

export type DaemonStatusValue = 'running' | 'not-running'

/** Envelope codes the Go IPC layer can produce. */
export type ErrorCode =
  | 'daemon_not_running'
  | 'daemon_access_denied'
  | 'daemon_unreachable'
  | 'not_found'
  | 'bad_request'
  | 'method_not_allowed'
  | 'not_implemented'
  | 'internal'
  | 'conflict'
  | 'already_running'
  | 'invalid_task'
  | 'canceled'
  | 'unknown'

export class APIError extends Error {
  code: ErrorCode

  constructor(code: ErrorCode, message: string) {
    super(message)
    this.name = 'APIError'
    this.code = code
  }
}

const KNOWN_CODES: ReadonlySet<string> = new Set([
  'daemon_not_running',
  'daemon_access_denied',
  'daemon_unreachable',
  'not_found',
  'bad_request',
  'method_not_allowed',
  'not_implemented',
  'internal',
  'conflict',
  'already_running',
  'invalid_task',
])

function toAPIError(err: unknown): Error {
  const message = err instanceof Error ? err.message : String(err)
  const coded = /^([a-z_]+):\s?(.*)$/.exec(message)
  if (coded && KNOWN_CODES.has(coded[1])) {
    return new APIError(coded[1] as ErrorCode, coded[2] || message)
  }
  if (message.includes('not running')) {
    return new APIError('daemon_not_running', message)
  }
  if (message === 'dialog canceled') {
    return new APIError('canceled', message)
  }
  return new Error(message)
}

/**
 * Renders an APIError's problem list: 422 invalid_task carries a
 * JSON-encoded list of per-field problems (SPEC-13 §1); anything else is a
 * single item with the raw message.
 */
export function apiErrorDetails(err: unknown): string[] {
  if (err instanceof APIError && err.code === 'invalid_task') {
    try {
      const list = JSON.parse(err.message)
      if (Array.isArray(list) && list.every((x) => typeof x === 'string')) {
        return list as string[]
      }
    } catch {
      // fall through to single-message rendering
    }
  }
  const message = err instanceof Error ? err.message : String(err)
  return message ? [message] : []
}

/** Current daemon health, mapped to the shell DTO (SPEC-12 §1). */
export async function health(): Promise<HealthDTO> {
  try {
    return await Health()
  } catch (err) {
    throw toAPIError(err)
  }
}

/** "running" | "not-running" — the pill/banner inputs (SPEC-12 §3). */
export async function daemonStatus(): Promise<DaemonStatusValue> {
  try {
    // The generated binding is loosely typed as string; the Go method only
    // ever produces the two contract values (SPEC-12 §1).
    return (await DaemonStatus()) as DaemonStatusValue
  } catch (err) {
    throw toAPIError(err)
  }
}

/** Spawns the daemon detached — same path the CLI uses (SPEC-08). */
export async function startDaemon(): Promise<void> {
  try {
    await StartDaemon()
  } catch (err) {
    throw toAPIError(err)
  }
}

// ---- Task surface (SPEC-13) — thin wrappers over the task bindings.

/** task.task mirror for generated model fields (snake_case, DTO-shaped). */
export interface TaskSummary {
  slug: string
  name: string
  type: string
  runtime: string
  enabled: boolean
  updated_at: string
  last_status?: string
  last_run_at?: string
}

/** The editor payload: index state + the parsed task model (SPEC-13 §2). */
export interface TaskDetail {
  enabled: boolean
  updated_at: string
  task: task.Task
}

export interface RunResponse {
  group_id: string
  status: string
}

export async function listTasks(): Promise<TaskSummary[]> {
  try {
    return await ListTasks()
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function getTask(slug: string): Promise<TaskDetail> {
  try {
    return await GetTask(slug)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function createTask(yaml: string): Promise<TaskDetail> {
  try {
    return await CreateTask(yaml)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function updateTask(slug: string, yaml: string): Promise<TaskDetail> {
  try {
    return await UpdateTask(slug, yaml)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function deleteTask(slug: string): Promise<void> {
  try {
    await DeleteTask(slug)
  } catch (err) {
    throw toAPIError(err)
  }
}

/** Raw canonical YAML for the editor's YAML tab (SPEC-13 §2). */
export async function getTaskYAML(slug: string): Promise<string> {
  try {
    return await GetTaskYAML(slug)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function validateTaskYAML(yaml: string): Promise<string[]> {
  try {
    return await ValidateTaskYAML(yaml)
  } catch (err) {
    throw toAPIError(err)
  }
}

/** Parse + validate without persisting — the YAML→Form tab handoff. */
export async function parseTaskYAML(yaml: string): Promise<TaskDetail> {
  try {
    return await ParseTaskYAML(yaml)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function runTask(slug: string, trigger?: string): Promise<RunResponse> {
  try {
    return await RunTask(slug, trigger ?? 'manual')
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function setTaskEnabled(slug: string, enabled: boolean): Promise<void> {
  try {
    await SetTaskEnabled(slug, enabled)
  } catch (err) {
    throw toAPIError(err)
  }
}

/** Opens the picker and imports a task file; cancel resolves to code 'canceled'. */
export async function importTaskFromFile(): Promise<TaskDetail> {
  try {
    return await ImportTaskFromFile()
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function exportTaskYAML(slug: string): Promise<void> {
  try {
    await ExportTaskYAML(slug)
  } catch (err) {
    throw toAPIError(err)
  }
}

/** Path pickers for the editor (SPEC-13 §4); cancel resolves to 'canceled'. */
export async function pickScriptFile(): Promise<string> {
  try {
    return await PickScriptFile()
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function pickWorkingDir(): Promise<string> {
  try {
    return await PickWorkingDir()
  } catch (err) {
    throw toAPIError(err)
  }
}

// ---- Secrets vault (SPEC-11): names only; values go in and never come back.

export async function listSecrets(): Promise<string[]> {
  try {
    return await ListSecrets()
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function setSecret(key: string, value: string): Promise<void> {
  try {
    await SetSecret(key, value)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function deleteSecret(key: string): Promise<void> {
  try {
    await DeleteSecret(key)
  } catch (err) {
    throw toAPIError(err)
  }
}

// ---- Schedules surface (SPEC-14 §2).

export interface Schedule {
  id: string
  slug: string
  task_slug: string
  kind: string // "recurring" | "onetime"
  cron?: string
  run_at?: string
  enabled: boolean
  missed_policy: string
  next_run_at?: string
  last_run_at?: string
  last_status?: string
  latest_run_id?: string
  latest_run_status?: string
  latest_run_started_at?: string
  latest_run_finished_at?: string
  skipped_count: number
  missed_count: number
}

export async function listSchedules(kind?: string): Promise<Schedule[]> {
  try {
    return await ListSchedules(kind ?? '')
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function reconcileSchedules(): Promise<void> {
  try {
    await ReconcileSchedules()
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function createSchedule(
  slug: string,
  taskSlug: string,
  kind: string,
  cron: string,
  runAt: string,
  missedPolicy: string
): Promise<Schedule> {
  try {
    return await CreateSchedule(slug, taskSlug, kind, cron, runAt, missedPolicy)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function updateSchedule(
  id: string,
  slug: string,
  taskSlug: string,
  kind: string,
  cron: string,
  runAt: string,
  missedPolicy: string
): Promise<Schedule> {
  try {
    return await UpdateSchedule(id, slug, taskSlug, kind, cron, runAt, missedPolicy)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function deleteSchedule(id: string): Promise<void> {
  try {
    await DeleteSchedule(id)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function enableSchedule(id: string): Promise<void> {
  try {
    await EnableSchedule(id)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function disableSchedule(id: string): Promise<void> {
  try {
    await DisableSchedule(id)
  } catch (err) {
    throw toAPIError(err)
  }
}

// ---- Runs surface (SPEC-14 §1, §4).

export interface Run {
  run_id: string
  group_id: string
  attempt: number
  task_slug: string
  schedule_id?: string
  trigger: string
  status: string
  started_at?: string
  finished_at?: string
  duration_ms?: number
  exit_code?: number
  pid?: number
  stdout?: string
  stderr?: string
}

export interface RunListResult {
  runs: Run[]
  total: number
  next_cursor?: string
}

export async function listRuns(filters: {
  task?: string
  status?: string
  from?: string
  to?: string
  q?: string
  cursor?: string
  limit?: number
  order?: string
} = {}): Promise<RunListResult> {
  try {
    return await ListRuns(
      filters.task ?? '',
      filters.status ?? '',
      filters.from ?? '',
      filters.to ?? '',
      filters.q ?? '',
      filters.cursor ?? '',
      filters.order ?? '',
      filters.limit ?? 0
    )
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function getRun(runID: string): Promise<Run> {
  try {
    return await GetRun(runID)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function cancelRun(slug: string): Promise<void> {
  try {
    await CancelRun(slug)
  } catch (err) {
    throw toAPIError(err)
  }
}

// ---- Startup / Watchdog / Scheduler control (SPEC-15).

export async function startupEnabled(): Promise<boolean> {
  try {
    return await StartupEnabled()
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function startupSet(on: boolean): Promise<void> {
  try {
    await StartupSet(on)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function watchdogEnabled(): Promise<{installed: boolean; intervalMinutes: number}> {
  try {
    // The binding returns a WatchdogStatusDTO struct: {installed, interval_minutes}.
    const result = (await WatchdogEnabled()) as unknown as {
      installed: boolean
      interval_minutes: number
    }
    return {
      installed: !!result?.installed,
      intervalMinutes: result?.interval_minutes ?? 0,
    }
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function watchdogSet(on: boolean): Promise<void> {
  try {
    await WatchdogSet(on)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function pauseScheduler(): Promise<void> {
  try {
    await PauseScheduler()
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function resumeScheduler(): Promise<void> {
  try {
    await ResumeScheduler()
  } catch (err) {
    throw toAPIError(err)
  }
}

// ---- Dashboard stats (SPEC-16 §1).

export interface StatsResult {
  tasks: number
  tasks_enabled: number
  schedules_enabled: number
  running: number
  runs_today: number
  success_today: number
  failed_today: number
  run_history: {date: string; success: number; failed: number; total: number}[]
  status_distribution: {status: string; count: number}[]
  recent_activity: {run_id: string; task_slug: string; status: string; at: string}[]
}

export async function getStats(): Promise<StatsResult> {
  try {
    return await Stats()
  } catch (err) {
    throw toAPIError(err)
  }
}

// ---- Data directory (SPEC-16 §2).

export async function getDataDir(): Promise<string> {
  try {
    return await DataDir()
  } catch (err) {
    return ''
  }
}

export async function getTasksDir(): Promise<string> {
  try {
    return await TasksDir()
  } catch (err) {
    return ''
  }
}

export async function openDataDir(): Promise<void> {
  try {
    await OpenDataDir()
  } catch (err) {
    throw toAPIError(err)
  }
}

// ---- Settings (SPEC-16 §2).

export interface Settings {
  log_retention_days: number
  sound_success: string
  sound_failure: string
  sound_timeout: string
  reconcile_interval_min: number
  /** Mirrors the generated SettingsDTO (required); 0 on the Go side means "keep current". */
  watchdog_interval_min: number
}

export async function getSettings(): Promise<Settings> {
  try {
    return await GetSettings() as Settings
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function updateSettings(s: Settings): Promise<void> {
  try {
    await UpdateSettings(s)
  } catch (err) {
    throw toAPIError(err)
  }
}

export async function previewSound(preset: string): Promise<void> {
  try {
    await PreviewSound(preset)
  } catch (err) {
    throw toAPIError(err)
  }
}