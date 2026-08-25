// AppLayout (SPEC-12 §3) — single-column app shell: floating top pill bar,
// one scrollable content column. The scroll container reveals its scrollbar
// only while the user is actually scrolling (fades ~600ms after).
import {useEffect, useRef, useState} from 'react'
import {Outlet} from 'react-router-dom'
import {TopNav} from '../components/TopNav'
import {DaemonDownBanner} from '../components/DaemonDownBanner'

const SCROLLBAR_FADE_MS = 600

export function AppLayout() {
  const mainRef = useRef<HTMLElement>(null)
  const [scrolling, setScrolling] = useState(false)

  useEffect(() => {
    const el = mainRef.current
    if (!el) return
    let timer: ReturnType<typeof setTimeout> | undefined
    const onScroll = () => {
      setScrolling(true)
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => setScrolling(false), SCROLLBAR_FADE_MS)
    }
    el.addEventListener('scroll', onScroll, {passive: true})
    return () => {
      el.removeEventListener('scroll', onScroll)
      if (timer) clearTimeout(timer)
    }
  }, [])

  return (
    <div className="flex h-screen flex-col bg-gradient-to-b from-zinc-100 via-zinc-50 to-zinc-100 text-zinc-900 dark:from-zinc-950 dark:via-zinc-950 dark:to-zinc-900 dark:text-zinc-100">
      <TopNav />
      <main
        ref={mainRef}
        className={`heka-scroll ${scrolling ? 'heka-scroll-show' : ''} mx-auto w-full max-w-6xl min-h-0 flex-1 overflow-y-auto px-4 py-6`}
      >
        <DaemonDownBanner />
        <Outlet />
      </main>
    </div>
  )
}