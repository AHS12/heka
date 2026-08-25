import {useState} from 'react'
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
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">Schedules</h2>
        <button
          type="button"
          onClick={() => setShowForm(!showForm)}
          className={showForm ? pillBtn : primaryBtn}
        >
          {showForm ? 'Cancel' : '+ New Schedule'}
        </button>
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
      ) : (
        <ScheduleTable
          schedules={schedules.data ?? []}
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
