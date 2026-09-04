import {useState} from 'react'
import {Field, TimePickerField} from '../controls'
import type {TimeValue} from '../../lib/schedulePattern'
import {sortTimes} from '../../lib/schedulePattern'

// Chip list of HH:MM times with an adder. Shared by the daily, weekly and
// monthly pattern panels.
export function TimesEditor({
  times,
  onChange,
  max = 8,
}: {
  times: TimeValue[]
  onChange: (times: TimeValue[]) => void
  max?: number
}) {
  const [pending, setPending] = useState('09:00')
  const [duplicate, setDuplicate] = useState(false)

  const add = () => {
    const [hh, mm] = pending.split(':')
    if (!hh || !mm) return
    const key = `${hh.padStart(2, '0')}:${mm.padStart(2, '0')}`
    if (times.some((t) => `${t.hh}:${t.mm}` === key)) {
      setDuplicate(true)
      return
    }
    setDuplicate(false)
    if (times.length >= max) return
    onChange(sortTimes([...times, {hh: key.slice(0, 2), mm: key.slice(3)}]))
  }

  const remove = (hh: string, mm: string) => {
    onChange(times.filter((t) => !(t.hh === hh && t.mm === mm)))
  }

  return (
    <Field label="Times" error={duplicate ? 'That time is already added.' : undefined}>
      <div className="flex flex-wrap items-center gap-2">
        {times.map((t) => (
          <span
            key={`${t.hh}:${t.mm}`}
            data-testid={`time-chip-${t.hh}:${t.mm}`}
            className="inline-flex items-center gap-1 rounded-lg border border-field-border bg-surface/70 py-1 pl-2.5 pr-1 font-mono text-xs text-foreground/80"
          >
            {t.hh}:{t.mm}
            <button
              type="button"
              aria-label={`Remove ${t.hh}:${t.mm}`}
              onClick={() => remove(t.hh, t.mm)}
              className="rounded p-0.5 text-foreground/45 outline-none hover:bg-red-100 hover:text-red-600 focus-visible:ring-2 focus-visible:ring-accent-ring dark:hover:bg-red-950/50 dark:hover:text-red-400"
            >
              <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                <path d="M18 6 6 18M6 6l12 12" />
              </svg>
            </button>
          </span>
        ))}
        {times.length < max && (
          <span className="flex items-center gap-1.5">
            <TimePickerField
              value={pending}
              onChange={(v) => {
                setPending(v)
                setDuplicate(false)
              }}
              aria-label="New time"
              className="w-28"
            />
            <button
              type="button"
              onClick={add}
              data-testid="add-time"
              className="rounded-lg border border-field-border bg-surface/70 px-2.5 py-1.5 text-xs font-medium text-foreground/70 outline-none transition-colors hover:border-accent/60 hover:text-accent focus-visible:ring-2 focus-visible:ring-accent-ring"
            >
              + Add
            </button>
          </span>
        )}
      </div>
    </Field>
  )
}
