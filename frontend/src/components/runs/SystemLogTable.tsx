import type {SystemLogEntry} from '../../lib/api'

function formatTime(iso?: string) {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

const levelCls: Record<string, string> = {
  info: 'text-emerald-600 dark:text-emerald-400',
  warn: 'text-amber-600 dark:text-amber-400',
  error: 'text-red-600 dark:text-red-400',
}

/** The daemon's own event log (scheduler reconcile, lifecycle, wake detection). */
export function SystemLogTable({entries}: {entries: SystemLogEntry[]}) {
  return (
    <div className="overflow-hidden rounded-2xl border border-border/80 bg-surface/70 shadow-sm shadow-zinc-900/5 backdrop-blur-sm">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-border/80 text-xs uppercase tracking-wide text-foreground/50">
            <th className="px-4 py-2.5 font-medium">Time</th>
            <th className="px-3 py-2.5 font-medium">Level</th>
            <th className="px-3 py-2.5 font-medium">Event</th>
            <th className="px-3 py-2.5 font-medium">Message</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e) => (
            <tr
              key={e.id}
              data-testid={`system-log-row-${e.id}`}
              className="border-b border-border/60 last:border-0 hover:bg-surface-secondary/60"
            >
              <td className="whitespace-nowrap px-4 py-2.5 text-xs text-foreground/55">
                {formatTime(e.ts)}
              </td>
              <td className="px-3 py-2.5">
                <span
                  className={`text-xs font-semibold uppercase ${
                    levelCls[e.level] ?? 'text-foreground/55'
                  }`}
                >
                  {e.level}
                </span>
              </td>
              <td className="px-3 py-2.5 text-xs text-foreground/55">{e.event}</td>
              <td className="px-3 py-2.5 break-words">{e.message}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
