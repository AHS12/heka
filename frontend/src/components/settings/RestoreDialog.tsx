// components/settings/RestoreDialog.tsx — the guided restore flow: pick an
// archive, verify it with the passphrase, preview what it contains, then
// stop the daemon and replace local state. Restore refuses while the daemon
// owns the data; this component orchestrates the stop → restore → restart
// sequence so the user never has to touch a terminal.
import {useState} from 'react'
import {Modal, Toast} from '@heroui/react'
import {
  apiErrorDetails,
  inspectBackup,
  restoreBackup,
  pickBackupFile,
  shutdownDaemon,
  daemonStatus,
  startDaemon,
  type RestoreManifest,
  type RestoreResult,
} from '../../lib/api'
import {formatStamp} from '../../lib/backup'
import {Field, FormErrors, TextInput, pillBtn, primaryBtn} from '../controls'
import {AppDialog, dialogHeaderCls, dialogBodyCls, dialogFooterCls} from '../AppDialog'

type Phase = 'pick' | 'preview' | 'working' | 'done'

const waitForShutdown = async (timeoutMs = 20000): Promise<boolean> => {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const status = await daemonStatus()
      if (status === 'not-running') return true
    } catch {
      return true // unreachable counts as down for our purposes
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  return false
}

export function RestoreDialog({onClose, onDone}: {onClose: () => void; onDone: () => void}) {
  const [phase, setPhase] = useState<Phase>('pick')
  const [path, setPath] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [inspect, setInspect] = useState<RestoreManifest | null>(null)
  const [includeConfig, setIncludeConfig] = useState(true)
  const [includeArtifacts, setIncludeArtifacts] = useState(false)
  const [result, setResult] = useState<RestoreResult | null>(null)
  const [workingLabel, setWorkingLabel] = useState('')
  const [errors, setErrors] = useState<string[]>([])
  const [daemonRestarted, setDaemonRestarted] = useState(false)

  const handleBrowse = async () => {
    try {
      setPath(await pickBackupFile())
    } catch {
      // canceled — no-op
    }
  }

  const handleInspect = async () => {
    if (!path || !passphrase) return
    setErrors([])
    try {
      const m = await inspectBackup(path, passphrase)
      if (m.preview_error) {
        setErrors([m.preview_error])
        return
      }
      setInspect(m)
      setIncludeConfig(m.has_config)
      setPhase('preview')
    } catch (err) {
      setErrors(apiErrorDetails(err))
    }
  }

  const handleRestore = async () => {
    setErrors([])
    setPhase('working')
    setWorkingLabel('Stopping the daemon…')
    try {
      const stopped = await waitForShutdownAfterRequest()
      if (!stopped) {
        throw new Error(
          'The daemon did not stop in time. Quit Heka from the tray (or run "heka daemon stop"), then retry.'
        )
      }
      setWorkingLabel('Restoring your data…')
      const res = await restoreBackup(path, passphrase, includeConfig, includeArtifacts)
      setResult(res)
      setPhase('done')
    } catch (err) {
      setErrors(apiErrorDetails(err))
      setPhase('preview')
    }
  }

  const waitForShutdownAfterRequest = async (): Promise<boolean> => {
    // Ask politely first; the daemon finishing in-flight work may take a beat.
    try {
      await shutdownDaemon()
    } catch {
      // already down — fine
    }
    return waitForShutdown()
  }

  const handleRestart = async () => {
    setWorkingLabel('Starting the daemon…')
    try {
      await startDaemon()
      setDaemonRestarted(true)
    } catch (err) {
      Toast.toast.danger('Could not start the daemon', {description: apiErrorDetails(err)[0]})
    }
  }

  const m = inspect?.manifest
  const busy = phase === 'working'

  return (
    <AppDialog isOpen onOpenChange={(open) => !open && !busy && onClose()} size="md">
      <Modal.Header className={dialogHeaderCls}>
        <div>
          <Modal.Heading className="text-lg font-semibold">Restore from backup</Modal.Heading>
          <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
            Replaces your current tasks, schedules, secrets, and settings with the archive's.
          </p>
        </div>
        <Modal.CloseTrigger aria-label="Close restore dialog" isDisabled={busy || phase === 'done'} />
      </Modal.Header>
      <Modal.Body className={dialogBodyCls}>
        {phase === 'pick' && (
          <div className="space-y-3">
            <div className="rounded-xl border border-amber-300/70 bg-amber-50/80 px-3 py-2.5 text-xs text-amber-800 dark:border-amber-700/60 dark:bg-amber-950/40 dark:text-amber-200">
              Restoring overwrites your current data. A safety backup of what is here now is
              written next to your other backups first. The daemon stops while the restore runs.
            </div>
            <Field label="Backup archive">
              <div className="flex gap-2">
                <TextInput
                  aria-label="Backup archive path"
                  value={path}
                  placeholder="Choose a heka-backup-….zip file"
                  readOnly
                />
                <button type="button" className={pillBtn} onClick={handleBrowse}>
                  Browse…
                </button>
              </div>
            </Field>
            <Field
              label="Passphrase"
              hint="The one set when the backup was created — check your password manager."
            >
              <TextInput
                aria-label="Archive passphrase"
                type="password"
                autoComplete="off"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
              />
            </Field>
            <FormErrors errors={errors} title="Cannot open this backup" />
          </div>
        )}

        {phase === 'preview' && m && (
          <div className="space-y-3">
            <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5 rounded-xl border border-zinc-200/80 bg-white/60 px-3.5 py-3 text-xs dark:border-zinc-800 dark:bg-zinc-900/50">
              <dt className="text-zinc-500 dark:text-zinc-400">Created</dt>
              <dd className="text-zinc-800 dark:text-zinc-200">{formatStamp(m.created_at)}</dd>
              <dt className="text-zinc-500 dark:text-zinc-400">Heka version</dt>
              <dd className="text-zinc-800 dark:text-zinc-200">{m.app_version || '—'}</dd>
              <dt className="text-zinc-500 dark:text-zinc-400">Created on</dt>
              <dd className="truncate text-zinc-800 dark:text-zinc-200">
                {m.hostname ? `${m.hostname} (${m.os}/${m.arch})` : '—'}
              </dd>
              <dt className="text-zinc-500 dark:text-zinc-400">Contents</dt>
              <dd className="text-zinc-800 dark:text-zinc-200">
                {m.counts.tasks} tasks · {m.counts.schedules} schedules · {m.counts.secrets} secrets
                {m.includes.run_history ? ` · ${m.counts.runs} runs` : ''}
              </dd>
            </dl>
            <label className="flex items-start gap-2 text-sm text-zinc-700 dark:text-zinc-200">
              <input
                type="checkbox"
                className="mt-0.5 accent-[var(--accent)]"
                checked={includeConfig}
                disabled={!inspect?.has_config}
                onChange={(e) => setIncludeConfig(e.target.checked)}
              />
              <span>
                Restore config.yaml
                {!inspect?.has_config && <span className="text-zinc-400"> (not in this archive)</span>}
                {inspect?.has_config && (
                  <span className="block text-xs text-zinc-500 dark:text-zinc-400">
                    It may contain path overrides for a different machine — review after restoring.
                  </span>
                )}
              </span>
            </label>
            <label className="flex items-start gap-2 text-sm text-zinc-700 dark:text-zinc-200">
              <input
                type="checkbox"
                className="mt-0.5 accent-[var(--accent)]"
                checked={includeArtifacts}
                disabled={!inspect?.has_artifacts}
                onChange={(e) => setIncludeArtifacts(e.target.checked)}
              />
              <span>
                Restore run artifact files
                {!inspect?.has_artifacts && <span className="text-zinc-400"> (not in this archive)</span>}
              </span>
            </label>
            <FormErrors errors={errors} title="Restore failed" />
          </div>
        )}

        {phase === 'working' && (
          <div className="flex flex-col items-center gap-3 py-8" data-testid="restore-working">
            <svg className="size-8 animate-spin text-accent" viewBox="0 0 24 24" fill="none" aria-hidden>
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            <p className="text-sm text-zinc-500 dark:text-zinc-400">{workingLabel}</p>
          </div>
        )}

        {phase === 'done' && result && (
          <div className="space-y-3" data-testid="restore-done">
            <div className="flex items-start gap-2.5 rounded-xl border border-emerald-300/70 bg-emerald-50/80 px-3 py-3 text-sm text-emerald-800 dark:border-emerald-700/60 dark:bg-emerald-950/40 dark:text-emerald-200">
              <svg aria-hidden viewBox="0 0 16 16" className="mt-0.5 size-4 shrink-0 fill-current">
                <path d="M6.5 12.5 2 8l1.5-1.5 3 3 6-6L14 5z" />
              </svg>
              <div>
                <p className="font-semibold">Restore complete</p>
                <p className="mt-0.5 text-xs">
                  {result.manifest.counts.tasks} tasks, {result.manifest.counts.schedules} schedules
                  and {result.manifest.counts.secrets} secrets were brought back
                  {result.restored_config ? ', config.yaml included' : ''}.
                  {result.safety_backup_path && (
                    <>
                      {' '}Your previous state was saved to{' '}
                      <code className="break-all rounded bg-emerald-100/70 px-1 dark:bg-emerald-900/40">
                        {result.safety_backup_path}
                      </code>
                      .
                    </>
                  )}
                </p>
              </div>
            </div>
            {daemonRestarted ? (
              <p className="text-xs text-zinc-500 dark:text-zinc-400">
                Daemon starting — your data will appear in a moment.
              </p>
            ) : (
              <p className="text-xs text-zinc-500 dark:text-zinc-400">
                The daemon is stopped. Start it to load the restored data.
              </p>
            )}
          </div>
        )}
      </Modal.Body>
      <Modal.Footer className={dialogFooterCls}>
        {phase === 'pick' && (
          <>
            <button type="button" className={pillBtn} onClick={onClose}>
              Cancel
            </button>
            <button
              type="button"
              className={primaryBtn}
              disabled={!path || !passphrase}
              onClick={handleInspect}
            >
              Verify archive
            </button>
          </>
        )}
        {phase === 'preview' && (
          <>
            <button type="button" className={pillBtn} onClick={() => setPhase('pick')}>
              Back
            </button>
            <button type="button" className={primaryBtn} onClick={handleRestore}>
              Restore now
            </button>
          </>
        )}
        {phase === 'done' && (
          <>
            {!daemonRestarted && (
              <button type="button" className={pillBtn} onClick={handleRestart}>
                Start daemon
              </button>
            )}
            <button type="button" className={primaryBtn} onClick={onDone}>
              Done
            </button>
          </>
        )}
      </Modal.Footer>
    </AppDialog>
  )
}
