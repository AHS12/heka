// lib/revision.ts — the revision pulse. One cheap aggregate request every
// few seconds tells the shell WHEN task/schedule/run data changed anywhere —
// GUI mutations, CLI commands, scheduled runs, on-disk YAML edits — without
// refetching list pages. A changed signature invalidates exactly that
// domain's query prefix; TanStack refetches only the mounted queries.
import {useEffect, useRef} from 'react'
import {useQuery, useQueryClient} from '@tanstack/react-query'
import type {DataRevision} from './api'
import * as api from './api'

/** Poll cadence for the pulse; tests lower this instead of fake-timer
 *  gymnastics. */
export const pulseConfig = {intervalMs: 5_000}

/** Query-key prefixes whose signature moved between two observations.
 *  The first observation (prev == null) is the baseline: nothing changed. */
export function changedPrefixes(
  prev: DataRevision | undefined,
  next: DataRevision
): readonly (readonly string[])[] {
  if (!prev) return []
  const prefixes: (readonly string[])[] = []
  if (next.tasks !== prev.tasks) prefixes.push(['tasks'])
  if (next.schedules !== prev.schedules) prefixes.push(['schedules'])
  if (next.runs !== prev.runs) prefixes.push(['runs'])
  return prefixes
}

export function useDataPulse() {
  const qc = useQueryClient()
  const {data} = useQuery({
    queryKey: ['revision'],
    queryFn: api.getRevision,
    refetchInterval: pulseConfig.intervalMs,
    retry: false,
  })

  const seen = useRef<DataRevision | undefined>(undefined)
  useEffect(() => {
    if (!data) return
    const prev = seen.current
    seen.current = data
    for (const prefix of changedPrefixes(prev, data)) {
      void qc.invalidateQueries({queryKey: prefix})
    }
  }, [data, qc])
}
