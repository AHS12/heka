// DaemonDownBanner (SPEC-12 §3) — non-blocking banner + Start Daemon when
// the daemon is down; the button flips to "starting" while the spawn runs and
// re-polls on completion.
import {useDaemonMode} from '../lib/query'

export function DaemonDownBanner() {
  const {mode, start} = useDaemonMode()
  if (mode === 'running') {
    return null
  }
  const starting = mode === 'starting'
  return (
    <div
      role="alert"
      data-testid="daemon-banner"
      className="mb-4 flex items-center justify-between gap-4 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200"
    >
      <span>
        Heka daemon is {starting ? 'starting' : 'not running'}. Tasks and
        schedules are unavailable until it is up.
      </span>
      <button
        type="button"
        onClick={() => void start()}
        disabled={starting}
        className="shrink-0 rounded-md bg-amber-600 px-3 py-1.5 font-medium text-white outline-none transition-colors hover:bg-amber-700 focus-visible:ring-2 focus-visible:ring-amber-300 disabled:opacity-60"
      >
        {starting ? 'Starting…' : 'Start Daemon'}
      </button>
    </div>
  )
}