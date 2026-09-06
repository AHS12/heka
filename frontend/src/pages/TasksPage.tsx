// pages/TasksPage.tsx (SPEC-13 §3) — the Tasks surface: server-side search +
// type/enabled filters (cursor-paginated, accumulated pages kept), Run Now
// (toasts the group_id), inline delete confirm, and the Import / New task
// affordances. Freshness comes from mutation invalidations + the revision
// pulse, not polling.
import {useEffect, useMemo, useState} from 'react'
import {apiErrorDetails} from '../lib/api'
import {
  useDeleteTask,
  useImportTask,
  useRunTask,
  useSetTaskEnabled,
  useTasksPage,
  taskPageRows,
} from '../lib/tasks'
import {useDebounced, useSentinel} from '../lib/hooks'
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

const SearchIcon = () => (
  <svg
    className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-foreground/50"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2.5"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    <circle cx="11" cy="11" r="8" />
    <line x1="21" y1="21" x2="16.65" y2="16.65" />
  </svg>
)

const SpinnerIcon = ({testid}: {testid?: string}) => (
  <svg
    className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 animate-spin text-foreground/50"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2.5"
    strokeLinecap="round"
    data-testid={testid}
  >
    <path d="M21 12a9 9 0 1 1-6.219-8.56" />
  </svg>
)

export function TasksPage() {
  const run = useRunTask()
  const del = useDeleteTask()
  const toggle = useSetTaskEnabled()
  const importer = useImportTask()
  const [toast, toastMsg] = useToast()
  const [errors, setErrors] = useState<string[]>([])
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<string | null>(null)

  const [search, setSearch] = useState('')
  const debouncedQ = useDebounced(search)
  const [typeFilter, setTypeFilter] = useState<'all' | 'script' | 'binary'>('all')
  const [enabledFilter, setEnabledFilter] = useState<'all' | 'enabled' | 'disabled'>('all')

  // Server-side search + filters: any change produces a new query key, so
  // the scroll accumulation resets and the daemon filters for us.
  const filters = useMemo(
    () => ({
      q: debouncedQ.trim() || undefined,
      type: typeFilter === 'all' ? undefined : typeFilter,
      enabled: enabledFilter === 'all' ? undefined : enabledFilter,
    }),
    [debouncedQ, typeFilter, enabledFilter]
  )
  const tasks = useTasksPage(filters)
  const rows = taskPageRows(tasks.data)
  const total = tasks.data?.pages[0]?.total ?? 0
  const hasFilters = !!(filters.q || filters.type || filters.enabled)
  const searching = tasks.isFetching

  const sentinelRef = useSentinel(() => {
    if (tasks.hasNextPage && !tasks.isFetchingNextPage) void tasks.fetchNextPage()
  })

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

      <div className="flex flex-wrap gap-2">
        <div className="relative">
          {searching ? <SpinnerIcon testid="tasks-search-spinner" /> : <SearchIcon />}
          <input
            type="text"
            placeholder="Search tasks…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-44 rounded-full border border-field-border bg-surface/80 py-1.5 pl-8 pr-3 text-sm text-foreground/75 outline-none transition-colors placeholder:text-foreground/40 focus:border-accent focus:ring-1 focus:ring-accent-ring"
          />
        </div>
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
          {total === 0 && !hasFilters
            ? 'No tasks yet — create one or import a YAML file.'
            : `No tasks match the filters${filters.q ? ` for "${search.trim()}"` : ''}.`}
        </div>
      ) : (
        <>
          <TaskTable
            tasks={rows}
            onRun={onRun}
            onDelete={onDelete}
            onToggle={(slug, enabled) => toggle.mutate({slug, enabled})}
            onOpen={(slug) => setEditing(slug)}
          />
          {/* Infinite scroll trigger: load the next keyset page on approach. */}
          <div ref={sentinelRef} aria-hidden className="h-px" />
          {tasks.isFetchingNextPage && (
            <p className="flex items-center justify-center gap-2 py-2 text-xs text-foreground/50">
              <svg className="size-3.5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
              Loading more tasks…
            </p>
          )}
          <p data-testid="tasks-footer" className="flex items-center justify-center gap-2 pb-1 text-[11px] text-foreground/45">
            {rows.length} shown · {total} total
            {searching && (
              <svg className="size-3 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
            )}
          </p>
        </>
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
