// SchedulesPage tests (SPEC-14 §2): the missed-run reconcile button surfaces
// HeroUI success/danger toasts, and the schedule card shows the missed policy.
import {afterEach, describe, expect, it, vi} from 'vitest'
import {fireEvent, render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {MemoryRouter} from 'react-router-dom'
import {Toast} from '@heroui/react'
import {CreateSchedule, ListSchedules, ListTasks, ReconcileSchedules, UpdateSchedule} from '@wailsjs/go/app/App'
import type {ipc} from '@wailsjs/go/models'
import {SchedulesPage} from './SchedulesPage'

const mList = vi.mocked(ListSchedules)
const mReconcile = vi.mocked(ReconcileSchedules)
const mUpdate = vi.mocked(UpdateSchedule)
const mCreate = vi.mocked(CreateSchedule)
const mListTasks = vi.mocked(ListTasks)

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

  it('shows a humanized rule with the raw cron underneath', async () => {
    mList.mockResolvedValue(seed)
    renderPage()

    const rule = await screen.findByTestId('schedule-rule-s1')
    expect(rule).toHaveTextContent('At 09:00, every day')
    expect(screen.getByText('0 9 * * *', {selector: 'span'})).toBeTruthy()
  })

  it('opens the edit dialog prefilled and saves through UpdateSchedule', async () => {
    mList.mockResolvedValue(seed)
    mUpdate.mockResolvedValue({...seed[0], slug: 'daily-backup'})
    const user = userEvent.setup()

    renderPage()
    await user.click(await screen.findByTestId('schedule-edit-s1'))

    expect(await screen.findByText('Edit schedule')).toBeTruthy()
    const slugInput = screen.getByLabelText('Schedule slug') as HTMLInputElement
    expect(slugInput.value).toBe('daily-backup')

    await user.click(screen.getByTestId('save-schedule'))
    await waitFor(() => {
      expect(mUpdate).toHaveBeenCalledTimes(1)
    })
    const [id, slug, taskSlug, kind, cron, runAt, missedPolicy] = mUpdate.mock.calls[0]
    expect(id).toBe('s1')
    expect(slug).toBe('daily-backup')
    expect(taskSlug).toBe('backup')
    expect(kind).toBe('recurring')
    expect(cron).toBe('0 9 * * *')
    expect(runAt).toBe('')
    expect(missedPolicy).toBe('run_now')
    expect(await screen.findByRole('alert')).toHaveTextContent('Schedule updated')
  })

  it('keeps foreign crons intact through an edit round-trip', async () => {
    mList.mockResolvedValue([{...seed[0], cron: '*/15 9-17 * * 1-5'}])
    mUpdate.mockResolvedValue({...seed[0], cron: '*/15 9-17 * * 1-5'})
    const user = userEvent.setup()

    renderPage()
    await user.click(await screen.findByTestId('schedule-edit-s1'))
    await screen.findByText('Edit schedule')

    await user.click(screen.getByTestId('save-schedule'))
    await waitFor(() => {
      expect(mUpdate).toHaveBeenCalledTimes(1)
    })
    expect(mUpdate.mock.calls[0][4]).toBe('*/15 9-17 * * 1-5')
  })

  it('creates a schedule through the dialog', async () => {
    mList.mockResolvedValue(seed)
    mCreate.mockResolvedValue(seed[0])
    mListTasks.mockResolvedValue([
      {slug: 'backup', name: 'Backup', type: 'script', runtime: 'powershell', enabled: true, updated_at: ''},
    ])
    const user = userEvent.setup()

    renderPage()
    await user.click(await screen.findByRole('button', {name: '+ New schedule'}))

    await screen.findByRole('heading', {name: 'Create schedule'})
    await user.type(screen.getByLabelText('Schedule slug'), 'nightly-job')
    // Pick the task through the hidden native select (HeroUI popover does not
    // open in jsdom).
    const selects = document.querySelectorAll('select')
    fireEvent.change(selects[0], {target: {value: 'backup'}})

    await user.click(screen.getByTestId('save-schedule'))
    await waitFor(() => {
      expect(mCreate).toHaveBeenCalledTimes(1)
    })
    const [slug, taskSlug, kind, cron] = mCreate.mock.calls[0]
    expect(slug).toBe('nightly-job')
    expect(taskSlug).toBe('backup')
    expect(kind).toBe('recurring')
    expect(cron).toBe('@every 15m')
  })
})
