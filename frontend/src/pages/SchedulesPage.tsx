import {useEffect, useState, useMemo} from 'react'
import {useSearchParams} from 'react-router-dom'
import {Modal, Toast} from '@heroui/react'
import {apiErrorDetails} from '../lib/api'
import type {Schedule} from '../lib/api'
import {
  useSchedulesPage,
  schedulePageRows,
  useCreateSchedule,
  useUpdateSchedule,
  useDeleteSchedule,
  useToggleSchedule,
  useReconcileSchedules,
} from '../lib/schedules'
import {useTasks} from '../lib/tasks'
import {useDebounced, useSentinel} from '../lib/hooks'
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
  const debouncedQ = useDebounced(search)
  const [searchParams, setSearchParams] = useSearchParams()

  // Server-side search over slug + task_slug; changing it produces a new
  // query key so the scroll accumulation resets cleanly.
  const filters = useMemo(() => ({q: debouncedQ.trim() || undefined}), [debouncedQ])
  const schedules = useSchedulesPage(filters)
  const rows = schedulePageRows(schedules.data)
  const total = schedules.data?.pages[0]?.total ?? 0
  const searching = schedules.isFetching

  const sentinelRef = useSentinel(() => {
    if (schedules.hasNextPage && !schedules.isFetchingNextPage) void schedules.fetchNextPage()
  })

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
            {searching ? (
              <svg
                className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 animate-spin text-foreground/50"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2.5"
                strokeLinecap="round"
                data-testid="schedules-search-spinner"
              >
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
            ) : (
              <svg className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-foreground/50" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="11" cy="11" r="8" />
                <line x1="21" y1="21" x2="16.65" y2="16.65" />
              </svg>
            )}
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
      ) : rows.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-border px-4 py-10 text-center text-sm text-foreground/50">
          {total === 0 && !filters.q
            ? 'No schedules yet — create one to automate task runs.'
            : `No schedules match "${search.trim()}".`}
        </div>
      ) : (
        <>
          <ScheduleTable
            schedules={rows}
            onToggle={(id, enabled) => toggle.mutate({id, enabled})}
            onEdit={openEdit}
            onDelete={(id) => del.mutate(id)}
          />
          {/* Infinite scroll trigger: load the next keyset page on approach. */}
          <div ref={sentinelRef} aria-hidden className="h-px" />
          {schedules.isFetchingNextPage && (
            <p className="flex items-center justify-center gap-2 py-2 text-xs text-foreground/50">
              <svg className="size-3.5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
              Loading more schedules…
            </p>
          )}
          <p data-testid="schedules-footer" className="flex items-center justify-center gap-2 pb-1 text-[11px] text-foreground/45">
            {rows.length} shown · {total} total
            {searching && (
              <svg className="size-3 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
            )}
          </p>
        </>
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
