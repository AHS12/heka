import type {Run} from '../../lib/api'
import {StatusChip} from '../tasks/TaskTable'
import {Link} from 'react-router-dom'

function formatDuration(ms?: number) {
  if (!ms) return '—'
  if (ms < 1000) return `${ms}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  return `${m}m ${Math.round(s % 60)}s`
}

function formatTime(iso?: string) {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

export function RunsTable({
  runs,
  linkPrefix,
  highlight,
}: {
  runs: Run[]
  linkPrefix: string
  highlight?: string
}) {
  const highlightSnippet = (text: string) => {
    if (!highlight || !text) return text
    const idx = text.toLowerCase().indexOf(highlight.toLowerCase())
    if (idx === -1) return text.slice(0, 120)
    const start = Math.max(0, idx - 30)
    const snippet = text.slice(start, start + 120)
    const matchStart = idx - start
    const matchEnd = matchStart + highlight.length
    return (
      <>
        {start > 0 && '…'}
        {snippet.slice(0, matchStart)}
        <mark className="bg-yellow-200 text-zinc-900 dark:bg-yellow-800 dark:text-yellow-100">
          {snippet.slice(matchStart, matchEnd)}
        </mark>
        {snippet.slice(matchEnd)}
        {start + 120 < text.length && '…'}
      </>
    )
  }

  return (
    <div className="overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/70 shadow-sm shadow-zinc-900/5 backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-zinc-200/80 text-xs uppercase tracking-wide text-zinc-400 dark:border-zinc-800 dark:text-zinc-500">
            <th className="px-4 py-2.5 font-medium">Task</th>
            <th className="px-3 py-2.5 font-medium">Status</th>
            <th className="px-3 py-2.5 font-medium">Trigger</th>
            <th className="px-3 py-2.5 font-medium">Started</th>
            <th className="px-3 py-2.5 font-medium">Duration</th>
            <th className="px-3 py-2.5 font-medium">Exit</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((r) => (
            <tr
              key={r.run_id}
              data-testid={`run-row-${r.run_id}`}
              className="border-b border-zinc-100 last:border-0 hover:bg-zinc-100/60 dark:border-zinc-800/60 dark:hover:bg-zinc-800/40"
            >
              <td className="px-4 py-2.5">
                <Link
                  to={`${linkPrefix}/${r.run_id}`}
                  className="font-medium text-accent hover:underline dark:text-accent"
                >
                  {r.task_slug}
                </Link>
                {highlight && r.stdout && (
                  <div className="mt-0.5 max-w-xs truncate text-xs text-zinc-400 dark:text-zinc-500">
                    {highlightSnippet(r.stdout)}
                  </div>
                )}
              </td>
              <td className="px-3 py-2.5">
                <StatusChip status={r.status} />
              </td>
              <td className="px-3 py-2.5 text-xs text-zinc-500 dark:text-zinc-400">
                {r.trigger}
              </td>
              <td className="px-3 py-2.5 text-xs text-zinc-500 dark:text-zinc-400">
                {formatTime(r.started_at)}
              </td>
              <td className="px-3 py-2.5 text-xs text-zinc-500 dark:text-zinc-400">
                {formatDuration(r.duration_ms)}
              </td>
              <td className="px-3 py-2.5">
                {r.exit_code !== undefined && r.exit_code !== null ? (
                  <span className={`font-mono text-xs ${
                    r.exit_code === 0
                      ? 'text-emerald-600 dark:text-emerald-400'
                      : 'text-red-600 dark:text-red-400'
                  }`}>
                    {r.exit_code}
                  </span>
                ) : (
                  <span className="text-xs text-zinc-400">—</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
