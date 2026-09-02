// DashboardPage tests (SPEC-16 §1): metric tiles, human-readable status
// labels, run deep-links, quick-action navigation, engine health chip, and
// the next-run "Run now" affordance.
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {MemoryRouter, Route, Routes} from 'react-router-dom'
import {Toast} from '@heroui/react'
import {
  DaemonStatus,
  ListSchedules,
  ListTasks,
  RunTask,
  Stats,
} from '@wailsjs/go/app/App'
import {ipc} from '@wailsjs/go/models'
import {DashboardPage} from './DashboardPage'

const mStats = vi.mocked(Stats)
const mListSchedules = vi.mocked(ListSchedules)
const mListTasks = vi.mocked(ListTasks)
const mRun = vi.mocked(RunTask)
const mDaemonStatus = vi.mocked(DaemonStatus)

const seedStats = new ipc.Stats({
  tasks: 3,
  tasks_enabled: 2,
  schedules_enabled: 1,
  running: 0,
  runs_today: 4,
  success_today: 3,
  failed_today: 1,
  run_history: [
    {date: '2026-08-27', success: 2, failed: 1, total: 3},
    {date: '2026-08-28', success: 1, failed: 0, total: 1},
  ],
  status_distribution: [
    {status: 'success', count: 63},
    {status: 'failed', count: 14},
    {status: 'timed_out', count: 1},
  ],
  recent_activity: [
    {run_id: 'run-1', task_slug: 'backup', status: 'success', at: new Date().toISOString()},
    {
      run_id: 'run-2',
      task_slug: 'pack',
      status: 'timed_out',
      at: new Date(Date.now() - 5 * 60000).toISOString(),
    },
  ],
})

const seedSchedule: ipc.Schedule = {
  id: 's1',
  slug: 'daily-backup',
  task_slug: 'backup',
  kind: 'recurring',
  cron: '0 9 * * *',
  enabled: true,
  missed_policy: 'run_now',
  next_run_at: new Date(Date.now() + 60 * 60000).toISOString(),
  skipped_count: 0,
  missed_count: 0,
}

function renderPage(initial = '/') {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <Toast.Provider />
      <MemoryRouter initialEntries={[initial]}>
        <Routes>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/tasks" element={<div>tasks-page</div>} />
          <Route path="/tasks/new" element={<div>tasks-new-page</div>} />
          <Route path="/schedules" element={<div>schedules-page</div>} />
          <Route path="/logs" element={<div>logs-page</div>} />
          <Route path="/logs/:runId" element={<div>run-detail-page</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  mStats.mockResolvedValue(seedStats)
  mListSchedules.mockResolvedValue([])
  mListTasks.mockResolvedValue([])
})

afterEach(() => {
  Toast.toast.clear()
})

describe('DashboardPage', () => {
  it('renders metric tiles and human-readable status labels', async () => {
    renderPage()

    expect(await screen.findByText('Overview')).toBeInTheDocument()
    expect(screen.getByText('Tasks')).toBeInTheDocument()
    expect(screen.getByText('Schedules')).toBeInTheDocument()
    expect(screen.getByText('Running')).toBeInTheDocument()
    expect(screen.getByText('All workers idle')).toBeInTheDocument()
    expect(screen.getAllByText('Timed out').length).toBeGreaterThan(0)
    expect(screen.queryByText('timed_out')).not.toBeInTheDocument()
  })

  it('shows the enabled/idle context under the metrics', async () => {
    renderPage()

    expect(await screen.findByText('2 enabled')).toBeInTheDocument()
    expect(screen.getByText('3 ok · 1 failed')).toBeInTheDocument()
  })

  it('deep-links recent activity rows into run detail', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Overview')
    await user.click(screen.getByTestId('activity-row-run-1'))

    expect(await screen.findByText('run-detail-page')).toBeInTheDocument()
  })

  it('opens the create dialog from the New Task quick action', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Overview')
    await user.click(screen.getByRole('button', {name: '+ New Task'}))
    expect(
      await screen.findByRole('heading', {name: 'Create task'})
    ).toBeInTheDocument()
  })

  it('navigates to schedules from the New Schedule quick action', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Overview')
    await user.click(screen.getByRole('button', {name: '+ New Schedule'}))
    expect(await screen.findByText('schedules-page')).toBeInTheDocument()
  })

  it('shows a healthy engine chip when the daemon is up', async () => {
    mDaemonStatus.mockResolvedValue('running')
    renderPage()

    await screen.findByText('Overview')
    await waitFor(() =>
      expect(screen.getByTestId('engine-health')).toHaveTextContent(
        'Engine healthy'
      )
    )
  })

  it('shows the next scheduled run and triggers Run now', async () => {
    mListSchedules.mockResolvedValue([seedSchedule])
    mRun.mockResolvedValue({group_id: 'group-7', status: 'running'})
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('Next scheduled run')).toBeInTheDocument()
    await user.click(screen.getByRole('button', {name: 'Run backup now'}))

    await waitFor(() => expect(mRun).toHaveBeenCalledWith('backup', 'manual'))
    await waitFor(() => {
      const alerts = screen.getAllByRole('alert')
      expect(alerts.some((a) => a.textContent?.includes('Started backup'))).toBe(true)
    })
  })

  it('opens the Run dialog with a task picker and cancels cleanly', async () => {
    mListTasks.mockResolvedValue([
      {
        slug: 'backup',
        name: 'Backup',
        type: 'script',
        runtime: 'custom',
        enabled: true,
        updated_at: '2026-08-25T10:00:00Z',
      },
    ])
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Overview')
    await user.click(screen.getByRole('button', {name: '▶ Run…'}))

    expect(await screen.findByText('Run a task')).toBeInTheDocument()
    await user.click(screen.getByRole('button', {name: 'Cancel'}))
    await waitFor(() =>
      expect(screen.queryByText('Run a task')).not.toBeInTheDocument()
    )
  })
})
