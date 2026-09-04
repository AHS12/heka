import {useState} from 'react'
import type {Schedule} from '../../lib/api'
import {StatusChip} from '../tasks/TaskTable'
import {Toggle} from '../controls'

function splitDateTime(iso?: string) {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return {date: iso, time: ''}
  return {
    date: d.toLocaleDateString([], {month: 'short', day: 'numeric', year: 'numeric'}),
    time: d.toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'}),
  }
}

function DetailBlock({label, children}: {label: string; children: React.ReactNode}) {
  return (
    <div className="min-w-0">
      <div className="mb-0.5 text-[10px] font-medium uppercase tracking-wider text-foreground/50">
        {label}
      </div>
      <div className="min-w-0 text-sm text-foreground/75">{children}</div>
    </div>
  )
}

export {DetailBlock}

export function DateTime({iso, empty = '—'}: {iso?: string; empty?: string}) {
  const value = splitDateTime(iso)
  if (!value) return <span className="text-xs text-foreground/50">{empty}</span>
  return (
    <span className="block whitespace-nowrap text-xs leading-tight" title={iso}>
      <span className="text-foreground/75">{value.date}</span>
      {value.time && (
        <span className="ml-1.5 text-foreground/50">{value.time}</span>
      )}
    </span>
  )
}

function ScheduleCard({
  schedule,
  onToggle,
  onDelete,
}: {
  schedule: Schedule
  onToggle: (id: string, enabled: boolean) => void
  onDelete: (id: string) => void
}) {
  const [confirming, setConfirming] = useState(false)
  const s = schedule
  const status = s.latest_run_status || s.last_status
  const finishedAt = s.latest_run_finished_at || s.latest_run_started_at || s.last_run_at
  const hasIssues = s.skipped_count > 0 || s.missed_count > 0

  return (
    <div
      data-testid={`schedule-row-${s.id}`}
      className="group rounded-2xl border border-border/80 bg-surface/70 p-4 shadow-sm shadow-zinc-900/5 backdrop-blur transition-shadow hover:shadow-md dark:hover:shadow-zinc-900/20"
    >
      {/* Header: identity + controls */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-semibold text-foreground" title={s.slug}>
              {s.slug}
            </span>
            <span
              className={`inline-flex shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                s.kind === 'recurring'
                  ? 'bg-blue-50 text-blue-600 dark:bg-blue-950 dark:text-blue-400'
                  : 'bg-amber-50 text-amber-600 dark:bg-amber-950 dark:text-amber-400'
              }`}
            >
              {s.kind}
            </span>
          </div>
          <div className="mt-0.5 truncate text-xs text-foreground/50" title={s.task_slug}>
            {s.task_slug}
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2" onClick={(e) => e.stopPropagation()}>
          <Toggle
            checked={s.enabled}
            onChange={(next) => onToggle(s.id, next)}
            label={`${s.enabled ? 'Disable' : 'Enable'} ${s.slug}`}
          />
          {confirming ? (
            <span className="inline-flex items-center gap-1.5">
              <button
                type="button"
                onClick={() => { setConfirming(false); onDelete(s.id) }}
                className="rounded-md bg-red-600 px-2 py-1 text-xs font-medium text-white outline-none focus-visible:ring-2 focus-visible:ring-accent-ring"
              >
                Yes
              </button>
              <button
                type="button"
                onClick={() => setConfirming(false)}
                className="rounded-md px-2 py-1 text-xs text-foreground/55 outline-none hover:bg-surface-secondary/70"
              >
                No
              </button>
            </span>
          ) : (
            <button
              type="button"
              onClick={() => setConfirming(true)}
              className="rounded-lg p-1.5 text-foreground/50 outline-none hover:bg-red-100 hover:text-red-600 focus-visible:ring-2 focus-visible:ring-accent-ring dark:hover:bg-red-950/50 dark:hover:text-red-400"
              aria-label={`Delete schedule ${s.slug}`}
            >
              <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="3 6 5 6 21 6" />
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
              </svg>
            </button>
          )}
        </div>
      </div>

      {/* Detail row */}
      <div className="mt-3 flex flex-wrap items-start gap-x-6 gap-y-3 border-t border-border/60 pt-3">
        <DetailBlock label="Rule">
          {s.kind === 'recurring' ? (
            <span className="font-mono text-xs text-foreground/75" title={s.cron}>
              {s.cron}
            </span>
          ) : (
            <DateTime iso={s.run_at} />
          )}
        </DetailBlock>

        <DetailBlock label="Next run">
          <DateTime iso={s.next_run_at} />
        </DetailBlock>

        <DetailBlock label="Last run">
          {status ? (
            <div className="flex items-center gap-2">
              <StatusChip status={status} />
              <DateTime iso={finishedAt} />
            </div>
          ) : (
            <span className="text-xs text-foreground/50">Never</span>
          )}
        </DetailBlock>

        <DetailBlock label="Missed policy">
          <span
            className={
              s.missed_policy === 'run_now'
                ? 'text-xs font-medium text-emerald-600 dark:text-emerald-400'
                : 'text-xs font-medium text-foreground/50'
            }
            title={
              s.missed_policy === 'run_now'
                ? 'Missed runs fire immediately on next reconcile'
                : 'Missed runs are recorded but not executed'
            }
            data-testid={`schedule-missed-policy-${s.id}`}
          >
            {s.missed_policy === 'run_now' ? 'Run now' : 'Skip'}
          </span>
        </DetailBlock>

        {hasIssues && (
          <DetailBlock label="Issues">
            <div className="flex items-center gap-2 text-xs">
              {s.skipped_count > 0 && (
                <span className="text-amber-600 dark:text-amber-400">
                  {s.skipped_count} skipped
                </span>
              )}
              {s.missed_count > 0 && (
                <span className="text-red-600 dark:text-red-400">
                  {s.missed_count} missed
                </span>
              )}
            </div>
          </DetailBlock>
        )}
      </div>
    </div>
  )
}

export function ScheduleTable({
  schedules,
  onToggle,
  onDelete,
}: {
  schedules: Schedule[]
  onToggle: (id: string, enabled: boolean) => void
  onDelete: (id: string) => void
}) {
  return (
    <div className="grid gap-3">
      {schedules.map((s) => (
        <ScheduleCard
          key={s.id}
          schedule={s}
          onToggle={onToggle}
          onDelete={onDelete}
        />
      ))}
    </div>
  )
}
