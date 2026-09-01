// DaemonDownBanner tests (SPEC-12 §6.6): shown while down, Start invokes the
// binding, and the banner clears once the daemon answers (re-poll via query
// invalidation).
import {describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {DaemonStatus, StartDaemon} from '@wailsjs/go/app/App'
import {DaemonDownBanner} from './DaemonDownBanner'

const mStatus = vi.mocked(DaemonStatus)
const mStart = vi.mocked(StartDaemon)

function renderBanner() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <DaemonDownBanner />
    </QueryClientProvider>
  )
}

describe('DaemonDownBanner', () => {
  it('is hidden while the daemon runs', async () => {
    mStatus.mockResolvedValue('running')
    renderBanner()
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  })

  it('shows the message + Start button while down', async () => {
    mStatus.mockResolvedValue('not-running')
    renderBanner()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Heka daemon is not running'
    )
    expect(screen.getByRole('button', {name: 'Start Daemon'})).toBeInTheDocument()
  })

  it('starts the daemon, re-polls, and clears once it is up', async () => {
    mStatus.mockResolvedValue('not-running')
    mStart.mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderBanner()
    await screen.findByRole('alert')

    // The daemon comes up after the spawn completes.
    mStatus.mockResolvedValue('running')
    await user.click(screen.getByRole('button', {name: 'Start Daemon'}))

    expect(mStart).toHaveBeenCalledTimes(1)
    await waitFor(() =>
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    )
  })
})