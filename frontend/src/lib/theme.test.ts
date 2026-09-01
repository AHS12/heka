// theme.ts tests (SPEC-12 §6.2): system default, explicit persistence (the
// store is a module singleton, so reload it like a fresh app), and the
// data-theme side effect on <html>.
import {beforeEach, describe, expect, it, vi} from 'vitest'

function setPrefersDark(prefers: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query === '(prefers-color-scheme: dark)' ? prefers : false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
}

async function loadTheme() {
  vi.resetModules()
  return await import('./theme')
}

beforeEach(() => {
  localStorage.clear()
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.classList.remove('dark')
  setPrefersDark(false)
})

describe('theme store', () => {
  it('defaults to system (light here) and applies data-theme as khaki', async () => {
    const {useTheme, resolveTheme} = await loadTheme()
    expect(useTheme.getState().choice).toBe('system')
    expect(useTheme.getState().resolved).toBe('light')
    expect(resolveTheme('light')).toBe('light')
    // Default light variant is khaki
    expect(document.documentElement.dataset.theme).toBe('khaki')
  })

  it('resolves system → dark when the OS prefers dark', async () => {
    setPrefersDark(true)
    const {useTheme} = await loadTheme()
    expect(useTheme.getState().resolved).toBe('dark')
    // Default dark variant is gradient (gradient-dark data-theme)
    expect(document.documentElement.dataset.theme).toBe('gradient-dark')
  })

  it('persists an explicit choice to localStorage', async () => {
    const {useTheme} = await loadTheme()
    useTheme.getState().setTheme('dark')
    expect(localStorage.getItem('heka-theme')).toBe('dark')
    // Dark mode applies gradient-dark by default
    expect(document.documentElement.dataset.theme).toBe('gradient-dark')
  })

  it('restores the persisted choice on next launch', async () => {
    localStorage.setItem('heka-theme', 'dark')
    const {useTheme} = await loadTheme()
    expect(useTheme.getState().choice).toBe('dark')
    expect(useTheme.getState().resolved).toBe('dark')
    expect(document.documentElement.dataset.theme).toBe('gradient-dark')
  })

  it('ignores garbage in localStorage', async () => {
    localStorage.setItem('heka-theme', 'neon')
    const {useTheme} = await loadTheme()
    expect(useTheme.getState().choice).toBe('system')
  })

  it('rejects unknown choices at the storage boundary', async () => {
    const {useTheme, resolveTheme} = await loadTheme()
    // @ts-expect-error deliberate bad input
    useTheme.getState().setTheme('neon')
    expect(localStorage.getItem('heka-theme')).toBeNull()
    expect(useTheme.getState().choice).toBe('system')
    // Default light variant is khaki
    expect(document.documentElement.dataset.theme).toBe('khaki')
  })
})
