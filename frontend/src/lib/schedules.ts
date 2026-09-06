import {useMutation, useQuery, useQueryClient, useInfiniteQuery} from '@tanstack/react-query'
import type {QueryClient, QueryKey} from '@tanstack/react-query'
import type {Schedule, ScheduleListResult} from './api'
import * as api from './api'

const SCHEDULES_KEY = ['schedules'] as const

/** Page size for the paginated schedules list (server default/cap: 50/200). */
export const SCHEDULES_PAGE_LIMIT = 50

/** Server-side filters for the paginated schedules list. */
export interface SchedulePageQuery {
  q?: string
  kind?: string
}

export function schedulesKey(kind?: string) {
  return kind ? (['schedules', kind] as const) : SCHEDULES_KEY
}

/** Dashboard / form pickers: full list, no pagination. */
export function useSchedules(kind?: string) {
  return useQuery({
    queryKey: schedulesKey(kind),
    queryFn: () => api.listSchedules(kind),
  })
}

/** Infinite, accumulating schedules list for the schedules page (infinite
 *  scroll, slug keyset cursors). No polling — freshness comes from mutation
 *  invalidations + the revision pulse. */
export function useSchedulesPage(filters: SchedulePageQuery) {
  return useInfiniteQuery({
    queryKey: [...SCHEDULES_KEY, filters] as const,
    queryFn: ({pageParam}) =>
      api.listSchedulesPage({
        ...filters,
        cursor: pageParam || undefined,
        limit: SCHEDULES_PAGE_LIMIT,
      }),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_cursor || undefined,
  })
}

/** Flattened rows across all loaded pages. */
export function schedulePageRows(
  data: {pages: {schedules: Schedule[]}[]} | undefined
): Schedule[] {
  return data?.pages.flatMap((p) => p.schedules) ?? []
}

// ---- Optimistic row patching across every cached ['schedules'] shape
// (flat picker/dashboard cache and all paginated page sets).

type ScheduleCacheSnapshot = [QueryKey, ScheduleListResult | Schedule[] | InfiniteScheduleData | undefined][]

/** One page of the useInfiniteQuery cache ({pages, pageParams}). */
interface InfiniteScheduleData {
  pages: ScheduleListResult[]
}

function patchScheduleRows(
  qc: QueryClient,
  patch: (s: Schedule) => Schedule
): ScheduleCacheSnapshot {
  const snapshots = qc.getQueriesData<ScheduleListResult | Schedule[] | InfiniteScheduleData>({
    queryKey: SCHEDULES_KEY,
  })
  for (const [key, data] of snapshots) {
    if (!data) continue
    if (Array.isArray(data)) {
      // Flat list cache (dashboard next-run panel, form picker).
      qc.setQueryData(key, data.map(patch))
    } else if ('pages' in data) {
      // Infinite query cache: patch every loaded page.
      qc.setQueryData(key, {
        ...data,
        pages: data.pages.map((p) => ({...p, schedules: p.schedules.map(patch)})),
      })
    } else {
      // Envelope-shaped flat cache (defensive).
      qc.setQueryData<ScheduleListResult>(key, {...data, schedules: data.schedules.map(patch)})
    }
  }
  return snapshots
}

function restoreScheduleRows(qc: QueryClient, snapshots: ScheduleCacheSnapshot) {
  for (const [key, data] of snapshots) {
    qc.setQueryData(key, data)
  }
}

export function useCreateSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: {
      slug: string
      taskSlug: string
      kind: string
      cron: string
      runAt: string
      missedPolicy: string
    }) =>
      api.createSchedule(
        args.slug,
        args.taskSlug,
        args.kind,
        args.cron,
        args.runAt,
        args.missedPolicy
      ),
    onSuccess: () => void qc.invalidateQueries({queryKey: SCHEDULES_KEY}),
  })
}

export function useUpdateSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: {
      id: string
      slug: string
      taskSlug: string
      kind: string
      cron: string
      runAt: string
      missedPolicy: string
    }) =>
      api.updateSchedule(
        args.id,
        args.slug,
        args.taskSlug,
        args.kind,
        args.cron,
        args.runAt,
        args.missedPolicy
      ),
    onSuccess: () => void qc.invalidateQueries({queryKey: SCHEDULES_KEY}),
  })
}

export function useDeleteSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.deleteSchedule(id),
    onSuccess: () => void qc.invalidateQueries({queryKey: SCHEDULES_KEY}),
  })
}

export function useToggleSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({id, enabled}: {id: string; enabled: boolean}) =>
      enabled ? api.enableSchedule(id) : api.disableSchedule(id),
    onMutate: async ({id, enabled}) => {
      await qc.cancelQueries({queryKey: SCHEDULES_KEY})
      const previous = patchScheduleRows(qc, (s) =>
        s.id === id ? {...s, enabled} : s
      )
      return {previous}
    },
    onError: (_err, _vars, ctx) => {
      restoreScheduleRows(qc, ctx?.previous ?? [])
    },
    onSettled: () => void qc.invalidateQueries({queryKey: SCHEDULES_KEY}),
  })
}

export function useReconcileSchedules() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.reconcileSchedules(),
    onSuccess: () => {
      qc.invalidateQueries({queryKey: SCHEDULES_KEY})
      // Reconcile fires runs — task chips and the runs surface move too.
      qc.invalidateQueries({queryKey: ['runs']})
      qc.invalidateQueries({queryKey: ['tasks']})
    },
  })
}
