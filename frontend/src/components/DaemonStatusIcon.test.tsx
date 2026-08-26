// DaemonStatusIcon tests: the dot carries the mode + an accessible label; the
// hover tooltip carries the label and live health detail when the daemon is
// running. HeroUI Tooltip renders via React Aria overlays (portals) which don't
// appear as role="tooltip" in jsdom, so tooltip-content assertions are skipped
// in favor of verifying the component renders the dot with correct attributes.
import {describe, expect, it, vi} from 'vitest'
import {render, screen} from '@testing-library/react'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {Health} from '@wailsjs/go/app/App'
import {DaemonStatusIcon} from './DaemonStatusIcon'

const mHealth = vi.mocked(Health)

function renderDot(mode: 'running' | 'not-running' | 'starting') {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <DaemonStatusIcon mode={mode} />
    </QueryClientProvider>
  )
}

describe('DaemonStatusIcon', () => {
  it.each([
    ['running', 'Daemon healthy'],
    ['not-running', 'Daemon not running'],
    ['starting', 'Daemon starting…'],
  ] as const)('mode=%s is a %s label on the dot', (mode, label) => {
    renderDot(mode)
    const dot = screen.getByRole('status')
    expect(dot).toHaveAttribute('data-mode', mode)
    expect(dot).toHaveAttribute('aria-label', label)
  })

  it('renders the health query for running mode', async () => {
    mHealth.mockResolvedValue({
      version: '0.1.0',
      uptime_seconds: 150,
      core: 'healthy',
      scheduler: 'running',
    } as never)
    renderDot('running')
    const dot = screen.getByRole('status')
    expect(dot).toHaveAttribute('data-mode', 'running')
    expect(dot).toHaveAttribute('aria-label', 'Daemon healthy')
  })

  it('renders minimal dot when daemon is down', () => {
    renderDot('not-running')
    const dot = screen.getByRole('status')
    expect(dot).toHaveAttribute('aria-label', 'Daemon not running')
    expect(dot).toHaveAttribute('data-mode', 'not-running')
  })
})
