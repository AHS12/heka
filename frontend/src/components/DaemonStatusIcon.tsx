// components/DaemonStatusIcon.tsx — compact daemon status: a small colored
// dot in the top chrome (green healthy / red not running / amber starting).
// Hover reveals an info panel (HealthTooltipBody): version, core, scheduler
// state, uptime — plus a paused note when scheduled runs are on hold.
import {Tooltip} from '@heroui/react'
import {Focusable} from 'react-aria-components'
import {useHealth} from '../lib/query'
import type {DaemonMode} from '../lib/query'
import type {HealthDTO} from '../lib/api'

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

export function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const m = Math.floor(seconds / 60)
  return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${m % 60}m`
}

function InfoRow({label, value}: {label: string; value: string}) {
  return (
    <span className="flex items-center justify-between gap-6">
      <span>{label}</span>
      <span className="font-medium text-foreground/75">{value}</span>
    </span>
  )
}

/** Shared hover panel for the daemon status: status line + live health
 *  detail. Used by the navbar dot and the dashboard's engine health chip. */
export function HealthTooltipBody({
  mode,
  health,
}: {
  mode: DaemonMode
  health: {data?: HealthDTO}
}) {
  const d = health.data
  const live = mode === 'running' && d
  const paused = live && d.scheduler !== 'running'
  return (
    <div className="flex min-w-[190px] flex-col gap-1 py-0.5">
      <span className="text-xs font-medium text-foreground">
        {LABEL[mode]}
      </span>
      {live && (
        <div className="flex flex-col gap-0.5 text-[11px] text-foreground/55">
          <InfoRow label="Version" value={`v${d.version}`} />
          <InfoRow label="Core" value={d.core} />
          <InfoRow label="Scheduler" value={d.scheduler} />
          <InfoRow label="Uptime" value={formatUptime(d.uptime_seconds)} />
        </div>
      )}
      {paused && (
        <span className="text-[11px] font-medium text-amber-600 dark:text-amber-400">
          Scheduled runs are on hold
        </span>
      )}
      {mode === 'not-running' && (
        <span className="text-[11px] text-foreground/55">
          Start it from the banner above the page.
        </span>
      )}
    </div>
  )
}

export function DaemonStatusIcon({mode}: {mode: DaemonMode}) {
  const health = useHealth()
  return (
    <Tooltip delay={200}>
      {/* RAC's TooltipTrigger only delivers hover/focus props to elements
          wrapped in a Focusable — raw DOM triggers never open the tooltip. */}
      <Focusable>
        <button
          type="button"
          role="status"
          data-mode={mode}
          aria-label={LABEL[mode]}
          className="grid size-6 place-items-center rounded-full outline-none focus-visible:ring-2 focus-visible:ring-accent-ring"
        >
          <span
            className={`inline-block size-2.5 rounded-full ring-2 ring-border/80 ring-offset-1 ring-offset-background transition-colors ${DOT[mode]}`}
          />
        </button>
      </Focusable>
      <Tooltip.Content showArrow placement="bottom">
        <HealthTooltipBody mode={mode} health={health} />
      </Tooltip.Content>
    </Tooltip>
  )
}
