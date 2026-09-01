// components/tasks/TaskTable.tsx (SPEC-13 §3) — the tasks list: name+slug,
// type, runtime, enabled toggle, last run status/time, Run + delete actions.
// Row click opens the editor; delete is a two-step inline confirm.
import {useState} from 'react'
import {Chip} from '@heroui/react'
import {Toggle} from '../controls'
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

function formatTime(iso?: string) {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
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
  const [confirming, setConfirming] = useState<string | null>(null)

  return (
    <div className="overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/70 shadow-sm shadow-zinc-900/5 backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-zinc-200/80 text-xs uppercase tracking-wide text-zinc-400 dark:border-zinc-800 dark:text-zinc-500">
            <th className="px-4 py-2.5 font-medium">Task</th>
            <th className="px-3 py-2.5 font-medium">Type</th>
            <th className="px-3 py-2.5 font-medium">Runtime</th>
            <th className="px-3 py-2.5 font-medium">Enabled</th>
            <th className="px-3 py-2.5 font-medium">Last run</th>
            <th className="px-3 py-2.5 text-right font-medium">Actions</th>
          </tr>
        </thead>
        <tbody>
          {tasks.map((t) => (
            <tr
              key={t.slug}
              data-testid={`task-row-${t.slug}`}
              className="cursor-pointer border-b border-zinc-100 last:border-0 hover:bg-zinc-100/60 dark:border-zinc-800/60 dark:hover:bg-zinc-800/40"
              onClick={() => onOpen(t.slug)}
            >
              <td className="px-4 py-2.5">
                <div className="font-medium text-zinc-800 dark:text-zinc-100">{t.name}</div>
                <div className="text-xs text-zinc-400 dark:text-zinc-500">{t.slug}</div>
              </td>
              <td className="px-3 py-2.5 text-zinc-600 dark:text-zinc-300">{t.type}</td>
              <td className="px-3 py-2.5 text-zinc-600 dark:text-zinc-300">{t.runtime}</td>
              <td className="px-3 py-2.5">
                <Toggle
                  checked={t.enabled}
                  onChange={(next) => onToggle(t.slug, next)}
                  label={`${t.enabled ? 'Disable' : 'Enable'} ${t.slug}`}
                />
              </td>
              <td className="px-3 py-2.5">
                {t.last_status ? (
                  <div className="flex items-center gap-2">
                    <StatusChip status={t.last_status} />
                    <span className="text-xs text-zinc-400 dark:text-zinc-500">
                      {formatTime(t.last_run_at)}
                    </span>
                  </div>
                ) : (
                  <span className="text-xs text-zinc-400 dark:text-zinc-500">Never</span>
                )}
              </td>
              <td className="px-3 py-2.5" onClick={(e) => e.stopPropagation()}>
                {confirming === t.slug ? (
                  <span className="flex items-center justify-end gap-2 text-xs">
                    <span className="text-zinc-400">Delete?</span>
                    <button
                      type="button"
                      onClick={() => {
                        setConfirming(null)
                        onDelete(t.slug)
                      }}
                      className="rounded-md bg-red-600 px-2 py-1 font-medium text-white outline-none focus-visible:ring-2 focus-visible:ring-accent-ring"
                    >
                      Yes
                    </button>
                    <button
                      type="button"
                      onClick={() => setConfirming(null)}
                      className="rounded-md px-2 py-1 text-zinc-500 outline-none hover:bg-zinc-200/70 focus-visible:ring-2 focus-visible:ring-accent-ring dark:hover:bg-zinc-800"
                    >
                      No
                    </button>
                  </span>
                ) : (
                  <span className="flex items-center justify-end gap-1">
                    <button
                      type="button"
                      aria-label={`Run ${t.slug}`}
                      title="Run now"
                      onClick={() => onRun(t.slug)}
                      className="rounded-lg p-1.5 text-zinc-500 outline-none hover:bg-zinc-200/70 hover:text-zinc-900 focus-visible:ring-2 focus-visible:ring-accent-ring dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
                    >
                      <svg className="size-4" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M8 5v14l11-7z" />
                      </svg>
                    </button>
                    <button
                      type="button"
                      aria-label={`Delete ${t.slug}`}
                      title="Delete task"
                      onClick={() => setConfirming(t.slug)}
                      className="rounded-lg p-1.5 text-zinc-400 outline-none hover:bg-red-100 hover:text-red-600 focus-visible:ring-2 focus-visible:ring-accent-ring dark:hover:bg-red-950/50 dark:hover:text-red-400"
                    >
                      <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <polyline points="3 6 5 6 21 6" />
                        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                      </svg>
                    </button>
                  </span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}