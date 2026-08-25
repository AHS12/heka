// pages/SettingsPage.tsx — the settings surface: appearance (theme + accent,
// both persisted shell stores) and the secrets vault manager (names in,
// values go to the daemon's encrypted store and never come back).
import {useState} from 'react'
import {useQuery, useQueryClient, useMutation} from '@tanstack/react-query'
import {apiErrorDetails, listSecrets, setSecret, deleteSecret} from '../lib/api'
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