// pages/DashboardPage.tsx (SPEC-16 §1) — the home surface: engine health +
// quick actions header, metric tiles, next scheduled run, 7-day chart +
// status donut, and recent activity deep-linking into run details.
// Surfaces/borders use the theme tokens (bg-surface, border-border) so every
// theme variant renders correctly — no hardcoded zinc pairs.
import {useMemo, useState} from 'react'
import {Link, useNavigate} from 'react-router-dom'
import {useQueryClient} from '@tanstack/react-query'
import {
  BarChart,
  Bar,
  CartesianGrid,
  XAxis,
  YAxis,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from 'recharts'
import {Modal, Toast, Tooltip} from '@heroui/react'
import {Focusable} from 'react-aria-components'
import {
  useDaemonMode,
  useHealth,
} from '../lib/query'
import {useStats, statusLabel} from '../lib/runs'
import {useSchedules} from '../lib/schedules'
import {useRunTask, useTasks} from '../lib/tasks'
import {apiErrorDetails, type Schedule, type StatsResult} from '../lib/api'
import {describeCron} from '../lib/cron'
import {useAccent, ACCENT_COLORS, type PresetAccent} from '../lib/accent'
import {useTheme} from '../lib/theme'
import {AppDialog, dialogBodyCls, dialogFooterCls, dialogHeaderCls} from '../components/AppDialog'
import {SelectField, pillBtn, primaryBtn} from '../components/controls'
import {HealthTooltipBody} from '../components/DaemonStatusIcon'
import {TaskEditorPage} from './TaskEditorPage'

const STATUS_COLORS: Record<string, string> = {
  success: '#22c55e',
  failed: '#ef4444',
  timed_out: '#f59e0b',
  cancelled: '#6b7280',
  skipped: '#94a3b8',
  missed: '#a78bfa',
  queued: '#3b82f6',
  running: '#06b6d4',
}

function statusColor(status: string, accentHex: string): string {
  return status === 'success' ? accentHex : (STATUS_COLORS[status] ?? '#6b7280')
}

function getAccentHex(accent: PresetAccent | 'custom', customColor: string): string {
  if (accent === 'custom') return customColor
  const raw = ACCENT_COLORS[accent] ?? '#2563eb'
  if (typeof document === 'undefined' || /jsdom/i.test(navigator.userAgent)) return '#2563eb'
  try {
    const ctx = document.createElement('canvas').getContext('2d')
    if (ctx) {
      ctx.fillStyle = raw
      return ctx.fillStyle
    }
  } catch {}
  return '#2563eb'
}

/** Theme-derived chart colors, recomputed when the resolved theme flips. */
function useChartTheme() {
  const resolved = useTheme((s) => s.resolved)
  return useMemo(() => {
    let fg = '#71717a'
    let border = '#e4e4e7'
    try {
      const cs = getComputedStyle(document.documentElement)
      fg = cs.getPropertyValue('--foreground').trim() || fg
      border = cs.getPropertyValue('--border').trim() || border
    } catch {}
    return {fg, border}
  }, [resolved])
}

/** "2m ago" style relative label for run timestamps. */
function relativeTime(iso: string): string {
  const d = new Date(iso)
  const diff = Date.now() - d.getTime()
  if (Number.isNaN(diff)) return ''
  const s = Math.floor(diff / 1000)
  if (s < 60) return 'just now'
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const days = Math.floor(h / 24)
  if (days < 7) return `${days}d ago`
  return d.toLocaleDateString(undefined, {month: 'short', day: 'numeric'})
}

/** "in 14h 2m" style countdown until a future date. */
function countdownLabel(date: Date): string {
  const ms = date.getTime() - Date.now()
  if (Number.isNaN(ms) || ms <= 0) return 'now'
  const m = Math.floor(ms / 60000)
  if (m < 1) return '<1m'
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`
  const d = Math.floor(h / 24)
  return `${d}d ${h % 24}h`
}

// ---- Engine health chip ------------------------------------------------------

const HEALTH_DOT: Record<string, string> = {
  healthy: 'bg-emerald-500',
  'not-running': 'bg-red-500',
  starting: 'bg-amber-400 animate-pulse',
  paused: 'bg-amber-400',
}

const HEALTH_LABEL: Record<string, string> = {
  healthy: 'Engine healthy',
  'not-running': 'Daemon not running',
  starting: 'Daemon starting…',
  paused: 'Scheduler paused',
}

function EngineHealth() {
  const {mode} = useDaemonMode()
  const health = useHealth()
  const schedulerPaused = mode === 'running' && health.data?.scheduler !== 'running'
  const state = mode !== 'running' ? mode : schedulerPaused ? 'paused' : 'healthy'

  return (
    <Tooltip delay={300}>
      <Focusable>
        <span
          role="status"
          aria-label={HEALTH_LABEL[state]}
          data-testid="engine-health"
          className="inline-flex cursor-default items-center gap-1.5 rounded-full border border-border bg-surface px-2.5 py-1 text-[11px] font-medium text-foreground/70 outline-none focus-visible:ring-2 focus-visible:ring-accent-ring"
        >
          <span className={`size-2 rounded-full ${HEALTH_DOT[state]}`} />
          {HEALTH_LABEL[state]}
        </span>
      </Focusable>
      <Tooltip.Content showArrow placement="bottom">
        <HealthTooltipBody mode={mode} health={health} />
      </Tooltip.Content>
    </Tooltip>
  )
}

// ---- Metric tiles ------------------------------------------------------------

function StatTile({
  label,
  value,
  sub,
  accentHex,
  onClick,
}: {
  label: string
  value: number
  sub?: string
  accentHex?: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="rounded-lg border border-border bg-surface p-4 text-left shadow-sm shadow-zinc-900/5 outline-none transition-colors hover:border-accent/40 focus-visible:ring-2 focus-visible:ring-accent-ring"
    >
      <div
        className="font-display text-3xl font-bold tabular-nums leading-none tracking-tight"
        style={accentHex ? {color: accentHex} : undefined}
      >
        {value}
      </div>
      <div className="mt-1.5 text-[11px] font-medium uppercase tracking-wider text-foreground/50">
        {label}
      </div>
      {sub && <div className="mt-0.5 text-xs text-foreground/40">{sub}</div>}
    </button>
  )
}

// ---- Next scheduled run ------------------------------------------------------

function scheduleTimeLabel(nextDate: Date): string {
  const now = new Date()
  const isToday = nextDate.toDateString() === now.toDateString()
  const tomorrow = new Date(now)
  tomorrow.setDate(tomorrow.getDate() + 1)
  const isTomorrow = nextDate.toDateString() === tomorrow.toDateString()
  const time = nextDate.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'})
  if (isToday) return `Today ${time}`
  if (isTomorrow) return `Tomorrow ${time}`
  return (
    nextDate.toLocaleDateString(undefined, {month: 'short', day: 'numeric'}) + ' ' + time
  )
}

function NextUpPanel({accentHex}: {accentHex: string}) {
  const navigate = useNavigate()
  const schedules = useSchedules()
  const run = useRunTask()

  const upcoming = useMemo(() => {
    const now = new Date()
    return (schedules.data ?? [])
      .filter((s) => s.enabled && s.next_run_at)
      .map((s) => ({...s, _date: new Date(s.next_run_at!)}))
      .filter((s) => !Number.isNaN(s._date.getTime()) && s._date > now)
      .sort((a, b) => a._date.getTime() - b._date.getTime())[0] as Schedule | undefined
  }, [schedules.data])

  if (schedules.isLoading) {
    return (
      <div className="flex items-center gap-3 rounded-lg border border-border bg-surface px-4 py-3">
        <span className="size-8 animate-pulse rounded-lg bg-surface-secondary" />
        <span className="text-xs text-foreground/40">Loading schedule…</span>
      </div>
    )
  }

  if (!upcoming) {
    return (
      <button
        type="button"
        onClick={() => navigate('/schedules')}
        className="flex w-full items-center gap-3 rounded-lg border border-dashed border-border bg-surface px-4 py-3 text-left outline-none transition-colors hover:border-accent/40 focus-visible:ring-2 focus-visible:ring-accent-ring"
      >
        <span
          className="flex size-8 items-center justify-center rounded-lg"
          style={{backgroundColor: accentHex + '12', color: accentHex + '80'}}
        >
          <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="12" y1="8" x2="12" y2="12" />
            <line x1="12" y1="16" x2="12.01" y2="16" />
          </svg>
        </span>
        <span className="text-xs text-foreground/45">
          No upcoming runs — schedule a task to automate it
        </span>
      </button>
    )
  }

  const nextDate = new Date(upcoming.next_run_at!)
  const onRunNow = () => {
    run.mutate(
      {slug: upcoming.task_slug, trigger: 'manual'},
      {
        onSuccess: (resp) =>
          Toast.toast.success(`Started ${upcoming.task_slug}`, {
            description: `group ${resp.group_id}`,
          }),
        onError: (err) =>
          Toast.toast.danger('Run failed', {
            description: apiErrorDetails(err)[0] ?? 'Unknown error',
          }),
      }
    )
  }

  return (
    <div className="flex flex-wrap items-center gap-3 rounded-lg border border-border bg-surface px-4 py-3 shadow-sm shadow-zinc-900/5">
      <button
        type="button"
        onClick={() => navigate('/schedules')}
        title="Open Schedules"
        className="flex min-w-0 flex-1 items-center gap-3 rounded text-left outline-none focus-visible:ring-2 focus-visible:ring-accent-ring"
      >
        <span
          className="flex size-8 shrink-0 items-center justify-center rounded-lg"
          style={{backgroundColor: accentHex + '15', color: accentHex}}
        >
          <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <circle cx="12" cy="12" r="10" />
            <polyline points="12 6 12 12 16 14" />
          </svg>
        </span>
        <span className="min-w-0">
          <span className="block text-[11px] font-semibold uppercase tracking-wider text-foreground/45">
            Next scheduled run
          </span>
          <span className="block truncate text-sm font-medium text-foreground">
            {upcoming.task_slug}
          </span>
        </span>
        <span className="ml-auto flex items-baseline gap-2 pl-2">
          <span className="font-display text-lg font-bold tabular-nums tracking-tight text-foreground">
            {scheduleTimeLabel(nextDate)}
          </span>
          <span className="text-xs tabular-nums text-foreground/45">
            in {countdownLabel(nextDate)}
          </span>
        </span>
      </button>
      {upcoming.kind === 'recurring' && upcoming.cron && (
        <span
          className="hidden max-w-52 truncate rounded-full px-2 py-0.5 text-[10px] sm:inline"
          style={{color: accentHex, backgroundColor: accentHex + '15'}}
          title={upcoming.cron}
        >
          {describeCron(upcoming.cron)}
        </span>
      )}
      <button
        type="button"
        onClick={onRunNow}
        disabled={run.isPending}
        className={pillBtn}
        aria-label={`Run ${upcoming.task_slug} now`}
      >
        {run.isPending ? 'Starting…' : 'Run now'}
      </button>
    </div>
  )
}

// ---- Run dialog --------------------------------------------------------------

function RunDialog({open, onClose}: {open: boolean; onClose: () => void}) {
  const tasks = useTasks()
  const run = useRunTask()
  const [slug, setSlug] = useState('')

  const items = (tasks.data ?? []).map((t) => ({
    id: t.slug,
    label: t.name,
    isDisabled: !t.enabled,
  }))

  const submit = () => {
    if (!slug) return
    run.mutate(
      {slug, trigger: 'manual'},
      {
        onSuccess: (resp) => {
          Toast.toast.success(`Started ${slug}`, {description: `group ${resp.group_id}`})
          onClose()
        },
        onError: (err) =>
          Toast.toast.danger('Run failed', {
            description: apiErrorDetails(err)[0] ?? 'Unknown error',
          }),
      }
    )
  }

  return (
    <AppDialog isOpen={open} onOpenChange={(v) => !v && onClose()} size="sm">
      <Modal.Header className={dialogHeaderCls}>
        <div>
          <Modal.Heading className="text-lg font-semibold">Run a task</Modal.Heading>
          <p className="mt-1 text-xs text-foreground/55">
            Trigger a manual run — output lands in Logs.
          </p>
        </div>
        <Modal.CloseTrigger aria-label="Close run dialog" isDisabled={run.isPending} />
      </Modal.Header>
      <Modal.Body className={dialogBodyCls}>
        <SelectField
          aria-label="Task to run"
          value={slug}
          onChange={setSlug}
          items={items}
          placeholder={tasks.isLoading ? 'Loading tasks…' : 'Select a task…'}
          className="w-full"
        />
      </Modal.Body>
      <Modal.Footer className={dialogFooterCls}>
        <button type="button" className={pillBtn} disabled={run.isPending} onClick={onClose}>
          Cancel
        </button>
        <button
          type="button"
          className={primaryBtn}
          disabled={!slug || run.isPending}
          onClick={submit}
        >
          {run.isPending ? 'Starting…' : 'Run task'}
        </button>
      </Modal.Footer>
    </AppDialog>
  )
}

// ---- Charts ------------------------------------------------------------------

type ChartTooltipProps = {
  active?: boolean
  label?: string | number
  payload?: {dataKey?: string | number; value?: number | string; color?: string}[]
}

function ChartTooltip({active, payload, label}: ChartTooltipProps) {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-lg border border-border bg-surface px-2.5 py-1.5 text-xs shadow-md">
      <div className="mb-1 font-medium text-foreground/55">{String(label)}</div>
      {payload.map((p) => (
        <div key={String(p.dataKey)} className="flex items-center gap-1.5">
          <span className="size-1.5 rounded-full" style={{backgroundColor: p.color}} />
          <span className="text-foreground">{statusLabel(String(p.dataKey))}</span>
          <span className="ml-auto pl-3 tabular-nums text-foreground/55">{p.value}</span>
        </div>
      ))}
    </div>
  )
}

function RunHistoryChart({stats, accentHex}: {stats: StatsResult; accentHex: string}) {
  const t = useChartTheme()
  return (
    <section className="rounded-lg border border-border bg-surface p-4">
      <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-foreground/45">
        Last 7 Days
      </h3>
      <ResponsiveContainer width="100%" height={170}>
        <BarChart data={stats.run_history} margin={{top: 4, right: 0, left: 0, bottom: 0}}>
          <CartesianGrid strokeDasharray="3 3" stroke={t.border} vertical={false} />
          <XAxis
            dataKey="date"
            tick={{fontSize: 10, fill: t.fg}}
            tickLine={false}
            axisLine={{stroke: t.border}}
            tickFormatter={(v) => String(v).slice(5)}
          />
          <YAxis
            tick={{fontSize: 10, fill: t.fg}}
            width={28}
            tickLine={false}
            axisLine={false}
            allowDecimals={false}
          />
          <RechartsTooltip content={<ChartTooltip />} cursor={{fill: t.border, opacity: 0.35}} />
          <Bar dataKey="success" stackId="a" fill={accentHex} radius={[2, 2, 0, 0]} />
          <Bar dataKey="failed" stackId="a" fill="#ef4444" radius={[2, 2, 0, 0]} />
        </BarChart>
      </ResponsiveContainer>
    </section>
  )
}

function StatusDonut({stats, accentHex}: {stats: StatsResult; accentHex: string}) {
  const dist = useMemo(
    () => [...stats.status_distribution].sort((a, b) => b.count - a.count),
    [stats.status_distribution]
  )
  const total = dist.reduce((n, d) => n + d.count, 0)

  return (
    <section className="rounded-lg border border-border bg-surface p-4">
      <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-foreground/45">
        Status Distribution
      </h3>
      {dist.length > 0 ? (
        <div className="flex items-center gap-5">
          <div className="relative shrink-0">
            <ResponsiveContainer width={140} height={140}>
              <PieChart>
                <Pie
                  data={dist}
                  dataKey="count"
                  nameKey="status"
                  cx="50%"
                  cy="50%"
                  innerRadius={38}
                  outerRadius={62}
                  strokeWidth={0}
                >
                  {dist.map((entry) => (
                    <Cell key={entry.status} fill={statusColor(entry.status, accentHex)} />
                  ))}
                </Pie>
              </PieChart>
            </ResponsiveContainer>
            <div className="pointer-events-none absolute inset-0 grid place-items-center">
              <div className="text-center">
                <div className="font-display text-lg font-bold tabular-nums leading-none text-foreground">
                  {total}
                </div>
                <div className="mt-0.5 text-[10px] uppercase tracking-wider text-foreground/45">
                  runs
                </div>
              </div>
            </div>
          </div>
          <div className="min-w-0 flex-1 space-y-1.5">
            {dist.map((s) => (
              <div key={s.status} className="flex items-center gap-2 text-xs">
                <span
                  className="size-2 shrink-0 rounded-full"
                  style={{backgroundColor: statusColor(s.status, accentHex)}}
                />
                <span className="truncate text-foreground/80">{statusLabel(s.status)}</span>
                <span className="ml-auto tabular-nums text-foreground/55">{s.count}</span>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <p className="py-6 text-center text-xs text-foreground/40">No data</p>
      )}
    </section>
  )
}

// ---- Recent activity ---------------------------------------------------------

function StatusDot({status, accentHex}: {status: string; accentHex: string}) {
  const color = statusColor(status, accentHex)
  return (
    <span
      className="inline-flex w-20 shrink-0 items-center gap-1.5 text-[11px] font-medium"
      style={{color}}
    >
      <span className="size-1.5 rounded-full" style={{backgroundColor: color}} />
      {statusLabel(status)}
    </span>
  )
}

function RecentActivity({stats, accentHex}: {stats: StatsResult; accentHex: string}) {
  const navigate = useNavigate()
  if (stats.recent_activity.length === 0) return null
  return (
    <section className="overflow-hidden rounded-lg border border-border bg-surface">
      <header className="flex items-center justify-between border-b border-border px-4 py-2.5">
        <h3 className="text-[11px] font-semibold uppercase tracking-wider text-foreground/45">
          Recent Activity
        </h3>
        <Link
          to="/logs"
          className="text-[11px] font-medium text-accent outline-none hover:underline focus-visible:ring-2 focus-visible:ring-accent-ring"
        >
          View all
        </Link>
      </header>
      <div>
        {stats.recent_activity.map((item) => (
          <button
            key={item.run_id}
            type="button"
            data-testid={`activity-row-${item.run_id}`}
            onClick={() => navigate(`/logs/${item.run_id}`)}
            title={`${statusLabel(item.status)} · ${new Date(item.at).toLocaleString()}`}
            className="flex w-full items-center gap-3 border-b border-border/60 px-4 py-2 text-left outline-none transition-colors last:border-0 hover:bg-surface-secondary focus-visible:bg-surface-secondary focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-accent-ring"
          >
            <StatusDot status={item.status} accentHex={accentHex} />
            <span className="min-w-0 flex-1 truncate text-xs font-medium text-foreground">
              {item.task_slug}
            </span>
            <span className="shrink-0 font-mono text-[10px] text-foreground/45">
              {relativeTime(item.at)}
            </span>
          </button>
        ))}
      </div>
    </section>
  )
}

// ---- Empty state -------------------------------------------------------------

function EmptyState({onCreate}: {onCreate: () => void}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border bg-surface px-4 py-14 text-center">
      <svg
        className="mb-3 size-10 text-foreground/25"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
      >
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <path d="M3 9h18M9 21V9" />
      </svg>
      <p className="text-sm font-medium text-foreground/60">No runs yet</p>
      <p className="mt-1 text-xs text-foreground/40">
        Create a task and run it — stats and charts land here.
      </p>
      <button type="button" className={primaryBtn + ' mt-4'} onClick={onCreate}>
        + New Task
      </button>
    </div>
  )
}

// ---- Page --------------------------------------------------------------------

export function DashboardPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const {accent, customColor} = useAccent()
  const accentHex = getAccentHex(accent, customColor)
  const stats = useStats()
  const [runOpen, setRunOpen] = useState(false)
  const [creating, setCreating] = useState(false)

  const openCreate = () => setCreating(true)
  const onTaskSaved = (slug: string) => {
    Toast.toast.success(`Created ${slug}`)
    void queryClient.invalidateQueries({queryKey: ['stats']})
  }

  if (stats.isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div
          className="size-5 animate-spin rounded-full border-2 border-border"
          style={{borderTopColor: accentHex}}
        />
      </div>
    )
  }

  if (!stats.data) {
    return (
      <div className="mx-auto max-w-2xl py-10 text-center text-sm text-foreground/50">
        Could not load dashboard stats. Is the daemon running?
      </div>
    )
  }

  const s = stats.data
  const hasData = s.runs_today > 0 || s.run_history.length > 0

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* Header: title + engine health + quick actions */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-bold tracking-tight">Overview</h2>
          <EngineHealth />
        </div>
        <div className="flex items-center gap-2">
          <button type="button" className={pillBtn} onClick={openCreate}>
            + New Task
          </button>
          <button type="button" className={pillBtn} onClick={() => navigate('/schedules?new=1')}>
            + New Schedule
          </button>
          <button type="button" className={primaryBtn} onClick={() => setRunOpen(true)}>
            ▶ Run…
          </button>
        </div>
      </div>

      {/* Metric tiles */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile
          label="Tasks"
          value={s.tasks}
          sub={`${s.tasks_enabled} enabled`}
          accentHex={accentHex}
          onClick={() => navigate('/tasks')}
        />
        <StatTile
          label="Schedules"
          value={s.schedules_enabled}
          sub={s.schedules_enabled > 0 ? 'active' : 'none scheduled'}
          onClick={() => navigate('/schedules')}
        />
        <StatTile
          label="Running"
          value={s.running}
          sub={s.running > 0 ? 'executing now' : 'All workers idle'}
          accentHex={s.running > 0 ? accentHex : undefined}
          onClick={() => navigate('/logs?status=running')}
        />
        <StatTile
          label="Today"
          value={s.runs_today}
          sub={`${s.success_today} ok · ${s.failed_today} failed`}
          onClick={() => navigate('/logs')}
        />
      </div>

      {/* Next scheduled run */}
      <NextUpPanel accentHex={accentHex} />

      {hasData ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2">
            <RunHistoryChart stats={s} accentHex={accentHex} />
            <StatusDonut stats={s} accentHex={accentHex} />
          </div>
          <RecentActivity stats={s} accentHex={accentHex} />
        </>
      ) : (
        <EmptyState onCreate={openCreate} />
      )}

      <RunDialog open={runOpen} onClose={() => setRunOpen(false)} />
      {creating && (
        <TaskEditorPage
          dialog
          onClose={() => setCreating(false)}
          onSaved={onTaskSaved}
        />
      )}
    </div>
  )
}
