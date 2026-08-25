// router tests (SPEC-12 §6.3): every PRD §24 route renders its page,
// and unknown hashes redirect to Dashboard. AppRouter sits inside App's
// provider stack, so the test recreates the QueryClient wrapper.
import {beforeEach, describe, expect, it, vi} from 'vitest'
import {render} from '@testing-library/react'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {DaemonStatus} from '@wailsjs/go/app/App'
import {AppRouter} from './router'

vi.mocked(DaemonStatus).mockResolvedValue('running')

async function renderAt(hash: string) {
  window.location.hash = hash
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
  return render(
    <QueryClientProvider client={client}>
      <AppRouter />
    </QueryClientProvider>
  )
}

describe('router', () => {
  beforeEach(() => {
    window.location.hash = '#/'
  })

  it('renders Dashboard as placeholder', async () => {
    const {findByTestId, findByRole} = await renderAt('#/')
    expect(await findByTestId('placeholder-page')).toBeInTheDocument()
    expect(await findByRole('heading', {level: 2, name: 'Dashboard'})).toBeInTheDocument()
  })

  it.each([
    ['#/schedules', 'Schedules'],
    ['#/logs', 'Logs'],
  ])('renders %s as %s', async (hash, title) => {
    const {findByRole} = await renderAt(hash)
    expect(await findByRole('heading', {level: 2, name: title})).toBeInTheDocument()
  })

  it('renders the Tasks page at #/tasks', async () => {
    const {findByRole} = await renderAt('#/tasks')
    expect(
      await findByRole('heading', {level: 2, name: 'Tasks'})
    ).toBeInTheDocument()
  })

  it('renders Settings with the vault manager at #/settings', async () => {
    const {findByRole} = await renderAt('#/settings')
    expect(
      await findByRole('heading', {level: 2, name: 'Settings'})
    ).toBeInTheDocument()
    expect(
      await findByRole('heading', {level: 3, name: 'Secrets'})
    ).toBeInTheDocument()
  })

  it('redirects unknown routes to Dashboard', async () => {
    const {findByRole} = await renderAt('#/nope')
    expect(
      await findByRole('heading', {level: 2, name: 'Dashboard'})
    ).toBeInTheDocument()
    expect(window.location.hash).toBe('#/')
  })

  it('navigates between routes through the sidebar', async () => {
    const {findByRole} = await renderAt('#/')
    await findByRole('heading', {level: 2, name: 'Dashboard'})
    ;(await findByRole('link', {name: 'Tasks'})).click()
    expect(
      await findByRole('heading', {level: 2, name: 'Tasks'})
    ).toBeInTheDocument()
    expect(window.location.hash).toBe('#/tasks')
  })
})