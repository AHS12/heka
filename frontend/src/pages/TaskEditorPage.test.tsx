// TaskEditorPage tests (SPEC-13 §6.2, §4 preservation rule): invalid YAML
// shows errors, keeps the text byte-for-byte, and sends no save signal;
// valid YAML saves and navigates to the task.
import {beforeEach, describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor, fireEvent} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {MemoryRouter, Route, Routes} from 'react-router-dom'
import {
  ValidateTaskYAML,
  ParseTaskYAML,
  CreateTask,
  GetTask,
  GetTaskYAML,
  ListSecrets,
  UpdateTask,
} from '@wailsjs/go/app/App'
import type {app, task} from '@wailsjs/go/models'
import {TaskEditorPage} from './TaskEditorPage'

const mValidate = vi.mocked(ValidateTaskYAML)
const mParse = vi.mocked(ParseTaskYAML)
const mCreate = vi.mocked(CreateTask)
const mListSecrets = vi.mocked(ListSecrets)
const mGetTask = vi.mocked(GetTask)
const mGetTaskYAML = vi.mocked(GetTaskYAML)
const mUpdate = vi.mocked(UpdateTask)

const validTask = {
  version: 1,
  name: 'Nightly',
  slug: 'nightly',
  type: 'script',
  runtime: 'custom',
  script: 'run.sh',
  timeout: 60,
  capture_output: true,
} as unknown as task.Task

const validDTO = {enabled: true, updated_at: '', task: validTask} as unknown as app.TaskDTO

const validYAML = `version: 1
name: Nightly
slug: nightly
type: script
runtime: custom
script: run.sh
timeout: 60
capture_output: true
`

function renderEditor(initialEntry = '/tasks/new') {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <QueryClientProvider client={client}>
        <Routes>
          <Route path="/tasks/new" element={<TaskEditorPage />} />
          <Route path="/tasks/:slug" element={<TaskEditorPage />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>
  )
}

// CodeMirror's editor is a contenteditable [role=textbox]; typing through
// user-event fires the beforeinput/input events CM6 consumes.
async function typeYAML(user: ReturnType<typeof userEvent.setup>, text: string) {
  const editor = await screen.findByRole('textbox', {name: 'Task YAML'})
  await user.click(editor)
  await user.keyboard('{Control>}a{/Control}{Backspace}') // select all + clear
  await user.type(editor, text)
  return editor as HTMLElement
}

// CM6 renders one .cm-line per doc line and hides a measurement line inside
// the content; reading the editor back requires joining lines and stripping
// that constant measurement text.
function cmText(box: HTMLElement): string {
  return Array.from(box.querySelectorAll('.cm-line'))
    .map((line) => line.textContent ?? '')
    .join('\n')
    .replace(/abc def ghi jkl mno pqr stu/g, '')
}

describe('TaskEditorPage', () => {
  it('shows an empty form for a new task', async () => {
    renderEditor()
    expect(await screen.findByRole('heading', {name: 'New Task'})).toBeInTheDocument()
    expect(screen.getByRole('tab', {name: 'Visual'})).toBeInTheDocument()
    expect(screen.getByRole('tab', {name: 'YAML'})).toBeInTheDocument()
    // Runtime select is HeroUI-backed; text appears in trigger + hidden listbox.
    expect(screen.getAllByText('Choose runtime…').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByRole('button', {name: 'Browse…'})).toHaveLength(3)
    expect(screen.getByRole('link', {name: 'Back to tasks'})).toBeInTheDocument()
  })

  it('saves valid YAML through the daemon', async () => {
    mParse.mockResolvedValue(validDTO)
    mCreate.mockResolvedValue(validDTO)
    const user = userEvent.setup()

    renderEditor()
    await screen.findByRole('tab', {name: 'YAML'})
    await user.click(screen.getByRole('tab', {name: 'YAML'}))

    const editor = await typeYAML(user, validYAML)
    expect(cmText(editor)).toBe(validYAML)

    await user.click(screen.getByRole('button', {name: 'Save'}))

    await waitFor(() => expect(mCreate).toHaveBeenCalledTimes(1))
    expect(mCreate).toHaveBeenCalledWith(validYAML)
  })

  it('blocks save on invalid YAML, preserving the text byte-for-byte', async () => {
    mParse.mockRejectedValue(
      new Error('invalid_task: ["timeout: must be positive","script: required"]')
    )
    mValidate.mockResolvedValue([
      'timeout: must be positive',
      'script: required',
    ])
    const user = userEvent.setup()

    renderEditor()
    await screen.findByRole('tab', {name: 'YAML'})
    await user.click(screen.getByRole('tab', {name: 'YAML'}))

    const broken = 'version: 1\nname: X\nslug: x\ntype: script\nruntime: custom\n'
    const editor = await typeYAML(user, broken)
    await user.click(screen.getByRole('button', {name: 'Save'}))

    await waitFor(() =>
      expect(screen.getByTestId('yaml-errors')).toBeInTheDocument()
    )
    expect(screen.getByTestId('yaml-errors')).toHaveTextContent(
      'script: required'
    )
    // Preservation rule: text untouched, no save signal sent (the parse call
    // is the validation probe; it must not proceed to create).
    expect(cmText(editor)).toBe(broken)
    expect(mParse).toHaveBeenCalledWith(broken)
    expect(mCreate).not.toHaveBeenCalled()
  })

  it('auto-generates the slug from the name until overridden', async () => {
    const user = userEvent.setup()
    renderEditor()

    const name = await screen.findByLabelText('Task name')
    await user.type(name, 'Nightly Backup')
    const slug = screen.getByLabelText('Task slug')
    expect((slug as HTMLInputElement).value).toBe('nightly-backup')

    // A hand-typed slug sticks even as the name keeps changing.
    await user.clear(slug)
    await user.type(slug, 'my-own-slug')
    await user.clear(name)
    await user.type(name, 'Other Name')
    expect((name as HTMLInputElement).value).toBe('Other Name')
    expect((slug as HTMLInputElement).value).toBe('my-own-slug')
  })

  it('keeps env variables in sync between Visual and YAML tabs', async () => {
    mListSecrets.mockResolvedValue(['MY_SECRET'])
    const user = userEvent.setup()
    renderEditor()

    await user.click(await screen.findByRole('button', {name: '+ Variable'}))
    await user.type(screen.getByLabelText('Environment key 1'), 'API_KEY')

    // HeroUI Select renders a hidden <select> for form integration.
    // Find it via the first <select> inside the env section.
    const selects = document.querySelectorAll('select')
    const hiddenSelect = selects[selects.length - 1]
    expect(hiddenSelect).toBeTruthy()
    fireEvent.change(hiddenSelect, {target: {value: 'MY_SECRET'}})

    // The YAML view must reflect the form edit.
    await user.click(screen.getByRole('tab', {name: 'YAML'}))
    const editor = await screen.findByRole('textbox', {name: 'Task YAML'})
    expect(cmText(editor)).toContain('API_KEY: ${MY_SECRET}')

    // And switching back keeps the key field.
    await user.click(screen.getByRole('tab', {name: 'Visual'}))
    expect(screen.getByLabelText('Environment key 1')).toHaveValue('API_KEY')
  })

  it('picks secret values from the vault dropdown as ${KEY} refs', async () => {
    mListSecrets.mockResolvedValue(['OPENROUTER_API_KEY'])
    const user = userEvent.setup()
    renderEditor()

    await user.click(await screen.findByRole('button', {name: '+ Variable'}))
    await user.type(screen.getByLabelText('Environment key 1'), 'API_KEY')

    // HeroUI Select popovers don't open in jsdom; test the underlying data
    // flow by setting the vault reference via the hidden <select>.
    const selects = document.querySelectorAll('select')
    const hiddenSelect = selects[selects.length - 1]
    fireEvent.change(hiddenSelect, {target: {value: 'OPENROUTER_API_KEY'}})

    await user.click(screen.getByRole('tab', {name: 'YAML'}))
    const editor = await screen.findByRole('textbox', {name: 'Task YAML'})
    expect(cmText(editor)).toContain('API_KEY: ${OPENROUTER_API_KEY}')
  })

  it('switches back to Visual after invalid YAML without being blocked', async () => {
    mParse.mockRejectedValue(
      new Error('invalid_task: ["script: required","runtime: required"]')
    )
    const user = userEvent.setup()

    renderEditor()
    await screen.findByRole('tab', {name: 'YAML'})
    await user.click(screen.getByRole('tab', {name: 'YAML'}))

    const broken = 'version: 1\nname: X\nslug: x\ntype: script\n'
    await typeYAML(user, broken)

    // Switching back is never hard-blocked by validation — it warns instead.
    await user.click(screen.getByRole('tab', {name: 'Visual'}))
    expect(await screen.findByRole('tab', {name: 'Visual'})).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByTestId('tab-switch-notice')).toHaveTextContent(
      'script: required'
    )

    // The YAML text survives the round trip untouched.
    await user.click(screen.getByRole('tab', {name: 'YAML'}))
    const editor = await screen.findByRole('textbox', {name: 'Task YAML'})
    expect(cmText(editor)).toBe(broken)
  })

  it('renders a not-found state for a missing slug', async () => {
    renderEditor('/tasks/ghost')
    await waitFor(() =>
      expect(screen.getByTestId('task-missing')).toHaveTextContent('ghost')
    )
    expect(screen.getByText('Task not found')).toBeInTheDocument()
  })
})

describe('TaskEditorPage dialog mode', () => {
  function renderDialogEditor() {
    const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
    const onClose = vi.fn()
    const onSaved = vi.fn()
    render(
      <MemoryRouter>
        <QueryClientProvider client={client}>
          <TaskEditorPage dialog slug="nightly" onClose={onClose} onSaved={onSaved} />
        </QueryClientProvider>
      </MemoryRouter>
    )
    return {onClose, onSaved}
  }

  beforeEach(() => {
    mGetTask.mockResolvedValue(validDTO)
    mGetTaskYAML.mockResolvedValue(validYAML)
  })

  it('renders edit chrome with Export and Save changes', async () => {
    renderDialogEditor()

    // The loading dialog shares the heading; wait for the hydrated editor.
    expect(await screen.findByRole('button', {name: 'Save changes'})).toBeInTheDocument()
    expect(screen.getByText('Edit task')).toBeInTheDocument()
    expect(screen.getByText(/changes apply to future runs/)).toBeInTheDocument()
    expect(screen.getByRole('button', {name: 'Export'})).toBeInTheDocument()
  })

  it('saves updates in place and fires onSaved', async () => {
    mParse.mockResolvedValue(validDTO)
    mUpdate.mockResolvedValue(validDTO)
    const user = userEvent.setup()
    const {onSaved, onClose} = renderDialogEditor()

    await user.click(await screen.findByRole('button', {name: 'Save changes'}))

    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('nightly'))
    expect(mUpdate).toHaveBeenCalledTimes(1)
    expect(onClose).toHaveBeenCalled()
  })

  it('shows a not-found dialog for a missing slug', async () => {
    mGetTask.mockRejectedValue(new Error('not_found: task not found'))
    mGetTaskYAML.mockRejectedValue(new Error('not_found: task not found'))
    renderDialogEditor()

    await waitFor(() =>
      expect(screen.getByTestId('task-missing')).toHaveTextContent('nightly')
    )
    expect(screen.getByText('Edit task')).toBeInTheDocument()
  })
})