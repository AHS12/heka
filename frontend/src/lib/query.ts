// lib/query.ts (SPEC-12 §5) — TanStack Query hooks for daemon state.
// Health polls every 5 s; when the daemon is down polling backs off to 10 s.
import {useCallback, useEffect, useRef, useState} from 'react'
import {useQuery, useQueryClient} from '@tanstack/react-query'
import * as api from './api'

export const POLL_FRESH_MS = 5_000
export const POLL_DOWN_MS = 10_000

export type DaemonMode = 'running' | 'not-running' | 'starting'

/** Unified daemon mode with a start action for the pill + banner. */
export function useDaemonMode() {
  const queryClient = useQueryClient()
  const [starting, setStarting] = useState(false)

  const {data, status} = useQuery({
    queryKey: ['daemon-status'],
    queryFn: api.daemonStatus,
    refetchInterval: (query) =>
      query.state.data === 'running' ? POLL_FRESH_MS : POLL_DOWN_MS,
    retry: false,
  })

  // An unreachable daemon is a stopped daemon as far as the UI is
  // concerned: retry:false keeps the last good data in state on errors, so
  // `data ?? 'not-running'` alone would keep reporting 'running' forever
  // after the daemon dies (the stopped-but-still-healthy GUI bug).
  const mode: DaemonMode =
    starting
      ? 'starting'
      : data === 'running' && status !== 'error'
        ? 'running'
        : 'not-running'

  // Down → up recovery (SPEC-12 §5): with retry:false, every query that
  // errored while the daemon was down stays dead — tasks, runs, schedules
  // never refill on their own and the dashboard looks frozen after the
  // daemon starts. When the poll observes a real not-running → running
  // transition, refill everything mounted. A failed poll counts as
  // not-running; the initial unknown state (never observed) does not, so a
  // daemon that was already up on app open does not trigger a refill.
  const prevObserved = useRef<DaemonMode | undefined>(undefined)
  useEffect(() => {
    const observed: DaemonMode | undefined =
      status === 'error'
        ? 'not-running'
        : data === undefined
          ? undefined
          : data === 'running'
            ? 'running'
            : 'not-running'
    if (observed === 'running' && prevObserved.current === 'not-running') {
      void queryClient.invalidateQueries()
    }
    prevObserved.current = observed
  }, [data, status, queryClient])

  const start = useCallback(async () => {
    if (starting) return
    setStarting(true)
    try {
      await api.startDaemon()
    } finally {
      // startDaemon resolves once the daemon answers health, so every
      // query (not just the status poll) can refetch immediately.
      await queryClient.invalidateQueries()
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