// api.ts tests (SPEC-12 §6.1): DTO mapping, envelope-code parsing, and the
// daemon-not-running sentinel. The bindings come from the project-wide mock
// in src/test/setup.ts.
import {describe, expect, it, vi} from 'vitest'
import {Health, DaemonStatus, StartDaemon} from '@wailsjs/go/app/App'
import {APIError, health, daemonStatus, startDaemon} from './api'

const mHealth = vi.mocked(Health)
const mStatus = vi.mocked(DaemonStatus)
const mStart = vi.mocked(StartDaemon)

describe('health', () => {
  it('maps the raw DTO over the bridge', async () => {
    mHealth.mockResolvedValue({
      version: '0.1.0',
      uptime_seconds: 42,
      core: 'healthy',
      scheduler: 'running',
    })
    await expect(health()).resolves.toEqual({
      version: '0.1.0',
      uptime_seconds: 42,
      core: 'healthy',
      scheduler: 'running',
    })
  })

  it('parses the enveloped code from the flattened bridge error', async () => {
    mHealth.mockRejectedValue(new Error('not_found: no such task'))
    await expect(health()).rejects.toMatchObject({
      name: 'APIError',
      code: 'not_found',
      message: 'no such task',
    })
  })

  it('maps a raw bridge error to a sentinel daemon_not_running', async () => {
    mHealth.mockRejectedValue(new Error('ipc: heka daemon is not running'))
    await expect(health()).rejects.toBeInstanceOf(APIError)
    await expect(health()).rejects.toMatchObject({code: 'daemon_not_running'})
  })

  it('falls through with a plain Error for uncoded failures', async () => {
    mHealth.mockRejectedValue(new Error('window go is undefined'))
    const err = await health().catch((e) => e)
    expect(err).not.toBeInstanceOf(APIError)
    expect(err).toHaveProperty('message', 'window go is undefined')
  })
})

describe('daemonStatus', () => {
  it('passes the status through', async () => {
    mStatus.mockResolvedValue('running')
    await expect(daemonStatus()).resolves.toBe('running')
  })

  it('rewraps into APIError on bridge rejection', async () => {
    mStatus.mockRejectedValue(new Error('internal: boom'))
    await expect(daemonStatus()).rejects.toMatchObject({code: 'internal'})
  })
})

describe('startDaemon', () => {
  it('resolves when the spawn succeeded', async () => {
    mStart.mockResolvedValue(undefined)
    await expect(startDaemon()).resolves.toBeUndefined()
  })

  it('surfaces spawn failures with their code', async () => {
    mStart.mockRejectedValue(new Error('already_running: daemon busy'))
    await expect(startDaemon()).rejects.toMatchObject({code: 'already_running'})
  })
})