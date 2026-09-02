// SchedulerPausedBanner (SPEC-15 §2) — non-blocking notice while the scheduler
// is paused: cron ticks and missed-run reconciliation are silently skipped, so
// this is the only place the state is visible besides the tray checkbox.
import {useQueryClient} from '@tanstack/react-query'
import {useHealth} from '../lib/query'
import {resumeScheduler, apiErrorDetails} from '../lib/api'
import {Toast} from '@heroui/react'

export function SchedulerPausedBanner() {
  const qc = useQueryClient()
  const health = useHealth()
  if (health.data?.scheduler !== 'paused') {
    return null
  }

  const resume = () => {
    resumeScheduler()
      .then(() => qc.invalidateQueries({queryKey: ['health']}))
      .catch((err) => {
        Toast.toast.danger('Failed to resume scheduler', {
          description: apiErrorDetails(err)[0] ?? 'Unknown error',
        })
      })
  }

  return (
    <div
      role="status"
      data-testid="scheduler-paused-banner"
      className="mb-4 flex items-center justify-between gap-4 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200"
    >
      <span>
        Scheduler is paused — schedules won't run and missed runs won't
        reconcile until resumed.
      </span>
      <button
        type="button"
        onClick={resume}
        className="shrink-0 rounded-md bg-amber-600 px-3 py-1.5 font-medium text-white outline-none transition-colors hover:bg-amber-700 focus-visible:ring-2 focus-visible:ring-amber-300"
      >
        Resume
      </button>
    </div>
  )
}
