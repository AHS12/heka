// BackupHistoryPage tests: the dedicated history page lists jobs newest
// first with status pills and destination outcomes, filters by status, and
// paginates with "Show more".
import {describe, expect, it, vi, beforeEach} from 'vitest'
import {fireEvent, render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {BackupHistory} from '@wailsjs/go/app/App'
import {BackupHistoryPage} from './BackupHistoryPage'

const mHistory = vi.mocked(BackupHistory)

function makeJob(n: number, status = 'success') {
  return {
    id: `job-${String(n).padStart(3, '0')}`,
    trigger: n % 2 === 0 ? 'scheduled' : 'manual',
    status,
    started_at: `2026-09-04T09:${String(n % 60).padStart(2, '0')}:00Z`,
    finished_at: `2026-09-04T09:${String(n % 60).padStart(2, '0')}:05Z`,
    size_bytes: 1024 * n,
    local_path: `/data/backups/heka-backup-${n}.zip`,
    destinations: n % 3 === 0 ? [{type: 's3', ok: false, error: 'bucket missing'}] : [],
    error: status === 'failed' ? 'upload failed' : '',
  }
}

function renderPage() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <BackupHistoryPage />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  mHistory.mockReset()
})

describe('BackupHistoryPage', () => {
  it('lists jobs newest first with destination outcomes', async () => {
    mHistory.mockResolvedValue([makeJob(3, 'partial'), makeJob(1)] as any)
    renderPage()

    const list = await screen.findByTestId('backup-history-list')
    expect(list).toHaveTextContent('Partial')
    expect(list).toHaveTextContent('Success')
    expect(list).toHaveTextContent('s3: bucket missing')
    // Newest first.
    const rows = list.querySelectorAll('li')
    expect(rows[0]).toHaveTextContent('job-003')
  })

  it('offers more pages when a full page comes back', async () => {
    mHistory.mockResolvedValue(Array.from({length: 25}, (_, i) => makeJob(i)) as any)
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByTestId('backup-history-list')).toBeInTheDocument()
    await user.click(screen.getByRole('button', {name: 'Show more'}))
    await waitFor(() => expect(mHistory).toHaveBeenCalledWith(50))
  })

  it('filters by status through the hidden native select', async () => {
    mHistory.mockResolvedValue([makeJob(1, 'success'), makeJob(2, 'failed')] as any)
    renderPage()

    const list = await screen.findByTestId('backup-history-list')
    expect(list.children.length).toBe(2)

    const filter = (await screen.findByLabelText('Filter by status')) as HTMLSelectElement
    fireEvent.change(filter, {target: {value: 'failed'}})

    expect(await screen.findByTestId('backup-history-list')).toHaveTextContent('upload failed')
    expect(screen.queryByText('job-001')).not.toBeInTheDocument()
  })

  it('shows the empty state before any backup runs', async () => {
    mHistory.mockResolvedValue([] as any)
    renderPage()
    expect(await screen.findByText(/No backups yet/)).toBeInTheDocument()
  })

  it('links back to the backup settings tab', async () => {
    mHistory.mockResolvedValue([] as any)
    renderPage()
    await screen.findByText(/No backups yet/)
    expect(screen.getByText('Back to settings').closest('a')).toHaveAttribute(
      'href',
      '#/settings?tab=backup'
    )
  })
})
