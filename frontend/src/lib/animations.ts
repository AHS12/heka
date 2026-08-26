// lib/animations.ts — User-configurable animation toggle. Persisted to
// localStorage. When disabled, all CSS transitions and animations are
// suppressed via a data attribute on <html>, and prefers-reduced-motion
// is always respected regardless.
import {create} from 'zustand'

const STORAGE_KEY = 'heka-animations'

export function animationsEnabled(): boolean {
  if (typeof window === 'undefined') return true
  // Always disable if OS requests reduced motion
  if (window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches) return false
  const stored = localStorage.getItem(STORAGE_KEY)
  return stored !== 'false'
}

function applyAnimations(enabled: boolean) {
  document.documentElement.dataset.noAnimations = String(!enabled)
}

interface AnimationsState {
  enabled: boolean
  setEnabled: (enabled: boolean) => void
}

export const useAnimations = create<AnimationsState>((set) => {
  const enabled = animationsEnabled()
  applyAnimations(enabled)
  return {
    enabled,
    setEnabled: (next) => {
      localStorage.setItem(STORAGE_KEY, String(next))
      applyAnimations(next)
      set({enabled: next})
    },
  }
})

// Respect live OS preference changes
if (typeof window !== 'undefined' && window.matchMedia) {
  const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
  const onChange = () => {
    if (mq.matches) {
      applyAnimations(false)
      useAnimations.setState({enabled: false})
    }
  }
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', onChange)
  }
}
