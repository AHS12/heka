// SecretsPage tests: the vault manager lists keys with usage tracking,
// filters by search, and deletes in bulk through the bindings. Values are
// never rendered — only keys and usage counts (SPEC-11 invariant).
import {describe, expect, it, vi, beforeEach} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {ListSecrets, SecretsUsage, DeleteSecret, SetSecret} from '@wailsjs/go/app/App'
import {SecretsPage} from './SecretsPage'

const mList = vi.mocked(ListSecrets)
const mUsage = vi.mocked(SecretsUsage)
const mDelete = vi.mocked(DeleteSecret)
const mSet = vi.mocked(SetSecret)

function renderPage() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <SecretsPage />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  mList.mockResolvedValue(['API_TOKEN', 'OLD_KEY', 'TELEGRAM_CHAT_ID'])
  mUsage.mockResolvedValue({API_TOKEN: ['deploy', 'report'], TELEGRAM_CHAT_ID: []} as any)
  mDelete.mockResolvedValue(undefined)
  mSet.mockResolvedValue(undefined)
})

describe('SecretsPage', () => {
  it('lists keys with usage and unused badges', async () => {
    renderPage()
    const list = await screen.findByTestId('secret-list')
    expect(list).toHaveTextContent('API_TOKEN')
    expect(list).toHaveTextContent('used by 2 tasks')
    expect(list).toHaveTextContent('OLD_KEY')
    expect(list).toHaveTextContent('unused')
    // Values never render (there are none to render — keys only).
    expect(list).not.toHaveTextContent('secret-value')
  })

  it('filters keys by search', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('secret-list')
    await user.type(screen.getByLabelText('Search keys'), 'old')
    expect(screen.getByTestId('secret-list')).toHaveTextContent('OLD_KEY')
    expect(screen.getByTestId('secret-list')).not.toHaveTextContent('API_TOKEN')
  })

  it('deletes selected secrets in bulk after confirmation', async () => {
    const user = userEvent.setup()
    renderPage()
    const list = await screen.findByTestId('secret-list')

    await user.click(screen.getByLabelText('Select secret OLD_KEY'))
    await user.click(screen.getByLabelText('Select secret API_TOKEN'))
    await user.click(screen.getByRole('button', {name: 'Delete selected (2)'}))

    const dialog = await screen.findByRole('heading', {name: 'Delete 2 secrets?'})
    expect(dialog).toBeInTheDocument()
    await user.click(screen.getByRole('button', {name: 'Delete'}))

    await waitFor(() => {
      expect(mDelete).toHaveBeenCalledWith('OLD_KEY')
      expect(mDelete).toHaveBeenCalledWith('API_TOKEN')
    })
    expect(mDelete).toHaveBeenCalledTimes(2)
    expect(list).toBeInTheDocument()
  })

  it('sets a new value for an existing key', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('secret-list')

    await user.click(screen.getByRole('button', {name: 'Set value for API_TOKEN'}))
    await user.type(screen.getByLabelText('New secret value'), 'fresh-value')
    await user.click(screen.getByRole('button', {name: 'Save'}))

    await waitFor(() => expect(mSet).toHaveBeenCalledWith('API_TOKEN', 'fresh-value'))
  })

  it('badges backup-managed keys and confirms before deleting them', async () => {
    mList.mockResolvedValue(['BACKUP_PASSPHRASE', 'API_TOKEN'])
    const user = userEvent.setup()
    renderPage()
    const list = await screen.findByTestId('secret-list')

    // The backup badge marks the daemon-owned key.
    expect(screen.getByTitle("Managed by Heka's automatic backups")).toBeInTheDocument()
    expect(list).toHaveTextContent('backup')

    // Delete asks first for backup-managed keys.
    await user.click(screen.getByRole('button', {name: 'Delete secret BACKUP_PASSPHRASE'}))
    expect(await screen.findByRole('heading', {name: 'Delete BACKUP_PASSPHRASE?'})).toBeInTheDocument()
    expect(screen.getByText(/Scheduled backups will fail/)).toBeInTheDocument()
    expect(mDelete).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', {name: 'Delete anyway'}))
    await waitFor(() => expect(mDelete).toHaveBeenCalledWith('BACKUP_PASSPHRASE'))
  })

  it('deletes a regular key after a generic confirmation', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('secret-list')

    await user.click(screen.getByRole('button', {name: 'Delete secret API_TOKEN'}))
    expect(await screen.findByRole('heading', {name: 'Delete API_TOKEN?'})).toBeInTheDocument()
    expect(screen.getByText(/This cannot be undone/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', {name: 'Delete anyway'}))
    await waitFor(() => expect(mDelete).toHaveBeenCalledWith('API_TOKEN'))
  })

  it('warns about backup-managed keys in the bulk delete dialog', async () => {
    mList.mockResolvedValue(['BACKUP_PASSPHRASE', 'OLD_KEY'])
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('secret-list')

    await user.click(screen.getByLabelText('Select secret BACKUP_PASSPHRASE'))
    await user.click(screen.getByRole('button', {name: 'Delete selected (1)'}))
    expect(await screen.findByText(/managed by automatic backups/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', {name: 'Delete'}))
    await waitFor(() => expect(mDelete).toHaveBeenCalledWith('BACKUP_PASSPHRASE'))
  })

  it('shows an empty state when the vault is empty', async () => {
    mList.mockResolvedValue([])
    renderPage()
    expect(await screen.findByText('No secrets yet — add one above.')).toBeInTheDocument()
  })

  it('links back to the secrets settings tab', async () => {
    renderPage()
    await screen.findByTestId('secret-list')
    expect(screen.getByText('Back to settings').closest('a')).toHaveAttribute(
      'href',
      '#/settings?tab=secrets'
    )
  })
})

