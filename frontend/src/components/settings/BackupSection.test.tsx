// BackupSection tests: the Settings → Backup panel loads its config through
// the bindings, runs backups on demand, saves schedule changes, tracks the
// passphrase through the vault (setup AND re-setup after deletion), and
// reveals the S3 destination block when toggled.
import {describe, expect, it, vi, beforeEach} from 'vitest'
import {fireEvent, render, screen, waitFor, within} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {
  GetBackupConfig,
  UpdateBackupConfig,
  RunBackup,
  BackupStatus,
  BackupHistory,
  TestBackupDestinations,
  SetSecret,
  ListSecrets,
} from '@wailsjs/go/app/App'
import {BackupSection} from './BackupSection'

const mGetConfig = vi.mocked(GetBackupConfig)
const mUpdateConfig = vi.mocked(UpdateBackupConfig)
const mRunBackup = vi.mocked(RunBackup)
const mStatus = vi.mocked(BackupStatus)
const mHistory = vi.mocked(BackupHistory)
const mTestDestinations = vi.mocked(TestBackupDestinations)
const mSetSecret = vi.mocked(SetSecret)
const mListSecrets = vi.mocked(ListSecrets)

const configFixture = (over: Record<string, unknown> = {}) => ({
  schedule: {kind: 'off', every_hours: 0, at_time: ''},
  local_dir: '',
  keep_last_local: 5,
  s3: {endpoint: '', region: '', bucket: '', prefix: '', use_ssl: false, keep_last: 0},
  includes: {run_history: true, artifacts: false},
  passphrase_set: false,
  ...over,
})

const lastJob = {
  id: 'job-1',
  trigger: 'scheduled',
  status: 'success',
  started_at: '2026-09-04T09:00:00Z',
  finished_at: '2026-09-04T09:00:04Z',
  size_bytes: 4096,
  local_path: '/data/backups/heka-backup.zip',
  destinations: [],
  error: '',
}

function renderSection() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <BackupSection />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  mGetConfig.mockResolvedValue(configFixture() as any)
  mStatus.mockResolvedValue({running: false} as any)
  mHistory.mockResolvedValue([])
  mUpdateConfig.mockResolvedValue(undefined)
  mRunBackup.mockResolvedValue('job-2')
  mTestDestinations.mockResolvedValue({local: {type: 'local', ok: true}} as any)
  mSetSecret.mockResolvedValue(undefined)
  mListSecrets.mockResolvedValue([])
})

describe('BackupSection', () => {
  it('renders config, last job status, and the configured passphrase state', async () => {
    mGetConfig.mockResolvedValue(configFixture({passphrase_set: true}) as any)
    mListSecrets.mockResolvedValue(['BACKUP_PASSPHRASE', 'OTHER_KEY'])
    mStatus.mockResolvedValue({running: false, last: lastJob} as any)
    renderSection()

    // Last successful job is summarized (via BackupStatus.last).
    expect(await screen.findByText('Success')).toBeInTheDocument()
    // Passphrase configured → status pill + change entry point.
    expect(await screen.findByText('Set up')).toBeInTheDocument()
    expect(screen.getByRole('button', {name: 'Change passphrase'})).toBeInTheDocument()
    expect(screen.queryByRole('button', {name: 'Set up passphrase'})).not.toBeInTheDocument()
    const save = screen.getByRole('button', {name: 'Save changes'})
    expect(save).toBeDisabled()
  })

  it('re-offers setup when the passphrase is deleted from the vault', async () => {
    // Config still remembers passphrase_set=true, but the vault no longer
    // has the key — the UI must follow the vault, not the cached config.
    mGetConfig.mockResolvedValue(configFixture({passphrase_set: true}) as any)
    mListSecrets.mockResolvedValue(['OTHER_KEY'])
    renderSection()

    expect(await screen.findByRole('button', {name: 'Set up passphrase'})).toBeInTheDocument()
    expect(screen.queryByText('Set up')).not.toBeInTheDocument()
  })

  it('offers the passphrase setup flow when not configured', async () => {
    const user = userEvent.setup()
    renderSection()
    await user.click(await screen.findByRole('button', {name: 'Set up passphrase'}))

    // Scope to the dialog: HeroUI's modal can briefly render a duplicate
    // portal copy outside it during React 18's concurrent mount.
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Archive passphrase'), 'correct horse')
    await user.type(within(dialog).getByLabelText('Confirm passphrase'), 'correct horse')
    await user.click(within(dialog).getByRole('button', {name: 'Store in vault'}))

    await waitFor(() => expect(mSetSecret).toHaveBeenCalledWith('BACKUP_PASSPHRASE', 'correct horse'))
  })

  it('warns about existing archives when changing the passphrase', async () => {
    const user = userEvent.setup()
    mListSecrets.mockResolvedValue(['BACKUP_PASSPHRASE'])
    renderSection()
    await user.click(await screen.findByRole('button', {name: 'Change passphrase'}))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', {name: 'Change archive passphrase'})).toBeInTheDocument()
    expect(
      within(dialog).getByText(/does not re-encrypt backups you already have/i)
    ).toBeInTheDocument()

    await user.type(within(dialog).getByLabelText('Archive passphrase'), 'new-secret')
    await user.type(within(dialog).getByLabelText('Confirm passphrase'), 'new-secret')
    await user.click(within(dialog).getByRole('button', {name: 'Update passphrase'}))

    await waitFor(() => expect(mSetSecret).toHaveBeenCalledWith('BACKUP_PASSPHRASE', 'new-secret'))
  })

  it('rejects a mismatched passphrase confirmation without touching the vault', async () => {
    const user = userEvent.setup()
    renderSection()
    await user.click(await screen.findByRole('button', {name: 'Set up passphrase'}))

    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Archive passphrase'), 'alpha')
    await user.type(within(dialog).getByLabelText('Confirm passphrase'), 'beta')
    await user.click(within(dialog).getByRole('button', {name: 'Store in vault'}))

    expect(await screen.findByText('The confirmation does not match.')).toBeInTheDocument()
    expect(mSetSecret).not.toHaveBeenCalled()
  })

  it('triggers a backup on demand', async () => {
    const user = userEvent.setup()
    renderSection()
    await user.click(await screen.findByRole('button', {name: 'Back up now'}))
    await waitFor(() => expect(mRunBackup).toHaveBeenCalledTimes(1))
  })

  it('saves an enabled daily schedule with the full config object', async () => {
    const user = userEvent.setup()
    renderSection()
    await screen.findByText('No backup yet')

    await user.click(screen.getByRole('switch', {name: 'Back up automatically'}))
    const save = await screen.findByRole('button', {name: 'Save changes'})
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)

    await waitFor(() => expect(mUpdateConfig).toHaveBeenCalledTimes(1))
    const sent = mUpdateConfig.mock.calls[0][0]
    expect(sent.schedule.kind).toBe('daily')
    expect(sent.includes.run_history).toBe(true)
    expect(sent.keep_last_local).toBe(5)
  })

  it('reveals and keeps the S3 destination block when toggled on', async () => {
    const user = userEvent.setup()
    renderSection()
    await screen.findByText('No backup yet')

    const toggle = screen.getByRole('switch', {name: 'Mirror to an S3-compatible bucket'})
    await user.click(toggle)

    expect(await screen.findByLabelText('S3 endpoint')).toBeInTheDocument()
    expect(screen.getByRole('button', {name: 'Test connection'})).toBeInTheDocument()
    // The toggle stays on — previously it snapped back because the checked
    // state was derived from an empty bucket/endpoint.
    expect(screen.getByRole('switch', {name: 'Mirror to an S3-compatible bucket'})).toHaveAttribute(
      'aria-checked',
      'true'
    )
  })

  it('saves a newly enabled S3 destination with HTTPS on by default', async () => {
    const user = userEvent.setup()
    renderSection()
    await screen.findByText('No backup yet')

    // Go's zero-value config carries use_ssl=false; enabling the destination
    // must flip it to TLS or every request gets 301-redirected to HTTPS.
    await user.click(screen.getByRole('switch', {name: 'Mirror to an S3-compatible bucket'}))
    await user.type(await screen.findByLabelText('S3 endpoint'), 'acct.r2.cloudflarestorage.com')
    await user.type(screen.getByLabelText('S3 bucket'), 'heka')

    const save = screen.getByRole('button', {name: 'Save changes'})
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)

    await waitFor(() => expect(mUpdateConfig).toHaveBeenCalledTimes(1))
    const sent = mUpdateConfig.mock.calls[0][0]
    expect(sent.s3.use_ssl).toBe(true)
    expect(sent.s3.endpoint).toBe('acct.r2.cloudflarestorage.com')
    expect(sent.s3.bucket).toBe('heka')
  })

  it('saves pending changes before testing the connection', async () => {
    const user = userEvent.setup()
    renderSection()
    await screen.findByText('No backup yet')

    // The daemon tests the SAVED config — unsaved edits (endpoint, keys,
    // HTTPS toggle) must be persisted first or the test reports stale state.
    await user.click(screen.getByRole('switch', {name: 'Mirror to an S3-compatible bucket'}))
    await user.type(await screen.findByLabelText('S3 endpoint'), 'acct.r2.cloudflarestorage.com')
    await user.type(screen.getByLabelText('S3 bucket'), 'heka')

    await user.click(screen.getByRole('button', {name: 'Test connection'}))

    await waitFor(() => expect(mTestDestinations).toHaveBeenCalledTimes(1))
    expect(mUpdateConfig).toHaveBeenCalledTimes(1)
    expect(mUpdateConfig.mock.invocationCallOrder[0]).toBeLessThan(
      mTestDestinations.mock.invocationCallOrder[0]
    )
  })

  it('shows the stored cadence preset in the dropdown', async () => {
    mGetConfig.mockResolvedValue(
      configFixture({schedule: {kind: 'interval', every_hours: 360}}) as any
    )
    renderSection()

    // HeroUI Select mirrors its state into a hidden native <select>
    // (AGENTS.md pitfall #1) — find it directly; getByLabelText resolves to
    // a different hidden node whose value never syncs.
    await waitFor(() => {
      const sel = document.querySelector('select') as HTMLSelectElement | null
      expect(sel).toBeTruthy()
      expect(sel!.value).toBe('360') // "Every 15 days"
    })
    // The trigger shows the human label.
    expect(await screen.findByText('Every 15 days')).toBeInTheDocument()
  })

  it('switches cadence kinds through the hidden native select', async () => {
    mGetConfig.mockResolvedValue(
      configFixture({schedule: {kind: 'interval', every_hours: 360}}) as any
    )
    renderSection()

    const sel = (await screen.findByText('Every 15 days').then(() => document.querySelector('select'))) as HTMLSelectElement
    fireEvent.change(sel, {target: {value: 'monthly'}})

    expect(await screen.findByLabelText('Monthly backup day')).toBeInTheDocument()
    // react-aria's TimeField labels the outer group and the inner input with
    // the same aria-label — assert existence via *AllBy.
    expect(screen.getAllByLabelText('Backup time of day').length).toBeGreaterThan(0)
  })
})
