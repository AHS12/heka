// WhatsNewModal tests: scope-based changelog slicing — 'newer' shows the
// releases past the seen version, 'latest' only the current release, 'all'
// everything — plus stamp-on-dismiss vs no-persist-in-force-mode.
import {describe, expect, it, vi, beforeEach} from 'vitest'
import {render, screen, waitFor} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {Changelog, OpenURL} from '@wailsjs/go/app/App'
import {WhatsNewModal, HEKA_CHANGELOG_URL} from './WhatsNewModal'
import {useOnboarding, type WhatsNewScope} from '../lib/onboarding'
import {APP_VERSION} from '../lib/version'

const SAMPLE_CHANGELOG = [
  '# Changelog',
  '',
  '## [0.8.2] - 2026-09-05',
  '',
  'The schedule creator grows out of its dropdowns.',
  '',
  '### Added',
  '- Pattern-based schedule builder.',
  '',
  '## [0.8.1] - 2026-09-04',
  '',
  'A correctness pass.',
  '',
  '### Fixed',
  '- Webhook notifications name the task.',
].join('\n')

beforeEach(() => {
  localStorage.clear()
  useOnboarding.setState({
    tourCompleted: true,
    seenVersion: '',
    mode: 'none',
    force: false,
    scope: 'newer',
  })
  vi.mocked(Changelog).mockResolvedValue(SAMPLE_CHANGELOG)
})

function renderModal() {
  return render(<WhatsNewModal />)
}

function openWhatsNew(scope: WhatsNewScope, force: boolean, seenVersion: string) {
  useOnboarding.setState({mode: 'whats-new', scope, force, seenVersion, tourCompleted: true})
}

describe('WhatsNewModal', () => {
  it("scope 'newer' renders every release newer than the seen version, newest first", async () => {
    openWhatsNew('newer', false, '0.8.1')
    renderModal()
    expect(await screen.findByText(/schedule creator grows/)).toBeInTheDocument()
    expect(screen.getByText(/Pattern-based schedule builder/)).toBeInTheDocument()
    expect(screen.queryByText(/correctness pass/)).not.toBeInTheDocument()
  })

  it("scope 'newer' shows nothing-new fallback when no releases are newer", async () => {
    openWhatsNew('newer', false, APP_VERSION)
    renderModal()
    expect(await screen.findByText('No release notes found.')).toBeInTheDocument()
  })

  it("scope 'latest' renders only the current release", async () => {
    openWhatsNew('latest', false, APP_VERSION)
    renderModal()
    expect(await screen.findByText(/schedule creator grows/)).toBeInTheDocument()
    expect(screen.queryByText(/correctness pass/)).not.toBeInTheDocument()
  })

  it("scope 'all' renders every release (force mode)", async () => {
    openWhatsNew('all', true, APP_VERSION)
    renderModal()
    expect(await screen.findByText(/schedule creator grows/)).toBeInTheDocument()
    expect(await screen.findByText(/correctness pass/)).toBeInTheDocument()
  })

  it('Got it persists the current version as seen', async () => {
    openWhatsNew('newer', false, '0.8.1')
    renderModal()
    await screen.findByText(/schedule creator grows/)
    await userEvent.setup().click(screen.getByRole('button', {name: 'Got it'}))
    await waitFor(() => {
      expect(localStorage.getItem('heka-seen-version')).toBe(APP_VERSION)
      expect(useOnboarding.getState().mode).toBe('none')
    })
  })

  it('force-mode dismiss persists nothing', async () => {
    localStorage.setItem('heka-seen-version', '0.8.1')
    openWhatsNew('all', true, '0.8.1')
    renderModal()
    await screen.findByText(/schedule creator grows/)
    await userEvent.setup().click(screen.getByRole('button', {name: 'Got it'}))
    await waitFor(() => expect(useOnboarding.getState().mode).toBe('none'))
    expect(localStorage.getItem('heka-seen-version')).toBe('0.8.1')
  })

  it('View full changelog opens the project website', async () => {
    openWhatsNew('newer', false, '0.8.1')
    renderModal()
    await screen.findByText(/schedule creator grows/)
    await userEvent.setup().click(screen.getByRole('button', {name: 'View full changelog'}))
    expect(vi.mocked(OpenURL)).toHaveBeenCalledWith(HEKA_CHANGELOG_URL)
  })
})
