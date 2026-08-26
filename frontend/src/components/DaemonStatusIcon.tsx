// components/DaemonStatusIcon.tsx — compact daemon status: a small colored
// dot in the top chrome (green healthy / red not running / amber starting).
// Hover reveals the label + live health detail (core, scheduler, uptime).
import {Tooltip} from '@heroui/react'
import {useHealth} from '../lib/query'
import type {DaemonMode} from '../lib/query'

const DOT: Record<DaemonMode, string> = {
  running: 'bg-emerald-500',
  'not-running': 'bg-red-500',
  starting: 'bg-amber-400 animate-pulse',
}

const LABEL: Record<DaemonMode, string> = {
  running: 'Daemon healthy',
  'not-running': 'Daemon not running',
  starting: 'Daemon starting…',
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const m = Math.floor(seconds / 60)
  return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${m % 60}m`
}

export function DaemonStatusIcon({mode}: {mode: DaemonMode}) {
  const health = useHealth()
  const detail =
    health.data && mode === 'running'
      ? [
          `core ${health.data.core}`,
          `scheduler ${health.data.scheduler}`,
          `up ${formatUptime(health.data.uptime_seconds)}`,
        ]
      : null

  return (
    <Tooltip delay={300}>
      <span
        role="status"
        data-mode={mode}
        aria-label={LABEL[mode]}
        className={`inline-block size-2.5 rounded-full ring-2 ring-zinc-200/80 ring-offset-1 ring-offset-white transition-colors dark:ring-zinc-700/80 dark:ring-offset-zinc-950 ${DOT[mode]}`}
      />
      <Tooltip.Content showArrow placement="bottom">
        <div className="flex flex-col gap-0.5 py-0.5">
          <span className="text-xs font-medium text-zinc-900 dark:text-zinc-50">
            {LABEL[mode]}
          </span>
          {detail && (
            <span className="text-[11px] text-zinc-500 dark:text-zinc-400">
              {detail.join(' · ')}
            </span>
          )}
        </div>
      </Tooltip.Content>
    </Tooltip>
  )
}
