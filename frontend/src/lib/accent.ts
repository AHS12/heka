// lib/accent.ts — the accent foundation the Settings page surfaces. Accent =
// the brand color behind active pills, focus rings, primary buttons, links.
// Presets swap CSS variables via the data-accent attribute on <html>; a
// custom color is applied as an inline --accent override (wins over :root,
// so the same bg-accent etc. utilities follow it).
import {create} from 'zustand'

/** Every preset accent main.css defines a data-accent rule for. */
export const ACCENT_PRESETS = [
  'blue',
  'violet',
  'emerald',
  'amber',
  'rose',
  'cyan',
] as const

export type PresetAccent = (typeof ACCENT_PRESETS)[number]
export type Accent = PresetAccent | 'custom'

/** Swatch previews — mirrors main.css's oklch values so what you see is what
 *  the CSS variables render. */
export const ACCENT_COLORS: Record<PresetAccent, string> = {
  blue: 'oklch(0.55 0.22 250)',
  violet: 'oklch(0.56 0.23 290)',
  emerald: 'oklch(0.6 0.17 160)',
  amber: 'oklch(0.68 0.16 75)',
  rose: 'oklch(0.58 0.21 15)',
  cyan: 'oklch(0.65 0.15 210)',
}

const STORAGE_KEY = 'heka-accent'
const CUSTOM_STORAGE_KEY = 'heka-accent-custom'
const FALLBACK_CUSTOM = '#2563eb'

function isHexColor(value: unknown): value is string {
  return typeof value === 'string' && /^#[0-9a-f]{6}$/i.test(value)
}

function initial(): {accent: Accent; customColor: string} {
  const stored = localStorage.getItem(STORAGE_KEY)
  const customColor = isHexColor(localStorage.getItem(CUSTOM_STORAGE_KEY))
    ? (localStorage.getItem(CUSTOM_STORAGE_KEY) as string)
    : FALLBACK_CUSTOM
  if (stored === 'custom') {
    return {accent: 'custom', customColor}
  }
  return {
    accent: (ACCENT_PRESETS as readonly string[]).includes(stored ?? '')
      ? (stored as PresetAccent)
      : 'blue',
    customColor,
  }
}

function applyAccent(accent: Accent, customColor: string) {
  const el = document.documentElement
  if (accent === 'custom') {
    el.style.setProperty('--accent', customColor)
    el.dataset.accent = 'custom'
    return
  }
  el.style.removeProperty('--accent')
  el.dataset.accent = accent
}

interface AccentState {
  accent: Accent
  customColor: string
  setAccent: (accent: Accent) => void
  setCustomColor: (hex: string) => void
}

export const useAccent = create<AccentState>((set) => {
  const init = initial()
  applyAccent(init.accent, init.customColor)
  return {
    accent: init.accent,
    customColor: init.customColor,
    setAccent: (accent) => {
      // localStorage is read as an external boundary — reject junk.
      if (
        accent !== 'custom' &&
        !(ACCENT_PRESETS as readonly string[]).includes(accent)
      ) {
        return
      }
      localStorage.setItem(STORAGE_KEY, accent)
      set((s) => {
        applyAccent(accent, s.customColor)
        return {accent}
      })
    },
    setCustomColor: (hex) => {
      if (!isHexColor(hex)) {
        return
      }
      localStorage.setItem(CUSTOM_STORAGE_KEY, hex)
      localStorage.setItem(STORAGE_KEY, 'custom')
      set(() => {
        applyAccent('custom', hex)
        return {accent: 'custom', customColor: hex}
      })
    },
  }
})