// pages/SettingsPage.tsx — the settings surface: appearance (theme + accent,
// both persisted shell stores), daemon startup/reliability toggles, and the
// secrets vault manager (names in, values go to the daemon's encrypted store
// and never come back).
import {useState} from 'react'
import {useQuery, useQueryClient, useMutation} from '@tanstack/react-query'
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
} from '../lib/api'
import {useTheme} from '../lib/theme'
import {useAccent, ACCENT_COLORS, ACCENT_PRESETS} from '../lib/accent'
import type {Accent} from '../lib/accent'
import type {ThemeChoice} from '../lib/theme'
import {Field, SelectField, TextInput, pillBtn} from '../components/controls'

const SECRETS_KEY = ['secrets'] as const

export function SettingsPage() {
  return (
    <div className="mx-auto max-w-2xl space-y-8">
      <h2 className="text-lg font-semibold">Settings</h2>
      <AppearanceSection />
      <DataDirSection />
      <StartupSection />
      <ReliabilitySection />
      <RetentionSection />
      <SecretsSection />
    </div>
  )
}

function AppearanceSection() {
  const {choice, setTheme} = useTheme()
  const {accent, customColor, setAccent, setCustomColor} = useAccent()
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
  const [error, setError] = useState<string | null>(null)

  const toggle = useMutation({
    mutationFn: (on: boolean) => watchdogSet(on),
    onSuccess: () => {
      setError(null)
      void qc.invalidateQueries({queryKey: ['watchdog']})
    },
    onError: (err) => setError(apiErrorDetails(err).join('; ')),
  })

  const installed = status.data?.installed ?? false
  const interval = status.data?.intervalMinutes ?? 5

  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
        Reliability
      </h3>
      <ToggleRow
        label="Watchdog guard"
        hint={
          installed
            ? `Checks every ${interval}m — restarts the daemon if it goes down`
            : 'Periodically checks the daemon and restarts it if it goes down'
        }
        checked={installed}
        disabled={status.isLoading || toggle.isPending}
        onChange={(on) => { setError(null); toggle.mutate(on) }}
      />
      {error && (
        <p className="text-xs text-red-600 dark:text-red-400">{error}</p>
      )}
    </section>
  )
}

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
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-ring disabled:cursor-not-allowed disabled:opacity-50 ${
          checked
            ? 'bg-accent'
            : 'bg-zinc-300 dark:bg-zinc-600'
        }`}
      >
        <span
          className={`pointer-events-none block h-4 w-4 rounded-full bg-white shadow-sm transition-transform ${
            checked ? 'translate-x-6' : 'translate-x-1'
          }`}
        />
      </button>
    </div>
  )
}

const SETTINGS_KEY = ['settings'] as const

function RetentionSection() {
  const qc = useQueryClient()
  const settings = useQuery({queryKey: SETTINGS_KEY, queryFn: getSettings})
  const [days, setDays] = useState(90)
  const [saved, setSaved] = useState(false)
  const initialized = useState(false)

  // Sync from server once loaded.
  if (settings.data && !initialized[0]) {
    setDays(settings.data.log_retention_days)
    initialized[1](true)
  }

  const mutation = useMutation({
    mutationFn: (v: number) => updateSettings({log_retention_days: v}),
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

function SecretsSection() {
  const qc = useQueryClient()
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const [errors, setErrors] = useState<string[]>([])

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
      <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
        Secrets
      </h3>
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
              <code className="font-mono text-xs text-zinc-700 dark:text-zinc-300">
                {k}
              </code>
              <button
                type="button"
                aria-label={`Delete secret ${k}`}
                onClick={() => remove.mutate(k)}
                className="text-xs text-zinc-400 outline-none hover:text-red-500 focus-visible:ring-2 focus-visible:ring-accent-ring"
              >
                Delete
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}