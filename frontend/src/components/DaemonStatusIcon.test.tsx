// DaemonStatusIcon tests: the dot carries the mode + an accessible label; the
// hover tooltip carries the label and live health detail when the daemon is
// running.
import {describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
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

  it('shows live health detail in the hover tooltip', async () => {
    mHealth.mockResolvedValue({
      version: '0.1.0',
      uptime_seconds: 150,
      core: 'healthy',
      scheduler: 'running',
    } as never)
    renderDot('running')
    await waitFor(() =>
      expect(screen.getByRole('tooltip')).toHaveTextContent(
        'core healthy · scheduler running · up 2m'
      )
    )
    expect(screen.getByRole('tooltip')).toHaveTextContent('Daemon healthy')
  })

  it('keeps the tooltip minimal when the daemon is down', () => {
    renderDot('not-running')
    const tooltip = screen.getByRole('tooltip')
    expect(tooltip).toHaveTextContent('Daemon not running')
    expect(tooltip).not.toHaveTextContent('core ')
  })
})