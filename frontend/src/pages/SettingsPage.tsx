// pages/SettingsPage.tsx — the settings surface: appearance (theme + accent,
// both persisted shell stores), daemon startup/reliability toggles, and the
// secrets vault manager (names in, values go to the daemon's encrypted store
// and never come back).
import {useEffect, useRef, useState} from 'react'
import type {ReactNode} from 'react'
import {useQuery, useQueryClient, useMutation} from '@tanstack/react-query'
import {useSearchParams} from 'react-router-dom'
import {
  apiErrorDetails,
  listSecrets,
  setSecret,
  deleteSecret,
  startupEnabled,
  startupSet,
  watchdogEnabled,
  watchdogSet,
  getDataDir,
  getTasksDir,
  openDataDir,
  getSettings,
  updateSettings,
  previewSound,
  pauseScheduler,
  resumeScheduler,
} from '../lib/api'
import {useHealth} from '../lib/query'
import {SECRETS_KEY, isBackupSecret, backupSecretWarning} from '../lib/secrets'
import {useTheme, LIGHT_VARIANTS, DARK_VARIANTS} from '../lib/theme'
import type {ThemeVariant} from '../lib/theme'
import {useAccent, ACCENT_COLORS, ACCENT_PRESETS} from '../lib/accent'
import type {Accent} from '../lib/accent'
import type {ThemeChoice} from '../lib/theme'
import {useAnimations} from '../lib/animations'
import {Field, SelectField, TextInput, pillBtn, primaryBtn} from '../components/controls'
import {Switch, Tabs, Modal} from '@heroui/react'
import {AppDialog, dialogHeaderCls, dialogBodyCls, dialogFooterCls} from '../components/AppDialog'
import {BackupSection} from '../components/settings/BackupSection'

type SettingsTab = 'appearance' | 'data' | 'backup' | 'startup' | 'reliability' | 'retention' | 'notifications' | 'secrets'

const SETTINGS_TABS: Array<{id: SettingsTab; label: string; detail: string}> = [
  {id: 'appearance', label: 'Appearance', detail: 'Theme, accent, and motion'},
  {id: 'data', label: 'Data', detail: 'Local storage locations'},
  {id: 'backup', label: 'Backup', detail: 'Automatic backups & restore'},
  {id: 'startup', label: 'Startup', detail: 'Launch with your system'},
  {id: 'reliability', label: 'Reliability', detail: 'Scheduler and watchdog'},
  {id: 'retention', label: 'Retention', detail: 'Run history lifetime'},
  {id: 'notifications', label: 'Notifications', detail: 'Sounds and previews'},
  {id: 'secrets', label: 'Secrets', detail: 'Encrypted credentials'},
]

function useNarrowSettings() {
  const query = '(max-width: 767px)'
  const [narrow, setNarrow] = useState(() => window.matchMedia?.(query).matches ?? false)
  useEffect(() => {
    const media = window.matchMedia?.(query)
    if (!media) return
    const update = () => setNarrow(media.matches)
    update()
    media.addEventListener?.('change', update)
    return () => media.removeEventListener?.('change', update)
  }, [])
  return narrow
}

export function SettingsPage() {
  // The active tab lives in the URL (?tab=…) so /secrets and /backups can
  // link back to the exact section they came from, and refreshes keep it.
  const [searchParams, setSearchParams] = useSearchParams()
  const raw = searchParams.get('tab')
  const activeTab: SettingsTab = SETTINGS_TABS.some((t) => t.id === raw)
    ? (raw as SettingsTab)
    : 'appearance'
  const selectTab = (tab: SettingsTab) => setSearchParams({tab}, {replace: true})
  const narrow = useNarrowSettings()
  const panels: Record<SettingsTab, ReactNode> = {
    appearance: <AppearanceSection />,
    data: <DataDirSection />,
    backup: <BackupSection />,
    startup: <StartupSection />,
    reliability: <ReliabilitySection />,
    retention: <RetentionSection />,
    notifications: <NotificationSection />,
    secrets: <SecretsSection />,
  }

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div>
        <h2 className="text-lg font-semibold">Settings</h2>
        <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">Shape how Heka looks, starts, stores data, and protects credentials.</p>
      </div>
      <Tabs
        orientation={narrow ? 'horizontal' : 'vertical'}
        selectedKey={activeTab}
        onSelectionChange={(key) => selectTab(key as SettingsTab)}
        className="grid items-start gap-4 md:grid-cols-[13rem_minmax(0,1fr)]"
      >
        <div className="self-start overflow-x-auto rounded-2xl border border-zinc-200/80 bg-white/45 p-1.5 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-950/25 md:sticky md:top-6 md:overflow-visible">
          <Tabs.List aria-label="Settings sections" className="flex min-w-max gap-1.5 md:min-w-0 md:flex-col md:items-stretch">
            {SETTINGS_TABS.map((tab) => (
              <Tabs.Tab
                key={tab.id}
                id={tab.id}
                className="group relative flex min-h-12 min-w-40 flex-none items-center justify-start rounded-xl px-3.5 py-2 text-left outline-none transition-colors data-[hovered=true]:bg-white/65 data-[selected=true]:bg-white/90 data-[selected=true]:shadow-sm data-[focus-visible=true]:ring-2 data-[focus-visible=true]:ring-accent-ring dark:data-[hovered=true]:bg-zinc-900/65 dark:data-[selected=true]:bg-zinc-900/90 md:min-w-0 md:items-stretch md:px-3"
              >
                <span className="flex min-w-0 flex-col gap-0.5">
                  <span className="block truncate text-sm font-semibold leading-tight">{tab.label}</span>
                  <span className="block truncate text-[11px] font-normal leading-tight text-zinc-500 dark:text-zinc-400">{tab.detail}</span>
                </span>
                <Tabs.Indicator className="absolute bottom-1 left-3 right-3 h-0.5 rounded-full bg-accent md:bottom-2 md:left-0 md:right-auto md:top-2 md:h-auto md:w-0.5" />
              </Tabs.Tab>
            ))}
          </Tabs.List>
        </div>
        <Tabs.Panel id={activeTab} className="min-w-0 rounded-2xl border border-zinc-200/80 bg-white/45 p-4 shadow-sm backdrop-blur-sm outline-none dark:border-zinc-800 dark:bg-zinc-950/25 sm:p-5">
          {panels[activeTab]}
        </Tabs.Panel>
      </Tabs>
    </div>
  )
}

const VARIANT_LABELS: Record<ThemeVariant, string> = {
  'khaki': 'Khaki',
  'crt': 'CRT',
  'gradient': 'Gradient',
  'high-contrast': 'High Contrast',
}

function AppearanceSection() {
  const {choice, effectiveVariant, setTheme, setVariant} = useTheme()
  const {accent, customColor, setAccent, setCustomColor} = useAccent()
  const {enabled: animationsOn, setEnabled: setAnimations} = useAnimations()

  // Show mode-specific variants
  const variants = choice === 'dark' || (choice === 'system' && window.matchMedia?.('(prefers-color-scheme: dark)')?.matches)
    ? DARK_VARIANTS
    : LIGHT_VARIANTS
  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
        Appearance
      </h3>
      <div className="flex flex-wrap items-center gap-3">
        <Field label="Theme">
          <SelectField
            aria-label="Theme"
            value={choice}
            onChange={(v) => setTheme(v as ThemeChoice)}
            className="w-40"
            items={[
              {id: 'light', label: 'Light'},
              {id: 'dark', label: 'Dark'},
              {id: 'system', label: 'System'},
            ]}
          />
        </Field>
        <Field label="Style">
          <SelectField
            aria-label="Theme variant"
            value={effectiveVariant}
            onChange={(v) => setVariant(v as ThemeVariant)}
            className="w-40"
            items={variants.map((v) => ({id: v, label: VARIANT_LABELS[v]}))}
          />
        </Field>
        <div className="flex items-center gap-2 pt-5">
          {ACCENT_PRESETS.map((name) => (
            <button
              key={name}
              type="button"
              aria-label={`Accent ${name}`}
              aria-pressed={accent === name}
              onClick={() => setAccent(name as Accent)}
              data-accent-swatch={name}
              title={name}
              className={`size-6 rounded-full outline-none transition-transform focus-visible:ring-2 focus-visible:ring-accent-ring ${
                accent === name
                  ? 'scale-110 ring-2 ring-zinc-400 dark:ring-zinc-500'
                  : ''
              }`}
              style={{background: ACCENT_COLORS[name]}}
            />
          ))}
          <label
            className={`ml-1 flex cursor-pointer items-center gap-2 rounded-full border px-3 py-1 text-xs outline-none focus-within:ring-2 focus-within:ring-accent-ring ${
              accent === 'custom'
                ? 'border-accent text-zinc-900 dark:text-zinc-100'
                : 'border-zinc-200 dark:border-zinc-700'
            }`}
          >
            <input
              type="color"
              aria-label="Custom accent color"
              value={accent === 'custom' ? customColor : undefined}
              onChange={(e) => setCustomColor(e.target.value)}
              className="h-5 w-6 cursor-pointer rounded border-0 bg-transparent p-0"
            />
            Custom
          </label>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex items-center justify-between rounded-xl border border-zinc-200/80 bg-white/60 px-4 py-3 dark:border-zinc-800 dark:bg-zinc-900/50">
          <div>
            <p className="text-sm font-medium text-zinc-800 dark:text-zinc-200">
              Animations
            </p>
            <p className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">
              Enable motion effects and transitions
            </p>
          </div>
          <Switch
            isSelected={animationsOn}
            onChange={setAnimations}
            aria-label="Toggle animations"
          >
            <Switch.Content>
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
            </Switch.Content>
          </Switch>
        </div>
      </div>
    </section>
  )
}

const DATA_DIRS_KEY = ['data-dirs'] as const

function DataDirSection() {
  const dirs = useQuery({
    queryKey: DATA_DIRS_KEY,
    queryFn: async () => ({data: await getDataDir(), tasks: await getTasksDir()}),
  })
  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Data</h3>
      <div className="rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
        <div className="space-y-2">
          <div>
            <div className="text-[11px] font-medium text-zinc-500 dark:text-zinc-400">Data directory</div>
            <code className="mt-0.5 block truncate font-mono text-xs text-zinc-700 dark:text-zinc-200">
              {dirs.data?.data ?? '—'}
            </code>
          </div>
          <div>
            <div className="text-[11px] font-medium text-zinc-500 dark:text-zinc-400">Tasks directory</div>
            <code className="mt-0.5 block truncate font-mono text-xs text-zinc-700 dark:text-zinc-200">
              {dirs.data?.tasks ?? '—'}
            </code>
          </div>
        </div>
        <button
          type="button"
          onClick={() => openDataDir()}
          className="mt-3 inline-flex items-center gap-1.5 rounded-full border border-zinc-200/80 bg-white/80 px-3 py-1 text-xs font-medium shadow-sm transition-colors hover:border-accent hover:text-accent dark:border-zinc-700/60 dark:bg-zinc-900/70"
        >
          <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
          </svg>
          Open data directory
        </button>
      </div>
    </section>
  )
}

function StartupSection() {
  const qc = useQueryClient()
  const enabled = useQuery({queryKey: ['startup'], queryFn: startupEnabled})
  const [error, setError] = useState<string | null>(null)

  const toggle = useMutation({
    mutationFn: (on: boolean) => startupSet(on),
    onSuccess: () => {
      setError(null)
      void qc.invalidateQueries({queryKey: ['startup']})
    },
    onError: (err) => setError(apiErrorDetails(err).join('; ')),
  })

  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
        Startup
      </h3>
      <ToggleRow
        label="Start with system"
        hint="Register the daemon to start when you log in"
        checked={enabled.data ?? false}
        disabled={enabled.isLoading || toggle.isPending}
        onChange={(on) => { setError(null); toggle.mutate(on) }}
      />
      {error && (
        <p className="text-xs text-red-600 dark:text-red-400">{error}</p>
      )}
    </section>
  )
}

function ReliabilitySection() {
  const qc = useQueryClient()
  const status = useQuery({queryKey: ['watchdog'], queryFn: watchdogEnabled})
  const settings = useQuery({queryKey: SETTINGS_KEY, queryFn: getSettings})
  const health = useHealth()
  const paused = health.data?.scheduler === 'paused'
  const [interval, setInterval] = useState(2)
  const [wdMins, setWdMins] = useState(5)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const initRef = useRef(false)

  useEffect(() => {
    if (settings.data && !initRef.current) {
      initRef.current = true
      setInterval(settings.data.reconcile_interval_min ?? 2)
      setWdMins(settings.data.watchdog_interval_min ?? 5)
    }
  }, [settings.data])

  const pauseToggle = useMutation({
    mutationFn: (pause: boolean) => (pause ? pauseScheduler() : resumeScheduler()),
    onSuccess: () => {
      setError(null)
      void qc.invalidateQueries({queryKey: ['health']})
    },
    onError: (err) => setError(apiErrorDetails(err).join('; ')),
  })

  const toggle = useMutation({
    mutationFn: (on: boolean) => watchdogSet(on),
    onSuccess: () => {
      setError(null)
      void qc.invalidateQueries({queryKey: ['watchdog']})
    },
    onError: (err) => setError(apiErrorDetails(err).join('; ')),
  })

  const saveInterval = useMutation({
    mutationFn: (mins: {reconcile: number; watchdog: number}) => updateSettings({
      log_retention_days: settings.data?.log_retention_days ?? 90,
      sound_success: settings.data?.sound_success ?? 'system',
      sound_failure: settings.data?.sound_failure ?? 'system',
      sound_timeout: settings.data?.sound_timeout ?? 'system',
      reconcile_interval_min: mins.reconcile,
      watchdog_interval_min: mins.watchdog,
    }),
    onSuccess: () => {
      qc.invalidateQueries({queryKey: SETTINGS_KEY})
      qc.invalidateQueries({queryKey: ['watchdog']})
      setSaved(true)
      setTimeout(() => setSaved(false), 1500)
    },
    onError: (err) => setError(apiErrorDetails(err).join('; ')),
  })

  const installed = status.data?.installed ?? false
  const wdInterval = status.data?.intervalMinutes ?? 5
  const reconcileCurrent = settings.data?.reconcile_interval_min ?? 2
  const watchdogCurrent = settings.data?.watchdog_interval_min ?? 5
  const dirty = interval !== reconcileCurrent || wdMins !== watchdogCurrent

  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
        Reliability
      </h3>
      <ToggleRow
        label="Pause scheduler"
        hint={
          paused
            ? 'Paused — schedules are skipped; resuming catches up missed runs'
            : 'Temporarily skip all scheduled runs; missed runs catch up on resume'
        }
        checked={paused}
        disabled={health.isLoading || pauseToggle.isPending}
        onChange={(pause) => { setError(null); pauseToggle.mutate(pause) }}
      />
      <ToggleRow
        label="Watchdog guard"
        hint={
          installed
            ? `Checks every ${wdInterval}m — restarts the daemon if it goes down`
            : 'Periodically checks the daemon and restarts it if it goes down'
        }
        checked={installed}
        disabled={status.isLoading || toggle.isPending}
        onChange={(on) => { setError(null); toggle.mutate(on) }}
      />
      <div className="rounded-xl border border-zinc-200/80 bg-white/60 px-4 py-3 dark:border-zinc-800 dark:bg-zinc-900/50">
        <div className="flex flex-wrap items-center gap-3">
          <Field label="Missed-run reconciliation">
            <SelectField
              aria-label="Missed-run reconciliation interval"
              value={String(interval)}
              onChange={(v) => setInterval(parseInt(v, 10))}
              className="w-40"
              items={RECONCILE_OPTIONS}
            />
          </Field>
          {installed && (
            <Field label="Watchdog check">
              <SelectField
                aria-label="Watchdog check interval"
                value={String(wdMins)}
                onChange={(v) => setWdMins(parseInt(v, 10))}
                className="w-40"
                items={WATCHDOG_OPTIONS}
              />
            </Field>
          )}
          <button
            type="button"
            disabled={saveInterval.isPending || !dirty}
            onClick={() => saveInterval.mutate({reconcile: interval, watchdog: wdMins})}
            className={`mt-5 ml-auto rounded-full px-3 py-1 text-xs font-medium shadow-sm transition-opacity ${
              saved
                ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                : 'bg-accent text-accent-contrast hover:opacity-90'
            } disabled:opacity-50`}
          >
            {saved ? 'Saved' : 'Save'}
          </button>
        </div>
        <p className="mt-1.5 text-xs text-zinc-500 dark:text-zinc-400">
          How often the daemon checks for missed recurring schedule activations
          (e.g. after the PC was off or sleeping). The watchdog cadence
          recreates the OS task so crashes are caught sooner.
        </p>
      </div>
      {error && (
        <p className="text-xs text-red-600 dark:text-red-400">{error}</p>
      )}
    </section>
  )
}

const RECONCILE_OPTIONS = [
  {id: '2', label: 'Every 2 minutes'},
  {id: '5', label: 'Every 5 minutes'},
  {id: '8', label: 'Every 8 minutes'},
  {id: '10', label: 'Every 10 minutes'},
]

const WATCHDOG_OPTIONS = [
  {id: '1', label: 'Every minute'},
  {id: '2', label: 'Every 2 minutes'},
  {id: '5', label: 'Every 5 minutes'},
  {id: '10', label: 'Every 10 minutes'},
  {id: '15', label: 'Every 15 minutes'},
  {id: '30', label: 'Every 30 minutes'},
  {id: '60', label: 'Every hour'},
]

function ToggleRow({
  label,
  hint,
  checked,
  disabled,
  onChange,
}: {
  label: string
  hint?: string
  checked: boolean
  disabled?: boolean
  onChange: (on: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between rounded-xl border border-zinc-200/80 bg-white/60 px-4 py-3 dark:border-zinc-800 dark:bg-zinc-900/50">
      <div>
        <p className="text-sm font-medium text-zinc-800 dark:text-zinc-200">
          {label}
        </p>
        {hint && (
          <p className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">
            {hint}
          </p>
        )}
      </div>
      <Switch
        isSelected={checked}
        isDisabled={disabled}
        onChange={onChange}
        aria-label={label}
      >
        <Switch.Content>
          <Switch.Control>
            <Switch.Thumb />
          </Switch.Control>
        </Switch.Content>
      </Switch>
    </div>
  )
}

const SETTINGS_KEY = ['settings'] as const

function RetentionSection() {
  const qc = useQueryClient()
  const settings = useQuery({queryKey: SETTINGS_KEY, queryFn: getSettings})
  const [days, setDays] = useState(90)
  const [saved, setSaved] = useState(false)
  const initialized = useRef(false)

  useEffect(() => {
    if (settings.data && !initialized.current) {
      initialized.current = true
      setDays(settings.data.log_retention_days)
    }
  }, [settings.data])

  const mutation = useMutation({
    mutationFn: (v: number) => updateSettings({
      log_retention_days: v,
      sound_success: settings.data?.sound_success ?? 'system',
      sound_failure: settings.data?.sound_failure ?? 'system',
      sound_timeout: settings.data?.sound_timeout ?? 'system',
      reconcile_interval_min: settings.data?.reconcile_interval_min ?? 2,
      watchdog_interval_min: settings.data?.watchdog_interval_min ?? 5,
    }),
    onSuccess: () => {
      qc.invalidateQueries({queryKey: SETTINGS_KEY})
      setSaved(true)
      setTimeout(() => setSaved(false), 1500)
    },
  })

  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Retention</h3>
      <div className="rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
        <Field label="Log retention (days)">
          <div className="flex items-center gap-2">
            <input
              type="number"
              min={1}
              value={days}
              onChange={(e) => setDays(Math.max(1, parseInt(e.target.value) || 1))}
              className="w-20 rounded-lg border border-zinc-200 bg-white px-2.5 py-1.5 text-xs font-medium outline-none transition-colors focus:border-accent focus:ring-1 focus:ring-accent/30 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-100"
            />
            <span className="text-[11px] text-zinc-400">days</span>
            <button
              type="button"
              disabled={mutation.isPending || days === settings.data?.log_retention_days}
              onClick={() => mutation.mutate(days)}
              className={`ml-auto rounded-full px-3 py-1 text-xs font-medium shadow-sm transition-opacity ${
                saved
                  ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                  : 'bg-accent text-accent-contrast hover:opacity-90'
              } disabled:opacity-50`}
            >
              {saved ? 'Saved' : 'Save'}
            </button>
          </div>
        </Field>
        <p className="mt-2 text-[11px] text-zinc-400 dark:text-zinc-500">
          Run history older than this is automatically pruned. Requires daemon restart to take full effect.
        </p>
      </div>
    </section>
  )
}

const SOUND_OPTIONS = [
  {id: 'system', label: 'System default'},
  {id: 'silent', label: 'Silent'},
  {id: 'chime', label: 'Chime'},
  {id: 'alert', label: 'Alert'},
  {id: 'bell', label: 'Bell'},
]

function NotificationSection() {
  const qc = useQueryClient()
  const settings = useQuery({queryKey: SETTINGS_KEY, queryFn: getSettings})
  const [success, setSuccess] = useState('system')
  const [failure, setFailure] = useState('system')
  const [timeoutSound, setTimeoutSound] = useState('system')
  const [saved, setSaved] = useState(false)
  const initialized = useRef(false)

  useEffect(() => {
    if (settings.data && !initialized.current) {
      initialized.current = true
      setSuccess(settings.data.sound_success ?? 'system')
      setFailure(settings.data.sound_failure ?? 'system')
      setTimeoutSound(settings.data.sound_timeout ?? 'system')
    }
  }, [settings.data])

  const mutation = useMutation({
    mutationFn: (s: {sound_success: string; sound_failure: string; sound_timeout: string}) =>
      updateSettings({
        log_retention_days: settings.data?.log_retention_days ?? 90,
        reconcile_interval_min: settings.data?.reconcile_interval_min ?? 2,
        watchdog_interval_min: settings.data?.watchdog_interval_min ?? 5,
        ...s,
      }),
    onSuccess: () => {
      qc.invalidateQueries({queryKey: SETTINGS_KEY})
      setSaved(true)
      setTimeout(() => setSaved(false), 1500)
    },
  })

  const [previewing, setPreviewing] = useState<string | null>(null)

  const handlePreview = async (preset: string) => {
    setPreviewing(preset)
    try {
      await previewSound(preset)
    } catch {
      // Preview errors are non-critical
    }
    setPreviewing(null)
  }

  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
        Notification Sounds
      </h3>
      <div className="rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
        <div className="space-y-3">
          <SoundRow
            label="Success sound"
            value={success}
            onChange={setSuccess}
            onPreview={() => handlePreview(success)}
            isPreviewing={previewing === success}
          />
          <SoundRow
            label="Failure sound"
            value={failure}
            onChange={setFailure}
            onPreview={() => handlePreview(failure)}
            isPreviewing={previewing === failure}
          />
          <SoundRow
            label="Timeout sound"
            value={timeoutSound}
            onChange={setTimeoutSound}
            onPreview={() => handlePreview(timeoutSound)}
            isPreviewing={previewing === timeoutSound}
          />
        </div>
        <div className="mt-3 flex items-center gap-2">
          <button
            type="button"
            disabled={
              mutation.isPending ||
              (success === (settings.data?.sound_success ?? 'system') &&
                failure === (settings.data?.sound_failure ?? 'system') &&
                timeoutSound === (settings.data?.sound_timeout ?? 'system'))
            }
            onClick={() => mutation.mutate({sound_success: success, sound_failure: failure, sound_timeout: timeoutSound})}
            className={`rounded-full px-3 py-1 text-xs font-medium shadow-sm transition-opacity ${
              saved
                ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                : 'bg-accent text-accent-contrast hover:opacity-90'
            } disabled:opacity-50`}
          >
            {saved ? 'Saved' : 'Save'}
          </button>
        </div>
      </div>
    </section>
  )
}

function SoundRow({
  label,
  value,
  onChange,
  onPreview,
  isPreviewing,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  onPreview: () => void
  isPreviewing: boolean
}) {
  return (
    <div className="flex items-center gap-3">
      <Field label={label}>
        <SelectField
          aria-label={label}
          value={value}
          onChange={onChange}
          className="w-44"
          items={SOUND_OPTIONS}
        />
      </Field>
      <button
        type="button"
        aria-label={`Preview ${label}`}
        onClick={onPreview}
        disabled={isPreviewing || value === 'silent'}
        className="mt-5 inline-flex size-8 shrink-0 items-center justify-center rounded-full border border-zinc-200/80 bg-white/80 text-zinc-500 shadow-sm transition-colors hover:border-accent hover:text-accent focus-visible:ring-2 focus-visible:ring-accent-ring dark:border-zinc-700/60 dark:bg-zinc-900/70 dark:text-zinc-400 disabled:opacity-50"
      >
        {isPreviewing ? (
          <svg className="size-4 animate-spin" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
        ) : (
          <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M11 5L6 9H2v6h4l5 4V5z" />
            <path d="M19.07 4.93a10 10 0 010 14.14M15.54 8.46a5 5 0 010 7.07" />
          </svg>
        )}
      </button>
    </div>
  )
}

function SecretsSection() {
  const qc = useQueryClient()
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const [errors, setErrors] = useState<string[]>([])
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)

  const keys = useQuery({queryKey: SECRETS_KEY, queryFn: listSecrets})
  const add = useMutation({
    mutationFn: ({key, value}: {key: string; value: string}) =>
      setSecret(key, value),
    onSuccess: () => {
      setKey('')
      setValue('')
      setErrors([])
      void qc.invalidateQueries({queryKey: SECRETS_KEY})
    },
    onError: (err) => setErrors(apiErrorDetails(err)),
  })

  const remove = useMutation({
    mutationFn: (key: string) => deleteSecret(key),
    onSuccess: () => void qc.invalidateQueries({queryKey: SECRETS_KEY}),
    onError: (err) => setErrors(apiErrorDetails(err)),
  })

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
          Secrets
        </h3>
        <a
          href="#/secrets"
          className="inline-flex items-center gap-1.5 rounded-full border border-zinc-200/80 bg-white/80 px-3 py-1 text-xs font-medium shadow-sm transition-colors hover:border-accent hover:text-accent dark:border-zinc-700/60 dark:bg-zinc-900/70"
        >
          Browse &amp; manage all secrets
          <svg aria-hidden viewBox="0 0 16 16" className="size-3 fill-current">
            <path d="M4.5 3 8 6.5 11.5 3 13 4.5 8 9.5 3 4.5z" transform="rotate(-90 8 6.5)" />
          </svg>
        </a>
      </div>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">
        Values are encrypted at rest and injected at run time. Reference them
        in tasks as <code className="rounded bg-zinc-100 px-1 py-0.5 dark:bg-zinc-800">{'${KEY}'}</code> in
        environment or webhook fields. Keys are the only thing shown here.
      </p>

      <div className="flex gap-2">
        <Field label="Key">
          <TextInput
            aria-label="Secret key"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="OPENROUTER_API_KEY"
            className="w-56"
          />
        </Field>
        <Field label="Value">
          <TextInput
            aria-label="Secret value"
            type="password"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="sk-…"
            className="w-64"
          />
        </Field>
        <button
          type="button"
          className={`${pillBtn} mt-5`}
          disabled={!key || add.isPending}
          onClick={() => add.mutate({key, value})}
        >
          Add
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

      {keys.isLoading ? (
        <p className="text-xs text-zinc-400">Loading…</p>
      ) : keys.data?.length === 0 ? (
        <p className="text-xs text-zinc-400 dark:text-zinc-500">
          No secrets yet — add one above.
        </p>
      ) : (
        <ul className="space-y-1.5" data-testid="secret-list">
          {(keys.data ?? []).map((k) => (
            <li
              key={k}
              className="flex items-center justify-between rounded-xl border border-zinc-200/80 bg-white/60 px-3 py-2 text-sm dark:border-zinc-800 dark:bg-zinc-900/50"
            >
              <span className="flex min-w-0 items-center gap-2">
                <code className="truncate font-mono text-xs text-zinc-700 dark:text-zinc-300">
                  {k}
                </code>
                {isBackupSecret(k) && (
                  <span
                    title="Managed by Heka's automatic backups"
                    className="shrink-0 rounded-full bg-accent/10 px-2 py-0.5 text-[11px] font-medium text-accent"
                  >
                    backup
                  </span>
                )}
              </span>
              <button
                type="button"
                aria-label={`Delete secret ${k}`}
                onClick={() => setConfirmDelete(k)}
                className="text-xs text-zinc-400 outline-none hover:text-red-500 focus-visible:ring-2 focus-visible:ring-accent-ring"
              >
                Delete
              </button>
            </li>
          ))}
        </ul>
      )}

      {/* Delete confirm — every key asks first; backup-managed keys get an
          extra warning because the daemon (not tasks) depends on them. */}
      {confirmDelete !== null && (
        <AppDialog isOpen onOpenChange={(open) => !open && setConfirmDelete(null)} size="sm">
          <Modal.Header className={dialogHeaderCls}>
            <div>
              <Modal.Heading className="text-lg font-semibold">Delete {confirmDelete}?</Modal.Heading>
              <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
                {isBackupSecret(confirmDelete)
                  ? 'This key is managed by Heka\'s automatic backups — the daemon uses it directly, tasks never reference it.'
                  : 'Tasks referencing it will fail to resolve it at run time.'}
              </p>
            </div>
            <Modal.CloseTrigger aria-label="Close delete confirmation" isDisabled={remove.isPending} />
          </Modal.Header>
          <Modal.Body className={dialogBodyCls}>
            <div className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950/60 dark:text-amber-300">
              {isBackupSecret(confirmDelete)
                ? backupSecretWarning(confirmDelete)
                : 'This cannot be undone. The key must be added again before tasks can use it.'}
            </div>
          </Modal.Body>
          <Modal.Footer className={dialogFooterCls}>
            <button
              type="button"
              className={pillBtn}
              onClick={() => setConfirmDelete(null)}
              disabled={remove.isPending}
            >
              Cancel
            </button>
            <button
              type="button"
              className={primaryBtn}
              disabled={remove.isPending}
              onClick={() => {
                const k = confirmDelete
                setConfirmDelete(null)
                remove.mutate(k)
              }}
            >
              {remove.isPending ? 'Deleting…' : 'Delete anyway'}
            </button>
          </Modal.Footer>
        </AppDialog>
      )}
    </section>
  )
}