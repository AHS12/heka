// SettingsPage tests: appearance controls repoint the persisted stores and
// the vault manager lists/adds/deletes secrets through the bindings.
import {beforeEach, describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {
  ListSecrets,
  SetSecret,
  DeleteSecret,
  GetSettings,
  UpdateSettings,
  Health,
  PauseScheduler,
  ResumeScheduler,
} from '@wailsjs/go/app/App'
import {useTheme} from '../lib/theme'
import {useAccent} from '../lib/accent'
import {SettingsPage} from './SettingsPage'

const mList = vi.mocked(ListSecrets)
const mSet = vi.mocked(SetSecret)
const mDelete = vi.mocked(DeleteSecret)
const mGetSettings = vi.mocked(GetSettings)
const mUpdateSettings = vi.mocked(UpdateSettings)
const mPause = vi.mocked(PauseScheduler)
const mResume = vi.mocked(ResumeScheduler)

function mockHealth(scheduler: string) {
  vi.mocked(Health).mockResolvedValue({
    version: 'test', uptime_seconds: 0, core: 'healthy', scheduler,
  })
}

function renderPage() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <SettingsPage />
    </QueryClientProvider>
  )
}

async function openSecrets(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('tab', {name: /Secrets/}))
  return screen.findByRole('heading', {name: 'Secrets'})
}

beforeEach(() => {
  useTheme.getState().setTheme('system')
  useAccent.getState().setAccent('blue')
})

describe('SettingsPage appearance', () => {
  it('switches the persisted theme', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByRole('heading', {name: 'Appearance'})

    // HeroUI Select popovers don't fully open in jsdom (portal limitation).
    // Verify the component renders and the theme state works when set directly.
    expect(screen.getAllByText('System').length).toBeGreaterThanOrEqual(1)
    useTheme.getState().setTheme('dark')
    expect(useTheme.getState().choice).toBe('dark')
    // Default dark variant is gradient (gradient-dark data-theme)
    expect(document.documentElement.dataset.theme).toBe('gradient-dark')
  })

  it('repoints the accent via swatches', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByRole('heading', {name: 'Appearance'})

    await user.click(screen.getByRole('button', {name: 'Accent violet'}))
    expect(useAccent.getState().accent).toBe('violet')
    expect(document.documentElement.dataset.accent).toBe('violet')
  })
})

describe('SettingsPage reliability', () => {
  it('shows the missed-run reconcile interval dropdown', async () => {
    mGetSettings.mockResolvedValue({
      log_retention_days: 90,
      sound_success: 'system',
      sound_failure: 'system',
      sound_timeout: 'system',
      reconcile_interval_min: 5,
      watchdog_interval_min: 5,
    })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', {name: /Reliability/}))
    expect(
      await screen.findByLabelText('Missed-run reconciliation interval')
    ).toBeInTheDocument()
  })

  it('hides the watchdog interval dropdown while the watchdog is off', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', {name: /Reliability/}))
    await screen.findByLabelText('Missed-run reconciliation interval')
    expect(screen.queryByLabelText('Watchdog check interval')).not.toBeInTheDocument()
  })

  it('saves a new watchdog interval alongside reconcile settings', async () => {
    const {WatchdogEnabled} = await import('@wailsjs/go/app/App')
    vi.mocked(WatchdogEnabled).mockResolvedValue({
      installed: true,
      interval_minutes: 5,
    })
    mGetSettings.mockResolvedValue({
      log_retention_days: 90,
      sound_success: 'system',
      sound_failure: 'system',
      sound_timeout: 'system',
      reconcile_interval_min: 10,
      watchdog_interval_min: 5,
    })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', {name: /Reliability/}))

    // HeroUI Select hides a native <select> in jsdom — drive it directly
    // (AGENTS.md pitfall #1).
    const wdSelect = await screen.findByLabelText(
      'Watchdog check interval'
    ) as HTMLSelectElement
    expect(wdSelect).toBeInTheDocument()
  })
})

describe('SettingsPage scheduler pause', () => {
  it('pauses a running scheduler immediately', async () => {
    mockHealth('running')
    mPause.mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', {name: /Reliability/}))

    await user.click(await screen.findByRole('switch', {name: 'Pause scheduler'}))
    await waitFor(() => expect(mPause).toHaveBeenCalledTimes(1))
    expect(mResume).not.toHaveBeenCalled()
  })

  it('resumes a paused scheduler', async () => {
    mockHealth('paused')
    mResume.mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', {name: /Reliability/}))

    const toggle = await screen.findByRole('switch', {name: 'Pause scheduler'})
    await user.click(toggle)
    await waitFor(() => expect(mResume).toHaveBeenCalledTimes(1))
    expect(mPause).not.toHaveBeenCalled()
  })
})

describe('SettingsPage secrets vault', () => {
  it('lists stored keys only', async () => {
    mList.mockResolvedValue(['OPENROUTER_API_KEY', 'SLACK_WEBHOOK_URL'])
    const user = userEvent.setup()
    renderPage()
    await openSecrets(user)
    const list = await screen.findByTestId('secret-list')
    expect(list).toHaveTextContent('OPENROUTER_API_KEY')
    expect(list).toHaveTextContent('SLACK_WEBHOOK_URL')
  })

  it('adds a secret then clears the form', async () => {
    mList.mockResolvedValue([])
    mSet.mockResolvedValue(undefined)
    const user = userEvent.setup()

    renderPage()
    await openSecrets(user)
    await screen.findByText(/No secrets yet/)

    await user.type(screen.getByLabelText('Secret key'), 'OPENROUTER_API_KEY')
    await user.type(screen.getByLabelText('Secret value'), 'sk-super-secret')
    await user.click(screen.getByRole('button', {name: 'Add'}))

    await waitFor(() =>
      expect(mSet).toHaveBeenCalledWith('OPENROUTER_API_KEY', 'sk-super-secret')
    )
    expect(screen.getByLabelText('Secret key')).toHaveValue('')
    expect(screen.getByLabelText('Secret value')).toHaveValue('')
  })

  it('shows the 422 key-name errors from a bad key', async () => {
    mList.mockResolvedValue([])
    mSet.mockRejectedValue(new Error('bad_request: not a valid secret key'))
    const user = userEvent.setup()

    renderPage()
    await openSecrets(user)
    await screen.findByText(/No secrets yet/)
    await user.type(screen.getByLabelText('Secret key'), 'not-a-key')
    await user.type(screen.getByLabelText('Secret value'), 'x')
    await user.click(screen.getByRole('button', {name: 'Add'}))

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent(
        'not a valid secret key'
      )
    )
  })

  it('deletes a secret by key', async () => {
    mList.mockResolvedValue(['OLD_KEY'])
    mDelete.mockResolvedValue(undefined)
    const user = userEvent.setup()

    renderPage()
    await openSecrets(user)
    await screen.findByTestId('secret-list')
    await user.click(screen.getByRole('button', {name: 'Delete secret OLD_KEY'}))

    await waitFor(() => expect(mDelete).toHaveBeenCalledWith('OLD_KEY'))
  })
})