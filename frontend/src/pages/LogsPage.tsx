import {useCallback, useMemo} from 'react'
import {useSearchParams} from 'react-router-dom'
import {useTasks} from '../lib/tasks'
import {usePollingRuns, filtersToParams, paramsToFilters} from '../lib/runs'
import {useSystemLog} from '../lib/systemLog'
import {RunsTable} from '../components/runs/RunsTable'
import {RunFilters} from '../components/runs/RunFilters'
import {SystemLogTable} from '../components/runs/SystemLogTable'
import {SelectField} from '../components/controls'

type LogView = 'runs' | 'system'

function ViewToggle({view, onChange}: {view: LogView; onChange: (v: LogView) => void}) {
  const btn = (v: LogView, label: string) => (
    <button
      type="button"
      onClick={() => onChange(v)}
      aria-pressed={view === v}
      className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
        view === v
          ? 'bg-accent text-accent-contrast shadow-sm shadow-zinc-900/10'
          : 'text-foreground/55 hover:bg-surface-secondary/70'
      }`}
    >
      {label}
    </button>
  )
  return (
    <div className="inline-flex items-center rounded-full border border-border/80 bg-surface/80 p-0.5 shadow-sm shadow-zinc-900/5">
      {btn('runs', 'Runs')}
      {btn('system', 'System')}
    </div>
  )
}

function RunsView() {
  const [searchParams, setSearchParams] = useSearchParams()
  const filters = useMemo(() => paramsToFilters(searchParams), [searchParams])
  const tasks = useTasks()

  const order = (searchParams.get('order') as string) ?? 'desc'
  const isDesc = order !== 'asc'

  const effectiveFilters = useMemo(
    () => ({...filters, order: isDesc ? undefined : 'asc'}),
    [filters, isDesc]
  )
  const {data, isLoading, hasActive} = usePollingRuns(effectiveFilters)

  const updateFilters = useCallback(
    (f: typeof filters) => {
      const p = filtersToParams(f)
      // Preserve sort order and clear cursor when filters change
      p.delete('cursor')
      setSearchParams(p, {replace: true})
    },
    [setSearchParams]
  )

  const toggleSort = () => {
    setSearchParams((prev) => {
      const p = new URLSearchParams(prev)
      const current = p.get('order') ?? 'desc'
      p.set('order', current === 'desc' ? 'asc' : 'desc')
      p.delete('cursor')
      return p
    }, {replace: true})
  }

  const goNext = () => {
    const cursor = data?.next_cursor
    if (!cursor) return
    setSearchParams((prev) => {
      const p = new URLSearchParams(prev)
      p.set('cursor', cursor)
      return p
    }, {replace: true})
  }

  const goPrev = () => {
    setSearchParams((prev) => {
      const p = new URLSearchParams(prev)
      p.delete('cursor')
      return p
    }, {replace: true})
  }

  const runs = data?.runs ?? []
  const hasCursor = !!filters.cursor
  const hasFilters = !!(filters.task || filters.status || filters.from || filters.to || filters.q)

  return (
    <>
      <div className="flex flex-wrap items-center justify-end gap-2">
        {hasActive && (
          <span className="text-xs text-blue-500 dark:text-blue-400">Live</span>
        )}
        <SelectField
          aria-label="Per page"
          value={String(filters.limit ?? 25)}
          onChange={(v) => {
            const p = filtersToParams(filters)
            p.set('limit', v)
            p.delete('cursor')
            setSearchParams(p, {replace: true})
          }}
          className="w-20"
          items={[
            {id: '25', label: '25'},
            {id: '50', label: '50'},
            {id: '100', label: '100'},
          ]}
        />
        <button
          type="button"
          onClick={toggleSort}
          aria-label={isDesc ? 'Sort oldest first' : 'Sort newest first'}
          title={isDesc ? 'Newest first' : 'Oldest first'}
          className="inline-flex items-center gap-1 rounded-full border border-border/80 bg-surface/80 px-2.5 py-1 text-xs font-medium shadow-sm shadow-zinc-900/5 outline-none transition-colors hover:bg-surface-secondary/70"
        >
          <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="10" />
            <polyline points="12 6 12 12 16 14" />
          </svg>
          <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <line x1="12" y1="5" x2="12" y2="19" />
            <polyline points={isDesc ? '19 12 12 19 5 12' : '5 12 12 5 19 12'} />
            <text x="18" y="8" fill="currentColor" stroke="none" fontSize="7" fontWeight="bold">1</text>
            <text x="18" y="20" fill="currentColor" stroke="none" fontSize="7" fontWeight="bold">2</text>
          </svg>
        </button>
        {hasFilters && (
          <button
            type="button"
            onClick={() => {
              const p = new URLSearchParams()
              p.set('order', order)
              setSearchParams(p, {replace: true})
            }}
            className="rounded-full border border-border/80 bg-surface/80 px-2.5 py-1 text-xs font-medium shadow-sm shadow-zinc-900/5 outline-none transition-colors hover:bg-surface-secondary/70"
          >
            Clear
          </button>
        )}
      </div>

      <RunFilters
        filters={filters}
        onChange={updateFilters}
        tasks={tasks.data ?? []}
        showSearch
      />

      {isLoading ? (
        <p className="text-sm text-foreground/50">Loading logs…</p>
      ) : runs.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-border px-4 py-10 text-center text-sm text-foreground/50">
          {filters.q ? 'No matching log output found.' : 'No logs yet.'}
        </div>
      ) : (
        <>
          <RunsTable
            runs={runs}
            linkPrefix="/logs"
            highlight={filters.q}
          />
          <div className="flex items-center justify-between text-xs text-foreground/50">
            <span>
              {runs.length} shown · {data?.total ?? 0} total
            </span>
            <div className="flex gap-2">
              {hasCursor && (
                <button
                  type="button"
                  onClick={goPrev}
                  className="rounded-lg border border-border px-2 py-1"
                >
                  ← Newer
                </button>
              )}
              {data?.next_cursor && (
                <button
                  type="button"
                  onClick={goNext}
                  className="rounded-lg border border-border px-2 py-1"
                >
                  Older →
                </button>
              )}
            </div>
          </div>
        </>
      )}
    </>
  )
}

function SystemView() {
  const systemLog = useSystemLog(200)
  const entries = systemLog.data ?? []

  return (
    <>
      <div className="flex flex-wrap items-center justify-end gap-2">
        <span className="text-xs text-blue-500 dark:text-blue-400">Live</span>
        <button
          type="button"
          onClick={() => void systemLog.refetch()}
          className="rounded-full border border-border/80 bg-surface/80 px-2.5 py-1 text-xs font-medium shadow-sm shadow-zinc-900/5 outline-none transition-colors hover:bg-surface-secondary/70"
        >
          Refresh
        </button>
      </div>

      {systemLog.isLoading ? (
        <p className="text-sm text-foreground/50">Loading system log…</p>
      ) : entries.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-border px-4 py-10 text-center text-sm text-foreground/50">
          No system events yet. The daemon logs here when it starts, reconciles
          missed schedules or backups, runs an archive backup, or wakes from sleep.
        </div>
      ) : (
        <SystemLogTable entries={entries} />
      )}
    </>
  )
}

export function LogsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const view: LogView = searchParams.get('view') === 'system' ? 'system' : 'runs'

  const setView = (v: LogView) => {
    setSearchParams((prev) => {
      const p = new URLSearchParams(prev)
      if (v === 'system') p.set('view', 'system')
      else p.delete('view')
      return p
    }, {replace: true})
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold">Logs</h2>
          <ViewToggle view={view} onChange={setView} />
        </div>
      </div>

      {view === 'system' ? <SystemView /> : <RunsView />}
    </div>
  )
}
