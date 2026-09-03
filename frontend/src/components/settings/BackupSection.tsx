// components/settings/BackupSection.tsx — the Settings → Backup panel:
// automatic schedule, destinations (local dir + S3/R2), archive contents,
// passphrase management, run status, and the restore entry point.
// Job history lives on its own page (/backups) — it can grow long.
import {useEffect, useRef, useState} from 'react'
import {useQueryClient} from '@tanstack/react-query'
import {Modal, Toast} from '@heroui/react'
import {
  apiErrorDetails,
  setSecret,
  pickWorkingDir,
  testBackupDestinations,
  type BackupConfig,
  type BackupSchedule,
  type BackupTestResult,
} from '../../lib/api'
import {useSecrets, SECRETS_KEY} from '../../lib/secrets'
import {
  useBackupConfig,
  useBackupStatus,
  useSaveBackupConfig,
  useRunBackup,
  formatBytes,
  formatStamp,
  BACKUP_CONFIG_KEY,
} from '../../lib/backup'
import {Field, FormErrors, SelectField, TextInput, NumberInput, TimePickerField, pillBtn, primaryBtn} from '../controls'
import {AppDialog, dialogHeaderCls, dialogBodyCls, dialogFooterCls} from '../AppDialog'
import {RestoreDialog} from './RestoreDialog'

export const BACKUP_PASSPHRASE_KEY = 'BACKUP_PASSPHRASE'
export const BACKUP_S3_ACCESS_KEY = 'BACKUP_S3_ACCESS_KEY_ID'
export const BACKUP_S3_SECRET_KEY = 'BACKUP_S3_SECRET_ACCESS_KEY'

/** Cadence picker entries. Hour presets map to ScheduleSpec.kind='interval';
 *  daily/weekly/monthly carry a time (and day) of their own. */
const CADENCE_PRESETS = [
  {id: '6', label: 'Every 6 hours'},
  {id: '12', label: 'Every 12 hours'},
  {id: '24', label: 'Every 24 hours'},
  {id: '48', label: 'Every 2 days'},
  {id: '72', label: 'Every 3 days'},
  {id: '168', label: 'Every week'},
  {id: '360', label: 'Every 15 days'},
  {id: '720', label: 'Every month (30 days)'},
  {id: '2160', label: 'Every 3 months'},
  {id: '4320', label: 'Every 6 months'},
  {id: '8760', label: 'Every year'},
  {id: 'daily', label: 'Daily at a specific time'},
  {id: 'weekly', label: 'Weekly on a specific day'},
  {id: 'monthly', label: 'Monthly on a specific day'},
  {id: 'custom', label: 'Custom interval…'},
]

const WEEKDAY_OPTIONS = [
  {id: '0', label: 'Sunday'},
  {id: '1', label: 'Monday'},
  {id: '2', label: 'Tuesday'},
  {id: '3', label: 'Wednesday'},
  {id: '4', label: 'Thursday'},
  {id: '5', label: 'Friday'},
  {id: '6', label: 'Saturday'},
]

/** Which picker entry matches the stored schedule (never a mismatched id —
 *  that is what left the dropdown blank before). */
function cadenceValue(s: BackupSchedule): string {
  if (s.kind === 'daily') return 'daily'
  if (s.kind === 'weekly') return 'weekly'
  if (s.kind === 'monthly') return 'monthly'
  if (s.kind === 'interval') {
    return CADENCE_PRESETS.some((p) => p.id === String(s.every_hours))
      ? String(s.every_hours)
      : 'custom'
  }
  return 'daily' // 'off' hides the picker anyway
}

function clampHours(n: number): number {
  return Math.min(8760, Math.max(1, Math.round(n) || 1))
}

function card() {
  return 'rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60'
}

function sectionTitle() {
  return 'text-xs font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400'
}

function fieldLabel() {
  return 'mb-1 block text-xs font-medium text-zinc-500 dark:text-zinc-400'
}

function ToggleRow({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint?: string
  checked: boolean
  onChange: (on: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between rounded-xl border border-zinc-200/80 bg-white/60 px-4 py-3 dark:border-zinc-800 dark:bg-zinc-900/50">
      <div className="min-w-0 pr-3">
        <p className="text-sm font-medium text-zinc-800 dark:text-zinc-200">{label}</p>
        {hint && <p className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">{hint}</p>}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        onClick={() => onChange(!checked)}
        className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full outline-none transition-colors focus-visible:ring-2 focus-visible:ring-accent-ring ${
          checked ? 'bg-accent' : 'bg-zinc-300 dark:bg-zinc-700'
        }`}
      >
        <span
          className={`inline-block size-3.5 transform rounded-full bg-white shadow transition-transform ${
            checked ? 'translate-x-[18px]' : 'translate-x-[3px]'
          }`}
        />
      </button>
    </div>
  )
}

/** Generate a passphrase in the browser: 32 bytes of CSPRNG, base64url. */
function generatePassphrase(): string {
  const raw = new Uint8Array(32)
  crypto.getRandomValues(raw)
  let bin = ''
  for (const b of raw) bin += String.fromCharCode(b)
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function BackupSection() {
  const qc = useQueryClient()
  const config = useBackupConfig()
  const status = useBackupStatus()
  const save = useSaveBackupConfig()
  const runNow = useRunBackup()
  // The passphrase state must track the vault itself: deleting
  // BACKUP_PASSPHRASE on the secrets page has to re-offer setup here.
  const secrets = useSecrets()
  const passphraseSet = secrets.data
    ? secrets.data.includes(BACKUP_PASSPHRASE_KEY)
    : !!config.data?.passphrase_set

  const [draft, setDraft] = useState<BackupConfig | null>(null)
  const initialized = useRef(false)
  const [errors, setErrors] = useState<string[]>([])
  const [savedFlash, setSavedFlash] = useState(false)
  const [s3Enabled, setS3Enabled] = useState(false)
  const [s3Access, setS3Access] = useState('')
  const [s3Secret, setS3Secret] = useState('')
  const [showPassphrase, setShowPassphrase] = useState(false)
  const [showRestore, setShowRestore] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<BackupTestResult | null>(null)
  const [pinned, setPinned] = useState(false)
  const headerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (config.data && !initialized.current) {
      initialized.current = true
      setDraft(config.data)
      setS3Enabled(!!config.data.s3.bucket && !!config.data.s3.endpoint)
    }
  }, [config.data])

  // The sticky header "docks" when it reaches the top of the scroll area —
  // detected from its live position so the corner animation triggers exactly
  // when it pins, not on raw scroll offset.
  useEffect(() => {
    const bar = headerRef.current
    if (!bar) return
    const scroller = bar.closest('main')
    if (!scroller) return
    const onScroll = () => {
      setPinned(bar.getBoundingClientRect().top <= scroller.getBoundingClientRect().top + 1)
    }
    onScroll()
    scroller.addEventListener('scroll', onScroll, {passive: true})
    return () => scroller.removeEventListener('scroll', onScroll)
  }, [])

  const patch = (p: Partial<BackupConfig>) => setDraft((d) => (d ? {...d, ...p} : d))
  const dirty =
    !!draft &&
    !!config.data &&
    (JSON.stringify({...draft, passphrase_set: false}) !==
      JSON.stringify({...config.data, passphrase_set: false}) ||
      s3Access !== '' ||
      s3Secret !== '')

  const performSave = async (): Promise<boolean> => {
    if (!draft) return false
    try {
      if (s3Enabled && draft.s3.bucket && s3Access) {
        await setSecret(BACKUP_S3_ACCESS_KEY, s3Access)
      }
      if (s3Enabled && draft.s3.bucket && s3Secret) {
        await setSecret(BACKUP_S3_SECRET_KEY, s3Secret)
      }
      await save.mutateAsync(draft)
      setS3Access('')
      setS3Secret('')
      setSavedFlash(true)
      setTimeout(() => setSavedFlash(false), 1500)
      return true
    } catch (err) {
      const details = apiErrorDetails(err)
      setErrors(details)
      Toast.toast.danger('Backup settings could not be saved', {description: details[0]})
      return false
    }
  }

  const handleSave = async () => {
    setErrors([])
    await performSave()
  }

  const handleTest = async () => {
    setErrors([])
    setTesting(true)
    setTestResult(null)
    // The daemon tests the saved config — persist pending edits first so the
    // test reflects exactly what is on screen.
    if (dirty && !(await performSave())) {
      setTesting(false)
      return
    }
    try {
      setTestResult(await testBackupDestinations())
    } catch (err) {
      setTestResult(null)
      setErrors(apiErrorDetails(err))
    }
    setTesting(false)
  }

  const handleBrowse = async () => {
    try {
      const dir = await pickWorkingDir()
      patch({local_dir: dir})
    } catch {
      // canceled — no-op
    }
  }

  const handleBackupNow = async () => {
    setErrors([])
    try {
      await runNow.mutateAsync()
      Toast.toast.success('Backup started', {description: 'Track progress below.'})
    } catch (err) {
      const details = apiErrorDetails(err)
      setErrors(details)
      Toast.toast.danger('Backup could not start', {description: details[0]})
    }
  }

  if (config.isError) {
    // Daemon unreachable, old daemon without this endpoint, etc. — never
    // leave the panel stuck on "Loading…".
    return (
      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Backup</h3>
        <FormErrors errors={apiErrorDetails(config.error)} title="Backup settings could not be loaded" />
        <button type="button" className={primaryBtn} onClick={() => config.refetch()}>
          Retry
        </button>
      </section>
    )
  }

  if (config.isLoading || !draft) {
    return <p className="text-sm text-zinc-400">Loading backup settings…</p>
  }

  const last = status.data?.last
  const running = !!status.data?.running
  const s = draft.schedule

  return (
    <section className="space-y-4">
      {/* Sticky header: Save + Restore stay reachable no matter how far the
          panel is scrolled — the save button must never be out of sight.
          Corners square off (animated) while docked so the bar reads as part
          of the card, and round again back at rest. */}
      <div
        ref={headerRef}
        className={`sticky top-0 z-10 -mx-4 -mt-4 flex flex-wrap items-center justify-between gap-x-3 gap-y-2 border-b border-zinc-200/80 bg-white/85 px-4 py-3 backdrop-blur-sm transition-[border-radius] duration-300 ease-out sm:-mx-5 sm:-mt-5 sm:px-5 dark:border-zinc-800 dark:bg-zinc-950/70 ${
          pinned ? 'rounded-none' : 'rounded-t-2xl'
        }`}
      >
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Backup</h3>
          <p className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">
            Encrypted archives of your tasks, schedules, secrets, and settings.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <button
            type="button"
            className={primaryBtn}
            disabled={!dirty || save.isPending}
            onClick={handleSave}
          >
            {savedFlash ? 'Saved' : save.isPending ? 'Saving…' : 'Save changes'}
          </button>
          <button type="button" className={pillBtn} onClick={() => setShowRestore(true)}>
            Restore from backup…
          </button>
        </div>
      </div>

      {/* Status */}
      <div className={card()}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className={sectionTitle()}>Status</p>
            <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-zinc-600 dark:text-zinc-300">
              {running ? (
                <span className="inline-flex items-center gap-1.5 font-medium text-accent">
                  <svg className="size-3.5 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden>
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                  </svg>
                  Backup running…
                </span>
              ) : last ? (
                <>
                  <span>
                    Last backup:{' '}
                    <JobStatus job={last} />
                  </span>
                  <span className="text-zinc-500 dark:text-zinc-400">{formatStamp(last.finished_at || last.started_at)}</span>
                  {!!last.size_bytes && <span className="text-zinc-500 dark:text-zinc-400">{formatBytes(last.size_bytes)}</span>}
                </>
              ) : (
                <span className="text-zinc-500 dark:text-zinc-400">No backup yet</span>
              )}
              {status.data?.next_run_at && (
                <span className="text-zinc-500 dark:text-zinc-400">Next: {formatStamp(status.data.next_run_at)}</span>
              )}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <a href="#/backups" className={pillBtn}>
              History
            </a>
            <button
              type="button"
              className={primaryBtn}
              onClick={handleBackupNow}
              disabled={running || runNow.isPending}
            >
              {running ? 'Backing up…' : 'Back up now'}
            </button>
          </div>
        </div>
      </div>

      {/* Automatic schedule */}
      <div className={card()}>
        <p className={sectionTitle()}>Automatic backups</p>
        <div className="mt-2 space-y-3">
          <ToggleRow
            label="Back up automatically"
            hint="If Heka is closed at the scheduled time, the missed backup runs once at startup."
            checked={s.kind !== 'off'}
            onChange={(on) => patch({schedule: on ? {kind: 'daily', at_time: s.at_time || '03:00'} : {kind: 'off'}})}
          />
          {s.kind !== 'off' && (
            <div className="flex flex-wrap items-end gap-3">
              <Field label="Cadence">
                <SelectField
                  aria-label="Backup cadence"
                  className="w-56"
                  value={cadenceValue(s)}
                  onChange={(v) => {
                    if (v === 'daily') {
                      patch({schedule: {kind: 'daily', at_time: s.at_time || '03:00'}})
                    } else if (v === 'weekly') {
                      patch({schedule: {kind: 'weekly', weekday: s.weekday ?? 0, at_time: s.at_time || '03:00'}})
                    } else if (v === 'monthly') {
                      patch({schedule: {kind: 'monthly', day_of_month: s.day_of_month || 1, at_time: s.at_time || '03:00'}})
                    } else if (v === 'custom') {
                      patch({schedule: {kind: 'interval', every_hours: clampHours(s.every_hours || 6)}})
                    } else {
                      patch({schedule: {kind: 'interval', every_hours: clampHours(parseInt(v) || 24)}})
                    }
                  }}
                  items={CADENCE_PRESETS}
                />
              </Field>
              {s.kind === 'interval' && (
                <Field label="Hours between backups">
                  <NumberInput
                    aria-label="Hours between backups"
                    className="w-24"
                    min={1}
                    max={8760}
                    value={s.every_hours || 24}
                    onChange={(e) => patch({schedule: {kind: 'interval', every_hours: clampHours(parseInt(e.target.value) || 1)}})}
                  />
                </Field>
              )}
              {s.kind === 'weekly' && (
                <Field label="Day of week">
                  <SelectField
                    aria-label="Weekly backup day"
                    className="w-40"
                    value={String(s.weekday ?? 0)}
                    onChange={(v) => patch({schedule: {...s, weekday: parseInt(v) || 0}})}
                    items={WEEKDAY_OPTIONS}
                  />
                </Field>
              )}
              {(s.kind === 'daily' || s.kind === 'weekly' || s.kind === 'monthly') && (
                <Field label="Time of day (local)">
                  <TimePickerField
                    aria-label="Backup time of day"
                    className="w-32"
                    value={s.at_time || '03:00'}
                    onChange={(v) => patch({schedule: {...s, at_time: v}})}
                  />
                </Field>
              )}
              {s.kind === 'monthly' && (
                <Field label="Day of month" hint="1–28 (safe for every month length)">
                  <NumberInput
                    aria-label="Monthly backup day"
                    className="w-20"
                    min={1}
                    max={28}
                    value={s.day_of_month || 1}
                    onChange={(e) =>
                      patch({schedule: {...s, day_of_month: Math.min(28, Math.max(1, parseInt(e.target.value) || 1))}})
                    }
                  />
                </Field>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Destinations */}
      <div className={card()}>
        <p className={sectionTitle()}>Destinations</p>
        <div className="mt-2 space-y-3">
          <div className="flex flex-wrap items-start gap-x-3 gap-y-2">
            <div className="min-w-0 flex-1 basis-72">
              <span className={fieldLabel()}>Local folder</span>
              <div className="flex items-stretch gap-2">
                <TextInput
                  aria-label="Local backup folder"
                  className="min-w-0 flex-1"
                  value={draft.local_dir}
                  placeholder="Default: inside your data directory"
                  onChange={(e) => patch({local_dir: e.target.value})}
                />
                <button type="button" className={`${pillBtn} shrink-0 self-stretch`} onClick={handleBrowse}>
                  Browse…
                </button>
              </div>
            </div>
            <div className="w-44">
              <span className={fieldLabel()}>Keep latest</span>
              <div className="flex items-center gap-1.5">
                <NumberInput
                  aria-label="Local backups to keep"
                  className="w-20"
                  min={1}
                  value={draft.keep_last_local || 5}
                  onChange={(e) => patch({keep_last_local: Math.max(1, parseInt(e.target.value) || 1)})}
                />
                <span className="text-xs text-zinc-400">backups</span>
              </div>
            </div>
          </div>
          <p className="text-xs text-zinc-400 dark:text-zinc-500">
            Archives are AES-256 encrypted zip files.
          </p>

          <ToggleRow
            label="Mirror to an S3-compatible bucket"
            hint="Works with AWS S3, Cloudflare R2, Backblaze B2, MinIO."
            checked={s3Enabled}
            onChange={(on) => {
              setS3Enabled(on)
              if (on) {
                // Go's zero-value config has use_ssl=false; default to TLS so a
                // freshly enabled destination is never silently plain HTTP.
                patch({s3: {...draft.s3, use_ssl: true}})
              } else {
                // Fully clears the destination; nothing partial is saved.
                patch({s3: {bucket: '', endpoint: '', region: '', prefix: '', use_ssl: true, keep_last: 0}})
                setS3Access('')
                setS3Secret('')
              }
            }}
          />
          {s3Enabled && (
            <div className="space-y-3 rounded-xl border border-zinc-200/70 bg-white/40 p-3 dark:border-zinc-800 dark:bg-zinc-950/20">
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="Endpoint" hint="R2 example: <account>.r2.cloudflarestorage.com">
                  <TextInput
                    aria-label="S3 endpoint"
                    value={draft.s3.endpoint || ''}
                    placeholder="s3.amazonaws.com"
                    onChange={(e) => patch({s3: {...draft.s3, endpoint: e.target.value.trim()}})}
                  />
                </Field>
                <Field label="Bucket">
                  <TextInput
                    aria-label="S3 bucket"
                    value={draft.s3.bucket || ''}
                    onChange={(e) => patch({s3: {...draft.s3, bucket: e.target.value.trim()}})}
                  />
                </Field>
                <Field label="Key prefix (optional)">
                  <TextInput
                    aria-label="S3 key prefix"
                    value={draft.s3.prefix || ''}
                    placeholder="heka"
                    onChange={(e) => patch({s3: {...draft.s3, prefix: e.target.value}})}
                  />
                </Field>
                <Field label="Region (optional)" hint='R2 uses "auto".'>
                  <TextInput
                    aria-label="S3 region"
                    value={draft.s3.region || ''}
                    placeholder="auto"
                    onChange={(e) => patch({s3: {...draft.s3, region: e.target.value.trim()}})}
                  />
                </Field>
                <Field label="Access key ID" hint="Stored in the vault; leave blank to keep the current value.">
                  <TextInput
                    aria-label="S3 access key ID"
                    type="password"
                    autoComplete="off"
                    value={s3Access}
                    placeholder={draft.s3.bucket ? '••••••••' : ''}
                    onChange={(e) => setS3Access(e.target.value)}
                  />
                </Field>
                <Field label="Secret access key">
                  <TextInput
                    aria-label="S3 secret access key"
                    type="password"
                    autoComplete="off"
                    value={s3Secret}
                    placeholder={draft.s3.bucket ? '••••••••' : ''}
                    onChange={(e) => setS3Secret(e.target.value)}
                  />
                </Field>
                <Field label="Keep latest (remote)">
                  <div className="flex items-center gap-1.5">
                    <NumberInput
                      aria-label="Remote backups to keep"
                      className="w-20"
                      min={0}
                      value={draft.s3.keep_last ?? 0}
                      onChange={(e) => patch({s3: {...draft.s3, keep_last: Math.max(0, parseInt(e.target.value) || 0)}})}
                    />
                    <span className="text-xs text-zinc-400">0 = keep all</span>
                  </div>
                </Field>
                <div className="flex items-end pb-1">
                  <ToggleRow
                    label="Use HTTPS"
                    hint="Leave on — plain HTTP is redirected (301) by remote endpoints."
                    checked={draft.s3.use_ssl ?? true}
                    onChange={(on) => patch({s3: {...draft.s3, use_ssl: on}})}
                  />
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <button type="button" className={pillBtn} disabled={testing} onClick={handleTest}>
                  {testing ? 'Testing…' : 'Test connection'}
                </button>
                {testResult?.local && (
                  <span className={`text-xs ${testResult.local.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>
                    Local: {testResult.local.ok ? 'OK' : testResult.local.error}
                  </span>
                )}
                {testResult?.s3 && (
                  <span className={`text-xs ${testResult.s3.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>
                    S3: {testResult.s3.ok ? 'OK' : testResult.s3.error}
                  </span>
                )}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Archive contents */}
      <div className={card()}>
        <p className={sectionTitle()}>What to include</p>
        <div className="mt-2 space-y-2">
          <ToggleRow
            label="Run history"
            hint="Past runs and their captured output. Keep on for a complete restore."
            checked={draft.includes.run_history}
            onChange={(on) => patch({includes: {...draft.includes, run_history: on}})}
          />
          <ToggleRow
            label="Run artifacts (files)"
            hint="Per-run output folders on disk. Can grow large — off by default."
            checked={draft.includes.artifacts}
            onChange={(on) => patch({includes: {...draft.includes, artifacts: on}})}
          />
        </div>
      </div>

      {/* Encryption */}
      <div className={card()}>
        <p className={sectionTitle()}>Encryption</p>
        <div className="mt-2 flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-medium text-zinc-800 dark:text-zinc-200">Archive passphrase</p>
            <p className="mt-0.5 max-w-xl text-xs text-zinc-500 dark:text-zinc-400">
              Every archive is encrypted with it. Heka keeps a copy in the vault so scheduled
              backups run unattended — but restoring on a new machine asks for it.
              <span className="font-medium text-zinc-700 dark:text-zinc-200">
                {' '}If you lose the passphrase, the backup cannot be restored — store it safely.
              </span>
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {passphraseSet && (
              <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-100/70 px-3 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
                <svg aria-hidden viewBox="0 0 16 16" className="size-3 fill-current">
                  <path d="M8 1a3.5 3.5 0 0 0-3.5 3.5V6H4a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1V7a1 1 0 0 0-1-1h-.5V4.5A3.5 3.5 0 0 0 8 1zm2 5H5.5V4.5a2.5 2.5 0 1 1 5 0V6z" />
                </svg>
                Set up
              </span>
            )}
            <button
              type="button"
              className={passphraseSet ? pillBtn : primaryBtn}
              onClick={() => setShowPassphrase(true)}
            >
              {passphraseSet ? 'Change passphrase' : 'Set up passphrase'}
            </button>
          </div>
        </div>
      </div>

      {errors.length > 0 && <FormErrors errors={errors} title="Backup settings could not be saved" />}

      {/* Passphrase setup / change dialog */}
      {showPassphrase && (
        <PassphraseDialog
          changing={passphraseSet}
          onClose={() => setShowPassphrase(false)}
          onSaved={() => {
            setShowPassphrase(false)
            void qc.invalidateQueries({queryKey: SECRETS_KEY})
            void qc.invalidateQueries({queryKey: BACKUP_CONFIG_KEY})
          }}
        />
      )}

      {/* Restore flow */}
      {showRestore && (
        <RestoreDialog
          onClose={() => setShowRestore(false)}
          onDone={() => {
            setShowRestore(false)
            void qc.invalidateQueries({queryKey: BACKUP_CONFIG_KEY})
          }}
        />
      )}
    </section>
  )
}

function JobStatus({job}: {job: {status: string}}) {
  const styles: Record<string, string> = {
    success: 'bg-emerald-100/70 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
    partial: 'bg-amber-100/70 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
    failed: 'bg-red-100/70 text-red-700 dark:bg-red-900/30 dark:text-red-400',
    running: 'bg-sky-100/70 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400',
  }
  const labels: Record<string, string> = {
    success: 'Success',
    partial: 'Partial',
    failed: 'Failed',
    running: 'Running',
  }
  return (
    <span className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold ${styles[job.status] ?? ''}`}>
      {labels[job.status] ?? job.status}
    </span>
  )
}

function PassphraseDialog({
  changing,
  onClose,
  onSaved,
}: {
  changing: boolean
  onClose: () => void
  onSaved: () => void
}) {
  const [value, setValue] = useState('')
  const [confirm, setConfirm] = useState('')
  const [errors, setErrors] = useState<string[]>([])
  const [pending, setPending] = useState(false)
  const [revealed, setRevealed] = useState(false)

  const generate = () => {
    const next = generatePassphrase()
    setValue(next)
    setConfirm(next)
    setRevealed(true)
  }

  const handleSave = async () => {
    if (!value) {
      setErrors(['Enter or generate a passphrase first.'])
      return
    }
    if (value !== confirm) {
      setErrors(['The confirmation does not match.'])
      return
    }
    setPending(true)
    setErrors([])
    try {
      await setSecret(BACKUP_PASSPHRASE_KEY, value)
      Toast.toast.success(changing ? 'Passphrase updated' : 'Passphrase stored', {
        description: changing
          ? 'New backups use it; older archives still need the old passphrase.'
          : 'It lives in the vault; scheduled backups use it automatically.',
      })
      onSaved()
    } catch (err) {
      setErrors(apiErrorDetails(err))
    }
    setPending(false)
  }

  return (
    <AppDialog isOpen onOpenChange={(open) => !open && onClose()} size="md">
      <Modal.Header className={dialogHeaderCls}>
        <div>
          <Modal.Heading className="text-lg font-semibold">
            {changing ? 'Change archive passphrase' : 'Archive passphrase'}
          </Modal.Heading>
          <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
            {changing
              ? 'Existing archives stay encrypted with the old passphrase — you still need it to restore them.'
              : 'Encrypts every backup archive, local and remote.'}
          </p>
        </div>
        <Modal.CloseTrigger aria-label="Close passphrase dialog" isDisabled={pending} />
      </Modal.Header>
      <Modal.Body className={dialogBodyCls}>
        <div className="space-y-3">
          {changing ? (
            <div className="rounded-xl border border-amber-300/70 bg-amber-50/80 px-3 py-2.5 text-xs text-amber-800 dark:border-amber-700/60 dark:bg-amber-950/40 dark:text-amber-200">
              Changing the passphrase does not re-encrypt backups you already have. Keep the old
              passphrase until every archive made with it has been restored or safely discarded —
              and store the new one in your password manager now.
            </div>
          ) : (
            <div className="rounded-xl border border-amber-300/70 bg-amber-50/80 px-3 py-2.5 text-xs text-amber-800 dark:border-amber-700/60 dark:bg-amber-950/40 dark:text-amber-200">
              Losing this passphrase means losing your backups — nothing can decrypt them without
              it. Copy it into your password manager before continuing.
            </div>
          )}
          <Field label="Passphrase">
            <div className="flex gap-2">
              <TextInput
                aria-label="Archive passphrase"
                type={revealed ? 'text' : 'password'}
                autoComplete="off"
                value={value}
                onChange={(e) => setValue(e.target.value)}
              />
              <button type="button" className={pillBtn} onClick={generate}>
                Generate
              </button>
            </div>
          </Field>
          <Field label="Confirm passphrase">
            <TextInput
              aria-label="Confirm passphrase"
              type={revealed ? 'text' : 'password'}
              autoComplete="off"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
            />
          </Field>
          <FormErrors errors={errors} title="Cannot save the passphrase" />
        </div>
      </Modal.Body>
      <Modal.Footer className={dialogFooterCls}>
        <button type="button" className={pillBtn} onClick={onClose} disabled={pending}>
          Cancel
        </button>
        <button type="button" className={primaryBtn} disabled={pending || !value} onClick={handleSave}>
          {pending ? 'Saving…' : changing ? 'Update passphrase' : 'Store in vault'}
        </button>
      </Modal.Footer>
    </AppDialog>
  )
}
