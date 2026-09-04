import {useEffect, useState, useMemo} from 'react'
import {useSearchParams} from 'react-router-dom'
import {Modal, Toast} from '@heroui/react'
import {apiErrorDetails} from '../lib/api'
import type {Schedule} from '../lib/api'
import {
  useSchedules,
  useCreateSchedule,
  useUpdateSchedule,
  useDeleteSchedule,
  useToggleSchedule,
  useReconcileSchedules,
} from '../lib/schedules'
import {useTasks} from '../lib/tasks'
import {ScheduleTable} from '../components/schedules/ScheduleTable'
import {
  ScheduleForm,
  emptyScheduleDraft,
  draftFromSchedule,
  draftToPayload,
  validateScheduleDraft,
} from '../components/schedules/ScheduleForm'
import type {ScheduleDraft} from '../components/schedules/ScheduleForm'
import {pillBtn, primaryBtn} from '../components/controls'
import {AppDialog, dialogBodyCls, dialogFooterCls, dialogHeaderCls} from '../components/AppDialog'

export function SchedulesPage() {
  const schedules = useSchedules()
  const tasks = useTasks()
  const create = useCreateSchedule()
  const update = useUpdateSchedule()
  const del = useDeleteSchedule()
  const toggle = useToggleSchedule()
  const reconcile = useReconcileSchedules()
  const [showForm, setShowForm] = useState(false)
  const [editing, setEditing] = useState<Schedule | null>(null)
  const [draft, setDraft] = useState<ScheduleDraft>(emptyScheduleDraft())
  const [errors, setErrors] = useState<string[]>([])
  const [search, setSearch] = useState('')
  const [searchParams, setSearchParams] = useSearchParams()

  // Deep-link from the dashboard quick action: /schedules?new=1 opens the
  // create form once, then the param is stripped so refresh stays clean.
  useEffect(() => {
    if (searchParams.get('new') === '1') {
      setShowForm(true)
      const next = new URLSearchParams(searchParams)
      next.delete('new')
      setSearchParams(next, {replace: true})
    }
  }, [searchParams, setSearchParams])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return schedules.data ?? []
    return (schedules.data ?? []).filter(
      (s) => s.slug.toLowerCase().includes(q) || s.task_slug.toLowerCase().includes(q)
    )
  }, [schedules.data, search])

  const closeForm = () => {
    setShowForm(false)
    setEditing(null)
    setErrors([])
  }

  const openEdit = (schedule: Schedule) => {
    setEditing(schedule)
    setDraft(draftFromSchedule(schedule))
    setErrors([])
    setShowForm(true)
  }

  const handleSave = () => {
    const validationErrors = validateScheduleDraft(draft)
    if (validationErrors.length > 0) {
      setErrors(validationErrors)
      return
    }
    const payload = draftToPayload(draft)
    const onSuccess = () => {
      Toast.toast.success(editing ? 'Schedule updated' : 'Schedule created')
      closeForm()
      setDraft(emptyScheduleDraft())
    }
    const onError = (err: unknown) => setErrors(apiErrorDetails(err))
    if (editing) {
      update.mutate({id: editing.id, ...payload}, {onSuccess, onError})
    } else {
      create.mutate(payload, {onSuccess, onError})
    }
  }

  const saving = create.isPending || update.isPending

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">Schedules</h2>
        <div className="flex items-center gap-2">
          <div className="relative">
            <svg className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-foreground/50" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="11" cy="11" r="8" />
              <line x1="21" y1="21" x2="16.65" y2="16.65" />
            </svg>
            <input
              type="text"
              placeholder="Search schedules…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-44 rounded-full border border-field-border bg-surface/80 py-1.5 pl-8 pr-3 text-sm text-foreground/75 outline-none transition-colors placeholder:text-foreground/40 focus:border-accent focus:ring-1 focus:ring-accent-ring"
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
            onClick={() => {
              setEditing(null)
              setDraft(emptyScheduleDraft())
              setErrors([])
              setShowForm(true)
            }}
            className={primaryBtn}
          >
            + New schedule
          </button>
        </div>
      </div>

      {schedules.isLoading ? (
        <p className="text-sm text-foreground/50">Loading schedules…</p>
      ) : (schedules.data ?? []).length === 0 ? (
        <div className="rounded-2xl border border-dashed border-border px-4 py-10 text-center text-sm text-foreground/50">
          No schedules yet — create one to automate task runs.
        </div>
      ) : filtered.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-border px-4 py-10 text-center text-sm text-foreground/50">
          No schedules match "{search}".
        </div>
      ) : (
        <ScheduleTable
          schedules={filtered}
          onToggle={(id, enabled) => toggle.mutate({id, enabled})}
          onEdit={openEdit}
          onDelete={(id) => del.mutate(id)}
        />
      )}

      {showForm && (
        <AppDialog
          isOpen
          onOpenChange={(open) => {
            if (!open && !saving) closeForm()
          }}
          size="lg"
          dialogClassName="max-w-2xl"
        >
          <Modal.Header className={dialogHeaderCls}>
            <div>
              <Modal.Heading className="text-lg font-semibold">
                {editing ? 'Edit schedule' : 'Create schedule'}
              </Modal.Heading>
              <p className="mt-1 text-xs text-foreground/55">Choose a task and tell Heka exactly when it should run.</p>
            </div>
            <Modal.CloseTrigger aria-label="Close schedule dialog" isDisabled={saving} />
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
              errorTitle={editing ? 'Schedule could not be updated' : 'Schedule could not be created'}
            />
          </Modal.Body>
          <Modal.Footer className={dialogFooterCls}>
            <button type="button" className={pillBtn} disabled={saving} onClick={closeForm}>
              Cancel
            </button>
            <button type="button" className={primaryBtn} disabled={saving} onClick={handleSave} data-testid="save-schedule">
              {saving ? 'Saving…' : editing ? 'Save changes' : 'Create schedule'}
            </button>
          </Modal.Footer>
        </AppDialog>
      )}
    </div>
  )
}
