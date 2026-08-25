import {useCallback, useMemo, useState} from 'react'
import {useSearchParams} from 'react-router-dom'
import {useTasks} from '../lib/tasks'
import {usePollingRuns, filtersToParams, paramsToFilters} from '../lib/runs'
import {RunsTable} from '../components/runs/RunsTable'
import {RunFilters} from '../components/runs/RunFilters'

export function LogsPage() {
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

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">Logs</h2>
        <div className="flex items-center gap-2">
          {hasActive && (
            <span className="text-xs text-blue-500 dark:text-blue-400">Live</span>
          )}
          <select
            value={filters.limit ?? 25}
            onChange={(e) => {
              const p = filtersToParams(filters)
              p.set('limit', e.target.value)
              p.delete('cursor')
              setSearchParams(p, {replace: true})
            }}
            aria-label="Per page"
            className="rounded-full border border-zinc-200/80 bg-white/80 px-2 py-0.5 text-xs font-medium shadow-sm shadow-zinc-900/5 outline-none dark:border-zinc-700/60 dark:bg-zinc-900/70 dark:text-zinc-300"
          >
            <option value="25">25</option>
            <option value="50">50</option>
            <option value="100">100</option>
          </select>
          <button
            type="button"
            onClick={toggleSort}
            aria-label={isDesc ? 'Sort oldest first' : 'Sort newest first'}
            title={isDesc ? 'Newest first' : 'Oldest first'}
            className="inline-flex items-center gap-1 rounded-full border border-zinc-200/80 bg-white/80 px-2.5 py-1 text-xs font-medium shadow-sm shadow-zinc-900/5 outline-none transition-colors hover:bg-zinc-200/70 dark:border-zinc-700/60 dark:bg-zinc-900/70 dark:text-zinc-300 dark:hover:bg-zinc-800/70"
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
        </div>
      </div>

      <RunFilters
        filters={filters}
        onChange={updateFilters}
        tasks={tasks.data ?? []}
        showSearch
      />

      {isLoading ? (
        <p className="text-sm text-zinc-400">Loading logs…</p>
      ) : runs.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-zinc-300 px-4 py-10 text-center text-sm text-zinc-400 dark:border-zinc-700">
          {filters.q ? 'No matching log output found.' : 'No logs yet.'}
        </div>
      ) : (
        <>
          <RunsTable
            runs={runs}
            linkPrefix="/logs"
            highlight={filters.q}
          />
          <div className="flex items-center justify-between text-xs text-zinc-400 dark:text-zinc-500">
            <span>
              {runs.length} shown · {data?.total ?? 0} total
            </span>
            <div className="flex gap-2">
              {hasCursor && (
                <button
                  type="button"
                  onClick={goPrev}
                  className="rounded-lg border border-zinc-200 px-2 py-1 dark:border-zinc-700"
                >
                  ← Newer
                </button>
              )}
              {data?.next_cursor && (
                <button
                  type="button"
                  onClick={goNext}
                  className="rounded-lg border border-zinc-200 px-2 py-1 dark:border-zinc-700"
                >
                  Older →
                </button>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  )
}
