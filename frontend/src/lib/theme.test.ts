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
  setPrefersDark(false)
})

describe('theme store', () => {
  it('defaults to system (light here) and applies data-theme', async () => {
    const {useTheme, resolveTheme} = await loadTheme()
    expect(useTheme.getState().choice).toBe('system')
    expect(useTheme.getState().resolved).toBe('light')
    expect(resolveTheme('light')).toBe('light')
    expect(document.documentElement.dataset.theme).toBe('light')
  })

  it('resolves system → dark when the OS prefers dark', async () => {
    setPrefersDark(true)
    const {useTheme} = await loadTheme()
    expect(useTheme.getState().resolved).toBe('dark')
    expect(document.documentElement.dataset.theme).toBe('dark')
  })

  it('persists an explicit choice to localStorage', async () => {
    const {useTheme} = await loadTheme()
    useTheme.getState().setTheme('dark')
    expect(localStorage.getItem('heka-theme')).toBe('dark')
    expect(document.documentElement.dataset.theme).toBe('dark')
  })

  it('restores the persisted choice on next launch', async () => {
    localStorage.setItem('heka-theme', 'dark')
    const {useTheme} = await loadTheme()
    expect(useTheme.getState().choice).toBe('dark')
    expect(useTheme.getState().resolved).toBe('dark')
    expect(document.documentElement.dataset.theme).toBe('dark')
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
    expect(document.documentElement.dataset.theme).toBe(resolveTheme('system'))
  })
})