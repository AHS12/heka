import {SelectField, Field, FormErrors, TextInput, DateTimePickerField} from '../controls'
import {RecurrenceBuilder, emptyRecurrence, recurrenceToCron} from './RecurrenceBuilder'
import type {RecurrenceValue} from './RecurrenceBuilder'
import type {TaskSummary} from '../../lib/api'

export interface ScheduleDraft {
  slug: string
  taskSlug: string
  kind: 'recurring' | 'onetime'
  recurrence: RecurrenceValue
  runAt: string
  missedPolicy: string
}

export function emptyScheduleDraft(): ScheduleDraft {
  return {slug: '', taskSlug: '', kind: 'recurring', recurrence: emptyRecurrence(), runAt: '', missedPolicy: 'skip'}
}

export function draftToCron(draft: ScheduleDraft): string {
  return draft.kind === 'onetime' ? '' : recurrenceToCron(draft.recurrence)
}

export function validateScheduleDraft(draft: ScheduleDraft): string[] {
  const errors: string[] = []
  if (!draft.slug.trim()) errors.push('slug: Enter a schedule slug.')
  else if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(draft.slug.trim())) errors.push('slug: Use lowercase letters, numbers, and single dashes.')
  if (!draft.taskSlug) errors.push('task: Choose a task.')
  if (draft.kind === 'recurring' && !draftToCron(draft).trim()) errors.push('cron: Enter a cron expression.')
  if (draft.kind === 'onetime') {
    if (!draft.runAt) errors.push('run_at: Choose a future date and time.')
    else if (new Date(draft.runAt).getTime() <= Date.now()) errors.push('run_at: Choose a date and time in the future.')
  }
  return errors
}

export function ScheduleForm({draft, onChange, tasks, errors}: {
  draft: ScheduleDraft
  onChange: (draft: ScheduleDraft) => void
  tasks: TaskSummary[]
  errors: string[]
}) {
  const update = (patch: Partial<ScheduleDraft>) => onChange({...draft, ...patch})
  const fieldError = (...fields: string[]) => {
    const match = errors.find((error) => fields.some((field) => error.toLowerCase().startsWith(`${field.toLowerCase()}:`)))
    return match?.replace(/^[^:]+:\s*/, '')
  }

  return (
    <div className="space-y-5">
      <FormErrors errors={errors} title="Schedule could not be created" testId="schedule-errors" />
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
        <Field label="Schedule type">
          <SelectField
            aria-label="Schedule type"
            value={draft.kind}
            onChange={(kind) => update({kind: kind as ScheduleDraft['kind']})}
            items={[{id: 'recurring', label: 'Recurring'}, {id: 'onetime', label: 'One-time'}]}
          />
        </Field>
        <Field label="If a run was missed">
          <SelectField
            aria-label="Missed run policy"
            value={draft.missedPolicy}
            onChange={(missedPolicy) => update({missedPolicy})}
            items={[{id: 'skip', label: 'Skip it'}, {id: 'run_now', label: 'Run when Heka resumes'}]}
          />
        </Field>
      </div>
      <div className="rounded-xl border border-zinc-200/80 bg-white/55 p-4 dark:border-zinc-800 dark:bg-zinc-950/25">
        {draft.kind === 'recurring' ? (
          <RecurrenceBuilder value={draft.recurrence} onChange={(recurrence) => update({recurrence})} />
        ) : (
          <div>
            <DateTimePickerField
              label="Run at"
              value={draft.runAt}
              onChange={(runAt) => update({runAt: runAt ?? ''})}
              className="w-full sm:max-w-sm"
            />
            {fieldError('run_at') && (
              <p id="schedule-run-at-error" className="mt-1.5 text-xs font-medium text-red-600 dark:text-red-400">
                • {fieldError('run_at')}
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
