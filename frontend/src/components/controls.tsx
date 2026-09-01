// components/controls.tsx — small form primitives shared by the editor
// sections (SPEC-13 §4). Tailwind-styled to match the shell's floating look;
// focus states use the accent-ring token (accent foundation).
import type {ReactNode, InputHTMLAttributes} from 'react'
import {Select, Label, ListBox, DatePicker, DateField, Calendar, TimeField} from '@heroui/react'
import type {TimeValue} from '@heroui/react'
import type {DateValue, CalendarDateTime} from '@internationalized/date'
import {parseDate, parseDateTime} from '@internationalized/date'

export const inputCls =
  'w-full rounded-lg border border-zinc-200 bg-white/70 px-2.5 py-1.5 text-sm ' +
  'text-zinc-900 outline-none transition-colors placeholder:text-zinc-400 ' +
  'focus:border-accent focus:ring-1 focus:ring-accent-ring ' +
  'dark:border-zinc-700 dark:bg-zinc-900/70 dark:text-zinc-100 dark:placeholder:text-zinc-500'

export function Field({
  label,
  hint,
  error,
  errorId,
  children,
}: {
  label: string
  hint?: ReactNode
  error?: string
  errorId?: string
  children: ReactNode
}) {
  return (
    <div className="block">
      <span className="mb-1 block text-xs font-medium text-zinc-500 dark:text-zinc-400">
        {label}
      </span>
      {children}
      {error ? (
        <span id={errorId} className="mt-1.5 flex items-start gap-1.5 text-xs font-medium text-red-600 dark:text-red-400">
          <span aria-hidden="true">•</span>
          {error}
        </span>
      ) : hint ? (
        <span className="mt-1 block text-xs text-zinc-400 dark:text-zinc-500">{hint}</span>
      ) : null}
    </div>
  )
}

export function FormErrors({
  errors,
  title = 'Check the highlighted fields',
  testId,
}: {
  errors: string[]
  title?: string
  testId?: string
}) {
  if (errors.length === 0) return null
  return (
    <div
      role="alert"
      data-testid={testId}
      className="rounded-xl border border-red-200 bg-red-50/90 px-3 py-2.5 text-xs text-red-800 shadow-sm dark:border-red-900 dark:bg-red-950/70 dark:text-red-200"
    >
      <p className="font-semibold">{title}</p>
      <ul className="mt-1 list-inside list-disc space-y-0.5">
        {errors.map((error) => <li key={error}>{error}</li>)}
      </ul>
    </div>
  )
}

export function TextInput(props: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={`${inputCls} ${props['aria-invalid'] ? 'border-red-400 focus:border-red-500 focus:ring-red-300 dark:border-red-700' : ''} ${props.className ?? ''}`}
    />
  )
}

export function NumberInput(props: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      type="number"
      {...props}
      className={`${inputCls} ${props.className ?? ''}`}
    />
  )
}

interface SelectItemData {
  id: string
  label: string
  isDisabled?: boolean
}

export function SelectField({
  value,
  onChange,
  items,
  placeholder,
  isDisabled,
  className,
  isInvalid,
  'aria-label': ariaLabel,
  'aria-describedby': ariaDescribedBy,
}: {
  value: string
  onChange: (value: string) => void
  items: SelectItemData[]
  placeholder?: string
  isDisabled?: boolean
  isInvalid?: boolean
  className?: string
  'aria-label'?: string
  'aria-describedby'?: string
}) {
  const triggerCls =
    'w-full rounded-lg border border-zinc-200 bg-white/70 px-2.5 py-1.5 text-sm ' +
    'text-zinc-900 outline-none transition-colors ' +
    'data-[hovered=true]:border-zinc-300 ' +
    'data-[focus-visible=true]:border-accent data-[focus-visible=true]:ring-1 data-[focus-visible=true]:ring-accent-ring ' +
    'data-[disabled=true]:opacity-50 ' +
    'dark:border-zinc-700 dark:bg-zinc-900/70 dark:text-zinc-100 ' +
    'dark:data-[hovered=true]:border-zinc-600'

  const popoverCls =
    'rounded-xl border border-zinc-200 bg-white p-1 shadow-xl ' +
    'dark:border-zinc-700 dark:bg-zinc-900'

  const itemCls =
    'rounded-lg px-2.5 py-1.5 text-sm text-zinc-900 ' +
    'data-[focused=true]:bg-zinc-100 data-[selected=true]:bg-accent/10 data-[selected=true]:text-accent ' +
    'dark:text-zinc-100 dark:data-[focused=true]:bg-zinc-800 dark:data-[selected=true]:bg-accent/20 dark:data-[selected=true]:text-accent'

  return (
    <Select
      value={value}
      onChange={(key) => {
        if (key != null) onChange(String(key))
      }}
      placeholder={placeholder ?? 'Select…'}
      isDisabled={isDisabled}
      isInvalid={isInvalid}
      aria-describedby={ariaDescribedBy}
      className={className}
    >
      <Label className="sr-only">{ariaLabel}</Label>
      <Select.Trigger className={`${triggerCls} ${isInvalid ? 'border-red-400 data-[focus-visible=true]:border-red-500 data-[focus-visible=true]:ring-red-300 dark:border-red-700' : ''}`}>
        <Select.Value className="text-sm" />
        <Select.Indicator className="size-4 text-zinc-400 dark:text-zinc-500" />
      </Select.Trigger>
      <Select.Popover className={popoverCls}>
        <ListBox className="max-h-[300px] overflow-auto p-1">
          {items.map((item) => (
            <ListBox.Item key={item.id} id={item.id} isDisabled={item.isDisabled} className={itemCls}>
              {item.label}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: (next: boolean) => void
  label: string
}) {
  return (
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
  )
}

export const pillBtn =
  'inline-flex items-center gap-1.5 rounded-full border border-zinc-200/80 bg-white/80 ' +
  'px-3.5 py-1.5 text-sm font-medium shadow-sm shadow-zinc-900/5 outline-none ' +
  'transition-colors focus-visible:ring-2 focus-visible:ring-accent-ring ' +
  'hover:bg-zinc-200/70 dark:border-zinc-700/60 dark:bg-zinc-900/70 dark:text-zinc-200 ' +
  'dark:hover:bg-zinc-800/70 disabled:opacity-50'

export const primaryBtn =
  'inline-flex items-center gap-1.5 rounded-full bg-accent px-3.5 py-1.5 ' +
  'text-sm font-medium text-accent-contrast shadow-sm outline-none ' +
  'transition-opacity focus-visible:ring-2 focus-visible:ring-accent-ring ' +
  'hover:opacity-90 disabled:opacity-50'

/** Convert an ISO date/datetime string to a DateValue, or null. */
export function isoToDateValue(iso: string | null | undefined): DateValue | null {
  if (!iso) return null
  try {
    if (iso.includes('T')) {
      // Strip timezone suffix (Z or +HH:MM) so parseDateTime gets a clean format
      const cleaned = iso.replace(/Z$/, '').replace(/\+\d{2}:\d{2}$/, '')
      return parseDateTime(cleaned)
    }
    return parseDate(iso)
  } catch {
    return null
  }
}

/** Convert a DateValue/CalendarDateTime to an ISO date string, or null. */
export function dateValueToISO(v: DateValue | CalendarDateTime | null): string | null {
  if (!v) return null
  const s = v.toString()
  // If it's a datetime (contains T), return the full ISO without timezone
  return s.includes('T') ? s : s
}

/** Shared className for date/time input groups — matches SelectField styling
 *  so all form controls look consistent across every theme. */
const dateFieldGroupCls =
  'w-full rounded-lg border border-zinc-200 bg-white/70 px-2.5 py-1.5 text-sm ' +
  'text-zinc-900 shadow-sm outline-none transition-colors ' +
  'data-[hovered=true]:border-zinc-300 ' +
  'data-[focus-within=true]:border-accent data-[focus-within=true]:ring-1 data-[focus-within=true]:ring-accent-ring ' +
  'data-[disabled=true]:opacity-50 ' +
  'dark:border-zinc-700 dark:bg-zinc-900/70 dark:text-zinc-100 ' +
  'dark:data-[hovered=true]:border-zinc-600'

/** A styled DatePicker field that works with ISO string values. */
export function DatePickerField({
  label,
  value,
  onChange,
  granularity = 'day',
  className,
}: {
  label: string
  value: string | null | undefined
  onChange: (iso: string | null) => void
  granularity?: 'day' | 'hour' | 'minute' | 'second'
  className?: string
}) {
  const dateValue = isoToDateValue(value)
  return (
    <DatePicker
      granularity={granularity}
      value={dateValue}
      onChange={(v) => onChange(dateValueToISO(v as DateValue | CalendarDateTime | null))}
      className={className}
    >
      <Label>{label}</Label>
      <DateField.Group fullWidth className={dateFieldGroupCls}>
        <DateField.Input>{(segment) => <DateField.Segment segment={segment} />}</DateField.Input>
        <DateField.Suffix>
          <DatePicker.Trigger>
            <DatePicker.TriggerIndicator />
          </DatePicker.Trigger>
        </DateField.Suffix>
      </DateField.Group>
      <DatePicker.Popover>
        <Calendar aria-label={label}>
          <Calendar.Header>
            <Calendar.YearPickerTrigger>
              <Calendar.YearPickerTriggerHeading />
              <Calendar.YearPickerTriggerIndicator />
            </Calendar.YearPickerTrigger>
            <Calendar.NavButton slot="previous" />
            <Calendar.NavButton slot="next" />
          </Calendar.Header>
          <Calendar.Grid>
            <Calendar.GridHeader>
              {(day) => <Calendar.HeaderCell>{day}</Calendar.HeaderCell>}
            </Calendar.GridHeader>
            <Calendar.GridBody>{(date) => <Calendar.Cell date={date} />}</Calendar.GridBody>
          </Calendar.Grid>
          <Calendar.YearPickerGrid>
            <Calendar.YearPickerGridBody>
              {({year}) => <Calendar.YearPickerCell year={year} />}
            </Calendar.YearPickerGridBody>
          </Calendar.YearPickerGrid>
        </Calendar>
      </DatePicker.Popover>
    </DatePicker>
  )
}

/** A styled DatePicker + TimeField that works with ISO datetime strings. */
export function DateTimePickerField({
  label,
  value,
  onChange,
  className,
}: {
  label: string
  value: string | null | undefined
  onChange: (iso: string | null) => void
  className?: string
}) {
  const dateValue = isoToDateValue(value)
  return (
    <DatePicker
      granularity="minute"
      value={dateValue}
      onChange={(v) => onChange(dateValueToISO(v as DateValue | CalendarDateTime | null))}
      className={className}
    >
      {({state}) => (
        <>
          <Label>{label}</Label>
          <DateField.Group fullWidth className={dateFieldGroupCls}>
            <DateField.Input>{(segment) => <DateField.Segment segment={segment} />}</DateField.Input>
            <DateField.Suffix>
              <DatePicker.Trigger>
                <DatePicker.TriggerIndicator />
              </DatePicker.Trigger>
            </DateField.Suffix>
          </DateField.Group>
          <DatePicker.Popover className="flex flex-col gap-3">
            <Calendar aria-label={label}>
              <Calendar.Header>
                <Calendar.YearPickerTrigger>
                  <Calendar.YearPickerTriggerHeading />
                  <Calendar.YearPickerTriggerIndicator />
                </Calendar.YearPickerTrigger>
                <Calendar.NavButton slot="previous" />
                <Calendar.NavButton slot="next" />
              </Calendar.Header>
              <Calendar.Grid>
                <Calendar.GridHeader>
                  {(day) => <Calendar.HeaderCell>{day}</Calendar.HeaderCell>}
                </Calendar.GridHeader>
                <Calendar.GridBody>{(date) => <Calendar.Cell date={date} />}</Calendar.GridBody>
              </Calendar.Grid>
              <Calendar.YearPickerGrid>
                <Calendar.YearPickerGridBody>
                  {({year}) => <Calendar.YearPickerCell year={year} />}
                </Calendar.YearPickerGridBody>
              </Calendar.YearPickerGrid>
            </Calendar>
            <div className="flex items-center justify-between border-t border-zinc-200 pt-2 dark:border-zinc-700">
              <Label>Time</Label>
              <TimeField
                aria-label="Time"
                granularity="minute"
                value={state.timeValue as TimeValue | null}
                onChange={(v) => state.setTimeValue(v as TimeValue)}
              >
                <TimeField.Group variant="secondary">
                  <TimeField.Input>
                    {(segment) => <TimeField.Segment segment={segment} />}
                  </TimeField.Input>
                </TimeField.Group>
              </TimeField>
            </div>
          </DatePicker.Popover>
        </>
      )}
    </DatePicker>
  )
}

/** Icon-only cross used to delete list rows (env vars, webhooks). */
export function RemoveRow({label, onClick, className}: {label: string; onClick: () => void; className?: string}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      className={`inline-flex size-5 shrink-0 items-center justify-center rounded text-zinc-400 outline-none transition-colors hover:text-red-500 focus-visible:text-red-500 dark:hover:text-red-400 ${className ?? ''}`}
    >
      <svg aria-hidden viewBox="0 0 16 16" className="size-3 fill-current">
        <path d="M4.646 4.646a.5.5 0 0 1 .708 0L8 7.293l2.646-2.647a.5.5 0 0 1 .708.708L8.707 8l2.647 2.646a.5.5 0 0 1-.708.708L8 8.707l-2.646 2.647a.5.5 0 0 1-.708-.708L7.293 8 4.646 5.354a.5.5 0 0 1 0-.708z" />
      </svg>
    </button>
  )
}