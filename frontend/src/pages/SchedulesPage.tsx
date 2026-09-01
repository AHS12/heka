import {useState, useMemo} from 'react'
import {apiErrorDetails} from '../lib/api'
import {useSchedules, useCreateSchedule, useDeleteSchedule, useToggleSchedule} from '../lib/schedules'
import {useTasks} from '../lib/tasks'
import {ScheduleTable} from '../components/schedules/ScheduleTable'
import {ScheduleForm, emptyScheduleDraft, draftToCron} from '../components/schedules/ScheduleForm'
import {pillBtn, primaryBtn} from '../components/controls'

export function SchedulesPage() {
  const schedules = useSchedules()
  const tasks = useTasks()
  const create = useCreateSchedule()
  const del = useDeleteSchedule()
  const toggle = useToggleSchedule()
  const [showForm, setShowForm] = useState(false)
  const [draft, setDraft] = useState(emptyScheduleDraft())
  const [toast, setToast] = useState<string | null>(null)
  const [errors, setErrors] = useState<string[]>([])
  const [search, setSearch] = useState('')

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return schedules.data ?? []
    return (schedules.data ?? []).filter(
      (s) => s.slug.toLowerCase().includes(q) || s.task_slug.toLowerCase().includes(q)
    )
  }, [schedules.data, search])

  const handleCreate = () => {
    create.mutate(
      {
        slug: draft.slug,
        taskSlug: draft.taskSlug,
        kind: draft.kind,
        cron: draftToCron(draft),
        runAt: draft.kind === 'onetime' ? draft.runAt : '',
        missedPolicy: draft.missedPolicy,
      },
      {
        onSuccess: () => {
          setShowForm(false)
          setDraft(emptyScheduleDraft())
          setErrors([])
          setToast('Schedule created')
          setTimeout(() => setToast(null), 4000)
        },
        onError: (err) => {
          setErrors(apiErrorDetails(err))
        },
      }
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">Schedules</h2>
        <div className="flex items-center gap-2">
          <div className="relative">
            <svg className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-zinc-400 dark:text-zinc-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="11" cy="11" r="8" />
              <line x1="21" y1="21" x2="16.65" y2="16.65" />
            </svg>
            <input
              type="text"
              placeholder="Search schedules…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-44 rounded-full border border-zinc-200/80 bg-white/80 py-1.5 pl-8 pr-3 text-sm text-zinc-700 outline-none transition-colors placeholder:text-zinc-400 focus:border-accent focus:ring-1 focus:ring-accent-ring dark:border-zinc-700/60 dark:bg-zinc-900/70 dark:text-zinc-200 dark:placeholder:text-zinc-500"
            />
          </div>
          <button
            type="button"
            onClick={() => setShowForm(!showForm)}
            className={showForm ? pillBtn : primaryBtn}
          >
            {showForm ? 'Cancel' : '+ New Schedule'}
          </button>
        </div>
      </div>

      {errors.length > 0 && (
        <ul
          role="alert"
          className="space-y-1 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900 dark:bg-red-950/60 dark:text-red-300"
        >
          {errors.map((e) => (
            <li key={e}>• {e}</li>
          ))}
        </ul>
      )}

      {showForm && (
        <ScheduleForm
          draft={draft}
          onChange={setDraft}
          tasks={tasks.data ?? []}
          onSave={handleCreate}
          onCancel={() => {
            setShowForm(false)
            setErrors([])
          }}
          isPending={create.isPending}
        />
      )}

      {schedules.isLoading ? (
        <p className="text-sm text-zinc-400">Loading schedules…</p>
      ) : (schedules.data ?? []).length === 0 ? (
        <div className="rounded-2xl border border-dashed border-zinc-300 px-4 py-10 text-center text-sm text-zinc-400 dark:border-zinc-700">
          No schedules yet — create one to automate task runs.
        </div>
      ) : filtered.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-zinc-300 px-4 py-10 text-center text-sm text-zinc-400 dark:border-zinc-700">
          No schedules match "{search}".
        </div>
      ) : (
        <ScheduleTable
          schedules={filtered}
          onToggle={(id, enabled) => toggle.mutate({id, enabled})}
          onDelete={(id) => del.mutate(id)}
        />
      )}

      {toast && (
        <div
          role="status"
          data-testid="toast"
          className="fixed bottom-5 right-5 rounded-xl border border-zinc-200 bg-white/90 px-4 py-2.5 text-sm shadow-lg backdrop-blur dark:border-zinc-700 dark:bg-zinc-900/90"
        >
          {toast}
        </div>
      )}
    </div>
  )
}
