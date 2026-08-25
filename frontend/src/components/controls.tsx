// components/controls.tsx — small form primitives shared by the editor
// sections (SPEC-13 §4). Tailwind-styled to match the shell's floating look;
// focus states use the accent-ring token (accent foundation).
import type {ReactNode, InputHTMLAttributes} from 'react'
import {Select, Label, ListBox} from '@heroui/react'

export const inputCls =
  'w-full rounded-lg border border-zinc-200 bg-white/70 px-2.5 py-1.5 text-sm ' +
  'text-zinc-900 outline-none transition-colors placeholder:text-zinc-400 ' +
  'focus:border-accent focus:ring-1 focus:ring-accent-ring ' +
  'dark:border-zinc-700 dark:bg-zinc-900/70 dark:text-zinc-100 dark:placeholder:text-zinc-500'

export function Field({label, hint, children}: {label: string; hint?: ReactNode; children: ReactNode}) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-zinc-500 dark:text-zinc-400">
        {label}
      </span>
      {children}
      {hint && (
        <span className="mt-1 block text-xs text-zinc-400 dark:text-zinc-500">{hint}</span>
      )}
    </label>
  )
}

export function TextInput(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={`${inputCls} ${props.className ?? ''}`} />
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
  'aria-label': ariaLabel,
}: {
  value: string
  onChange: (value: string) => void
  items: SelectItemData[]
  placeholder?: string
  isDisabled?: boolean
  className?: string
  'aria-label'?: string
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
      value={value || undefined}
      onChange={(key) => {
        if (key != null) onChange(String(key))
      }}
      placeholder={placeholder ?? 'Select…'}
      isDisabled={isDisabled}
      className={className}
    >
      <Label className="sr-only">{ariaLabel}</Label>
      <Select.Trigger className={triggerCls}>
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