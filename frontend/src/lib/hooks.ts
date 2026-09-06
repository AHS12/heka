// lib/hooks.ts — small shared React hooks.

import {useEffect, useRef, useState} from 'react'

/** Returns the value delayed by `delay` ms. The canonical input for
 *  server-side search so keystrokes don't storm the daemon. */
export function useDebounced<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(t)
  }, [value, delay])
  return debounced
}

/** Calls onIntersect whenever the sentinel element scrolls into (or starts
 *  near) the viewport — the infinite-scroll trigger. Returns a callback ref
 *  (re-observes if the sentinel mounts late, e.g. after the first page
 *  loads). */
export function useSentinel(onIntersect: () => void) {
  const cbRef = useRef(onIntersect)
  cbRef.current = onIntersect
  const [el, setEl] = useState<HTMLElement | null>(null)
  useEffect(() => {
    if (!el || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) cbRef.current()
      },
      {rootMargin: '200px'}
    )
    io.observe(el)
    return () => io.disconnect()
  }, [el])
  return setEl
}
