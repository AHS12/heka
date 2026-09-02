import {useQuery, useQueryClient} from '@tanstack/react-query'
import {useCallback} from 'react'
import * as api from './api'

export interface RunFilters {
  task?: string
  status?: string
  from?: string
  to?: string
  q?: string
  cursor?: string
  limit?: number
  order?: string
}

/** Human-readable run status labels — the backend stores snake_case ids,
 *  but the UI always renders these. Unknown statuses fall back to the id
 *  with underscores swapped for spaces. */
export const STATUS_LABELS: Record<string, string> = {
  success: 'Success',
  failed: 'Failed',
  timed_out: 'Timed out',
  running: 'Running',
  queued: 'Queued',
  cancelled: 'Cancelled',
  skipped: 'Skipped',
  missed: 'Missed',
}

export function statusLabel(status: string): string {
  return STATUS_LABELS[status] ?? status.replace(/_/g, ' ')
}

const STATS_KEY = ['stats'] as const

/** Dashboard aggregate (SPEC-16 §1): counts, 7-day history, recent runs. */
export function useStats() {
  return useQuery({
    queryKey: STATS_KEY,
    queryFn: api.getStats,
    refetchInterval: 15_000,
  })
}

const RUNS_KEY = ['runs'] as const

export function runsKey(filters: RunFilters) {
  return [...RUNS_KEY, filters] as const
}

export function useRuns(filters: RunFilters) {
  return useQuery({
    queryKey: runsKey(filters),
    queryFn: () => api.listRuns(filters),
    refetchInterval: (query) => {
      const runs = query.state.data?.runs ?? []
      const hasActive = runs.some((r) => r.status === 'running' || r.status === 'queued')
      return hasActive ? 3_000 : false
    },
  })
}

export function useRunDetail(runID: string | undefined) {
  return useQuery({
    queryKey: ['run', runID ?? ''],
    queryFn: () => api.getRun(runID as string),
    enabled: !!runID,
  })
}

export function usePollingRuns(filters: RunFilters) {
  const queryClient = useQueryClient()
  const query = useRuns(filters)

  const hasActive = (query.data?.runs ?? []).some(
    (r) => r.status === 'running' || r.status === 'queued'
  )

  const refresh = useCallback(() => {
    void queryClient.invalidateQueries({queryKey: runsKey(filters)})
  }, [queryClient, filters])

  return {...query, hasActive, refresh}
}

/** Builds URL search params from run filters for shareable URLs. */
export function filtersToParams(filters: RunFilters): URLSearchParams {
  const p = new URLSearchParams()
  if (filters.task) p.set('task', filters.task)
  if (filters.status) p.set('status', filters.status)
  if (filters.from) p.set('from', filters.from)
  if (filters.to) p.set('to', filters.to)
  if (filters.q) p.set('q', filters.q)
  if (filters.cursor) p.set('cursor', filters.cursor)
  if (filters.limit && filters.limit !== 25) p.set('limit', String(filters.limit))
  if (filters.order) p.set('order', filters.order)
  return p
}

/** Parses URL search params into run filters. */
export function paramsToFilters(sp: URLSearchParams): RunFilters {
  return {
    task: sp.get('task') ?? undefined,
    status: sp.get('status') ?? undefined,
    from: sp.get('from') ?? undefined,
    to: sp.get('to') ?? undefined,
    q: sp.get('q') ?? undefined,
    cursor: sp.get('cursor') ?? undefined,
    limit: sp.get('limit') ? Number(sp.get('limit')) : undefined,
    order: sp.get('order') ?? undefined,
  }
}
