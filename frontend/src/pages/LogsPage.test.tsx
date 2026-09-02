// LogsPage tests: the Runs view lists task runs, the System view shows the
// daemon's own event log (scheduler reconcile, lifecycle).
import {describe, expect, it, vi} from 'vitest'
import {render, screen} from '@testing-library/react'
import {MemoryRouter} from 'react-router-dom'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {ListRuns, ListSystemLog} from '@wailsjs/go/app/App'
import {LogsPage} from './LogsPage'

const mListRuns = vi.mocked(ListRuns)
const mListSystemLog = vi.mocked(ListSystemLog)

function renderPage(url = '/logs') {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[url]}>
        <LogsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('LogsPage', () => {
  it('defaults to the Runs view', async () => {
    mListRuns.mockResolvedValue({runs: [], total: 0} as any)
    renderPage()
    expect(await screen.findByText('No logs yet.')).toBeInTheDocument()
    expect(mListRuns).toHaveBeenCalled()
    expect(mListSystemLog).not.toHaveBeenCalled()
  })

  it('shows daemon events in the System view', async () => {
    mListSystemLog.mockResolvedValue([
      {
        id: 2,
        ts: '2026-09-02T07:15:00Z',
        level: 'warn',
        event: 'reconcile',
        message: 'schedule "nightly": start failed (spawn failed), will retry next pass',
      },
      {
        id: 1,
        ts: '2026-09-02T07:00:00Z',
        level: 'info',
        event: 'reconcile',
        message: 'schedule "nightly": missed 2 activation(s) — started catch-up run',
      },
    ])
    renderPage('/logs?view=system')

    expect(await screen.findByText(/missed 2 activation\(s\)/)).toBeInTheDocument()
    expect(screen.getByText(/will retry next pass/)).toBeInTheDocument()
    expect(mListSystemLog).toHaveBeenCalled()
    expect(mListRuns).not.toHaveBeenCalled()
  })

  it('shows an empty state when the system log is empty', async () => {
    mListSystemLog.mockResolvedValue([])
    renderPage('/logs?view=system')
    expect(await screen.findByText(/No system events yet/)).toBeInTheDocument()
  })
})
