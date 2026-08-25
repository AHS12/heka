import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query'
import * as api from './api'

const SCHEDULES_KEY = ['schedules'] as const

export function schedulesKey(kind?: string) {
  return kind ? (['schedules', kind] as const) : SCHEDULES_KEY
}

export function useSchedules(kind?: string) {
  return useQuery({
    queryKey: schedulesKey(kind),
    queryFn: () => api.listSchedules(kind),
    refetchInterval: 15_000,
  })
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
      const previous = qc.getQueryData<api.Schedule[]>(SCHEDULES_KEY)
      qc.setQueryData<api.Schedule[]>(SCHEDULES_KEY, (rows) =>
        (rows ?? []).map((s) => (s.id === id ? {...s, enabled} : s))
      )
      return {previous}
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.previous) {
        qc.setQueryData(SCHEDULES_KEY, ctx.previous)
      }
    },
    onSettled: () => void qc.invalidateQueries({queryKey: SCHEDULES_KEY}),
  })
}
