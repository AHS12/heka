import type {ReactNode} from 'react'
import type {PatternKind} from '../../lib/schedulePattern'

const OPTIONS: Array<{id: PatternKind; title: string; subtitle: string; example: string; icon: ReactNode}> = [
  {
    id: 'every',
    title: 'Every N',
    subtitle: 'Fixed interval between runs',
    example: '@every 15m',
    icon: <IconPath d="M12 6v6l4 2M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0Z" />,
  },
  {
    id: 'daily',
    title: 'Daily',
    subtitle: 'One or more times, every day',
    example: '0 9,17 * * *',
    icon: <IconPath d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4m11.4-11.4 1.4-1.4M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z" />,
  },
  {
    id: 'weekly',
    title: 'Weekly',
    subtitle: 'Pick weekdays, add times',
    example: '30 8 * * 1-5',
    icon: <IconPath d="M8 2v4m8-4v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Zm2 10 2 2 4-4" />,
  },
  {
    id: 'monthly',
    title: 'Monthly',
    subtitle: 'Days of the month, optional months',
    example: '0 9 23-26 9 *',
    icon: <IconPath d="M8 2v4m8-4v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Zm3 9h.01M12 13h.01m3.99 0h.01M8 17h.01M12 17h.01" />,
  },
  {
    id: 'once',
    title: 'Once',
    subtitle: 'A single run at a moment in time',
    example: 'one-time job',
    icon: <IconPath d="M12 19V5m-7 7 7-7 7 7" />,
  },
  {
    id: 'cron',
    title: 'Cron',
    subtitle: 'Full expression, every cron power',
    example: '*/5 9-17 * * 1-5',
    icon: <IconPath d="m8 8-4 4 4 4m8-8 4 4-4 4M14 4l-4 16" />,
  },
]

function IconPath({d}: {d: string}) {
  return (
    <svg
      className="size-4 shrink-0"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={d} />
    </svg>
  )
}

export function SchedulePatternPicker({
  value,
  onChange,
}: {
  value: PatternKind
  onChange: (kind: PatternKind) => void
}) {
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3" data-testid="schedule-pattern-picker">
      {OPTIONS.map((option) => {
        const selected = value === option.id
        return (
          <button
            key={option.id}
            type="button"
            aria-pressed={selected}
            data-testid={`pattern-option-${option.id}`}
            onClick={() => onChange(option.id)}
            className={`rounded-xl border p-3 text-left transition-colors outline-none focus-visible:ring-2 focus-visible:ring-accent-ring ${
              selected
                ? 'border-accent bg-accent/10'
                : 'border-border/80 bg-surface/60 hover:border-accent/50'
            }`}
          >
            <span className={`flex items-center gap-2 text-sm font-semibold ${selected ? 'text-accent' : 'text-foreground/85'}`}>
              {option.icon}
              {option.title}
            </span>
            <span className="mt-1 block text-[11px] leading-snug text-foreground/55">{option.subtitle}</span>
            <span className="mt-1.5 block font-mono text-[10px] text-foreground/40">{option.example}</span>
          </button>
        )
      })}
    </div>
  )
}
