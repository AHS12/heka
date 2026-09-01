// AppLayout (SPEC-12 §3) — single-column app shell: floating top pill bar,
// one scrollable content column. The scroll container reveals its scrollbar
// only while the user is actually scrolling (fades ~600ms after).
import {useEffect, useRef, useState} from 'react'
import {Outlet} from 'react-router-dom'
import {TopNav} from '../components/TopNav'
import {DaemonDownBanner} from '../components/DaemonDownBanner'
import {useQuery} from '@tanstack/react-query'
import {daemonStatus} from '../lib/api'

const SCROLLBAR_FADE_MS = 600

export function AppLayout() {
  const mainRef = useRef<HTMLElement>(null)
  const [scrolling, setScrolling] = useState(false)

  // Watch the daemon-status query directly (not via useDaemonMode) so we can
  // detect the down→up transition without coupling to the banner's start logic.
  const {data: status} = useQuery({
    queryKey: ['daemon-status'],
    queryFn: daemonStatus,
    retry: false,
  })

  // Bump the Outlet key when the daemon transitions from down → running.
  // `prevStatus` tracks the last *confirmed* poll result (not the initial
  // undefined) so we never false-bump on first render.
  const [daemonKey, setDaemonKey] = useState(0)
  const prevStatus = useRef(status)
  useEffect(() => {
    if (
      prevStatus.current === 'not-running' &&
      status === 'running'
    ) {
      setDaemonKey((k) => k + 1)
    }
    if (status !== undefined) prevStatus.current = status
  }, [status])

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
    <div
      className="scanline-overlay flex h-screen flex-col text-foreground"
      style={{
        background: `linear-gradient(160deg, var(--gradient-start), var(--gradient-mid), var(--gradient-end))`,
      }}
    >
      <TopNav />
      <main
        ref={mainRef}
        className={`heka-scroll ${scrolling ? 'heka-scroll-show' : ''} mx-auto w-full max-w-6xl min-h-0 flex-1 overflow-y-auto px-4 py-6`}
      >
        <DaemonDownBanner />
        <Outlet key={daemonKey} />
      </main>
    </div>
  )
}