// SchedulerPausedBanner tests: visible only while health reports the
// scheduler paused, with a Resume action that calls the daemon.
import {describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {Health, ResumeScheduler} from '@wailsjs/go/app/App'
import {SchedulerPausedBanner} from './SchedulerPausedBanner'

const mHealth = vi.mocked(Health)
const mResume = vi.mocked(ResumeScheduler)

function renderBanner() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <SchedulerPausedBanner />
    </QueryClientProvider>
  )
}

describe('SchedulerPausedBanner', () => {
  it('shows the banner with a Resume action while paused', async () => {
    mHealth.mockResolvedValue({
      version: 'test', uptime_seconds: 0, core: 'healthy', scheduler: 'paused',
    })
    renderBanner()

    expect(await screen.findByTestId('scheduler-paused-banner')).toHaveTextContent(
      'Scheduler is paused'
    )
    expect(screen.getByRole('button', {name: 'Resume'})).toBeInTheDocument()
  })

  it('stays hidden while the scheduler runs', async () => {
    mHealth.mockResolvedValue({
      version: 'test', uptime_seconds: 0, core: 'healthy', scheduler: 'running',
    })
    renderBanner()

    await waitFor(() =>
      expect(screen.queryByTestId('scheduler-paused-banner')).not.toBeInTheDocument()
    )
  })

  it('resumes the scheduler from the banner', async () => {
    mHealth.mockResolvedValue({
      version: 'test', uptime_seconds: 0, core: 'healthy', scheduler: 'paused',
    })
    mResume.mockResolvedValue()
    const user = userEvent.setup()
    renderBanner()

    await user.click(await screen.findByRole('button', {name: 'Resume'}))
    await waitFor(() => expect(mResume).toHaveBeenCalledTimes(1))
  })
})
