// lib/theme.ts (SPEC-12 §4) — 'light' | 'dark' | 'system' preference,
// persisted to localStorage and applied as data-theme on <html>. The HeroUI
// provider (App.tsx) reads the same store so components and the shell agree.
import {create} from 'zustand'

export type ThemeChoice = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

const STORAGE_KEY = 'heka-theme'

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

function applyTheme(choice: ThemeChoice): ResolvedTheme {
  const resolved = resolveTheme(choice)
  document.documentElement.dataset.theme = resolved
  // HeroUI's plugin scopes dark styles to a .dark ancestor; our Tailwind
  // custom variant uses the same class, so the shell and components agree.
  document.documentElement.classList.toggle('dark', resolved === 'dark')
  return resolved
}

function initialChoice(): ThemeChoice {
  const stored = localStorage.getItem(STORAGE_KEY)
  return stored === 'light' || stored === 'dark' || stored === 'system'
    ? stored
    : 'system'
}

interface ThemeState {
  choice: ThemeChoice
  resolved: ResolvedTheme
  setTheme: (choice: ThemeChoice) => void
}

export const useTheme = create<ThemeState>((set) => {
  const choice = initialChoice()
  return {
    choice,
    resolved: applyTheme(choice),
    setTheme: (choice) => {
      // localStorage is read as an external boundary — reject junk.
      if (choice !== 'light' && choice !== 'dark' && choice !== 'system') {
        return
      }
      localStorage.setItem(STORAGE_KEY, choice)
      set({choice, resolved: applyTheme(choice)})
    },
  }
})

// While on "system", live-track the OS preference (SPEC-12 §4).
if (typeof window !== 'undefined' && window.matchMedia) {
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  const onChange = () => {
    if (useTheme.getState().choice === 'system') {
      useTheme.setState({resolved: applyTheme('system')})
    }
  }
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', onChange)
  } else if (typeof mq.addListener === 'function') {
    mq.addListener(onChange) // legacy Safari
  }
}