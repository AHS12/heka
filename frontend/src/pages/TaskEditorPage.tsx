// pages/TaskEditorPage.tsx (SPEC-13 §4) — /tasks/new and /tasks/:slug.
// One canonical draft; the Form | YAML tabs are two *views* of it. YAML edits
// are preserved byte-for-byte on failure and nothing is saved. Rename on an
// existing task saves as copy + delete.
import {useEffect, useRef, useState} from 'react'
import {Link, useNavigate, useParams} from 'react-router-dom'
import {apiErrorDetails} from '../lib/api'
import {
  useCreateTask,
  useDeleteTask,
  useExportTask,
  useTask,
  useTaskYAML,
  useUpdateTask,
} from '../lib/tasks'
import {
  draftFromTask,
  draftToYAML,
  emptyDraft,
  renamePlan,
  validateTaskDraft,
  type TaskDraft,
} from '../lib/taskForm'
import {TaskForm} from '../components/tasks/TaskForm'
import {YamlEditor} from '../components/tasks/YamlEditor'
import {Modal, Tabs} from '@heroui/react'
import {pillBtn, primaryBtn} from '../components/controls'
import {AppDialog, dialogBodyCls, dialogFooterCls, dialogHeaderCls} from '../components/AppDialog'

type Tab = 'form' | 'yaml'

export function TaskEditorPage({
  dialog = false,
  slug: slugProp,
  onClose,
  onSaved,
}: {
  dialog?: boolean
  /** Explicit task to edit — the dialog mode has no route params. */
  slug?: string
  onClose?: () => void
  /** Fires with the saved task's slug after a successful create or update in dialog mode. */
  onSaved?: (slug: string) => void
} = {}) {
  const {slug} = useParams()
  const navigate = useNavigate()
  const editingSlug = slugProp ?? (!slug || slug === 'new' ? undefined : slug)
  const isNew = !editingSlug

  const taskQuery = useTask(editingSlug)
  const yamlQuery = useTaskYAML(editingSlug)
  const create = useCreateTask()
  const update = useUpdateTask()
  const del = useDeleteTask()
  const exporter = useExportTask()

  const [draft, setDraft] = useState<TaskDraft | null>(() =>
    isNew ? emptyDraft() : null
  )
  const [tab, setTab] = useState<Tab>('form')
  const [yamlText, setYamlText] = useState<string | null>(null)
  const yamlInitialized = useRef(false)
  // staleYaml: form edits since the YAML view was built — opening the YAML
  // tab regenerates it from the draft so the two views never drift.
  const [staleYaml, setStaleYaml] = useState(false)
  const [yamlErrors, setYamlErrors] = useState<string[]>([])
  const [saveErrors, setSaveErrors] = useState<string[]>([])
  const [switchNotice, setSwitchNotice] = useState<string[] | null>(null)

  // Hydrate the canonical draft + YAML text once the queries land.
  useEffect(() => {
    if (taskQuery.data && !draft) {
      setDraft(draftFromTask(taskQuery.data.task))
    }
  }, [taskQuery.data, draft])

  useEffect(() => {
    if (yamlQuery.data && !yamlInitialized.current) {
      yamlInitialized.current = true
      setYamlText(yamlQuery.data)
    }
  }, [yamlQuery.data])

  const switchToYAML = () => {
    if (!draft) return
    // The YAML view mirrors the current draft; regeneration only happens when
    // the form changed since the last view was built (never clobbering text
    // the user is hand-editing on the YAML tab).
    if (yamlText === null || staleYaml) {
      setYamlText(draftToYAML(draft))
      setStaleYaml(false)
    }
    setSaveErrors([])
    setSwitchNotice(null)
    setTab('yaml')
  }

  // YAML → Visual: switching is never blocked by validation — that belongs at
  // Save. When the YAML parses it syncs the form draft; when it does not, the
  // form keeps the last valid draft and a warning explains why.
  const switchToForm = async () => {
    if (yamlText === null || !draft) {
      setTab('form')
      return
    }
    const {parseTaskYAML} = await import('../lib/api')
    try {
      const dto = await parseTaskYAML(yamlText)
      setDraft(draftFromTask(dto.task))
      setYamlErrors([])
      setSwitchNotice(null)
    } catch (err) {
      // Last valid draft stays; the YAML tab's text is untouched.
      setSwitchNotice(apiErrorDetails(err))
    }
    setStaleYaml(false)
    setTab('form')
  }

  const submit = async () => {
    if (!draft) return
    setSaveErrors([])
    if (tab === 'form') {
      const localErrors = validateTaskDraft(draft)
      if (localErrors.length > 0) {
        setSaveErrors(localErrors)
        return
      }
    }
    const text = tab === 'yaml' ? (yamlText ?? '') : draftToYAML(draft)

    // Parse through the daemon (validation included) so the saved document is
    // canonical. On failure NOTHING is sent and any YAML stays exactly as
    // typed (SPEC-13 §4 preservation rule).
    const {parseTaskYAML} = await import('../lib/api')
    let dto: Awaited<ReturnType<typeof parseTaskYAML>>
    try {
      dto = await parseTaskYAML(text)
    } catch (err) {
      const details = apiErrorDetails(err)
      if (tab === 'yaml') {
        setYamlErrors(details)
      } else {
        setSaveErrors(details)
      }
      return
    }
    setDraft(draftFromTask(dto.task))
    setYamlErrors([])
    setSwitchNotice(null)

    const plan = renamePlan(editingSlug, draftFromTask(dto.task))
    try {
      if (isNew || plan.isRenaming) {
        // New task, or rename = copy (new slug) + remove (old slug).
        await create.mutateAsync(text)
        if (plan.isRenaming && editingSlug) {
          await del.mutateAsync(editingSlug)
        }
      } else {
        await update.mutateAsync({slug: editingSlug as string, yaml: text})
      }
      if (dialog) {
        onSaved?.(dto.task.slug)
        onClose?.()
      } else {
        navigate(`/tasks/${dto.task.slug}`, {replace: true})
      }
      setYamlText(text)
      setStaleYaml(false)
    } catch (err) {
      const details = apiErrorDetails(err)
      if (tab === 'yaml') {
        setYamlErrors(details)
      } else {
        setSaveErrors(details)
      }
    }
  }

  if (taskQuery.isLoading || (!isNew && !draft)) {
    if (!dialog) {
      if (taskQuery.isLoading) {
        return <p className="text-sm text-zinc-400">Loading…</p>
      }
      return (
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Task not found</h2>
          <p className="text-sm text-zinc-500 dark:text-zinc-400">
            <span data-testid="task-missing">{slug}</span> does not exist.
          </p>
          <Link to="/tasks" className={pillBtn}>
            Back to Tasks
          </Link>
        </div>
      )
    }
    return (
      <AppDialog isOpen onOpenChange={(open) => { if (!open) onClose?.() }}>
        <Modal.Header className={dialogHeaderCls}>
          <Modal.Heading className="text-lg font-semibold">
            {isNew ? 'Create task' : 'Edit task'}
          </Modal.Heading>
          <Modal.CloseTrigger aria-label="Close task dialog" />
        </Modal.Header>
        <Modal.Body className={dialogBodyCls}>
          {taskQuery.data || taskQuery.isLoading ? (
            <p className="text-sm text-zinc-400">Loading…</p>
          ) : (
            <p className="text-sm text-zinc-500 dark:text-zinc-400">
              <span data-testid="task-missing">{editingSlug}</span> does not exist. It may have
              been deleted from the tasks directory.
            </p>
          )}
        </Modal.Body>
        <Modal.Footer className={dialogFooterCls}>
          <button type="button" className={pillBtn} onClick={onClose}>
            Close
          </button>
        </Modal.Footer>
      </AppDialog>
    )
  }

  if (!draft) {
    return <p className="text-sm text-zinc-400">Loading…</p>
  }

  const renaming = renamePlan(editingSlug, draft).isRenaming
  const pending = create.isPending || update.isPending
  const editor = (
    <div className="space-y-4">
      {switchNotice && (
        <div
          data-testid="tab-switch-notice"
          className="space-y-1 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-800 dark:bg-amber-950/60 dark:text-amber-200"
        >
          <p className="font-medium">
            This form shows the last valid task. Your YAML is untouched and will be checked again when you save.
          </p>
          <ul className="list-inside list-disc">
            {switchNotice.map((err) => <li key={err}>{err}</li>)}
          </ul>
        </div>
      )}
      <Tabs
        selectedKey={tab}
        onSelectionChange={(key) => {
          const next = key as Tab
          if (next === 'yaml') switchToYAML()
          else switchToForm()
        }}
      >
        <Tabs.ListContainer>
          <Tabs.List aria-label="Editor view">
            <Tabs.Tab id="form">Visual<Tabs.Indicator /></Tabs.Tab>
            <Tabs.Tab id="yaml">YAML<Tabs.Indicator /></Tabs.Tab>
          </Tabs.List>
        </Tabs.ListContainer>
      </Tabs>
      {tab === 'form' ? (
        <TaskForm
          draft={draft}
          onChange={(patch) => {
            setStaleYaml(true)
            setSaveErrors([])
            setDraft({...draft, ...patch} as TaskDraft)
          }}
          errors={saveErrors}
          isNew={isNew}
          renaming={renaming}
        />
      ) : (
        <>
          <YamlEditor
            value={yamlText ?? ''}
            onChange={(text) => {
              setYamlText(text)
              setStaleYaml(false)
              setYamlErrors([])
              setSaveErrors([])
            }}
            errors={yamlErrors}
          />
          {yamlErrors.length > 0 && (
            <p className="text-xs text-red-600 dark:text-red-400">
              Fix the YAML above. Nothing was saved and your text remains exactly as typed.
            </p>
          )}
        </>
      )}
    </div>
  )

  if (dialog) {
    return (
      <AppDialog
        isOpen
        onOpenChange={(open) => { if (!open && !pending) onClose?.() }}
        dialogClassName="sm:max-w-3xl!"
      >
        <Modal.Header className={dialogHeaderCls}>
          <div>
            <Modal.Heading className="text-lg font-semibold">
              {isNew ? 'Create task' : 'Edit task'}
            </Modal.Heading>
            <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
              {isNew
                ? 'Define what runs, how it runs, and how Heka reports the result.'
                : `Editing “${draft.name || editingSlug}” — changes apply to future runs.`}
            </p>
          </div>
          <Modal.CloseTrigger
            aria-label={isNew ? 'Close create task dialog' : 'Close edit task dialog'}
            isDisabled={pending}
          />
        </Modal.Header>
        <Modal.Body className={dialogBodyCls}>{editor}</Modal.Body>
        <Modal.Footer className={dialogFooterCls}>
          <button type="button" className={pillBtn} onClick={onClose} disabled={pending}>Cancel</button>
          {!isNew && (
            <button
              type="button"
              className={pillBtn}
              disabled={pending}
              onClick={() =>
                exporter.mutate(editingSlug as string, {
                  onError: (err) => setSaveErrors(apiErrorDetails(err)),
                })
              }
            >
              Export
            </button>
          )}
          <button type="button" onClick={() => void submit()} className={primaryBtn} disabled={pending}>
            {pending
              ? isNew ? 'Creating…' : 'Saving…'
              : isNew ? 'Create task' : 'Save changes'}
          </button>
        </Modal.Footer>
      </AppDialog>
    )
  }

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Link to="/tasks" aria-label="Back to tasks" className={pillBtn}>Back</Link>
          <h2 className="text-lg font-semibold">{isNew ? 'New Task' : draft.name || draft.slug}</h2>
        </div>
        <div className="flex items-center gap-2">
          {!isNew && (
            <button
              type="button"
              className={pillBtn}
              onClick={() => exporter.mutate(editingSlug as string, {onError: (err) => setSaveErrors(apiErrorDetails(err))})}
            >
              Export
            </button>
          )}
          <button type="button" onClick={() => void submit()} className={primaryBtn} disabled={pending}>Save</button>
        </div>
      </div>
      {editor}
    </div>
  )
}