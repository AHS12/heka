// TopNav (SPEC-12 §3, app-chrome look): each control is its own floating
// pill — brand top-left, route pills, then tools (settings/theme/health)
// floated to the right. No grouped bar, no search.
import {NavLink} from 'react-router-dom'
import {useTheme} from '../lib/theme'
import {useDaemonMode} from '../lib/query'
import {DaemonStatusIcon} from './DaemonStatusIcon'

import {APP_VERSION} from '../lib/version'

const NAV_ITEMS = [
  {to: '/', label: 'Home', end: true},
  {to: '/tasks', label: 'Tasks'},
  {to: '/schedules', label: 'Schedules'},
  {to: '/logs', label: 'Logs'},
]

const PILL =
  'rounded-full border border-border/80 bg-surface/80 px-3.5 py-1.5 ' +
  'text-sm font-medium text-foreground/75 shadow-sm shadow-zinc-900/5 backdrop-blur-md ' +
  'outline-none transition-colors focus-visible:ring-2 focus-visible:ring-accent-ring ' +
  'aria-[current=page]:border-accent aria-[current=page]:bg-accent ' +
  'aria-[current=page]:text-accent-contrast ' +
  'hover:bg-surface-secondary/70 hover:text-foreground'

const ICON_PILL =
  'grid size-9 place-items-center rounded-full border border-border/80 ' +
  'bg-surface/80 text-foreground/75 shadow-sm shadow-zinc-900/5 backdrop-blur-md outline-none ' +
  'transition-colors focus-visible:ring-2 focus-visible:ring-accent-ring ' +
  'hover:bg-surface-secondary/70 hover:text-foreground'

function GearIcon() {
  return (
    <svg
      aria-hidden
      viewBox="0 0 24 24"
      className="size-4 fill-none stroke-current stroke-2"
    >
      <path d="M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h.01a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51h.01a1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v.01a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z" />
    </svg>
  )
}

function InfoIcon() {
  return (
    <svg aria-hidden viewBox="0 0 24 24" className="size-4 fill-none stroke-current stroke-2">
      <circle cx="12" cy="12" r="10" />
      <path d="M12 16v-4M12 8h.01" />
    </svg>
  )
}

function ThemeIcon({dark}: {dark: boolean}) {
  return dark ? (
    <svg aria-hidden viewBox="0 0 16 16" className="size-4 fill-none stroke-current stroke-2">
      <path d="M13 9.5A5.5 5.5 0 0 1 6.5 3a5.5 5.5 0 1 0 6.5 6.5Z" />
    </svg>
  ) : (
    <svg aria-hidden viewBox="0 0 16 16" className="size-4 fill-none stroke-current stroke-2">
      <circle cx="8" cy="8" r="3.5" />
      <path d="M8 1v2M8 13v2M1 8h2M13 8h2M3.2 3.2l1.4 1.4M11.4 11.4l1.4 1.4M12.8 3.2l-1.4 1.4M4.6 11.4l-1.4 1.4" />
    </svg>
  )
}

export function TopNav() {
  const {mode} = useDaemonMode()
  const {choice, setTheme} = useTheme()
  const next = choice === 'dark' ? 'light' : 'dark'

  return (
    <header className="flex shrink-0 flex-wrap items-center gap-2 px-5 pt-4">
      <div className="mr-2 flex items-baseline gap-1.5">
        <span className="font-brand text-sm font-bold tracking-wider">Heka</span>
        <span className="text-xs text-foreground/50">
          v{APP_VERSION}
        </span>
      </div>

      <nav aria-label="Main" className="flex flex-wrap items-center gap-2">
        {NAV_ITEMS.map((item) => (
          <NavLink key={item.to} to={item.to} end={item.end} className={PILL}>
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div className="flex-1" />

      <NavLink to="/settings" aria-label="Settings" className={ICON_PILL}>
        <GearIcon />
      </NavLink>

      <NavLink to="/about" aria-label="About" className={ICON_PILL}>
        <InfoIcon />
      </NavLink>

      <button
        type="button"
        aria-label={next === 'dark' ? 'Switch to dark theme' : 'Switch to light theme'}
        onClick={() => setTheme(next)}
        className={ICON_PILL}
      >
        <ThemeIcon dark={next === 'dark'} />
      </button>

      <DaemonStatusIcon mode={mode} />
    </header>
  )
}