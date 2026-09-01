// components/tasks/TaskForm.tsx (SPEC-13 §4) — the Form tab: two views of one
// canonical draft (props in, patch-out). Sections: Basics, Script/Binary,
// Execution, Retry, Environment, Notifications.
import {useState} from 'react'
import {Field, FormErrors, NumberInput, SelectField, TextInput, Toggle, pillBtn} from '../controls'
import {EnvEditor} from './EnvEditor'
import {WebhookEditor} from './WebhookEditor'
import {pickScriptFile, pickWorkingDir} from '../../lib/api'
import type {TaskDraft, TaskType} from '../../lib/taskForm'
import {RUNTIMES, argsFromString, slugify} from '../../lib/taskForm'

const NOTIFY_EVENTS = ['success', 'failure', 'timeout'] as const

// Browse wraps a path picker into the row layout used by the script and
// working-directory inputs. Canceling the dialog is silently ignored.
function Browse({
  pick,
  onPick,
}: {
  pick: () => Promise<string>
  onPick: (path: string) => void
}) {
  const [busy, setBusy] = useState(false)
  return (
    <button
      type="button"
      disabled={busy}
      onClick={() => {
        setBusy(true)
        pick()
          .catch(() => null)
          .then((path) => {
            if (path) onPick(path)
          })
          .finally(() => setBusy(false))
      }}
      className={`${pillBtn} text-xs`}
    >
      Browse…
    </button>
  )
}

export function TaskForm({
  draft,
  onChange,
  errors,
  isNew,
  renaming,
}: {
  draft: TaskDraft
  onChange: (patch: Partial<TaskDraft>) => void
  errors: string[]
  isNew: boolean
  renaming: boolean
}) {
  const patch = (p: Partial<TaskDraft>) => onChange(p)
  const type = draft.type
  const setType = (t: TaskType) => patch({type: t})
  const fieldError = (...fields: string[]) => {
    const match = errors.find((error) => fields.some((field) => error.toLowerCase().startsWith(`${field.toLowerCase()}:`)))
    return match?.replace(/^[^:]+:\s*/, '')
  }

  // Slug auto-generation (user can take over by typing in the field): the
  // slug follows the name while it's untouched and still matches what the
  // name-generated slug was — a hand-typed slug always stays put, including
  // across tab switches where this component remounts.
  const onNameChange = (value: string) => {
    const previousName = draft.name
    const previousSlug = draft.slug
    const next: Partial<TaskDraft> = {name: value}
    if (previousSlug === '' || previousSlug === slugify(previousName)) {
      next.slug = slugify(value)
    }
    onChange(next)
  }

  return (
    <div className="space-y-6">
      <FormErrors errors={errors} testId="form-errors" />

      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Basics</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Name" error={fieldError('name')} errorId="task-name-error">
            <TextInput
              aria-label="Task name"
              aria-invalid={Boolean(fieldError('name'))}
              aria-describedby={fieldError('name') ? 'task-name-error' : undefined}
              value={draft.name}
              onChange={(e) => onNameChange(e.target.value)}
              placeholder="Nightly backup"
            />
          </Field>
          <Field
            label="Slug"
            hint="Auto-generated from the name. You can override it."
            error={fieldError('slug')}
            errorId="task-slug-error"
          >
            <TextInput
              aria-label="Task slug"
              aria-invalid={Boolean(fieldError('slug'))}
              aria-describedby={fieldError('slug') ? 'task-slug-error' : undefined}
              value={draft.slug}
              onChange={(e) => patch({slug: e.target.value})}
              placeholder="nightly-backup"
            />
          </Field>
        </div>
        {renaming && !isNew && (
          <p
            data-testid="rename-notice"
            className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-950/60 dark:text-amber-300"
          >
            Slug changed — saving will create a copy and remove the original task.
          </p>
        )}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Type">
            <SelectField
              aria-label="Task type"
              value={type}
              onChange={(v) => setType(v as TaskType)}
              items={[
                {id: 'script', label: 'script'},
                {id: 'binary', label: 'binary'},
              ]}
            />
          </Field>
          <Field
            label="Runtime"
            hint={
              type === 'binary'
                ? 'Binaries run directly. No interpreter is needed.'
                : 'Need another runtime? Use the YAML tab.'
            }
            error={type === 'script' ? fieldError('runtime') : undefined}
            errorId="task-runtime-error"
          >
            <SelectField
              aria-label="Runtime"
              aria-describedby={fieldError('runtime') ? 'task-runtime-error' : undefined}
              value={draft.runtime}
              placeholder="Choose runtime…"
              isDisabled={type === 'binary'}
              isInvalid={type === 'script' && Boolean(fieldError('runtime'))}
              onChange={(v) => patch({runtime: v})}
              items={RUNTIMES.map((r) => ({id: r, label: r}))}
            />
          </Field>
        </div>
      </section>

      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
          {type === 'binary' ? 'Binary' : 'Script'}
        </h3>
        <Field
          label={type === 'binary' ? 'Command' : 'Script path'}
          error={fieldError(type === 'binary' ? 'command' : 'script')}
          errorId="task-executable-error"
        >
          <div className="flex gap-2">
            <TextInput
              aria-label={type === 'binary' ? 'Command' : 'Script path'}
              aria-invalid={Boolean(fieldError(type === 'binary' ? 'command' : 'script'))}
              aria-describedby={fieldError(type === 'binary' ? 'command' : 'script') ? 'task-executable-error' : undefined}
              value={type === 'binary' ? draft.command : draft.script}
              onChange={(e) =>
                patch(
                  type === 'binary'
                    ? {command: e.target.value}
                    : {script: e.target.value}
                )
              }
              placeholder={type === 'binary' ? './tool --flag' : 'backup.sh'}
            />
            {type === 'script' && (
              <Browse pick={pickScriptFile} onPick={(p) => patch({script: p})} />
            )}
          </div>
        </Field>
        <Field label="Args (space-separated)">
          <TextInput
            aria-label="Arguments"
            value={draft.args.join(' ')}
            onChange={(e) => patch({args: argsFromString(e.target.value)})}
            placeholder="--verbose --out=./tmp"
          />
        </Field>
      </section>

      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Execution</h3>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Field label="Timeout (seconds)" error={fieldError('timeout')} errorId="task-timeout-error">
            <NumberInput
              aria-label="Timeout seconds"
              aria-invalid={Boolean(fieldError('timeout'))}
              aria-describedby={fieldError('timeout') ? 'task-timeout-error' : undefined}
              min={0}
              value={draft.timeout}
              onChange={(e) => patch({timeout: Number(e.target.value)})}
            />
          </Field>
          <Field label="Working directory" hint="Relative to the task file.">
            <div className="flex gap-2">
              <TextInput
                aria-label="Working directory"
                value={draft.workingDirectory}
                onChange={(e) => patch({workingDirectory: e.target.value})}
                placeholder="./scripts"
              />
              <Browse
                pick={pickWorkingDir}
                onPick={(p) => patch({workingDirectory: p})}
              />
            </div>
          </Field>
          <div className="pt-5">
            <label className="flex items-center gap-2 text-xs font-medium text-zinc-500 dark:text-zinc-400">
              <Toggle
                checked={draft.captureOutput}
                onChange={(v) => patch({captureOutput: v})}
                label="Capture output"
              />
              Capture output
            </label>
          </div>
        </div>
        <Field
          label="Log directory"
          hint={
            <>
              Per-run log files (stdout.log, stderr.log, run.json). Leave empty
              to log in the working directory.
            </>
          }
        >
          <div className="flex gap-2">
            <TextInput
              aria-label="Log directory"
              value={draft.outputDir}
              onChange={(e) => patch({outputDir: e.target.value})}
              placeholder="./logs"
            />
            <Browse
              pick={pickWorkingDir}
              onPick={(p) => patch({outputDir: p})}
            />
          </div>
        </Field>
      </section>

      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Retry</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Max attempts" error={fieldError('retry.max_attempts', 'max_attempts')} errorId="task-attempts-error">
            <NumberInput
              aria-label="Max attempts"
              aria-invalid={Boolean(fieldError('retry.max_attempts', 'max_attempts'))}
              aria-describedby={fieldError('retry.max_attempts', 'max_attempts') ? 'task-attempts-error' : undefined}
              min={1}
              value={draft.maxAttempts}
              onChange={(e) => patch({maxAttempts: Number(e.target.value)})}
            />
          </Field>
          <Field label="Delay between attempts (seconds)" error={fieldError('retry.delay_seconds', 'delay_seconds')} errorId="task-delay-error">
            <NumberInput
              aria-label="Delay seconds"
              aria-invalid={Boolean(fieldError('retry.delay_seconds', 'delay_seconds'))}
              aria-describedby={fieldError('retry.delay_seconds', 'delay_seconds') ? 'task-delay-error' : undefined}
              min={0}
              value={draft.delaySeconds}
              onChange={(e) => patch({delaySeconds: Number(e.target.value)})}
            />
          </Field>
        </div>
      </section>

      <EnvEditor rows={draft.environment} onChange={(environment) => patch({environment})} />

      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Notifications</h3>
        <p
          data-testid="schedule-pointer"
          className="rounded-lg border border-zinc-200 bg-zinc-50 px-3 py-2 text-xs text-zinc-500 dark:border-zinc-800 dark:bg-zinc-900/50 dark:text-zinc-400"
        >
          Want this task on a schedule? Add one on the Schedules page after saving.
        </p>
        <Field label="Notify on">
          <div className="flex flex-wrap gap-4 pt-1">
            {NOTIFY_EVENTS.map((ev) => (
              <label key={ev} className="flex items-center gap-2 text-sm text-zinc-600 dark:text-zinc-300">
                <Toggle
                  checked={draft.notifyOn.includes(ev)}
                  onChange={(on) =>
                    patch({
                      notifyOn: on
                        ? [...draft.notifyOn, ev]
                        : draft.notifyOn.filter((e) => e !== ev),
                    })
                  }
                  label={`Notify on ${ev}`}
                />
                {ev}
              </label>
            ))}
          </div>
        </Field>
        <WebhookEditor rows={draft.webhooks} onChange={(webhooks) => patch({webhooks})} />
      </section>
    </div>
  )
}