// TasksPage tests (SPEC-13 §6.3/§6.4): seeded table, optimistic enable
// toggle with rollback, Run Now group_id toast, and import error list.
import {describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {MemoryRouter} from 'react-router-dom'
import {
  GetTask,
  GetTaskYAML,
  ListTasks,
  RunTask,
  SetTaskEnabled,
  ImportTaskFromFile,
} from '@wailsjs/go/app/App'
import type {app} from '@wailsjs/go/models'
import {TasksPage} from './TasksPage'

const mList = vi.mocked(ListTasks)
const mRun = vi.mocked(RunTask)
const mToggle = vi.mocked(SetTaskEnabled)
const mImport = vi.mocked(ImportTaskFromFile)
const mGetTask = vi.mocked(GetTask)
const mGetTaskYAML = vi.mocked(GetTaskYAML)

const seed: app.TaskSummaryDTO[] = [
  {
    slug: 'backup',
    name: 'Backup',
    type: 'script',
    runtime: 'custom',
    enabled: true,
    updated_at: '2026-08-25T10:00:00Z',
    last_status: 'success',
    last_run_at: '2026-08-25T09:00:00Z',
  },
  {
    slug: 'pack',
    name: 'Pack',
    type: 'binary',
    runtime: 'node',
    enabled: false,
    updated_at: '2026-08-25T08:00:00Z',
  },
]

function renderPage() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <MemoryRouter>
      <QueryClientProvider client={client}>
        <TasksPage />
      </QueryClientProvider>
    </MemoryRouter>
  )
}

describe('TasksPage', () => {
  it('renders the seeded tasks in the table', async () => {
    mList.mockResolvedValue(seed)
    renderPage()
    expect(await screen.findByText('Backup')).toBeInTheDocument()
    expect(screen.getByText('Pack')).toBeInTheDocument()
    expect(screen.getByText('Never')).toBeInTheDocument() // pack has no last run
    const chip = screen.getByText('success')
    expect(chip).toHaveAttribute('data-status', 'success')
  })

  it('toggles enable optimistically and rolls back on error', async () => {
    mList.mockResolvedValue(seed)
    let rejectToggle!: (e: Error) => void
    mToggle.mockImplementation(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectToggle = reject
        })
    )
    const user = userEvent.setup()

    renderPage()
    await screen.findByText('Backup')

    const switchBtn = screen.getAllByRole('switch')[0]
    expect(switchBtn).toHaveAttribute('aria-checked', 'true')
    await user.click(switchBtn)
    // Optimistic flip is applied once the mutation's onMutate runs.
    await waitFor(() =>
      expect(switchBtn).toHaveAttribute('aria-checked', 'false')
    )
    expect(mToggle).toHaveBeenCalledWith('backup', false)

    rejectToggle(new Error('internal: db locked'))
    await waitFor(() =>
      expect(switchBtn).toHaveAttribute('aria-checked', 'true')
    )
  })

  it('shows a run indicator while a task is active', async () => {
    mList.mockResolvedValue([
      {
        slug: 'slow-task',
        name: 'Slow',
        type: 'script',
        runtime: 'custom',
        enabled: true,
        updated_at: '2026-08-25T10:00:00Z',
        last_status: 'running',
        last_run_at: '2026-08-25T09:00:00Z',
      },
      {
        slug: 'done-task',
        name: 'Done',
        type: 'script',
        runtime: 'custom',
        enabled: true,
        updated_at: '2026-08-25T10:00:00Z',
        last_status: 'success',
        last_run_at: '2026-08-25T09:00:00Z',
      },
    ] as app.TaskSummaryDTO[])
    renderPage()
    await screen.findByText('Slow')
    const running = screen.getByText('running')
    expect(
      running.closest('[data-status="running"]')
    ).toBeInTheDocument()
    // Only the active row carries the spinning indicator.
    expect(screen.getAllByTestId('run-indicator')).toHaveLength(1)
  })

  it('toasts the group_id from Run Now', async () => {
    mList.mockResolvedValue(seed)
    mRun.mockResolvedValue({group_id: 'group-42', status: 'running'})
    const user = userEvent.setup()

    renderPage()
    await screen.findByText('Backup')
    await user.click(screen.getByRole('button', {name: 'Run backup'}))

    await waitFor(() =>
      expect(screen.getByTestId('toast')).toHaveTextContent('group-42')
    )
    expect(mRun).toHaveBeenCalledWith('backup', 'manual')
  })

  it('opens the edit dialog when a row is clicked', async () => {
    mList.mockResolvedValue(seed)
    mGetTask.mockResolvedValue({
      enabled: true,
      updated_at: '2026-08-25T10:00:00Z',
      task: {
        version: 1,
        name: 'Backup',
        slug: 'backup',
        type: 'script',
        runtime: 'custom',
        script: 'run.sh',
        timeout: 60,
        capture_output: true,
      },
    } as never)
    mGetTaskYAML.mockResolvedValue(
      'version: 1\nname: Backup\nslug: backup\ntype: script\nruntime: custom\nscript: run.sh\ntimeout: 60\ncapture_output: true\n'
    )
    const user = userEvent.setup()

    renderPage()
    await screen.findByText('Backup')

    await user.click(screen.getByTestId('task-row-backup'))
    expect(await screen.findByText('Edit task')).toBeInTheDocument()
    expect(screen.getByRole('button', {name: 'Save changes'})).toBeInTheDocument()
  })

  it('renders the 422 list from a failed import', async () => {
    mList.mockResolvedValue(seed)
    mImport.mockRejectedValue(new Error('invalid_task: ["script: required","timeout: must be positive"]'))
    const user = userEvent.setup()

    renderPage()
    await screen.findByText('Backup')
    await user.click(screen.getByRole('button', {name: 'Import Task'}))

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('script: required')
    )
    expect(screen.getByRole('alert')).toHaveTextContent(
      'timeout: must be positive'
    )
  })

  it('ignores a canceled import dialog', async () => {
    mList.mockResolvedValue(seed)
    mImport.mockRejectedValue(new Error('dialog canceled'))
    const user = userEvent.setup()

    renderPage()
    await screen.findByText('Backup')
    await user.click(screen.getByRole('button', {name: 'Import Task'}))

    // No error list rendered for a user cancel.
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})