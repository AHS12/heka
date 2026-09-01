// lib/tasks.ts (SPEC-13 §3) — server-state hooks for the Tasks surface:
// list/detail queries with active-run polling, CRUD mutations, optimistic
// enable toggle, and the import/export mutations.
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query'
import type {TaskDetail, TaskSummary} from './api'
import * as api from './api'

const TASKS_KEY = ['tasks'] as const

export function taskKey(slug: string) {
  return ['task', slug] as const
}

export function useTasks() {
  return useQuery({
    queryKey: TASKS_KEY,
    queryFn: api.listTasks,
    // Always-fresh list: runs flip status while the page is up, and the
    // completion is reflected on the next tick (self-updating, SPEC-13 §3).
    refetchInterval: 5_000,
  })
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
      const previous = qc.getQueryData<TaskSummary[]>(TASKS_KEY)
      qc.setQueryData<TaskSummary[]>(TASKS_KEY, (rows) =>
        (rows ?? []).map((t) =>
          t.slug === slug
            ? {...t, last_status: 'running', last_run_at: new Date().toISOString()}
            : t
        )
      )
      return {previous}
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.previous) {
        qc.setQueryData(TASKS_KEY, ctx.previous)
      }
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
      const previous = qc.getQueryData<TaskSummary[]>(TASKS_KEY)
      qc.setQueryData<TaskSummary[]>(TASKS_KEY, (rows) =>
        (rows ?? []).map((t) => (t.slug === slug ? {...t, enabled} : t))
      )
      return {previous}
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.previous) {
        qc.setQueryData(TASKS_KEY, ctx.previous)
      }
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