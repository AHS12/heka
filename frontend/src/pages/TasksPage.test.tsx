// TasksPage tests (SPEC-13 §6.3/§6.4): seeded table, server-side search with
// debounce, cursor pagination (sentinel-driven fetchNextPage + footer
// counts), optimistic enable toggle with rollback, Run Now group_id toast,
// and import error list.
import {describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {MemoryRouter} from 'react-router-dom'
import {
  GetTask,
  GetTaskYAML,
  ListTasksPage,
  RunTask,
  SetTaskEnabled,
  ImportTaskFromFile,
} from '@wailsjs/go/app/App'
import type {app} from '@wailsjs/go/models'
import {TasksPage} from './TasksPage'

const mListPage = vi.mocked(ListTasksPage)
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

function pageOf(tasks: app.TaskSummaryDTO[], opts: {total?: number; nextCursor?: string} = {}) {
  return {tasks, total: opts.total ?? tasks.length, next_cursor: opts.nextCursor ?? ''} as any
}

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
  it('renders the seeded tasks in the table with footer counts', async () => {
    mListPage.mockResolvedValue(pageOf(seed))
    renderPage()
    expect(await screen.findByText('Backup')).toBeInTheDocument()
    expect(screen.getByText('Pack')).toBeInTheDocument()
    expect(screen.getByText('Never')).toBeInTheDocument() // pack has no last run
    const chip = screen.getByText('success')
    expect(chip).toHaveAttribute('data-status', 'success')
    expect(screen.getByTestId('tasks-footer')).toHaveTextContent('2 shown · 2 total')
  })

  it('searches server-side with a debounce (one request for the final value)', async () => {
    mListPage.mockResolvedValue(pageOf(seed))
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Backup')

    await user.type(screen.getByPlaceholderText('Search tasks…'), 'backup')
    // 300ms debounce: exactly one fetch carries the final query.
    await waitFor(() =>
      expect(mListPage.mock.calls.some((c) => c[0] === 'backup')).toBe(true)
    )
    expect(mListPage.mock.calls.filter((c) => c[0] !== '')).toHaveLength(1)
  })

  it('loads the next page when the scroll sentinel intersects', async () => {
    mListPage
      .mockResolvedValueOnce(pageOf([seed[0]], {total: 2, nextCursor: 'cursor-1'}))
      .mockResolvedValue(pageOf([seed[1]], {total: 2}))
    renderPage()
    await screen.findByText('Backup')

    const io = (IntersectionObserver as any).instances.at(-1)
    io.trigger()
    await waitFor(() => {
      expect(mListPage).toHaveBeenLastCalledWith('', '', '', 'cursor-1', 50)
    })
    expect(await screen.findByText('Pack')).toBeInTheDocument()
    expect(screen.getByTestId('tasks-footer')).toHaveTextContent('2 shown · 2 total')
  })

  it('toggles enable optimistically and rolls back on error', async () => {
    mListPage.mockResolvedValue(pageOf(seed))
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
    mListPage.mockResolvedValue(
      pageOf([
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
    ] as app.TaskSummaryDTO[]))
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
    mListPage.mockResolvedValue(pageOf(seed))
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
    mListPage.mockResolvedValue(pageOf(seed))
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
      },
    } as never)
    mGetTaskYAML.mockResolvedValue(
      'version: 1\nname: Backup\nslug: backup\ntype: script\nruntime: custom\nscript: run.sh\ntimeout: 60\n'
    )
    const user = userEvent.setup()

    renderPage()
    await screen.findByText('Backup')

    await user.click(screen.getByTestId('task-row-backup'))
    expect(await screen.findByText('Edit task')).toBeInTheDocument()
    expect(screen.getByRole('button', {name: 'Save changes'})).toBeInTheDocument()
  })

  it('renders the 422 list from a failed import', async () => {
    mListPage.mockResolvedValue(pageOf(seed))
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
    mListPage.mockResolvedValue(pageOf(seed))
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
