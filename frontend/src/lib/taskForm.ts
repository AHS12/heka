// lib/taskForm.ts (SPEC-13 §4) — the editor's single canonical draft: the
// parsed Task, with the Form and YAML tabs as two *views* of it. Provides
// form↔model mapping, canonical YAML serialization (the schema is fixed, so a
// tiny emitter is deterministic), and rename detection (copy+delete flow).
//
// YAML → model parsing is deliberately NOT reimplemented here: the daemon
// owns the strict parser (the editor calls ParseTaskYAML / the create/update
// endpoints). This file only maps the parsed model in both directions and
// counts on the daemon for validation.
import type {task} from '@wailsjs/go/models'

export type TaskType = 'script' | 'binary'
export type WebhookFormat = 'slack' | 'pumble' | 'discord' | 'telegram' | 'generic'

/** The schema's runtime enum (SPEC-04 §runtime, executor runtime table).
 *  Anything beyond this list is expressible through the YAML tab. */
export const RUNTIMES = ['powershell', 'python', 'node', 'bash', 'custom'] as const
export type Runtime = (typeof RUNTIMES)[number]

export interface WebhookDraft {
  format: WebhookFormat
  url: string
  chatId: string // telegram only; others leave blank
}

export interface TaskDraft {
  version: number
  name: string
  slug: string
  type: TaskType
  runtime: string
  script: string
  command: string
  args: string[]
  workingDirectory: string
  outputDir: string
  environment: Array<[string, string]>
  timeout: number
  maxAttempts: number
  delaySeconds: number
  captureOutput: boolean
  notifyOn: string[]
  webhooks: WebhookDraft[]
}

export function emptyDraft(): TaskDraft {
  return {
    version: 1,
    name: '',
    slug: '',
    type: 'script',
    runtime: '',
    script: '',
    command: '',
    args: [],
    workingDirectory: '',
    outputDir: '',
    environment: [],
    timeout: 60,
    maxAttempts: 1,
    delaySeconds: 0,
    captureOutput: true,
    notifyOn: [],
    webhooks: [],
  }
}

/** Model → draft (server-loaded task or a parse result). */
export function draftFromTask(t: task.Task): TaskDraft {
  const env: Array<[string, string]> = t.environment
    ? Object.entries(t.environment)
    : []
  return {
    version: t.version,
    name: t.name,
    slug: t.slug,
    type: t.type === 'binary' ? 'binary' : 'script',
    runtime: t.runtime ?? '',
    script: t.script ?? '',
    command: t.command ?? '',
    args: t.args ?? [],
    workingDirectory: t.working_directory ?? '',
    outputDir: t.output_dir ?? '',
    environment: env,
    timeout: t.timeout,
    maxAttempts: t.retry?.max_attempts ?? 1,
    delaySeconds: t.retry?.delay_seconds ?? 0,
    captureOutput: t.capture_output ?? true,
    notifyOn: t.notify_on ?? [],
    webhooks: (t.notify?.webhooks ?? []).map((w) => ({
      format: (w.format as WebhookFormat) ?? 'generic',
      url: w.url,
      chatId: w.chat_id ?? '',
    })),
  }
}

/** Draft → model for saving via the daemon. Empty advanced fields are omitted. */
export function draftToTask(d: TaskDraft): task.Task {
  const model = {
    version: d.version,
    name: d.name,
    slug: d.slug,
    type: d.type,
    runtime: d.runtime,
    script: d.script,
    command: d.command,
    args: d.args.length ? d.args : undefined,
    working_directory: d.workingDirectory || undefined,
    output_dir: d.outputDir || undefined,
    environment:
      d.environment.length > 0
        ? Object.fromEntries(d.environment)
        : undefined,
    timeout: d.timeout,
    retry: {max_attempts: d.maxAttempts, delay_seconds: d.delaySeconds},
    capture_output: d.captureOutput,
    notify_on: d.notifyOn.length ? d.notifyOn : undefined,
    notify:
      d.webhooks.length > 0
        ? ({
            webhooks: d.webhooks.map((w) => ({
              format: w.format,
              url: w.url,
              chat_id: w.chatId || undefined,
            })),
          } as task.Notify)
        : undefined,
  }
  return model as task.Task
}

/**
 * Rename detection (SPEC-13 §1): an existing task whose draft slug differs
 * from the file's slug saves via copy (POST new) + delete (old).
 * Returns [newSlug, isRenaming].
 */
export function renamePlan(originalSlug: string | undefined, draft: TaskDraft) {
  if (!originalSlug) {
    return {newSlug: draft.slug, isRenaming: false}
  }
  return {newSlug: draft.slug, isRenaming: draft.slug !== originalSlug}
}

/** Canonical YAML for the editor tab (draft → text). Field order mirrors the
 *  Go exporter; only meaningful values are emitted. */
export function draftToYAML(d: TaskDraft): string {
  const lines: string[] = []
  lines.push('version: 1', `name: ${d.name}`, `slug: ${d.slug}`, `type: ${d.type}`)
  if (d.runtime) {
    lines.push(`runtime: ${d.runtime}`)
  }
  const exec = d.type === 'binary' ? d.command : d.script
  if (exec) {
    lines.push(`${d.type === 'binary' ? 'command' : 'script'}: ${exec}`)
  }
  if (d.args.length > 0) {
    lines.push(`args:`)
    for (const a of d.args) {
      lines.push(`  - ${a}`)
    }
  }
  if (d.workingDirectory) {
    lines.push(`working_directory: ${d.workingDirectory}`)
  }
  if (d.outputDir) {
    lines.push(`output_dir: ${d.outputDir}`)
  }
  if (d.environment.length > 0) {
    lines.push(`environment:`)
    for (const [k, v] of d.environment) {
      lines.push(`  ${k}: ${v}`)
    }
  }
  lines.push(`timeout: ${d.timeout}`)
  if (d.maxAttempts > 1 || d.delaySeconds > 0) {
    lines.push(`retry:`, `  max_attempts: ${d.maxAttempts}`, `  delay_seconds: ${d.delaySeconds}`)
  }
  lines.push(`capture_output: ${d.captureOutput}`)
  if (d.notifyOn.length > 0) {
    lines.push(`notify_on: [${d.notifyOn.join(', ')}]`)
  }
  if (d.webhooks.length > 0) {
    lines.push(`notify:`)
    lines.push(`  webhooks:`)
    for (const w of d.webhooks) {
      lines.push(`    - format: ${w.format}`)
      lines.push(`      url: ${w.url}`)
      if (w.chatId) {
        lines.push(`      chat_id: ${w.chatId}`)
      }
    }
  }
  return lines.join('\n') + '\n'
}

export function validateTaskDraft(draft: TaskDraft): string[] {
  const errors: string[] = []
  if (!draft.name.trim()) errors.push('name: Enter a task name.')
  if (!draft.slug.trim()) {
    errors.push('slug: Enter a task slug.')
  } else if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(draft.slug.trim())) {
    errors.push('slug: Use lowercase letters, numbers, and single dashes.')
  }
  if (draft.type === 'script') {
    if (!draft.runtime) errors.push('runtime: Choose a runtime.')
    if (!draft.script.trim()) errors.push('script: Choose or enter a script path.')
  } else if (!draft.command.trim()) {
    errors.push('command: Enter the binary command.')
  }
  if (draft.timeout < 0) errors.push('timeout: Use zero or a positive number.')
  if (draft.maxAttempts < 1) errors.push('retry.max_attempts: Use at least one attempt.')
  if (draft.delaySeconds < 0) errors.push('retry.delay_seconds: Use zero or a positive number.')
  return errors
}

/** Arg-string ↔ array conversations for the form inputs. */
export function argsFromString(s: string): string[] {
  return s
    .split(/\s+/)
    .map((x) => x.trim())
    .filter(Boolean)
}

/** Slug auto-generation from a task name (spec: a-z, 0-9, dashes). */
export function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}