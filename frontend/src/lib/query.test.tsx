// lib/query.test.tsx — useDaemonMode's down → up recovery: with retry:false
// everywhere, queries that errored while the daemon was down stay dead
// unless something refills them. The mode transition is that something.
import {renderHook, waitFor, act} from '@testing-library/react'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {describe, it, expect, vi, beforeEach, afterEach} from 'vitest'
import type {ReactNode} from 'react'
import {useDaemonMode} from './query'
import * as api from './api'

vi.mock('./api', () => ({
  daemonStatus: vi.fn(),
  startDaemon: vi.fn(),
  health: vi.fn(),
}))

function setup() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const wrapper = ({children}: {children: ReactNode}) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return {client, invalidate, wrapper}
}

describe('useDaemonMode', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('refills all queries on a not-running → running transition', async () => {
    vi.mocked(api.daemonStatus).mockResolvedValueOnce('not-running')
    vi.mocked(api.daemonStatus).mockResolvedValue('running')
    const {invalidate, wrapper} = setup()

    renderHook(() => useDaemonMode(), {wrapper})
    await act(async () => {}) // first poll: not-running
    expect(invalidate).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(10_000) // second poll: running
    })
    await vi.waitFor(() => expect(invalidate).toHaveBeenCalledWith())
    expect(invalidate).toHaveBeenCalledWith()
  })

  it('does not refill when the daemon is already running on mount', async () => {
    vi.mocked(api.daemonStatus).mockResolvedValue('running')
    const {invalidate, wrapper} = setup()

    renderHook(() => useDaemonMode(), {wrapper})
    await act(async () => {
      await vi.advanceTimersByTimeAsync(12_000)
    })
    expect(invalidate).not.toHaveBeenCalledWith()
  })

  it('reports not-running when the poll errors (stopped daemon, stale data)', async () => {
    const err = new Error('pipe gone')
    let calls = 0
    vi.mocked(api.daemonStatus).mockImplementation(() => {
      calls++
      // fail → ok → fail: the error-aware mode must flip on every failure
      return calls === 2
        ? Promise.resolve('running')
        : Promise.reject(err)
    })
    const {wrapper} = setup()

    const {result} = renderHook(() => useDaemonMode(), {wrapper})
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000) // first poll: fails
    })
    expect(result.current.mode).toBe('not-running')

    await act(async () => {
      vi.advanceTimersByTime(10_000) // second poll: running
    })
    await vi.waitFor(() => expect(result.current.mode).toBe('running'))

    await act(async () => {
      vi.advanceTimersByTime(5_000) // third poll: fails again
    })
    await vi.waitFor(() => expect(result.current.mode).toBe('not-running'))
  })

  it('start refills every query once the daemon answers', async () => {
    vi.mocked(api.daemonStatus).mockResolvedValue('not-running')
    vi.mocked(api.startDaemon).mockResolvedValue(undefined)
    const {invalidate, wrapper} = setup()

    const {result} = renderHook(() => useDaemonMode(), {wrapper})
    await act(async () => {
      await result.current.start()
    })
    expect(invalidate).toHaveBeenCalledWith()
  })
})
