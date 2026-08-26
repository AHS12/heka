// TopNav tests (SPEC-12 §6.4, app-shell layout): floating pill bar renders
// every route, marks the active pill via aria-current, and keeps links
// keyboard-focusable. Needs the QueryClient wrapper because the pill bar
// shows the health status.
import {describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import {MemoryRouter} from 'react-router-dom'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {DaemonStatus} from '@wailsjs/go/app/App'
import {TopNav} from './TopNav'

vi.mocked(DaemonStatus).mockResolvedValue('running')

function renderAt(route: string) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <MemoryRouter initialEntries={[route]}>
      <QueryClientProvider client={client}>
        <TopNav />
      </QueryClientProvider>
    </MemoryRouter>
  )
}

describe('TopNav', () => {
  it('renders every route as a floating pill', () => {
    renderAt('/')
    for (const label of [
      'Home',
      'Tasks',
      'Schedules',
      'Logs',
    ]) {
      expect(screen.getByRole('link', {name: label})).toBeInTheDocument()
    }
  })

  it('marks the current route active and others idle', () => {
    renderAt('/tasks')
    const tasks = screen.getByRole('link', {name: 'Tasks'})
    expect(tasks).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', {name: 'Home'})).not.toHaveAttribute(
      'aria-current'
    )
    // Active pill + focus ring are driven by the accent tokens (Settings will
    // let users repoint the accent later).
    expect(tasks.className).toContain('bg-accent')
    expect(tasks.className).toContain('ring-accent-ring')
  })

  it('keeps pills focusable for keyboard navigation', () => {
    renderAt('/')
    const tasks = screen.getByRole('link', {name: 'Tasks'})
    tasks.focus()
    expect(tasks).toHaveFocus()
  })

  it('offers the daemon status dot', async () => {
    renderAt('/')
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveAttribute(
        'aria-label',
        'Daemon healthy'
      )
    )
    expect(screen.getByRole('status')).toHaveAttribute('data-mode', 'running')
    // HeroUI Tooltip renders via React Aria overlays (portals) — not available in jsdom.
  })
})