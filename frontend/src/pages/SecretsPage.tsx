// pages/SecretsPage.tsx — the vault manager for large key sets: search,
// sort, usage tracking, bulk delete, and pagination. Keys are the only thing
// ever displayed — values are write-only (SPEC-11); the daemon injects them
// into task runs as ${KEY}.
import {useMemo, useState} from 'react'
import {useQuery, useQueryClient, useMutation} from '@tanstack/react-query'
import {Modal, Toast} from '@heroui/react'
import {listSecrets, setSecret, deleteSecret, getSecretsUsage, apiErrorDetails} from '../lib/api'
import {SECRETS_KEY, isBackupSecret, backupSecretWarning} from '../lib/secrets'
import {Field, FormErrors, TextInput, pillBtn, primaryBtn, SelectField} from '../components/controls'
import {AppDialog, dialogHeaderCls, dialogBodyCls, dialogFooterCls} from '../components/AppDialog'

const PAGE_SIZE = 50

export function SecretsPage() {
  const qc = useQueryClient()
  const keys = useQuery({queryKey: SECRETS_KEY, queryFn: listSecrets})
  const usage = useQuery({queryKey: ['secrets-usage'], queryFn: getSecretsUsage})

  const [search, setSearch] = useState('')
  const [sort, setSort] = useState('az')
  const [unusedOnly, setUnusedOnly] = useState(false)
  const [visible, setVisible] = useState(PAGE_SIZE)
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set())
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const [errors, setErrors] = useState<string[]>([])
  const [editKey, setEditKey] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')
  const [confirmBulk, setConfirmBulk] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)

  const allKeys = useMemo(() => keys.data ?? [], [keys.data])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    let list = allKeys
    if (q) list = list.filter((k) => k.toLowerCase().includes(q))
    if (unusedOnly) list = list.filter((k) => (usage.data?.[k]?.length ?? 0) === 0)
    if (sort === 'za') {
      list = [...list].sort((a, b) => b.localeCompare(a))
    } else {
      list = [...list].sort((a, b) => a.localeCompare(b))
    }
    return list
  }, [allKeys, search, sort, unusedOnly, usage.data])

  const invalidate = () => {
    void qc.invalidateQueries({queryKey: SECRETS_KEY})
    void qc.invalidateQueries({queryKey: ['secrets-usage']})
  }

  const add = useMutation({
    mutationFn: ({key, value}: {key: string; value: string}) => setSecret(key, value),
    onSuccess: () => {
      setKey('')
      setValue('')
      setErrors([])
      invalidate()
      Toast.toast.success('Secret stored', {description: 'Reference it in tasks as ${KEY}.'})
    },
    onError: (err) => setErrors(apiErrorDetails(err)),
  })

  const removeOne = useMutation({
    mutationFn: (k: string) => deleteSecret(k),
    onSuccess: () => {
      invalidate()
      Toast.toast.success('Secret deleted')
    },
    onError: (err) => setErrors(apiErrorDetails(err)),
  })

  const removeMany = useMutation({
    mutationFn: async (keys: string[]) => {
      for (const k of keys) {
        await deleteSecret(k)
      }
    },
    onSuccess: () => {
      setSelected(new Set())
      setConfirmBulk(false)
      invalidate()
      Toast.toast.success('Secrets deleted')
    },
    onError: (err) => setErrors(apiErrorDetails(err)),
  })

  const saveEdit = useMutation({
    mutationFn: ({key, value}: {key: string; value: string}) => setSecret(key, value),
    onSuccess: () => {
      setEditKey(null)
      setEditValue('')
      setErrors([])
      invalidate()
      Toast.toast.success('Secret updated')
    },
    onError: (err) => setErrors(apiErrorDetails(err)),
  })

  const toggleSelect = (k: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(k)) {
        next.delete(k)
      } else {
        next.add(k)
      }
      return next
    })
  }

  const validNewKey = /^[A-Za-z_][A-Za-z0-9_]*$/.test(key)

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Secrets</h2>
          <p className="mt-1 text-xs text-foreground/55">
            {allKeys.length} key{allKeys.length === 1 ? '' : 's'} in the vault · values are
            encrypted at rest and never shown — reference them in tasks as{' '}
            <code className="rounded bg-surface-secondary px-1 py-0.5">{'${KEY}'}</code>.
          </p>
        </div>
        <a
          href="#/settings?tab=secrets"
          className="inline-flex items-center gap-1.5 rounded-full border border-border/80 bg-surface/80 px-3 py-1 text-xs font-medium shadow-sm transition-colors hover:border-accent hover:text-accent"
        >
          Back to settings
        </a>
      </div>

      {/* Add */}
      <div className="rounded-2xl border border-border/80 bg-surface/70 p-4 shadow-sm backdrop-blur-sm">
        <div className="flex flex-wrap gap-2">
          <Field label="Key">
            <TextInput
              aria-label="Secret key"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder="OPENROUTER_API_KEY"
              className="w-56"
              aria-invalid={key !== '' && !validNewKey}
            />
          </Field>
          <Field label="Value">
            <TextInput
              aria-label="Secret value"
              type="password"
              autoComplete="off"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="sk-…"
              className="w-64"
            />
          </Field>
          <button
            type="button"
            className={`${pillBtn} mt-5`}
            disabled={!key || !validNewKey || add.isPending}
            onClick={() => add.mutate({key, value})}
          >
            Add
          </button>
        </div>
        {key !== '' && !validNewKey && (
          <p className="mt-2 text-xs text-red-600 dark:text-red-400">
            Keys must be letters, digits, and underscores, starting with a letter.
          </p>
        )}
        {errors.length > 0 && (
          <div className="mt-3">
            <FormErrors errors={errors} title="Vault operation failed" />
          </div>
        )}
      </div>

      {/* Browse */}
      <div className="rounded-2xl border border-border/80 bg-surface/70 p-4 shadow-sm backdrop-blur-sm">
        <div className="flex flex-wrap items-center gap-2">
          <TextInput
            aria-label="Search keys"
            placeholder="Search keys…"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value)
              setVisible(PAGE_SIZE)
            }}
            className="w-64"
          />
          <SelectField
            aria-label="Sort keys"
            className="w-36"
            value={sort}
            onChange={setSort}
            items={[
              {id: 'az', label: 'A → Z'},
              {id: 'za', label: 'Z → A'},
            ]}
          />
          <button
            type="button"
            aria-pressed={unusedOnly}
            onClick={() => setUnusedOnly((v) => !v)}
            className={`rounded-full border px-3 py-1.5 text-xs font-medium transition-colors ${
              unusedOnly
                ? 'border-accent bg-accent/10 text-accent'
                : 'border-border/80 bg-surface/80 text-foreground/75 hover:border-accent hover:text-accent'
            }`}
          >
            Unused only
          </button>
          {selected.size > 0 && (
            <button type="button" className={pillBtn} onClick={() => setConfirmBulk(true)}>
              Delete selected ({selected.size})
            </button>
          )}
        </div>

        {keys.isLoading || usage.isLoading ? (
          <p className="mt-3 text-xs text-foreground/50">Loading…</p>
        ) : filtered.length === 0 ? (
          <div className="mt-3 rounded-2xl border border-dashed border-field-border px-4 py-10 text-center text-sm text-foreground/50">
            {allKeys.length === 0
              ? 'No secrets yet — add one above.'
              : 'No keys match the current filters.'}
          </div>
        ) : (
          <ul className="mt-3 space-y-1.5" data-testid="secret-list">
            {filtered.slice(0, visible).map((k) => {
              const usedBy = usage.data?.[k] ?? []
              return (
                <li
                  key={k}
                  className="flex items-center justify-between gap-3 rounded-xl border border-border/80 bg-surface/60 px-3 py-2 text-sm"
                >
                  <label className="flex min-w-0 items-center gap-2.5">
                    <input
                      type="checkbox"
                      aria-label={`Select secret ${k}`}
                      className="accent-[var(--accent)]"
                      checked={selected.has(k)}
                      onChange={() => toggleSelect(k)}
                    />
                    <code className="truncate font-mono text-xs text-foreground/75">
                      {k}
                    </code>
                  </label>
                  <span className="flex shrink-0 items-center gap-2">
                    {isBackupSecret(k) && (
                      <span
                        title="Managed by Heka's automatic backups"
                        className="rounded-full bg-accent/10 px-2 py-0.5 text-[11px] font-medium text-accent"
                      >
                        backup
                      </span>
                    )}
                    {usedBy.length > 0 ? (
                      <span
                        title={`Used by: ${usedBy.join(', ')}`}
                        className="rounded-full bg-surface-secondary px-2 py-0.5 text-[11px] text-foreground/75"
                      >
                        used by {usedBy.length} task{usedBy.length === 1 ? '' : 's'}
                      </span>
                    ) : (
                      <span className="rounded-full bg-amber-100/70 px-2 py-0.5 text-[11px] text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
                        unused
                      </span>
                    )}
                    <button
                      type="button"
                      aria-label={`Set value for ${k}`}
                      onClick={() => {
                        setEditKey(k)
                        setEditValue('')
                      }}
                      className="text-xs text-foreground/50 outline-none hover:text-accent focus-visible:ring-2 focus-visible:ring-accent-ring"
                    >
                      Set value
                    </button>
                    <button
                      type="button"
                      aria-label={`Delete secret ${k}`}
                      onClick={() => setConfirmDelete(k)}
                      className="text-xs text-foreground/50 outline-none hover:text-red-500 focus-visible:ring-2 focus-visible:ring-accent-ring"
                    >
                      Delete
                    </button>
                  </span>
                </li>
              )
            })}
          </ul>
        )}

        {filtered.length > visible && (
          <div className="mt-3 text-center">
            <button
              type="button"
              className={pillBtn}
              onClick={() => setVisible((v) => v + PAGE_SIZE)}
            >
              Show more ({filtered.length - visible} remaining)
            </button>
          </div>
        )}
      </div>

      {/* Set value dialog */}
      {editKey !== null && (
        <AppDialog isOpen onOpenChange={(open) => !open && setEditKey(null)} size="sm">
          <Modal.Header className={dialogHeaderCls}>
            <div>
              <Modal.Heading className="text-lg font-semibold">Set value</Modal.Heading>
              <p className="mt-1 text-xs text-foreground/55">
                Replaces the stored value of <code className="font-mono">{editKey}</code>. The old
                value cannot be read back.
              </p>
            </div>
            <Modal.CloseTrigger aria-label="Close set-value dialog" isDisabled={saveEdit.isPending} />
          </Modal.Header>
          <Modal.Body className={dialogBodyCls}>
            <Field label="New value">
              <TextInput
                aria-label="New secret value"
                type="password"
                autoComplete="off"
                value={editValue}
                onChange={(e) => setEditValue(e.target.value)}
                autoFocus
              />
            </Field>
          </Modal.Body>
          <Modal.Footer className={dialogFooterCls}>
            <button type="button" className={pillBtn} onClick={() => setEditKey(null)} disabled={saveEdit.isPending}>
              Cancel
            </button>
            <button
              type="button"
              className={primaryBtn}
              disabled={!editValue || saveEdit.isPending}
              onClick={() => saveEdit.mutate({key: editKey, value: editValue})}
            >
              {saveEdit.isPending ? 'Saving…' : 'Save'}
            </button>
          </Modal.Footer>
        </AppDialog>
      )}

      {/* Bulk delete confirm */}
      {confirmBulk && (
        <AppDialog isOpen onOpenChange={(open) => !open && setConfirmBulk(false)} size="sm">
          <Modal.Header className={dialogHeaderCls}>
            <div>
              <Modal.Heading className="text-lg font-semibold">
                Delete {selected.size} secret{selected.size === 1 ? '' : 's'}?
              </Modal.Heading>
              <p className="mt-1 text-xs text-foreground/55">
                Tasks referencing them will fail to resolve {'${KEY}'} at run time.
              </p>
            </div>
            <Modal.CloseTrigger aria-label="Close bulk delete dialog" isDisabled={removeMany.isPending} />
          </Modal.Header>
          <Modal.Body className={dialogBodyCls}>
            <ul className="max-h-40 space-y-1 overflow-y-auto text-xs text-foreground/75">
              {[...selected].map((k) => (
                <li key={k} className="font-mono">
                  {k}
                </li>
              ))}
            </ul>
            {[...selected].some(isBackupSecret) && (
              <div className="mt-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950/60 dark:text-amber-300">
                {[...selected].filter(isBackupSecret).join(', ')} — managed by automatic backups:
                scheduled backups or S3 mirroring will fail until these keys are set up again in
                Settings → Backup.
              </div>
            )}
          </Modal.Body>
          <Modal.Footer className={dialogFooterCls}>
            <button type="button" className={pillBtn} onClick={() => setConfirmBulk(false)} disabled={removeMany.isPending}>
              Cancel
            </button>
            <button type="button" className={primaryBtn} disabled={removeMany.isPending} onClick={() => removeMany.mutate([...selected])}>
              {removeMany.isPending ? 'Deleting…' : 'Delete'}
            </button>
          </Modal.Footer>
        </AppDialog>
      )}

      {/* Delete confirm — every key asks first; backup-managed keys get an
          extra warning because the daemon (not tasks) depends on them. */}
      {confirmDelete !== null && (
        <AppDialog isOpen onOpenChange={(open) => !open && setConfirmDelete(null)} size="sm">
          <Modal.Header className={dialogHeaderCls}>
            <div>
              <Modal.Heading className="text-lg font-semibold">Delete {confirmDelete}?</Modal.Heading>
              <p className="mt-1 text-xs text-foreground/55">
                {isBackupSecret(confirmDelete)
                  ? 'This key is managed by Heka\'s automatic backups — the daemon uses it directly, tasks never reference it.'
                  : 'Tasks referencing it will fail to resolve it at run time.'}
              </p>
            </div>
            <Modal.CloseTrigger aria-label="Close delete confirmation" isDisabled={removeOne.isPending} />
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
              disabled={removeOne.isPending}
            >
              Cancel
            </button>
            <button
              type="button"
              className={primaryBtn}
              disabled={removeOne.isPending}
              onClick={() => {
                const k = confirmDelete
                setConfirmDelete(null)
                removeOne.mutate(k)
              }}
            >
              {removeOne.isPending ? 'Deleting…' : 'Delete anyway'}
            </button>
          </Modal.Footer>
        </AppDialog>
      )}
    </div>
  )
}
