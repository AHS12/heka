// pages/TasksPage.tsx (SPEC-13 §3) — the Tasks surface: filters, table,
// Run Now (toasts the group_id), inline delete confirm, and the Import /
// New task affordances. Client-side filters while the list is small.
import {useEffect, useMemo, useState} from 'react'
import {apiErrorDetails} from '../lib/api'
import {
  useDeleteTask,
  useImportTask,
  useRunTask,
  useSetTaskEnabled,
  useTasks,
} from '../lib/tasks'
import {SelectField, pillBtn, primaryBtn} from '../components/controls'
import {TaskTable} from '../components/tasks/TaskTable'
import {TaskEditorPage} from './TaskEditorPage'

function useToast(): [string | null, (text: string) => void] {
  const [text, setText] = useState<string | null>(null)
  useEffect(() => {
    if (!text) return
    const timer = setTimeout(() => setText(null), 4000)
    return () => clearTimeout(timer)
  }, [text])
  return [text, setText]
}

export function TasksPage() {
  const tasks = useTasks()
  const run = useRunTask()
  const del = useDeleteTask()
  const toggle = useSetTaskEnabled()
  const importer = useImportTask()
  const [toast, toastMsg] = useToast()
  const [errors, setErrors] = useState<string[]>([])
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<string | null>(null)

  const [typeFilter, setTypeFilter] = useState<'all' | 'script' | 'binary'>('all')
  const [enabledFilter, setEnabledFilter] = useState<'all' | 'enabled' | 'disabled'>('all')

  const rows = useMemo(() => {
    const list = tasks.data ?? []
    return list.filter(
      (t) =>
        (typeFilter === 'all' || t.type === typeFilter) &&
        (enabledFilter === 'all' ||
          (enabledFilter === 'enabled' ? t.enabled : !t.enabled))
    )
  }, [tasks.data, typeFilter, enabledFilter])

  const onRun = (slug: string) => {
    run.mutate(
      {slug, trigger: 'manual'},
      {
        onSuccess: (resp) => toastMsg(`Started ${slug} — group ${resp.group_id}`),
        onError: (err) => {
          const details = apiErrorDetails(err)
          setErrors(details)
          toastMsg(details[0] ?? 'Failed to run')
        },
      }
    )
  }

  const onDelete = (slug: string) => {
    del.mutate(slug, {
      onError: (err) => toastMsg(apiErrorDetails(err)[0] ?? 'Delete failed'),
    })
  }

  const onImport = () => {
    importer.mutate(undefined, {
      onSuccess: (result) => {
        setErrors([])
        setEditing(result.task.slug)
      },
      onError: (err) => {
        const details = apiErrorDetails(err)
        if (err instanceof Error && err.message === 'dialog canceled') {
          return // user closed the picker — not an error
        }
        setErrors(details)
      },
    })
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">Tasks</h2>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onImport} className={pillBtn}>
            Import Task
          </button>
          <button type="button" className={primaryBtn} onClick={() => setCreating(true)}>
            + New task
          </button>
        </div>
      </div>

      {errors.length > 0 && (
        <ul
          role="alert"
          className="space-y-1 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900 dark:bg-red-950/60 dark:text-red-300"
        >
          {errors.map((e) => (
            <li key={e}>• {e}</li>
          ))}
        </ul>
      )}

      <div className="flex gap-2">
        <SelectField
          aria-label="Filter by type"
          value={typeFilter}
          onChange={(v) => setTypeFilter(v as typeof typeFilter)}
          className="w-36"
          items={[
            {id: 'all', label: 'All types'},
            {id: 'script', label: 'Scripts'},
            {id: 'binary', label: 'Binaries'},
          ]}
        />
        <SelectField
          aria-label="Filter by enabled state"
          value={enabledFilter}
          onChange={(v) => setEnabledFilter(v as typeof enabledFilter)}
          className="w-40"
          items={[
            {id: 'all', label: 'All states'},
            {id: 'enabled', label: 'Enabled'},
            {id: 'disabled', label: 'Disabled'},
          ]}
        />
      </div>

      {tasks.isLoading ? (
        <p className="text-sm text-foreground/50">Loading tasks…</p>
      ) : rows.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-field-border px-4 py-10 text-center text-sm text-foreground/50">
          {tasks.data?.length === 0
            ? 'No tasks yet — create one or import a YAML file.'
            : 'No tasks match the filters.'}
        </div>
      ) : (
        <TaskTable
          tasks={rows}
          onRun={onRun}
          onDelete={onDelete}
          onToggle={(slug, enabled) => toggle.mutate({slug, enabled})}
          onOpen={(slug) => setEditing(slug)}
        />
      )}

      {creating && (
        <TaskEditorPage
          dialog
          onClose={() => setCreating(false)}
          onSaved={(slug) => toastMsg(`Created ${slug}`)}
        />
      )}

      {editing && (
        <TaskEditorPage
          dialog
          slug={editing}
          onClose={() => setEditing(null)}
          onSaved={(slug) => toastMsg(`Saved ${slug}`)}
        />
      )}

      {toast && (
        <div
          role="status"
          data-testid="toast"
          className="fixed bottom-5 right-5 rounded-xl border border-field-border bg-surface/90 px-4 py-2.5 text-sm shadow-lg backdrop-blur"
        >
          {toast}
        </div>
      )}
    </div>
  )
}