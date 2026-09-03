// pages/BackupHistoryPage.tsx — the full backup job history: filters,
// pagination, and per-destination outcomes for every archive job, newest
// first. Reached from Settings → Backup ("History").
import {useMemo, useState} from 'react'
import {useQueryClient} from '@tanstack/react-query'
import {apiErrorDetails, type BackupJob} from '../lib/api'
import {useBackupHistory, formatBytes, formatStamp, BACKUP_HISTORY_KEY} from '../lib/backup'
import {FormErrors, SelectField, pillBtn, primaryBtn} from '../components/controls'

const PAGE_SIZE = 25

const STATUS_OPTIONS = [
  {id: 'all', label: 'All statuses'},
  {id: 'success', label: 'Success'},
  {id: 'partial', label: 'Partial'},
  {id: 'failed', label: 'Failed'},
  {id: 'running', label: 'Running'},
]

export function BackupHistoryPage() {
  const qc = useQueryClient()
  const [limit, setLimit] = useState(PAGE_SIZE)
  const [statusFilter, setStatusFilter] = useState('all')

  const history = useBackupHistory(limit)

  const jobs = useMemo(() => history.data ?? [], [history.data])
  const filtered = useMemo(
    () => (statusFilter === 'all' ? jobs : jobs.filter((j) => j.status === statusFilter)),
    [jobs, statusFilter]
  )
  // The daemon returns the newest `limit` jobs; when a full page comes back
  // there may be more, so keep offering "Show more".
  const mayHaveMore = jobs.length >= limit

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Backup history</h2>
          <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
            Every archive job, newest first — manual runs and the automatic schedule.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <a
            href="#/settings?tab=backup"
            className="inline-flex items-center gap-1.5 rounded-full border border-zinc-200/80 bg-white/80 px-3 py-1 text-xs font-medium shadow-sm transition-colors hover:border-accent hover:text-accent dark:border-zinc-700/60 dark:bg-zinc-900/70"
          >
            Back to settings
          </a>
          <button
            type="button"
            className={pillBtn}
            onClick={() => void qc.invalidateQueries({queryKey: BACKUP_HISTORY_KEY})}
          >
            Refresh
          </button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <SelectField
          aria-label="Filter by status"
          className="w-44"
          value={statusFilter}
          onChange={setStatusFilter}
          items={STATUS_OPTIONS}
        />
        {history.data && (
          <span className="text-xs text-zinc-500 dark:text-zinc-400">
            {filtered.length} of {jobs.length} shown
          </span>
        )}
      </div>

      {history.isError && (
        <FormErrors errors={apiErrorDetails(history.error)} title="Backup history could not be loaded" />
      )}

      {history.isLoading ? (
        <p className="text-sm text-zinc-400">Loading history…</p>
      ) : filtered.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-zinc-300 px-4 py-10 text-center text-sm text-zinc-400 dark:border-zinc-700">
          {jobs.length === 0
            ? 'No backups yet — run one from Settings → Backup.'
            : 'No jobs match the current filter.'}
        </div>
      ) : (
        <ul className="space-y-2" data-testid="backup-history-list">
          {filtered.map((j) => (
            <JobRow key={j.id} job={j} />
          ))}
        </ul>
      )}

      {mayHaveMore && filtered.length === jobs.length && (
        <div className="text-center">
          <button
            type="button"
            className={history.isFetching ? pillBtn : primaryBtn}
            disabled={history.isFetching}
            onClick={() => setLimit((l) => l + PAGE_SIZE)}
          >
            {history.isFetching ? 'Loading…' : 'Show more'}
          </button>
        </div>
      )}
    </div>
  )
}

function JobRow({job}: {job: BackupJob}) {
  return (
    <li className="rounded-2xl border border-zinc-200/80 bg-white/70 px-4 py-3 text-sm shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="flex items-center gap-2.5">
          <JobStatus job={job} />
          <span className="font-medium text-zinc-800 dark:text-zinc-200">
            {formatStamp(job.finished_at || job.started_at)}
          </span>
          <span className="text-xs text-zinc-500 dark:text-zinc-400">
            {job.trigger === 'scheduled' ? 'automatic' : 'manual'}
            {job.size_bytes ? ` · ${formatBytes(job.size_bytes)}` : ''}
          </span>
        </span>
        <span className="text-[11px] text-zinc-400 dark:text-zinc-500">job {job.id}</span>
      </div>
      {job.local_path && (
        <p className="mt-1.5 truncate font-mono text-[11px] text-zinc-500 dark:text-zinc-400" title={job.local_path}>
          {job.local_path}
        </p>
      )}
      {job.error && <p className="mt-1.5 text-xs text-red-600 dark:text-red-400">{job.error}</p>}
      {!!job.destinations?.length && (
        <p className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-zinc-500 dark:text-zinc-400">
          {job.destinations.map((d) => (
            <span key={d.type}>
              {d.type}: {d.ok ? 'ok' : d.error || 'failed'}
            </span>
          ))}
        </p>
      )}
    </li>
  )
}

function JobStatus({job}: {job: BackupJob}) {
  const styles: Record<string, string> = {
    success: 'bg-emerald-100/70 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
    partial: 'bg-amber-100/70 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
    failed: 'bg-red-100/70 text-red-700 dark:bg-red-900/30 dark:text-red-400',
    running: 'bg-sky-100/70 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400',
  }
  const labels: Record<string, string> = {
    success: 'Success',
    partial: 'Partial',
    failed: 'Failed',
    running: 'Running',
  }
  return (
    <span className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold ${styles[job.status] ?? ''}`}>
      {labels[job.status] ?? job.status}
    </span>
  )
}
