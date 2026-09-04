import {useState} from 'react'
import {Field, NumberInput, SelectField, TextInput, DateTimePickerField} from '../controls'
import {TimesEditor} from './TimesEditor'
import {cronFields, DAY_LABELS, MONTH_LABELS, validateCron} from '../../lib/cron'
import type {SchedulePattern} from '../../lib/schedulePattern'

type PanelProps = {
  pattern: SchedulePattern
  onChange: (pattern: SchedulePattern) => void
  error?: string
}

function Chip({
  selected,
  onClick,
  children,
  label,
  muted,
  className = '',
}: {
  selected: boolean
  onClick: () => void
  children: React.ReactNode
  label: string
  muted?: boolean
  className?: string
}) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      aria-label={label}
      onClick={onClick}
      className={`rounded-lg border px-2 py-1.5 text-xs font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-accent-ring ${
        className
      } ${
        selected
          ? 'border-accent bg-accent text-accent-contrast'
          : `border-field-border bg-surface/70 hover:border-accent/60 ${
              muted ? 'text-foreground/45' : 'text-foreground/70'
            }`
      }`}
    >
      {children}
    </button>
  )
}

function PresetButton({onClick, children}: {onClick: () => void; children: React.ReactNode}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="rounded-md px-1.5 py-0.5 text-[11px] text-foreground/50 outline-none transition-colors hover:bg-surface-secondary hover:text-accent focus-visible:ring-2 focus-visible:ring-accent-ring"
    >
      {children}
    </button>
  )
}

function EveryPanel({pattern, onChange, error}: PanelProps) {
  if (pattern.kind !== 'every') return null
  return (
    <div className="space-y-3">
      <div className="flex items-end gap-2">
        <Field label="Every" error={error}>
          <NumberInput
            aria-label="Interval amount"
            min={1}
            value={String(pattern.n)}
            onChange={(e) => onChange({...pattern, n: Number(e.target.value)})}
            className="w-24"
          />
        </Field>
        <Field label="Unit">
          <SelectField
            aria-label="Interval unit"
            value={pattern.unit}
            onChange={(unit) => onChange({...pattern, unit: unit as 'm' | 'h'})}
            className="w-36"
            items={[
              {id: 'm', label: 'Minutes'},
              {id: 'h', label: 'Hours'},
            ]}
          />
        </Field>
      </div>
      <p className="text-xs text-foreground/50">
        Runs on a fixed delay from the previous attempt — for aligned clock times (e.g. every 15 minutes past the
        hour), use Cron.
      </p>
    </div>
  )
}

function DailyPanel({pattern, onChange, error}: PanelProps) {
  if (pattern.kind !== 'daily') return null
  return (
    <div className="space-y-3">
      <TimesEditor times={pattern.times} onChange={(times) => onChange({...pattern, times})} />
      {error && <p className="text-xs font-medium text-red-600 dark:text-red-400">{error}</p>}
    </div>
  )
}

function WeeklyPanel({pattern, onChange, error}: PanelProps) {
  if (pattern.kind !== 'weekly') return null
  const toggle = (day: number) => {
    const days = pattern.days.includes(day)
      ? pattern.days.filter((d) => d !== day)
      : [...pattern.days, day].sort((a, b) => a - b)
    onChange({...pattern, days})
  }
  return (
    <div className="space-y-4">
      <Field label="Days of week" error={error}>
        <div className="flex flex-wrap items-center gap-1.5">
          {DAY_LABELS.map((label, day) => (
            <Chip key={day} selected={pattern.days.includes(day)} onClick={() => toggle(day)} label={label}>
              {label}
            </Chip>
          ))}
        </div>
      </Field>
      <div className="flex flex-wrap items-center gap-1">
        <span className="text-[11px] uppercase tracking-wider text-foreground/40">Presets</span>
        <PresetButton onClick={() => onChange({...pattern, days: [1, 2, 3, 4, 5]})}>Weekdays</PresetButton>
        <PresetButton onClick={() => onChange({...pattern, days: [0, 6]})}>Weekend</PresetButton>
        <PresetButton onClick={() => onChange({...pattern, days: [0, 1, 2, 3, 4, 5, 6]})}>Every day</PresetButton>
        <PresetButton onClick={() => onChange({...pattern, days: []})}>Clear</PresetButton>
      </div>
      <TimesEditor times={pattern.times} onChange={(times) => onChange({...pattern, times})} />
    </div>
  )
}

const DAY_NUMBERS = Array.from({length: 31}, (_, i) => i + 1)

function MonthlyPanel({pattern, onChange, error}: PanelProps) {
  if (pattern.kind !== 'monthly') return null
  const coveredDays = new Set<number>(pattern.days)
  for (const [a, b] of pattern.ranges) {
    for (let d = a; d <= b; d++) coveredDays.add(d)
  }
  const toggleDay = (day: number) => {
    if (coveredDays.has(day)) {
      onChange({
        ...pattern,
        days: pattern.days.filter((d) => d !== day),
        ranges: pattern.ranges.filter(([a, b]) => !(a <= day && day <= b)),
      })
    } else {
      onChange({...pattern, days: [...pattern.days, day].sort((a, b) => a - b)})
    }
  }
  const toggleMonth = (month: number) => {
    const months = pattern.months.includes(month)
      ? pattern.months.filter((m) => m !== month)
      : [...pattern.months, month].sort((a, b) => a - b)
    onChange({...pattern, months})
  }

  return (
    <div className="space-y-4">
      <Field label="Days of month" error={error}>
        <div className="grid w-fit grid-cols-7 gap-1.5">
          {DAY_NUMBERS.map((day) => (
            <Chip
              key={day}
              selected={coveredDays.has(day)}
              onClick={() => toggleDay(day)}
              label={`Day ${day}`}
              muted={day >= 29}
              className="w-9"
            >
              {day}
            </Chip>
          ))}
        </div>
      </Field>

      <RangeAdder ranges={pattern.ranges} onChange={(ranges) => onChange({...pattern, ranges})} />

      {coveredDays.size > 0 && DAY_NUMBERS.filter((d) => d >= 29).some((d) => coveredDays.has(d)) && (
        <p className="text-xs text-amber-600 dark:text-amber-400">
          Some months have no 29th, 30th or 31st — runs on missing dates are skipped.
        </p>
      )}
      {coveredDays.size === 0 && (
        <p className="text-xs text-foreground/50">No days selected — runs every day of the selected months.</p>
      )}

      <Field label="Months">
        <div className="flex flex-wrap items-center gap-1.5">
          {MONTH_LABELS.map((label, i) => (
            <Chip
              key={label}
              selected={pattern.months.includes(i + 1)}
              onClick={() => toggleMonth(i + 1)}
              label={label}
            >
              {label}
            </Chip>
          ))}
        </div>
      </Field>
      <p className="text-xs text-foreground/50">
        {pattern.months.length === 0
          ? 'No months selected — runs every month.'
          : `Restricted to ${pattern.months.length} month${pattern.months.length > 1 ? 's' : ''}.`}
      </p>

      <TimesEditor times={pattern.times} onChange={(times) => onChange({...pattern, times})} />
    </div>
  )
}

function RangeAdder({
  ranges,
  onChange,
}: {
  ranges: Array<[number, number]>
  onChange: (ranges: Array<[number, number]>) => void
}) {
  const dayItems = DAY_NUMBERS.map((d) => ({id: String(d), label: String(d)}))
  const [from, setFrom] = useState('1')
  const [to, setTo] = useState('7')

  const add = () => {
    const a = Number(from)
    const b = Number(to)
    if (!a || !b || a > b || a < 1 || b > 31) return
    if (ranges.some(([x, y]) => a <= y && x <= b)) return
    onChange([...ranges, [a, b]])
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs font-medium text-foreground/60">Add range</span>
        <SelectField
          aria-label="Range start day"
          value={from}
          onChange={setFrom}
          items={dayItems}
          className="w-20"
        />
        <span className="text-xs text-foreground/45">to</span>
        <SelectField aria-label="Range end day" value={to} onChange={setTo} items={dayItems} className="w-20" />
        <button
          type="button"
          onClick={add}
          data-testid="add-day-range"
          className="rounded-lg border border-field-border bg-surface/70 px-2.5 py-1.5 text-xs font-medium text-foreground/70 outline-none transition-colors hover:border-accent/60 hover:text-accent focus-visible:ring-2 focus-visible:ring-accent-ring"
        >
          + Add
        </button>
      </div>
      {ranges.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {ranges.map(([a, b]) => (
            <span
              key={`${a}-${b}`}
              data-testid={`range-chip-${a}-${b}`}
              className="inline-flex items-center gap-1 rounded-lg border border-field-border bg-surface/70 py-1 pl-2.5 pr-1 font-mono text-xs text-foreground/80"
            >
              {a}–{b}
              <button
                type="button"
                aria-label={`Remove range ${a} to ${b}`}
                onClick={() => onChange(ranges.filter(([x, y]) => !(x === a && y === b)))}
                className="rounded p-0.5 text-foreground/45 outline-none hover:bg-red-100 hover:text-red-600 focus-visible:ring-2 focus-visible:ring-accent-ring dark:hover:bg-red-950/50 dark:hover:text-red-400"
              >
                <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                  <path d="M18 6 6 18M6 6l12 12" />
                </svg>
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

function OncePanel({pattern, onChange, error}: PanelProps) {
  if (pattern.kind !== 'once') return null
  return (
    <div className="space-y-2">
      <DateTimePickerField
        label="Run at"
        value={pattern.runAt || null}
        onChange={(runAt) => onChange({...pattern, runAt: runAt ?? ''})}
        className="w-full sm:max-w-sm"
      />
      {error && (
        <p className="text-xs font-medium text-red-600 dark:text-red-400">{error}</p>
      )}
    </div>
  )
}

function CronPanel({pattern, onChange, error}: PanelProps) {
  if (pattern.kind !== 'cron') return null
  const expr = pattern.expr
  const localError = expr.trim() ? validateCron(expr) : null
  const fields = cronFields(expr)
  return (
    <div className="space-y-3">
      <Field
        label="Cron expression"
        hint="Minute hour day-of-month month day-of-week — 5 or 6 fields (seconds first)"
        error={error ?? localError ?? undefined}
      >
        <TextInput
          value={expr}
          onChange={(e) => onChange({...pattern, expr: e.target.value})}
          placeholder="0 9 23-26 * *"
          aria-label="Cron expression"
          className="font-mono"
        />
      </Field>

      {fields && (
        <div className="flex flex-wrap gap-1.5">
          {fields.labels.map((f) => (
            <span
              key={f.name}
              className="inline-flex flex-col items-center rounded-lg border border-field-border bg-surface/70 px-2 py-1"
            >
              <span className="font-mono text-xs text-foreground/80">{f.value}</span>
              <span className="text-[9px] uppercase tracking-wide text-foreground/40">{f.name}</span>
            </span>
          ))}
        </div>
      )}

      {!localError && expr.trim().startsWith('@') && (
        <p className="text-xs text-foreground/50">Descriptor syntax — expanded to its equivalent schedule.</p>
      )}

      <div className="flex flex-wrap items-center gap-1.5 text-[10px] text-foreground/45">
        <span className="uppercase tracking-wider">Supports</span>
        {['lists 1,15', 'ranges 23-26', 'steps */5', 'names MON, JAN', '@every 90m'].map((hint) => (
          <span key={hint} className="rounded-md border border-field-border/70 bg-surface-secondary/60 px-1.5 py-0.5 font-mono">
            {hint}
          </span>
        ))}
      </div>
    </div>
  )
}

export function PatternPanel({pattern, onChange, error}: PanelProps) {
  switch (pattern.kind) {
    case 'every':
      return <EveryPanel pattern={pattern} onChange={onChange} error={error} />
    case 'daily':
      return <DailyPanel pattern={pattern} onChange={onChange} error={error} />
    case 'weekly':
      return <WeeklyPanel pattern={pattern} onChange={onChange} error={error} />
    case 'monthly':
      return <MonthlyPanel pattern={pattern} onChange={onChange} error={error} />
    case 'once':
      return <OncePanel pattern={pattern} onChange={onChange} error={error} />
    case 'cron':
      return <CronPanel pattern={pattern} onChange={onChange} error={error} />
  }
}
