import {useMemo} from 'react'
import {describeCron, nextRuns, parseCron} from '../../lib/cron'
import {crossProductWarning, patternToCron} from '../../lib/schedulePattern'
import type {SchedulePattern} from '../../lib/schedulePattern'

function relativeLabel(target: Date, now: Date): string {
  const mins = Math.round((target.getTime() - now.getTime()) / 60_000)
  if (mins < 1) return 'in under a minute'
  if (mins < 60) return `in ${mins} min`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `in ${hours}h ${mins % 60}m`
  const days = Math.floor(hours / 24)
  return `in ${days}d ${hours % 24}h`
}

export function CronPreview({pattern}: {pattern: SchedulePattern}) {
  const cron = useMemo(() => (pattern.kind === 'cron' ? pattern.expr.trim() : patternToCron(pattern)), [pattern])

  const runs = useMemo(
    () => (cron && pattern.kind !== 'once' ? nextRuns(cron, new Date(), 5) : null),
    [cron, pattern.kind]
  )
  const parsed = useMemo(() => parseCron(cron), [cron])
  const description = cron ? describeCron(cron) : ''
  const warning =
    pattern.kind === 'daily' || pattern.kind === 'weekly' || pattern.kind === 'monthly'
      ? crossProductWarning(pattern.times)
      : null

  const copy = () => {
    if (cron && navigator.clipboard) {
      navigator.clipboard.writeText(cron).catch(() => {})
    }
  }

  return (
    <div className="rounded-xl border border-border/80 bg-surface-secondary/50 p-4" data-testid="cron-preview">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="min-w-0 text-sm font-medium text-foreground/85" data-testid="cron-preview-description">
          {description || <span className="text-foreground/40">Complete the pattern to see a preview.</span>}
        </p>
        {cron && (
          <span className="inline-flex items-center gap-1.5">
            <code className="rounded-md border border-field-border/70 bg-surface/80 px-2 py-0.5 font-mono text-xs text-foreground/75" data-testid="cron-preview-expression">
              {cron}
            </code>
            <button
              type="button"
              onClick={copy}
              aria-label="Copy cron expression"
              className="rounded-md p-1 text-foreground/45 outline-none transition-colors hover:bg-surface-secondary hover:text-accent focus-visible:ring-2 focus-visible:ring-accent-ring"
            >
              <svg className="size-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <rect x="9" y="9" width="13" height="13" rx="2" />
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
              </svg>
            </button>
          </span>
        )}
      </div>

      {warning && (
        <p className="mt-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-2.5 py-1.5 text-xs text-amber-700 dark:text-amber-400" data-testid="cron-preview-warning">
          {warning}
        </p>
      )}

      {pattern.kind === 'once' ? (
        <p className="mt-2 text-xs text-foreground/50">
          Runs a single time{pattern.runAt ? ' at the chosen moment' : ''}, then disables itself.
        </p>
      ) : cron && !parsed.ok ? (
        <p className="mt-2 text-xs text-red-600 dark:text-red-400">{parsed.error} — fix the expression to see upcoming runs.</p>
      ) : runs !== null ? (
        <div className="mt-3 border-t border-border/60 pt-2.5">
          <div className="mb-1.5 text-[10px] font-medium uppercase tracking-wider text-foreground/45">Next runs</div>
          {runs.length === 0 ? (
            <p className="text-xs text-foreground/50">This schedule never fires — no matching date exists.</p>
          ) : (
            <ol className="space-y-1" data-testid="cron-preview-runs">
              {runs.map((run, i) => (
                <li key={i} className="flex items-baseline gap-2 text-xs">
                  <span className="w-28 shrink-0 text-foreground/75">
                    {run.toLocaleDateString([], {weekday: 'short', month: 'short', day: 'numeric'})}{' '}
                    {run.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit', hour12: false})}
                  </span>
                  <span className="text-foreground/45">{relativeLabel(run, new Date())}</span>
                </li>
              ))}
            </ol>
          )}
        </div>
      ) : null}

      {parsed.ok && parsed.tz && (
        <p className="mt-2 text-xs text-foreground/50">
          The daemon will run this in the {parsed.tz} timezone; the preview stays local.
        </p>
      )}
      <p className="mt-2 text-[10px] text-foreground/35">Times shown in your local timezone, matching the daemon.</p>
    </div>
  )
}
