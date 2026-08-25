import {useState} from 'react'
import {SelectField, Field, TextInput, pillBtn, primaryBtn} from '../controls'
import {RecurrenceBuilder, emptyRecurrence, recurrenceToCron} from './RecurrenceBuilder'
import type {RecurrenceValue} from './RecurrenceBuilder'
import type {TaskSummary} from '../../lib/api'

export interface ScheduleDraft {
  slug: string
  taskSlug: string
  kind: 'recurring' | 'onetime'
  recurrence: RecurrenceValue
  runAt: string // for onetime: RFC3339
  missedPolicy: string
}

export function emptyScheduleDraft(): ScheduleDraft {
  return {
    slug: '',
    taskSlug: '',
    kind: 'recurring',
    recurrence: emptyRecurrence(),
    runAt: '',
    missedPolicy: 'skip',
  }
}

export function draftToCron(d: ScheduleDraft): string {
  if (d.kind === 'onetime') return ''
  return recurrenceToCron(d.recurrence)
}

export function ScheduleForm({
  draft,
  onChange,
  tasks,
  onSave,
  onCancel,
  isPending,
}: {
  draft: ScheduleDraft
  onChange: (d: ScheduleDraft) => void
  tasks: TaskSummary[]
  onSave: () => void
  onCancel: () => void
  isPending: boolean
}) {
  const update = (patch: Partial<ScheduleDraft>) =>
    onChange({...draft, ...patch})

  const taskItems = tasks.map((t) => ({id: t.slug, label: `${t.name} (${t.slug})`}))
  const canSave =
    draft.slug.trim() !== '' &&
    draft.taskSlug !== '' &&
    (draft.kind === 'recurring' ? recurrenceToCron(draft.recurrence) !== '' : draft.runAt !== '')

  return (
    <div className="space-y-4 rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm shadow-zinc-900/5 backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
      <div className="flex flex-wrap items-end gap-3">
        <Field label="Slug">
          <TextInput
            value={draft.slug}
            onChange={(e) => update({slug: e.target.value})}
            placeholder="my-schedule"
            className="w-48"
          />
        </Field>
        <Field label="Task">
          <SelectField
            aria-label="Select task"
            value={draft.taskSlug}
            onChange={(v) => update({taskSlug: v})}
            className="w-64"
            items={taskItems}
            placeholder="Select task…"
          />
        </Field>
        <Field label="Kind">
          <SelectField
            aria-label="Schedule kind"
            value={draft.kind}
            onChange={(v) => update({kind: v as ScheduleDraft['kind']})}
            className="w-36"
            items={[
              {id: 'recurring', label: 'Recurring'},
              {id: 'onetime', label: 'One-time'},
            ]}
          />
        </Field>
        <Field label="Missed policy">
          <SelectField
            aria-label="Missed policy"
            value={draft.missedPolicy}
            onChange={(v) => update({missedPolicy: v})}
            className="w-36"
            items={[
              {id: 'skip', label: 'Skip'},
              {id: 'run_now', label: 'Run now'},
            ]}
          />
        </Field>
      </div>

      {draft.kind === 'recurring' ? (
        <RecurrenceBuilder
          value={draft.recurrence}
          onChange={(r) => update({recurrence: r})}
        />
      ) : (
        <Field label="Run at">
          <TextInput
            type="datetime-local"
            value={draft.runAt ? draft.runAt.slice(0, 16) : ''}
            onChange={(e) => {
              const v = e.target.value
              update({runAt: v ? new Date(v).toISOString() : ''})
            }}
            className="w-64"
          />
        </Field>
      )}

      <div className="flex items-center gap-2 pt-2">
        <button type="button" onClick={onSave} disabled={!canSave || isPending} className={primaryBtn}>
          Create schedule
        </button>
        <button type="button" onClick={onCancel} className={pillBtn}>
          Cancel
        </button>
      </div>
    </div>
  )
}
