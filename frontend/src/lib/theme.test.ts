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
    // Default dark variant is crt (crt-dark data-theme)
    expect(document.documentElement.dataset.theme).toBe('crt-dark')
  })

  it('persists an explicit choice to localStorage', async () => {
    const {useTheme} = await loadTheme()
    useTheme.getState().setTheme('dark')
    expect(localStorage.getItem('heka-theme')).toBe('dark')
    // Dark mode applies crt-dark by default
    expect(document.documentElement.dataset.theme).toBe('crt-dark')
  })

  it('restores the persisted choice on next launch', async () => {
    localStorage.setItem('heka-theme', 'dark')
    const {useTheme} = await loadTheme()
    expect(useTheme.getState().choice).toBe('dark')
    expect(useTheme.getState().resolved).toBe('dark')
    expect(document.documentElement.dataset.theme).toBe('crt-dark')
  })

  it('migrates the removed gradient dark variant to crt-dark', async () => {
    localStorage.setItem('heka-theme', 'dark')
    localStorage.setItem('heka-theme-variant', 'gradient')
    await loadTheme()
    expect(document.documentElement.dataset.theme).toBe('crt-dark')
  })

  it('applies high-contrast-light when light mode picks the high-contrast variant', async () => {
    localStorage.setItem('heka-theme', 'light')
    localStorage.setItem('heka-theme-variant', 'high-contrast')
    const {useTheme} = await loadTheme()
    expect(useTheme.getState().resolved).toBe('light')
    expect(document.documentElement.dataset.theme).toBe('high-contrast-light')
    // The .dark class must stay off so Tailwind dark: variants never fire.
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('keeps the shared high-contrast id when switching between modes', async () => {
    localStorage.setItem('heka-theme', 'light')
    localStorage.setItem('heka-theme-variant', 'high-contrast')
    const {useTheme} = await loadTheme()
    useTheme.getState().setTheme('dark')
    expect(document.documentElement.dataset.theme).toBe('high-contrast')
    useTheme.getState().setTheme('light')
    expect(document.documentElement.dataset.theme).toBe('high-contrast-light')
  })

  it('maps the dark khaki variant to the khaki-dark data-theme', async () => {
    localStorage.setItem('heka-theme', 'dark')
    localStorage.setItem('heka-theme-variant', 'khaki-dark')
    const {useTheme} = await loadTheme()
    expect(useTheme.getState().resolved).toBe('dark')
    expect(document.documentElement.dataset.theme).toBe('khaki-dark')
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
