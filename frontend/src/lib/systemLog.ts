import {useQuery} from '@tanstack/react-query'
import {listSystemLogs} from './api'

const SYSTEM_LOG_KEY = ['system-log'] as const

/** Polls the daemon's own event log (reconcile, lifecycle), newest first. */
export function useSystemLog(limit = 200) {
  return useQuery({
    queryKey: [...SYSTEM_LOG_KEY, limit],
    queryFn: () => listSystemLogs(limit),
    refetchInterval: 5_000,
  })
}
