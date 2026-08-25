import {useState} from 'react'
import type {Schedule} from '../../lib/api'
import {StatusChip} from '../tasks/TaskTable'
import {Toggle} from '../controls'

function formatTime(iso?: string) {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
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
  const [confirming, setConfirming] = useState<string | null>(null)

  return (
    <div className="overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/70 shadow-sm shadow-zinc-900/5 backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-zinc-200/80 text-xs uppercase tracking-wide text-zinc-400 dark:border-zinc-800 dark:text-zinc-500">
            <th className="px-4 py-2.5 font-medium">Slug</th>
            <th className="px-3 py-2.5 font-medium">Task</th>
            <th className="px-3 py-2.5 font-medium">Kind</th>
            <th className="px-3 py-2.5 font-medium">Schedule</th>
            <th className="px-3 py-2.5 font-medium">Enabled</th>
            <th className="px-3 py-2.5 font-medium">Next run</th>
            <th className="px-3 py-2.5 font-medium">Last</th>
            <th className="px-3 py-2.5 text-right font-medium">Actions</th>
          </tr>
        </thead>
        <tbody>
          {schedules.map((s) => (
            <tr
              key={s.id}
              data-testid={`schedule-row-${s.id}`}
              className="border-b border-zinc-100 last:border-0 hover:bg-zinc-100/60 dark:border-zinc-800/60 dark:hover:bg-zinc-800/40"
            >
              <td className="px-4 py-2.5 font-medium text-zinc-800 dark:text-zinc-100">
                {s.slug}
              </td>
              <td className="px-3 py-2.5 text-zinc-600 dark:text-zinc-300">
                {s.task_slug}
              </td>
              <td className="px-3 py-2.5">
                <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${
                  s.kind === 'recurring'
                    ? 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300'
                    : 'bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300'
                }`}>
                  {s.kind}
                </span>
              </td>
              <td className="px-3 py-2.5 font-mono text-xs text-zinc-600 dark:text-zinc-300">
                {s.kind === 'recurring' ? s.cron : formatTime(s.run_at)}
              </td>
              <td className="px-3 py-2.5">
                <Toggle
                  checked={s.enabled}
                  onChange={(next) => onToggle(s.id, next)}
                  label={`${s.enabled ? 'Disable' : 'Enable'} ${s.slug}`}
                />
              </td>
              <td className="px-3 py-2.5 text-xs text-zinc-500 dark:text-zinc-400">
                {formatTime(s.next_run_at) || '—'}
              </td>
              <td className="px-3 py-2.5">
                {s.last_status ? (
                  <div className="flex items-center gap-2">
                    <StatusChip status={s.last_status} />
                    <span className="text-xs text-zinc-400 dark:text-zinc-500">
                      {formatTime(s.last_run_at)}
                    </span>
                  </div>
                ) : (
                  <span className="text-xs text-zinc-400 dark:text-zinc-500">Never</span>
                )}
              </td>
              <td className="px-3 py-2.5" onClick={(e) => e.stopPropagation()}>
                {confirming === s.id ? (
                  <span className="flex items-center justify-end gap-2 text-xs">
                    <span className="text-zinc-400">Delete?</span>
                    <button
                      type="button"
                      onClick={() => {
                        setConfirming(null)
                        onDelete(s.id)
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
                  <button
                    type="button"
                    aria-label={`Delete schedule ${s.slug}`}
                    onClick={() => setConfirming(s.id)}
                    className="rounded-lg p-1.5 text-zinc-400 outline-none hover:bg-red-100 hover:text-red-600 focus-visible:ring-2 focus-visible:ring-accent-ring dark:hover:bg-red-950/50 dark:hover:text-red-400"
                  >
                    <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <polyline points="3 6 5 6 21 6" />
                      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                    </svg>
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
