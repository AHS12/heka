// AppLayout tests (SPEC-12 §6.7): the shell renders sidebar + topbar and its
// routed children through <Outlet/>.
import {describe, expect, it, vi} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import {MemoryRouter, Route, Routes} from 'react-router-dom'
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {DaemonStatus} from '@wailsjs/go/app/App'
import {AppLayout} from './AppLayout'

vi.mocked(DaemonStatus).mockResolvedValue('running')

describe('AppLayout', () => {
  it('renders shell chrome and the routed child', async () => {
    const client = new QueryClient({defaultOptions: {queries: {retry: false}}})
    render(
      <MemoryRouter initialEntries={['/']}>
        <QueryClientProvider client={client}>
          <Routes>
            <Route element={<AppLayout />}>
              <Route index element={<div>home-child</div>} />
            </Route>
          </Routes>
        </QueryClientProvider>
      </MemoryRouter>
    )

    expect(await screen.findByText('home-child')).toBeInTheDocument()
    expect(screen.getByRole('navigation', {name: 'Main'})).toBeInTheDocument()
    expect(screen.getByRole('link', {name: 'Tasks'})).toBeInTheDocument()
    expect(screen.getByText(/v0\.1\.0/)).toBeInTheDocument()
    // The banner starts visible while the first poll is in flight, then
    // disappears once the mocked status resolves (SPEC-12 §5 re-poll).
    await waitFor(() =>
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    )
  })
})