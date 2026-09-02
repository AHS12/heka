// SchedulesPage tests (SPEC-14 §2): the missed-run reconcile button surfaces
// HeroUI success/danger toasts, and the schedule card shows the missed policy.
import {afterEach, describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {MemoryRouter} from 'react-router-dom'
import {Toast} from '@heroui/react'
import {ListSchedules, ReconcileSchedules} from '@wailsjs/go/app/App'
import type {ipc} from '@wailsjs/go/models'
import {SchedulesPage} from './SchedulesPage'

const mList = vi.mocked(ListSchedules)
const mReconcile = vi.mocked(ReconcileSchedules)

const seed: ipc.Schedule[] = [
  {
    id: 's1',
    slug: 'daily-backup',
    task_slug: 'backup',
    kind: 'recurring',
    cron: '0 9 * * *',
    enabled: true,
    missed_policy: 'run_now',
    skipped_count: 0,
    missed_count: 1,
  },
]

function renderPage() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <Toast.Provider />
      <MemoryRouter>
        <SchedulesPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SchedulesPage', () => {
  afterEach(() => {
    Toast.toast.clear()
  })

  it('shows a success toast when reconcile starts', async () => {
    mList.mockResolvedValue(seed)
    mReconcile.mockResolvedValue()
    const user = userEvent.setup()

    renderPage()
    await user.click(await screen.findByRole('button', {name: 'Reconcile now'}))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Reconcile started'
    )
    expect(mReconcile).toHaveBeenCalledTimes(1)
  })

  it('shows a danger toast when reconcile fails', async () => {
    mList.mockResolvedValue(seed)
    mReconcile.mockRejectedValue(new Error('internal: db locked'))
    const user = userEvent.setup()

    renderPage()
    await user.click(await screen.findByRole('button', {name: 'Reconcile now'}))

    await waitFor(() => {
      const alerts = screen.getAllByRole('alert')
      expect(
        alerts.some((a) => a.textContent?.includes('Reconcile failed'))
      ).toBe(true)
    })
  })

  it('shows the missed policy on each schedule card', async () => {
    mList.mockResolvedValue(seed)
    renderPage()

    expect(await screen.findByTestId('schedule-missed-policy-s1')).toHaveTextContent(
      'Run now'
    )
  })
})
