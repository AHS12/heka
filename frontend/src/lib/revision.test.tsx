// revision.ts tests: the diff logic (baseline, per-prefix changes, no-op on
// equal signatures) and the hook wiring (baseline mounts silently, a changed
// tasks signature invalidates exactly ['tasks']).
import {describe, expect, it, vi, beforeEach} from 'vitest'
import {renderHook, waitFor, act} from '@testing-library/react'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {DataRevision} from '@wailsjs/go/app/App'
import {changedPrefixes, useDataPulse, pulseConfig} from './revision'
import type {DataRevision as Rev} from './api'

const rev = (tasks: string, schedules: string, runs: string): Rev => ({
  tasks,
  schedules,
  runs,
})

describe('changedPrefixes', () => {
  it('treats the first observation as a baseline, not a change', () => {
    expect(changedPrefixes(undefined, rev('a', 'b', 'c'))).toEqual([])
  })

  it('invalidates exactly the prefix whose signature moved', () => {
    expect(changedPrefixes(rev('a', 'b', 'c'), rev('a2', 'b', 'c'))).toEqual([['tasks']])
    expect(changedPrefixes(rev('a', 'b', 'c'), rev('a', 'b2', 'c'))).toEqual([['schedules']])
    expect(changedPrefixes(rev('a', 'b', 'c'), rev('a', 'b', 'c2'))).toEqual([['runs']])
    expect(changedPrefixes(rev('a', 'b', 'c'), rev('x', 'y', 'z'))).toEqual([
      ['tasks'],
      ['schedules'],
      ['runs'],
    ])
  })

  it('is silent when nothing changed', () => {
    expect(changedPrefixes(rev('a', 'b', 'c'), rev('a', 'b', 'c'))).toEqual([])
  })
})

describe('useDataPulse', () => {
  beforeEach(() => {
    pulseConfig.intervalMs = 10
    vi.mocked(DataRevision).mockReset()
  })

  function renderPulse() {
    const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const unwrap = renderHook(() => useDataPulse(), {
      wrapper: ({children}) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    })
    return {invalidate, unwrap}
  }

  it('mounts silently: the baseline never invalidates', async () => {
    vi.mocked(DataRevision).mockResolvedValue(rev('a', 'b', 'c'))
    const {invalidate} = renderPulse()
    await waitFor(() => expect(vi.mocked(DataRevision)).toHaveBeenCalledTimes(1))
    await new Promise((r) => setTimeout(r, 20))
    expect(invalidate).not.toHaveBeenCalled()
  })

  it('invalidates exactly the changed domain on the next pulse', async () => {
    vi.mocked(DataRevision)
      .mockResolvedValueOnce(rev('a', 'b', 'c')) // baseline
      .mockResolvedValue(rev('a2', 'b', 'c')) // a task changed somewhere
    const {invalidate} = renderPulse()
    await waitFor(() => expect(vi.mocked(DataRevision)).toHaveBeenCalledTimes(1))

    // Second pulse observes the changed tasks signature and invalidates
    // exactly that domain — once, no matter how many quiet pulses follow.
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({queryKey: ['tasks']}))
    expect(invalidate).toHaveBeenCalledTimes(1)
    expect(vi.mocked(DataRevision).mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it('stays quiet while the daemon is down (fetch errors)', async () => {
    vi.mocked(DataRevision).mockRejectedValue(new Error('daemon_not_running: nope'))
    const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    renderHook(() => useDataPulse(), {
      wrapper: ({children}) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    })
    await waitFor(() => expect(vi.mocked(DataRevision)).toHaveBeenCalled())
    await new Promise((r) => setTimeout(r, 20))
    expect(invalidate).not.toHaveBeenCalled()
  })
})
