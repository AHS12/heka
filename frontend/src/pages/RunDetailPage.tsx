import {useParams, Link} from 'react-router-dom'
import {useRunDetail} from '../lib/runs'
import {OutputViewer, AttemptList} from '../components/runs/OutputViewer'
import {StatusChip} from '../components/tasks/TaskTable'
import {useRuns} from '../lib/runs'

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

export function LogDetailPage() {
  const {runId} = useParams<{runId: string}>()
  const {data: run, isLoading} = useRunDetail(runId)

  // Fetch sibling attempts (same group_id)
  const {data: groupRuns} = useRuns({
    task: run?.task_slug,
    limit: 100,
  })

  const attempts = (groupRuns?.runs ?? []).filter(
    (r) => r.group_id === run?.group_id
  )

  if (isLoading) {
    return <p className="text-sm text-zinc-400">Loading log…</p>
  }

  if (!run) {
    return (
      <div className="text-sm text-zinc-500">
        Log not found.{' '}
        <Link to="/logs" className="text-accent hover:underline">Back to logs</Link>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link
          to="/logs"
          className="text-sm text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300"
        >
          ← Logs
        </Link>
        <h2 className="text-lg font-semibold">Log Detail</h2>
      </div>

      <div className="grid grid-cols-2 gap-4 rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm shadow-zinc-900/5 backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60 sm:grid-cols-3 lg:grid-cols-4">
        <MetaItem label="Task" value={run.task_slug} />
        <MetaItem label="Status">
          <StatusChip status={run.status} />
        </MetaItem>
        <MetaItem label="Trigger" value={run.trigger} />
        <MetaItem label="Group" value={run.group_id.slice(0, 12) + '…'} title={run.group_id} />
        <MetaItem label="Attempt" value={String(run.attempt + 1)} />
        <MetaItem label="Started" value={formatTime(run.started_at)} />
        <MetaItem label="Duration" value={formatDuration(run.duration_ms)} />
        <MetaItem
          label="Exit code"
          value={run.exit_code !== undefined && run.exit_code !== null ? String(run.exit_code) : '—'}
          className={
            run.exit_code === 0
              ? 'text-emerald-600 dark:text-emerald-400'
              : run.exit_code != null
                ? 'text-red-600 dark:text-red-400'
                : ''
          }
        />
        {run.schedule_id && (
          <MetaItem label="Schedule" value={run.schedule_id.slice(0, 12) + '…'} title={run.schedule_id} />
        )}
        {run.pid && (
          <MetaItem label="PID" value={String(run.pid)} />
        )}
      </div>

      <AttemptList runs={attempts} />
      <OutputViewer run={run} />
    </div>
  )
}

function MetaItem({
  label,
  value,
  children,
  className,
  title,
}: {
  label: string
  value?: string
  children?: React.ReactNode
  className?: string
  title?: string
}) {
  return (
    <div className="space-y-0.5">
      <div className="text-xs text-zinc-400 dark:text-zinc-500">{label}</div>
      <div
        title={title}
        className={`text-sm font-medium text-zinc-800 dark:text-zinc-100 ${className ?? ''}`}
      >
        {children ?? value}
      </div>
    </div>
  )
}
