// lib/query.ts (SPEC-12 §5) — TanStack Query hooks for daemon state.
// Health polls every 5 s; when the daemon is down polling backs off to 10 s.
import {useCallback, useState} from 'react'
import {useQuery, useQueryClient} from '@tanstack/react-query'
import * as api from './api'

export const POLL_FRESH_MS = 5_000
export const POLL_DOWN_MS = 10_000

export type DaemonMode = 'running' | 'not-running' | 'starting'

/** Unified daemon mode with a start action for the pill + banner. */
export function useDaemonMode() {
  const queryClient = useQueryClient()
  const [starting, setStarting] = useState(false)

  const {data} = useQuery({
    queryKey: ['daemon-status'],
    queryFn: api.daemonStatus,
    refetchInterval: (query) =>
      query.state.data === 'running' ? POLL_FRESH_MS : POLL_DOWN_MS,
    retry: false,
  })

  const mode: DaemonMode = starting ? 'starting' : (data ?? 'not-running')

  const start = useCallback(async () => {
    if (starting) return
    setStarting(true)
    try {
      await api.startDaemon()
    } finally {
      // Force an immediate re-poll so the pill flips as soon as the daemon
      // answers (SPEC-12 §5): not waiting a full 5 s cycle.
      await queryClient.invalidateQueries({queryKey: ['daemon-status']})
      setStarting(false)
    }
  }, [starting, queryClient])

  return {mode, start}
}

/** Polled health detail; the pill currently reads mode, pages can use this. */
export function useHealth() {
  return useQuery({
    queryKey: ['health'],
    queryFn: api.health,
    refetchInterval: (query) =>
      query.state.data ? POLL_FRESH_MS : POLL_DOWN_MS,
    retry: false,
  })
}