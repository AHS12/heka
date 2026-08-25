// components/tasks/TaskForm.tsx (SPEC-13 §4) — the Form tab: two views of one
// canonical draft (props in, patch-out). Sections: Basics, Script/Binary,
// Execution, Retry, Environment, Notifications.
import {useState} from 'react'
import {Field, NumberInput, SelectField, TextInput, Toggle, pillBtn} from '../controls'
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
      {errors.length > 0 && (
        <ul
          role="alert"
          data-testid="form-errors"
          className="space-y-1 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900 dark:bg-red-950/60 dark:text-red-300"
        >
          {errors.map((err) => (
            <li key={err}>• {err}</li>
          ))}
        </ul>
      )}

      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Basics</h3>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Name">
            <TextInput
              aria-label="Task name"
              value={draft.name}
              onChange={(e) => onNameChange(e.target.value)}
              placeholder="Nightly backup"
            />
          </Field>
          <Field
            label="Slug"
            hint="Auto-generated from the name — override it here if you want."
          >
            <TextInput
              aria-label="Task slug"
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
        <div className="grid grid-cols-2 gap-3">
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
                ? 'Binaries run directly — no interpreter.'
                : 'Need another runtime? Use the YAML tab.'
            }
          >
            <SelectField
              aria-label="Runtime"
              value={draft.runtime}
              placeholder="Choose runtime…"
              isDisabled={type === 'binary'}
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
        <Field label={type === 'binary' ? 'Command' : 'Script path'}>
          <div className="flex gap-2">
            <TextInput
              aria-label={type === 'binary' ? 'Command' : 'Script path'}
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
        <div className="grid grid-cols-3 gap-3">
          <Field label="Timeout (seconds)">
            <NumberInput
              aria-label="Timeout seconds"
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
        <div className="grid grid-cols-2 gap-3">
          <Field label="Max attempts">
            <NumberInput
              aria-label="Max attempts"
              min={1}
              value={draft.maxAttempts}
              onChange={(e) => patch({maxAttempts: Number(e.target.value)})}
            />
          </Field>
          <Field label="Delay between attempts (seconds)">
            <NumberInput
              aria-label="Delay seconds"
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