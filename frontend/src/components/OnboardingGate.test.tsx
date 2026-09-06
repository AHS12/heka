// OnboardingGate tests: the launch decision (tour vs What's New vs nothing)
// and the `heka dev …` trigger consumption — including that the poll loop
// swaps overlays while running.
import {describe, expect, it, vi, beforeEach} from 'vitest'
import {render, screen, waitFor, act} from '@testing-library/react'
import {MemoryRouter} from 'react-router-dom'
import {TakeDevTrigger} from '@wailsjs/go/app/App'
import {driver} from 'driver.js'
import {OnboardingGate} from './OnboardingGate'
import {useOnboarding} from '../lib/onboarding'
import {APP_VERSION} from '../lib/version'

beforeEach(() => {
  localStorage.clear()
  useOnboarding.setState({tourCompleted: false, seenVersion: '', mode: 'none', force: false, scope: 'newer'})
  vi.mocked(TakeDevTrigger).mockResolvedValue(null as any)
})

function renderGate() {
  return render(
    <MemoryRouter>
      <OnboardingGate />
    </MemoryRouter>
  )
}

describe('OnboardingGate', () => {
  it('auto-opens the tour on a fresh install', async () => {
    renderGate()
    await waitFor(() => {
      expect(useOnboarding.getState().mode).toBe('tour')
      expect(vi.mocked(driver)).toHaveBeenCalled()
    })
    // The tour itself renders nothing; driver.js owns the visuals.
    expect(screen.queryByText(/What.s new in Heka/)).not.toBeInTheDocument()
  })

  it('opens the What-new dialog when the tour is done and the version moved', async () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: '0.8.1'})
    renderGate()
    expect(await screen.findByText(/What.s new in Heka/)).toBeInTheDocument()
    expect(useOnboarding.getState().mode).toBe('whats-new')
  })

  it('does nothing when everything is up to date', async () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    renderGate()
    await waitFor(() => expect(useOnboarding.getState().mode).toBe('none'))
    expect(vi.mocked(driver)).not.toHaveBeenCalled()
  })

  it('consumes a whats-new trigger (force mode, every release)', async () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    vi.mocked(TakeDevTrigger).mockResolvedValue({trigger: 'whats-new'})
    renderGate()
    expect(await screen.findByText(/What.s new in Heka/)).toBeInTheDocument()
    const s = useOnboarding.getState()
    expect(s.force).toBe(true)
    expect(s.scope).toBe('all')
  })

  it('consumes a tour trigger mid-session', async () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    vi.mocked(TakeDevTrigger).mockResolvedValue({trigger: 'tour'})
    renderGate()
    await waitFor(() => {
      expect(useOnboarding.getState().mode).toBe('tour')
      expect(vi.mocked(driver)).toHaveBeenCalled()
    })
  })

  it('consumes an update-from trigger by stamping the seen version', async () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    vi.mocked(TakeDevTrigger).mockResolvedValue({trigger: 'update-from', version: '0.7.0'})
    renderGate()
    await waitFor(() => expect(useOnboarding.getState().seenVersion).toBe('0.7.0'))
    expect(useOnboarding.getState().mode).toBe('none')
  })

  it('consumes a reset trigger and lands on the fresh-install tour', async () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    vi.mocked(TakeDevTrigger).mockResolvedValue({trigger: 'reset'})
    renderGate()
    await waitFor(() => {
      expect(useOnboarding.getState().mode).toBe('tour')
      expect(useOnboarding.getState().tourCompleted).toBe(false)
    })
  })

  it('the poll loop picks up a trigger that arrives after mount', async () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    vi.mocked(TakeDevTrigger)
      .mockResolvedValueOnce(null as any) // mount-time consume: nothing pending
      .mockResolvedValue({trigger: 'whats-new'}) // first poll tick
    vi.useFakeTimers()
    renderGate()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4100)
    })
    expect(useOnboarding.getState().mode).toBe('whats-new')
    vi.useRealTimers()
  })
})
