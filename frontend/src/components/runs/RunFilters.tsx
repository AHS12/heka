import {SelectField, TextInput, Field} from '../controls'
import type {RunFilters as RunFilterState} from '../../lib/runs'
import type {TaskSummary} from '../../lib/api'

const STATUS_ITEMS = [
  {id: '', label: 'All statuses'},
  {id: 'success', label: 'Success'},
  {id: 'failed', label: 'Failed'},
  {id: 'timed_out', label: 'Timed out'},
  {id: 'running', label: 'Running'},
  {id: 'queued', label: 'Queued'},
  {id: 'cancelled', label: 'Cancelled'},
  {id: 'skipped', label: 'Skipped'},
  {id: 'missed', label: 'Missed'},
]

export function RunFilters({
  filters,
  onChange,
  tasks,
  showSearch,
}: {
  filters: RunFilterState
  onChange: (f: RunFilterState) => void
  tasks: TaskSummary[]
  showSearch?: boolean
}) {
  const taskItems = [{id: '', label: 'All tasks'}, ...tasks.map((t) => ({id: t.slug, label: t.name}))]

  return (
    <div className="flex flex-wrap items-end gap-3">
      <Field label="Task">
        <SelectField
          aria-label="Filter by task"
          value={filters.task ?? ''}
          onChange={(v) => onChange({...filters, task: v || undefined})}
          className="w-48"
          items={taskItems}
        />
      </Field>
      <Field label="Status">
        <SelectField
          aria-label="Filter by status"
          value={filters.status ?? ''}
          onChange={(v) => onChange({...filters, status: v || undefined})}
          className="w-40"
          items={STATUS_ITEMS}
        />
      </Field>
      <Field label="From">
        <TextInput
          type="date"
          value={filters.from ? filters.from.slice(0, 10) : ''}
          onChange={(e) =>
            onChange({...filters, from: e.target.value ? e.target.value + 'T00:00:00Z' : undefined})
          }
          className="w-40"
        />
      </Field>
      <Field label="To">
        <TextInput
          type="date"
          value={filters.to ? filters.to.slice(0, 10) : ''}
          onChange={(e) =>
            onChange({...filters, to: e.target.value ? e.target.value + 'T23:59:59Z' : undefined})
          }
          className="w-40"
        />
      </Field>
      {showSearch && (
        <Field label="Search">
          <TextInput
            value={filters.q ?? ''}
            onChange={(e) => onChange({...filters, q: e.target.value || undefined})}
            placeholder="Search output…"
            className="w-56"
          />
        </Field>
      )}
    </div>
  )
}
