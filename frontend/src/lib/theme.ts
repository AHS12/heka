// lib/theme.ts (SPEC-12 §4) — Theme mode (light/dark/system) and visual
// variant. Light variants: khaki, crt. Dark variants: gradient, high-contrast.
// Both are persisted to localStorage. The resolved data-theme value is
// applied on <html>, and the .dark class is toggled so HeroUI and Tailwind agree.
import {create} from 'zustand'

export type ThemeChoice = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

/** Light-only variants */
export type LightVariant = 'khaki' | 'crt'
/** Dark-only variants */
export type DarkVariant = 'gradient' | 'high-contrast'
/** Union of all variants */
export type ThemeVariant = LightVariant | DarkVariant

export const LIGHT_VARIANTS: LightVariant[] = ['khaki', 'crt']
export const DARK_VARIANTS: DarkVariant[] = ['gradient', 'high-contrast']
export const THEME_VARIANTS: ThemeVariant[] = [...LIGHT_VARIANTS, ...DARK_VARIANTS]

const MODE_KEY = 'heka-theme'
const VARIANT_KEY = 'heka-theme-variant'

export function systemPrefersDark(): boolean {
  return (
    typeof window !== 'undefined' &&
    !!window.matchMedia?.('(prefers-color-scheme: dark)')?.matches
  )
}

export function resolveTheme(choice: ThemeChoice): ResolvedTheme {
  if (choice === 'system') {
    return systemPrefersDark() ? 'dark' : 'light'
  }
  return choice
}

/** Map (mode, variant) → the data-theme string applied to <html>. */
function resolveDataTheme(mode: ResolvedTheme, variant: ThemeVariant): string {
  if (mode === 'light') {
    return variant === 'crt' ? 'crt' : 'khaki'
  }
  // dark
  return variant === 'high-contrast' ? 'high-contrast' : 'gradient-dark'
}

/** Return a valid variant for the given mode. If the stored variant belongs
 *  to the other mode, switch to the first variant of the new mode. */
function coerceVariant(mode: ResolvedTheme, variant: ThemeVariant): ThemeVariant {
  if (mode === 'light' && (LIGHT_VARIANTS as readonly string[]).includes(variant)) {
    return variant as LightVariant
  }
  if (mode === 'dark' && (DARK_VARIANTS as readonly string[]).includes(variant)) {
    return variant as DarkVariant
  }
  // Variant doesn't match mode — pick first for current mode
  return mode === 'light' ? LIGHT_VARIANTS[0] : DARK_VARIANTS[0]
}

function applyTheme(choice: ThemeChoice, variant: ThemeVariant): ResolvedTheme {
  const resolved = resolveTheme(choice)
  const coerced = coerceVariant(resolved, variant)
  const dataTheme = resolveDataTheme(resolved, coerced)
  document.documentElement.dataset.theme = dataTheme
  document.documentElement.classList.toggle('dark', resolved === 'dark')
  return resolved
}

function initialMode(): ThemeChoice {
  const stored = localStorage.getItem(MODE_KEY)
  return stored === 'light' || stored === 'dark' || stored === 'system'
    ? stored
    : 'system'
}

function initialVariant(): ThemeVariant {
  const stored = localStorage.getItem(VARIANT_KEY)
  if (stored === 'khaki' || stored === 'crt' || stored === 'gradient' || stored === 'high-contrast') {
    return stored as ThemeVariant
  }
  return 'khaki'
}

interface ThemeState {
  choice: ThemeChoice
  resolved: ResolvedTheme
  variant: ThemeVariant
  /** The effective variant after coercion for the current mode */
  effectiveVariant: ThemeVariant
  setTheme: (choice: ThemeChoice) => void
  setVariant: (variant: ThemeVariant) => void
}

export const useTheme = create<ThemeState>((set, get) => {
  const choice = initialMode()
  const variant = initialVariant()
  const resolved = applyTheme(choice, variant)
  const effective = coerceVariant(resolved, variant)
  return {
    choice,
    resolved,
    variant,
    effectiveVariant: effective,
    setTheme: (choice) => {
      if (choice !== 'light' && choice !== 'dark' && choice !== 'system') {
        return
      }
      localStorage.setItem(MODE_KEY, choice)
      const {variant} = get()
      const resolved = applyTheme(choice, variant)
      const effective = coerceVariant(resolved, variant)
      set({choice, resolved, effectiveVariant: effective})
    },
    setVariant: (variant) => {
      if (!(THEME_VARIANTS as readonly string[]).includes(variant)) {
        return
      }
      localStorage.setItem(VARIANT_KEY, variant)
      const {choice} = get()
      const resolved = applyTheme(choice, variant)
      const effective = coerceVariant(resolved, variant)
      set({variant, resolved, effectiveVariant: effective})
    },
  }
})

// While on "system", live-track the OS preference (SPEC-12 §4).
if (typeof window !== 'undefined' && window.matchMedia) {
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  const onChange = () => {
    if (useTheme.getState().choice === 'system') {
      const {variant} = useTheme.getState()
      const resolved = applyTheme('system', variant)
      const effective = coerceVariant(resolved, variant)
      useTheme.setState({resolved, effectiveVariant: effective})
    }
  }
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', onChange)
  } else if (typeof mq.addListener === 'function') {
    mq.addListener(onChange) // legacy Safari
  }
}
