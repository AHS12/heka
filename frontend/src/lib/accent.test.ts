// accent.ts tests: the foundation for the future Settings accent picker.
// Default 'blue', explicit choices persist, unknown values are rejected at the
// localStorage boundary, and data-accent lands on <html> so main.css swaps
// the CSS variables that power the accent-* utilities.
import {beforeEach, describe, expect, it, vi} from 'vitest'

async function loadAccent() {
  vi.resetModules()
  return await import('./accent')
}

beforeEach(() => {
  localStorage.clear()
  document.documentElement.removeAttribute('data-accent')
  document.documentElement.style.removeProperty('--accent')
})

describe('accent store', () => {
  it('defaults to blue and applies data-accent', async () => {
    const {useAccent} = await loadAccent()
    expect(useAccent.getState().accent).toBe('blue')
    expect(document.documentElement.dataset.accent).toBe('blue')
  })

  it('persists an explicit choice', async () => {
    const {useAccent} = await loadAccent()
    useAccent.getState().setAccent('violet')
    expect(localStorage.getItem('heka-accent')).toBe('violet')
    expect(document.documentElement.dataset.accent).toBe('violet')
  })

  it('restores the persisted choice on next launch', async () => {
    localStorage.setItem('heka-accent', 'amber')
    const {useAccent} = await loadAccent()
    expect(useAccent.getState().accent).toBe('amber')
    expect(document.documentElement.dataset.accent).toBe('amber')
  })

  it('falls back to blue for garbage in localStorage', async () => {
    localStorage.setItem('heka-accent', 'neon')
    const {useAccent} = await loadAccent()
    expect(useAccent.getState().accent).toBe('blue')
  })

  it('rejects unknown choices at the storage boundary', async () => {
    const {useAccent, ACCENT_PRESETS} = await loadAccent()
    // @ts-expect-error deliberate bad input
    useAccent.getState().setAccent('neon')
    expect(localStorage.getItem('heka-accent')).toBeNull()
    expect(useAccent.getState().accent).toBe('blue')
    expect(ACCENT_PRESETS).toHaveLength(6)
  })

  it('offers distinct swatch colors for every preset', async () => {
    const {ACCENT_COLORS, ACCENT_PRESETS} = await loadAccent()
    const seen = new Set(ACCENT_PRESETS.map((p) => ACCENT_COLORS[p]))
    expect(seen.size).toBe(ACCENT_PRESETS.length)
    for (const color of seen) {
      expect(color).toBeTruthy()
    }
  })
})

describe('accent custom color', () => {
  it('applies a custom color as an inline --accent override', async () => {
    const {useAccent} = await loadAccent()
    useAccent.getState().setCustomColor('#ff0088')
    expect(useAccent.getState().accent).toBe('custom')
    expect(document.documentElement.dataset.accent).toBe('custom')
    expect(
      document.documentElement.style.getPropertyValue('--accent')
    ).toBe('#ff0088')
    expect(localStorage.getItem('heka-accent-custom')).toBe('#ff0088')
  })

  it('restores a persisted custom color on next launch', async () => {
    localStorage.setItem('heka-accent', 'custom')
    localStorage.setItem('heka-accent-custom', '#00cc77')
    const {useAccent} = await loadAccent()
    expect(useAccent.getState().accent).toBe('custom')
    expect(
      document.documentElement.style.getPropertyValue('--accent')
    ).toBe('#00cc77')
  })

  it('switching back to a preset clears the inline override', async () => {
    const {useAccent} = await loadAccent()
    useAccent.getState().setCustomColor('#ff0088')
    useAccent.getState().setAccent('rose')
    expect(document.documentElement.dataset.accent).toBe('rose')
    expect(
      document.documentElement.style.getPropertyValue('--accent')
    ).toBe('')
  })

  it('rejects malformed color input', async () => {
    const {useAccent} = await loadAccent()
    useAccent.getState().setCustomColor('red')
    expect(useAccent.getState().accent).toBe('blue')
    expect(localStorage.getItem('heka-accent-custom')).toBeNull()
  })
})