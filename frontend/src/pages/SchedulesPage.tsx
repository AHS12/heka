import {useState, useMemo} from 'react'
import {Modal, Toast} from '@heroui/react'
import {apiErrorDetails} from '../lib/api'
import {useSchedules, useCreateSchedule, useDeleteSchedule, useToggleSchedule, useReconcileSchedules} from '../lib/schedules'
import {useTasks} from '../lib/tasks'
import {ScheduleTable} from '../components/schedules/ScheduleTable'
import {ScheduleForm, emptyScheduleDraft, draftToCron, validateScheduleDraft} from '../components/schedules/ScheduleForm'
import {pillBtn, primaryBtn} from '../components/controls'
import {AppDialog, dialogBodyCls, dialogFooterCls, dialogHeaderCls} from '../components/AppDialog'

function toRFC3339(localDateTime: string): string {
  if (!localDateTime) return ''
  const date = new Date(localDateTime)
  return Number.isNaN(date.getTime()) ? localDateTime : date.toISOString()
}

export function SchedulesPage() {
  const schedules = useSchedules()
  const tasks = useTasks()
  const create = useCreateSchedule()
  const del = useDeleteSchedule()
  const toggle = useToggleSchedule()
  const reconcile = useReconcileSchedules()
  const [showForm, setShowForm] = useState(false)
  const [draft, setDraft] = useState(emptyScheduleDraft())
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
    const validationErrors = validateScheduleDraft(draft)
    if (validationErrors.length > 0) {
      setErrors(validationErrors)
      return
    }
    create.mutate(
      {
        slug: draft.slug.trim(),
        taskSlug: draft.taskSlug,
        kind: draft.kind,
        cron: draftToCron(draft),
        runAt: draft.kind === 'onetime' ? toRFC3339(draft.runAt) : '',
        missedPolicy: draft.missedPolicy,
      },
      {
        onSuccess: () => {
          setShowForm(false)
          setDraft(emptyScheduleDraft())
          setErrors([])
          Toast.toast.success('Schedule created')
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
            onClick={() =>
              reconcile.mutate(undefined, {
                onSuccess: () => {
                  Toast.toast.success('Reconcile started', {
                    description: 'Checking for missed recurring schedule activations',
                  })
                },
                onError: (err) => {
                  Toast.toast.danger('Reconcile failed', {
                    description: apiErrorDetails(err)[0] ?? 'Unknown error',
                  })
                },
              })
            }
            disabled={reconcile.isPending}
            className={pillBtn}
            title="Fire any missed recurring schedule runs (PC was off, sleep, etc.)"
            data-testid="schedules-reconcile"
          >
            {reconcile.isPending ? 'Reconciling…' : 'Reconcile now'}
          </button>
          <button
            type="button"
            onClick={() => setShowForm(true)}
            className={primaryBtn}
          >
            + New schedule
          </button>
        </div>
      </div>

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

      {showForm && (
        <AppDialog
          isOpen
          onOpenChange={(open) => {
            if (!open && !create.isPending) {
              setShowForm(false)
              setErrors([])
            }
          }}
          size="md"
        >
          <Modal.Header className={dialogHeaderCls}>
            <div>
              <Modal.Heading className="text-lg font-semibold">Create schedule</Modal.Heading>
              <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">Choose a task and tell Heka exactly when it should run.</p>
            </div>
            <Modal.CloseTrigger aria-label="Close create schedule dialog" isDisabled={create.isPending} />
          </Modal.Header>
          <Modal.Body className={dialogBodyCls}>
            <ScheduleForm
              draft={draft}
              onChange={(next) => {
                setDraft(next)
                setErrors([])
              }}
              tasks={tasks.data ?? []}
              errors={errors}
            />
          </Modal.Body>
          <Modal.Footer className={dialogFooterCls}>
            <button
              type="button"
              className={pillBtn}
              disabled={create.isPending}
              onClick={() => {
                setShowForm(false)
                setErrors([])
              }}
            >
              Cancel
            </button>
            <button type="button" className={primaryBtn} disabled={create.isPending} onClick={handleCreate}>
              {create.isPending ? 'Creating…' : 'Create schedule'}
            </button>
          </Modal.Footer>
        </AppDialog>
      )}
    </div>
  )
}
