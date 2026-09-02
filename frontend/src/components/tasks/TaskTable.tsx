// components/tasks/TaskTable.tsx (SPEC-13 §3) — the tasks list as app-style
// cards (mirroring the schedule cards): name+type badge, runtime, enabled
// toggle, last run status/time, Run + delete actions. Card click opens the
// editor dialog; delete is a two-step inline confirm.
import {useState} from 'react'
import {Chip} from '@heroui/react'
import {Toggle} from '../controls'
import {DetailBlock, DateTime} from '../schedules/ScheduleTable'
import type {TaskSummary} from '../../lib/api'

const STATUS_COLOR: Record<string, 'success' | 'danger' | 'warning' | 'accent' | 'default'> = {
  success: 'success',
  failed: 'danger',
  timed_out: 'warning',
  cancelled: 'default',
  running: 'accent',
  queued: 'default',
  skipped: 'default',
  missed: 'default',
}

/** Statuses that render a spinning run indicator (SPEC-13 §3). */
const RUN_ACTIVE = new Set(['running', 'queued'])

export function StatusChip({status}: {status?: string}) {
  if (!status) {
    return null
  }
  const active = RUN_ACTIVE.has(status)
  return (
    <Chip
      color={STATUS_COLOR[status] ?? 'default'}
      size="sm"
      data-status={status}
    >
      {active && (
        <span data-testid="run-indicator" className="relative flex size-2">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-current opacity-60" />
          <span className="relative inline-flex size-2 rounded-full bg-current" />
        </span>
      )}
      {status.replace(/_/g, ' ')}
    </Chip>
  )
}

const TYPE_BADGE: Record<string, string> = {
  script: 'bg-blue-50 text-blue-600 dark:bg-blue-950 dark:text-blue-400',
  binary: 'bg-amber-50 text-amber-600 dark:bg-amber-950 dark:text-amber-400',
}

function TaskCard({
  task,
  onRun,
  onDelete,
  onToggle,
  onOpen,
}: {
  task: TaskSummary
  onRun: (slug: string) => void
  onDelete: (slug: string) => void
  onToggle: (slug: string, enabled: boolean) => void
  onOpen: (slug: string) => void
}) {
  const [confirming, setConfirming] = useState(false)
  const t = task

  return (
    <div
      data-testid={`task-row-${t.slug}`}
      onClick={() => onOpen(t.slug)}
      className="group cursor-pointer rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm shadow-zinc-900/5 backdrop-blur transition-shadow hover:shadow-md dark:border-zinc-800 dark:bg-zinc-900/60 dark:hover:shadow-zinc-900/20"
    >
      {/* Header: identity + controls */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-semibold text-zinc-800 dark:text-zinc-100" title={t.slug}>
              {t.name}
            </span>
            <span
              className={`inline-flex shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                TYPE_BADGE[t.type] ?? 'bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300'
              }`}
            >
              {t.type}
            </span>
          </div>
          <div className="mt-0.5 truncate text-xs text-zinc-400 dark:text-zinc-500" title={t.slug}>
            {t.slug}
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2" onClick={(e) => e.stopPropagation()}>
          <button
            type="button"
            onClick={() => onRun(t.slug)}
            aria-label={`Run ${t.slug}`}
            className="inline-flex items-center gap-1 rounded-lg border border-zinc-200/80 bg-white/80 px-2.5 py-1 text-xs font-medium text-zinc-700 outline-none transition-colors hover:border-accent/50 hover:text-accent focus-visible:ring-2 focus-visible:ring-accent-ring dark:border-zinc-700/60 dark:bg-zinc-900/70 dark:text-zinc-200"
          >
            <svg className="size-3" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
              <path d="M8 5v14l11-7z" />
            </svg>
            Run
          </button>
          <Toggle
            checked={t.enabled}
            onChange={(next) => onToggle(t.slug, next)}
            label={`${t.enabled ? 'Disable' : 'Enable'} ${t.slug}`}
          />
          {confirming ? (
            <span className="inline-flex items-center gap-1.5">
              <button
                type="button"
                onClick={() => { setConfirming(false); onDelete(t.slug) }}
                className="rounded-md bg-red-600 px-2 py-1 text-xs font-medium text-white outline-none focus-visible:ring-2 focus-visible:ring-accent-ring"
              >
                Yes
              </button>
              <button
                type="button"
                onClick={() => setConfirming(false)}
                className="rounded-md px-2 py-1 text-xs text-zinc-500 outline-none hover:bg-zinc-200/70 dark:hover:bg-zinc-800"
              >
                No
              </button>
            </span>
          ) : (
            <button
              type="button"
              onClick={() => setConfirming(true)}
              className="rounded-lg p-1.5 text-zinc-400 outline-none hover:bg-red-100 hover:text-red-600 focus-visible:ring-2 focus-visible:ring-accent-ring dark:hover:bg-red-950/50 dark:hover:text-red-400"
              aria-label={`Delete task ${t.slug}`}
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
      <div className="mt-3 flex flex-wrap items-start gap-x-6 gap-y-3 border-t border-zinc-100 pt-3 dark:border-zinc-800/60">
        <DetailBlock label="Runtime">
          <span className="text-xs text-zinc-600 dark:text-zinc-300">
            {t.type} · {t.runtime}
          </span>
        </DetailBlock>

        <DetailBlock label="Last run">
          {t.last_status ? (
            <div className="flex items-center gap-2">
              <StatusChip status={t.last_status} />
              <DateTime iso={t.last_run_at} />
            </div>
          ) : (
            <span className="text-xs text-zinc-400 dark:text-zinc-500">Never</span>
          )}
        </DetailBlock>

        <DetailBlock label="Updated">
          <DateTime iso={t.updated_at} />
        </DetailBlock>
      </div>
    </div>
  )
}

export function TaskTable({
  tasks,
  onRun,
  onDelete,
  onToggle,
  onOpen,
}: {
  tasks: TaskSummary[]
  onRun: (slug: string) => void
  onDelete: (slug: string) => void
  onToggle: (slug: string, enabled: boolean) => void
  onOpen: (slug: string) => void
}) {
  return (
    <div className="grid gap-3">
      {tasks.map((t) => (
        <TaskCard
          key={t.slug}
          task={t}
          onRun={onRun}
          onDelete={onDelete}
          onToggle={onToggle}
          onOpen={onOpen}
        />
      ))}
    </div>
  )
}
