// components/tasks/EnvEditor.tsx (SPEC-13 §4) — key/value environment rows.
// Values are picked from the vault (as ${KEY} references) with a literal
// fallback — secrets never sit in the YAML in plaintext. Deliberately NOT a
// <label>-wrapped Field: interactive rows inside a label foil accessible
// names.
import {RemoveRow, TextInput, pillBtn} from '../controls'
import {SecretValue} from './SecretValue'

export function EnvEditor({
  rows,
  onChange,
}: {
  rows: Array<[string, string]>
  onChange: (rows: Array<[string, string]>) => void
}) {
  const set = (i: number, next: [string, string]) => {
    onChange(rows.map((row, idx) => (idx === i ? next : row)))
  }
  return (
    <div>
      <div className="mb-1 text-xs font-medium text-zinc-500 dark:text-zinc-400">
        Environment
      </div>
      <p className="mb-2 text-xs text-zinc-400 dark:text-zinc-500">
        Values reference the vault (Settings → Secrets) or a literal string.
      </p>
      <div className="space-y-2">
        {rows.length === 0 && (
          <p className="text-xs text-zinc-400 dark:text-zinc-500">None set.</p>
        )}
        {rows.map(([key, value], i) => (
          <div key={i} className="grid grid-cols-[1fr_2fr_auto] items-center gap-2">
            <TextInput
              aria-label={`Environment key ${i + 1}`}
              placeholder="KEY"
              value={key}
              onChange={(e) => set(i, [e.target.value, value])}
            />
            <SecretValue
              ariaLabel={`Environment value ${i + 1}`}
              value={value}
              onChange={(v) => set(i, [key, v])}
            />
            <RemoveRow
              label={`Remove environment row ${i + 1}`}
              onClick={() => onChange(rows.filter((_, idx) => idx !== i))}
            />
          </div>
        ))}
        <button
          type="button"
          onClick={() => onChange([...rows, ['', '']])}
          className={pillBtn}
        >
          + Variable
        </button>
      </div>
    </div>
  )
}