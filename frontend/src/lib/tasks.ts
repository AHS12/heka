// lib/tasks.ts (SPEC-13 §3) — server-state hooks for the Tasks surface:
// list/detail queries, CRUD mutations, optimistic enable toggle, and the
// import/export mutations. The tasks page uses useTasksPage (cursor
// pagination, accumulated pages kept); pickers still use useTasks.
import {useMutation, useQuery, useQueryClient, useInfiniteQuery} from '@tanstack/react-query'
import type {QueryClient, QueryKey} from '@tanstack/react-query'
import type {TaskDetail, TaskSummary, TaskListResult} from './api'
import * as api from './api'

const TASKS_KEY = ['tasks'] as const

/** Page size for the paginated tasks list (server default/cap: 50/200). */
export const TASKS_PAGE_LIMIT = 50

/** Server-side filters for the paginated tasks list. Changing any of them
 *  produces a new query key, so the scroll accumulation resets cleanly. */
export interface TaskPageQuery {
  q?: string
  enabled?: 'enabled' | 'disabled'
  type?: string
}

export function taskKey(slug: string) {
  return ['task', slug] as const
}

export function useTasks() {
  return useQuery({
    queryKey: TASKS_KEY,
    queryFn: api.listTasks,
  })
}

/** Infinite, accumulating tasks list: pages are cached per filter set and
 *  kept as the user scrolls (slug keyset cursors stay stable). No polling —
 *  freshness comes from mutation invalidations + the revision pulse. */
export function useTasksPage(filters: TaskPageQuery) {
  return useInfiniteQuery({
    queryKey: [...TASKS_KEY, filters] as const,
    queryFn: ({pageParam}) =>
      api.listTasksPage({...filters, cursor: pageParam || undefined, limit: TASKS_PAGE_LIMIT}),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_cursor || undefined,
  })
}

/** Flattened rows across all loaded pages. */
export function taskPageRows(
  data: {pages: {tasks: TaskSummary[]}[]} | undefined
): TaskSummary[] {
  return data?.pages.flatMap((p) => p.tasks) ?? []
}

// ---- Optimistic row patching: mutates every cached ['tasks'] shape (the
// flat picker cache and all paginated page sets) so status chips flip
// instantly no matter which surface is open.

type TaskCacheSnapshot = [QueryKey, TaskListResult | TaskSummary[] | InfiniteTaskData | undefined][]

/** One page of the useInfiniteQuery cache ({pages, pageParams}). */
interface InfiniteTaskData {
  pages: TaskListResult[]
}

function patchTaskRows(
  qc: QueryClient,
  patch: (t: TaskSummary) => TaskSummary
): TaskCacheSnapshot {
  const snapshots = qc.getQueriesData<TaskListResult | TaskSummary[] | InfiniteTaskData>({
    queryKey: TASKS_KEY,
  })
  for (const [key, data] of snapshots) {
    if (!data) continue
    if (Array.isArray(data)) {
      // Flat list cache (pickers).
      qc.setQueryData(key, data.map(patch))
    } else if ('pages' in data) {
      // Infinite query cache: patch every loaded page.
      qc.setQueryData(key, {
        ...data,
        pages: data.pages.map((p) => ({...p, tasks: p.tasks.map(patch)})),
      })
    } else {
      // Envelope-shaped flat cache (defensive).
      qc.setQueryData<TaskListResult>(key, {...data, tasks: data.tasks.map(patch)})
    }
  }
  return snapshots
}

function restoreTaskRows(qc: QueryClient, snapshots: TaskCacheSnapshot) {
  for (const [key, data] of snapshots) {
    qc.setQueryData(key, data)
  }
}

export function useTask(slug: string | undefined) {
  return useQuery({
    queryKey: taskKey(slug ?? ''),
    queryFn: () => api.getTask(slug as string),
    enabled: !!slug,
  })
}

export function useTaskYAML(slug: string | undefined) {
  return useQuery({
    queryKey: ['task-yaml', slug ?? ''],
    queryFn: () => api.getTaskYAML(slug as string),
    enabled: !!slug,
  })
}

export function useCreateTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (yaml: string) => api.createTask(yaml),
    onSuccess: (result) => {
      void qc.invalidateQueries({queryKey: TASKS_KEY})
      void qc.invalidateQueries({queryKey: taskKey(result.task.slug)})
    },
  })
}

export function useUpdateTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: {slug: string; yaml: string}) =>
      api.updateTask(args.slug, args.yaml),
    onSuccess: (result) => {
      void qc.invalidateQueries({queryKey: TASKS_KEY})
      void qc.invalidateQueries({queryKey: taskKey(result.task.slug)})
    },
  })
}

export function useDeleteTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (slug: string) => api.deleteTask(slug),
    onSuccess: () => void qc.invalidateQueries({queryKey: TASKS_KEY}),
  })
}

export function useRunTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({slug, trigger}: {slug: string; trigger?: string}) =>
      api.runTask(slug, trigger),
    onMutate: async ({slug}) => {
      await qc.cancelQueries({queryKey: TASKS_KEY})
      const previous = patchTaskRows(qc, (t) =>
        t.slug === slug
          ? {...t, last_status: 'running', last_run_at: new Date().toISOString()}
          : t
      )
      return {previous}
    },
    onError: (_err, _vars, ctx) => {
      restoreTaskRows(qc, ctx?.previous ?? [])
    },
    onSettled: () => void qc.invalidateQueries({queryKey: TASKS_KEY}),
  })
}

/** Optimistic enabled toggle (SPEC-13 §3): cache flips immediately, rolls
 *  back on error, always re-invalidates on settle. */
export function useSetTaskEnabled() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({slug, enabled}: {slug: string; enabled: boolean}) =>
      api.setTaskEnabled(slug, enabled),
    onMutate: async ({slug, enabled}) => {
      await qc.cancelQueries({queryKey: TASKS_KEY})
      const previous = patchTaskRows(qc, (t) =>
        t.slug === slug ? {...t, enabled} : t
      )
      return {previous}
    },
    onError: (_err, _vars, ctx) => {
      restoreTaskRows(qc, ctx?.previous ?? [])
    },
    onSettled: () => void qc.invalidateQueries({queryKey: TASKS_KEY}),
  })
}

export function useImportTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.importTaskFromFile(),
    onSuccess: (result) => {
      void qc.invalidateQueries({queryKey: TASKS_KEY})
      void qc.invalidateQueries({queryKey: taskKey(result.task.slug)})
    },
  })
}

export function useExportTask() {
  return useMutation({mutationFn: (slug: string) => api.exportTaskYAML(slug)})
}

export type {TaskDetail}