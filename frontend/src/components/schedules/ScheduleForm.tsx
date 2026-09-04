import {SelectField, Field, FormErrors, TextInput} from '../controls'
import {SchedulePatternPicker} from './SchedulePatternPicker'
import {PatternPanel} from './PatternPanels'
import {CronPreview} from './CronPreview'
import {validateCron} from '../../lib/cron'
import {cronToPattern, emptyPattern, patternToCron, patternToKind} from '../../lib/schedulePattern'
import type {PatternKind, SchedulePattern} from '../../lib/schedulePattern'
import type {Schedule, TaskSummary} from '../../lib/api'

export interface ScheduleDraft {
  slug: string
  taskSlug: string
  missedPolicy: string
  pattern: SchedulePattern
}

export function emptyScheduleDraft(): ScheduleDraft {
  return {slug: '', taskSlug: '', missedPolicy: 'skip', pattern: emptyPattern('every')}
}

// Local datetime string for DateTimePickerField, converted from the RFC3339
// the API returns (it treats the value as local wall time).
function runAtToLocalInput(rfc3339: string): string {
  if (!rfc3339) return ''
  const d = new Date(rfc3339)
  if (Number.isNaN(d.getTime())) return rfc3339
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function localInputToRFC3339(localDateTime: string): string {
  if (!localDateTime) return ''
  const date = new Date(localDateTime)
  return Number.isNaN(date.getTime()) ? localDateTime : date.toISOString()
}

export function draftFromSchedule(s: Schedule): ScheduleDraft {
  return {
    slug: s.slug,
    taskSlug: s.task_slug,
    missedPolicy: s.missed_policy || 'skip',
    pattern:
      s.kind === 'onetime'
        ? {kind: 'once', runAt: runAtToLocalInput(s.run_at ?? '')}
        : cronToPattern(s.cron ?? ''),
  }
}

export interface SchedulePayload {
  slug: string
  taskSlug: string
  kind: 'recurring' | 'onetime'
  cron: string
  runAt: string
  missedPolicy: string
}

export function draftToPayload(draft: ScheduleDraft): SchedulePayload {
  return {
    slug: draft.slug.trim(),
    taskSlug: draft.taskSlug,
    kind: patternToKind(draft.pattern),
    cron: patternToCron(draft.pattern),
    runAt: draft.pattern.kind === 'once' ? localInputToRFC3339(draft.pattern.runAt) : '',
    missedPolicy: draft.missedPolicy,
  }
}

export function validateScheduleDraft(draft: ScheduleDraft): string[] {
  const errors: string[] = []
  if (!draft.slug.trim()) errors.push('slug: Enter a schedule slug.')
  else if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(draft.slug.trim())) errors.push('slug: Use lowercase letters, numbers, and single dashes.')
  if (!draft.taskSlug) errors.push('task: Choose a task.')

  const p = draft.pattern
  if (p.kind === 'once') {
    if (!p.runAt) errors.push('run_at: Choose a future date and time.')
    else if (new Date(p.runAt).getTime() <= Date.now()) errors.push('run_at: Choose a date and time in the future.')
  } else if (p.kind === 'cron') {
    if (!p.expr.trim()) errors.push('cron: Enter a cron expression.')
    else {
      const error = validateCron(p.expr)
      if (error) errors.push(`cron: ${error}`)
    }
  } else if (p.kind === 'every') {
    if (!Number.isFinite(p.n) || Math.floor(p.n) < 1) errors.push('cron: Enter an interval of at least 1.')
  } else if (p.kind === 'daily') {
    if (p.times.length === 0) errors.push('cron: Add at least one time.')
  } else if (p.kind === 'weekly') {
    if (p.days.length === 0) errors.push('cron: Pick at least one day of the week.')
    if (p.times.length === 0) errors.push('cron: Add at least one time.')
  } else if (p.kind === 'monthly') {
    if (p.days.length === 0 && p.ranges.length === 0 && p.months.length === 0) {
      errors.push('cron: Pick at least one day of the month, or restrict the months.')
    }
    if (p.times.length === 0) errors.push('cron: Add at least one time.')
  }
  return errors
}

export function ScheduleForm({draft, onChange, tasks, errors, errorTitle = 'Schedule could not be saved'}: {
  draft: ScheduleDraft
  onChange: (draft: ScheduleDraft) => void
  tasks: TaskSummary[]
  errors: string[]
  errorTitle?: string
}) {
  const update = (patch: Partial<ScheduleDraft>) => onChange({...draft, ...patch})
  const fieldError = (...fields: string[]) => {
    const match = errors.find((error) => fields.some((field) => error.toLowerCase().startsWith(`${field.toLowerCase()}:`)))
    return match?.replace(/^[^:]+:\s*/, '')
  }
  const recurring = patternToKind(draft.pattern) === 'recurring'

  const changePatternKind = (kind: PatternKind) => {
    if (kind === draft.pattern.kind) return
    update({pattern: emptyPattern(kind)})
  }

  return (
    <div className="space-y-5">
      <FormErrors errors={errors} title={errorTitle} testId="schedule-errors" />
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Slug" error={fieldError('slug')} errorId="schedule-slug-error">
          <TextInput
            aria-label="Schedule slug"
            aria-invalid={Boolean(fieldError('slug'))}
            aria-describedby={fieldError('slug') ? 'schedule-slug-error' : undefined}
            value={draft.slug}
            onChange={(event) => update({slug: event.target.value})}
            placeholder="weekday-backup"
          />
        </Field>
        <Field label="Task" error={fieldError('task', 'task_slug')} errorId="schedule-task-error">
          <SelectField
            aria-label="Select task"
            aria-describedby={fieldError('task', 'task_slug') ? 'schedule-task-error' : undefined}
            isInvalid={Boolean(fieldError('task', 'task_slug'))}
            value={draft.taskSlug}
            onChange={(taskSlug) => update({taskSlug})}
            items={tasks.map((task) => ({id: task.slug, label: task.name}))}
            placeholder="Choose task…"
          />
        </Field>
        {recurring && (
          <Field label="If a run was missed">
            <SelectField
              aria-label="Missed run policy"
              value={draft.missedPolicy}
              onChange={(missedPolicy) => update({missedPolicy})}
              items={[{id: 'skip', label: 'Skip it'}, {id: 'run_now', label: 'Run when Heka resumes'}]}
            />
          </Field>
        )}
      </div>

      <div className="space-y-3">
        <span className="block text-[10px] font-medium uppercase tracking-wider text-foreground/50">
          Schedule pattern
        </span>
        <SchedulePatternPicker value={draft.pattern.kind} onChange={changePatternKind} />
        <div className="rounded-xl border border-border/80 bg-surface/55 p-4">
          <PatternPanel
            pattern={draft.pattern}
            onChange={(pattern) => update({pattern})}
            error={fieldError('cron', 'run_at')}
          />
        </div>
        <CronPreview pattern={draft.pattern} />
      </div>
    </div>
  )
}
