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