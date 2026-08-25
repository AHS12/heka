import {useState} from 'react'
import {TextInput, SelectField, Field} from '../controls'

export interface RecurrenceValue {
  mode: 'every' | 'daily' | 'weekly' | 'monthly' | 'custom'
  everyN: number
  everyUnit: 'm' | 'h'
  dailyHH: string
  dailyMM: string
  weeklyDay: string
  weeklyHH: string
  weeklyMM: string
  monthlyDay: number
  monthlyHH: string
  monthlyMM: string
  customCron: string
}

export function emptyRecurrence(): RecurrenceValue {
  return {
    mode: 'every',
    everyN: 5,
    everyUnit: 'm',
    dailyHH: '09',
    dailyMM: '00',
    weeklyDay: '1',
    weeklyHH: '09',
    weeklyMM: '00',
    monthlyDay: 1,
    monthlyHH: '09',
    monthlyMM: '00',
    customCron: '',
  }
}

const DAYS = [
  {id: '0', label: 'Sunday'},
  {id: '1', label: 'Monday'},
  {id: '2', label: 'Tuesday'},
  {id: '3', label: 'Wednesday'},
  {id: '4', label: 'Thursday'},
  {id: '5', label: 'Friday'},
  {id: '6', label: 'Saturday'},
]

export function recurrenceToCron(v: RecurrenceValue): string {
  switch (v.mode) {
    case 'every': {
      const unit = v.everyUnit === 'm' ? 'm' : 'h'
      const n = Math.max(1, v.everyN)
      return unit === 'm' ? `@every ${n}m` : `@every ${n}h`
    }
    case 'daily':
      return `${v.dailyMM} ${v.dailyHH} * * *`
    case 'weekly':
      return `${v.weeklyMM} ${v.weeklyHH} * * ${v.weeklyDay}`
    case 'monthly':
      return `${v.monthlyMM} ${v.monthlyHH} ${Math.max(1, Math.min(31, v.monthlyDay))} * *`
    case 'custom':
      return v.customCron
  }
}

export function cronDescription(cron: string): string {
  if (cron.startsWith('@every ')) {
    return cron.slice(7)
  }
  const parts = cron.split(/\s+/)
  if (parts.length === 5) {
    const [mm, hh, dom, , dow] = parts
    if (dom !== '*' && dow === '*') return `Monthly at ${hh}:${mm} (day ${dom})`
    if (dom === '*' && dow !== '*') {
      const day = DAYS.find((d) => d.id === dow)
      return `Weekly on ${day?.label ?? dow} at ${hh}:${mm}`
    }
    if (dom !== '*' && dow !== '*') return cron
    return `Daily at ${hh}:${mm}`
  }
  return cron
}

export function RecurrenceBuilder({
  value,
  onChange,
}: {
  value: RecurrenceValue
  onChange: (v: RecurrenceValue) => void
}) {
  const update = (patch: Partial<RecurrenceValue>) =>
    onChange({...value, ...patch})

  return (
    <div className="space-y-3">
      <Field label="Recurrence">
        <SelectField
          aria-label="Recurrence type"
          value={value.mode}
          onChange={(v) => update({mode: v as RecurrenceValue['mode']})}
          className="w-48"
          items={[
            {id: 'every', label: 'Every N minutes/hours'},
            {id: 'daily', label: 'Daily at time'},
            {id: 'weekly', label: 'Weekly on day'},
            {id: 'monthly', label: 'Monthly on day'},
            {id: 'custom', label: 'Custom cron'},
          ]}
        />
      </Field>

      {value.mode === 'every' && (
        <div className="flex items-end gap-2">
          <Field label="Every">
            <TextInput
              type="number"
              min={1}
              value={String(value.everyN)}
              onChange={(e) => update({everyN: Number(e.target.value)})}
              className="w-24"
            />
          </Field>
          <Field label="Unit">
            <SelectField
              aria-label="Time unit"
              value={value.everyUnit}
              onChange={(v) => update({everyUnit: v as 'm' | 'h'})}
              className="w-32"
              items={[
                {id: 'm', label: 'Minutes'},
                {id: 'h', label: 'Hours'},
              ]}
            />
          </Field>
          <span className="mb-2 text-xs text-zinc-400 dark:text-zinc-500">
            → <code className="font-mono">{recurrenceToCron(value)}</code>
          </span>
        </div>
      )}

      {value.mode === 'daily' && (
        <div className="flex items-end gap-2">
          <Field label="Hour">
            <TextInput
              type="number"
              min={0}
              max={23}
              value={value.dailyHH}
              onChange={(e) => update({dailyHH: e.target.value})}
              className="w-20"
            />
          </Field>
          <Field label="Minute">
            <TextInput
              type="number"
              min={0}
              max={59}
              value={value.dailyMM}
              onChange={(e) => update({dailyMM: e.target.value})}
              className="w-20"
            />
          </Field>
          <span className="mb-2 text-xs text-zinc-400 dark:text-zinc-500">
            → <code className="font-mono">{recurrenceToCron(value)}</code>
          </span>
        </div>
      )}

      {value.mode === 'weekly' && (
        <div className="flex items-end gap-2">
          <Field label="Day">
            <SelectField
              aria-label="Day of week"
              value={value.weeklyDay}
              onChange={(v) => update({weeklyDay: v})}
              className="w-36"
              items={DAYS}
            />
          </Field>
          <Field label="Hour">
            <TextInput
              type="number"
              min={0}
              max={23}
              value={value.weeklyHH}
              onChange={(e) => update({weeklyHH: e.target.value})}
              className="w-20"
            />
          </Field>
          <Field label="Minute">
            <TextInput
              type="number"
              min={0}
              max={59}
              value={value.weeklyMM}
              onChange={(e) => update({weeklyMM: e.target.value})}
              className="w-20"
            />
          </Field>
          <span className="mb-2 text-xs text-zinc-400 dark:text-zinc-500">
            → <code className="font-mono">{recurrenceToCron(value)}</code>
          </span>
        </div>
      )}

      {value.mode === 'monthly' && (
        <div className="flex items-end gap-2">
          <Field label="Day of month">
            <TextInput
              type="number"
              min={1}
              max={31}
              value={String(value.monthlyDay)}
              onChange={(e) => update({monthlyDay: Number(e.target.value)})}
              className="w-24"
            />
          </Field>
          <Field label="Hour">
            <TextInput
              type="number"
              min={0}
              max={23}
              value={value.monthlyHH}
              onChange={(e) => update({monthlyHH: e.target.value})}
              className="w-20"
            />
          </Field>
          <Field label="Minute">
            <TextInput
              type="number"
              min={0}
              max={59}
              value={value.monthlyMM}
              onChange={(e) => update({monthlyMM: e.target.value})}
              className="w-20"
            />
          </Field>
          <span className="mb-2 text-xs text-zinc-400 dark:text-zinc-500">
            → <code className="font-mono">{recurrenceToCron(value)}</code>
          </span>
        </div>
      )}

      {value.mode === 'custom' && (
        <Field label="Cron expression" hint="5-field cron: minute hour day-of-month month day-of-week">
          <TextInput
            value={value.customCron}
            onChange={(e) => update({customCron: e.target.value})}
            placeholder="0 9 * * 1"
            className="font-mono"
          />
        </Field>
      )}
    </div>
  )
}
