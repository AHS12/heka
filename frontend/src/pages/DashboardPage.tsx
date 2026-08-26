import {useEffect, useState} from 'react'
import {useNavigate} from 'react-router-dom'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from 'recharts'
import {getStats, listSchedules, type StatsResult, type Schedule} from '../lib/api'
import {useAccent, ACCENT_COLORS, type PresetAccent} from '../lib/accent'

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

function getAccentHex(accent: PresetAccent | 'custom', customColor: string): string {
  if (accent === 'custom') return customColor
  const raw = ACCENT_COLORS[accent] ?? '#2563eb'
  // Convert oklch(...) to hex via canvas
  try {
    const ctx = document.createElement('canvas').getContext('2d')
    if (ctx) {
      ctx.fillStyle = raw
      return ctx.fillStyle
    }
  } catch {}
  return '#2563eb'
}

function StatTile({label, value, accent, accentHex}: {label: string; value: number; accent?: boolean; accentHex?: string}) {
  return (
    <div
      className="rounded-2xl border p-4 shadow-sm backdrop-blur-sm"
      style={accent && accentHex ? {
        borderColor: accentHex + '30',
        backgroundColor: accentHex + '08',
      } : undefined}
    >
      <div
        className="text-2xl font-bold tracking-tight"
        style={accent && accentHex ? {color: accentHex} : undefined}
      >
        {value}
      </div>
      <div className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">{label}</div>
    </div>
  )
}

function StatusChip({status}: {status: string}) {
  const color = STATUS_COLORS[status] ?? '#6b7280'
  return (
    <span className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium" style={{backgroundColor: color + '20', color}}>
      <span className="size-1.5 rounded-full" style={{backgroundColor: color}} />
      {status}
    </span>
  )
}

function UpcomingSchedule({accentHex}: {accentHex: string}) {
  const [schedule, setSchedule] = useState<Schedule | null>(null)
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    let active = true
    async function load() {
      try {
        const all = await listSchedules()
        if (!active) return
        const now = new Date()
        const upcoming = all
          .filter((s) => s.enabled && s.next_run_at)
          .map((s) => ({...s, _date: new Date(s.next_run_at!)}))
          .filter((s) => s._date > now)
          .sort((a, b) => a._date.getTime() - b._date.getTime())
        setSchedule(upcoming[0] ?? null)
      } catch {
        if (active) setSchedule(null)
      } finally {
        if (active) setLoaded(true)
      }
    }
    load()
    const id = setInterval(load, 30000)
    return () => { active = false; clearInterval(id) }
  }, [])

  if (!loaded) {
    return (
      <div className="flex items-center gap-3 rounded-2xl border border-zinc-200/80 bg-white/70 p-3 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
        <div className="flex size-8 items-center justify-center rounded-full bg-zinc-100 dark:bg-zinc-800">
          <div className="size-3 animate-pulse rounded-full bg-zinc-300 dark:bg-zinc-600" />
        </div>
        <span className="text-xs text-zinc-400 dark:text-zinc-500">Loading schedule...</span>
      </div>
    )
  }

  if (!schedule) {
    return (
      <div
        className="flex items-center gap-3 rounded-2xl border p-3 shadow-sm backdrop-blur-sm"
        style={{borderColor: accentHex + '15', backgroundColor: accentHex + '04'}}
      >
        <div className="flex size-8 items-center justify-center rounded-full" style={{backgroundColor: accentHex + '12'}}>
          <svg className="size-4" style={{color: accentHex + '60'}} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="12" y1="8" x2="12" y2="12" />
            <line x1="12" y1="16" x2="12.01" y2="16" />
          </svg>
        </div>
        <span className="text-xs text-zinc-400 dark:text-zinc-500">No upcoming schedules</span>
      </div>
    )
  }

  const nextDate = new Date(schedule.next_run_at!)
  const now = new Date()
  const isToday = nextDate.toDateString() === now.toDateString()
  const tomorrow = new Date(now)
  tomorrow.setDate(tomorrow.getDate() + 1)
  const isTomorrow = nextDate.toDateString() === tomorrow.toDateString()

  let timeLabel: string
  if (isToday) {
    timeLabel = `Today ${nextDate.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'})}`
  } else if (isTomorrow) {
    timeLabel = `Tomorrow ${nextDate.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'})}`
  } else {
    timeLabel = nextDate.toLocaleDateString(undefined, {month: 'short', day: 'numeric'}) +
      ' ' + nextDate.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'})
  }

  return (
    <div
      className="flex items-center gap-3 rounded-2xl border p-3 shadow-sm backdrop-blur-sm"
      style={{borderColor: accentHex + '25', backgroundColor: accentHex + '06'}}
    >
      <div className="flex size-8 items-center justify-center rounded-full" style={{backgroundColor: accentHex + '18'}}>
        <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke={accentHex} strokeWidth="2">
          <circle cx="12" cy="12" r="10" />
          <polyline points="12 6 12 12 16 14" />
        </svg>
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs font-medium text-zinc-700 dark:text-zinc-200">{schedule.task_slug}</div>
        <div className="text-[10px] text-zinc-400 dark:text-zinc-500">{timeLabel}</div>
      </div>
      {schedule.kind === 'recurring' && (
        <span className="shrink-0 rounded-full px-1.5 py-0.5 text-[9px] font-medium" style={{color: accentHex, backgroundColor: accentHex + '15'}}>
          {schedule.cron}
        </span>
      )}
    </div>
  )
}

function EmptyState({accentHex}: {accentHex: string}) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <svg className="mb-3 size-10" style={{color: accentHex + '40'}} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <path d="M3 9h18M9 21V9" />
      </svg>
      <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">No runs yet</p>
      <p className="mt-1 text-xs text-zinc-400 dark:text-zinc-500">Run a task to see stats and charts here.</p>
    </div>
  )
}

export function DashboardPage() {
  const [stats, setStats] = useState<StatsResult | null>(null)
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()
  const {accent, customColor} = useAccent()
  const accentHex = getAccentHex(accent, customColor)

  useEffect(() => {
    let active = true
    async function load() {
      try {
        const s = await getStats()
        if (active) setStats(s)
      } catch {
        if (active) setStats(null)
      } finally {
        if (active) setLoading(false)
      }
    }
    load()
    const id = setInterval(load, 15000)
    return () => {
      active = false
      clearInterval(id)
    }
  }, [])

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="size-5 animate-spin rounded-full border-2 border-zinc-200 dark:border-zinc-700" style={{borderTopColor: accentHex}} />
      </div>
    )
  }

  if (!stats) {
    return (
      <div className="mx-auto max-w-2xl py-10 text-center text-sm text-zinc-400 dark:text-zinc-500">
        Could not load dashboard stats. Is the daemon running?
      </div>
    )
  }

  const hasData = stats.runs_today > 0 || stats.run_history.length > 0

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Stat tiles */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile label="Tasks" value={stats.tasks} accent accentHex={accentHex} />
        <StatTile label="Schedules" value={stats.schedules_enabled} />
        <StatTile label="Running" value={stats.running} accent={stats.running > 0} accentHex={accentHex} />
        <StatTile label="Today" value={stats.runs_today} />
      </div>

      {/* Upcoming schedule */}
      <UpcomingSchedule accentHex={accentHex} />

      {!hasData && <EmptyState accentHex={accentHex} />}

      {hasData && (
        <>
          {/* Charts row */}
          <div className="grid gap-4 sm:grid-cols-2">
            {/* Run history bar chart */}
            <div className="rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
              <h3 className="mb-3 text-xs font-semibold text-zinc-500 dark:text-zinc-400">Last 7 Days</h3>
              <ResponsiveContainer width="100%" height={180}>
                <BarChart data={stats.run_history}>
                  <XAxis dataKey="date" tick={{fontSize: 10, fill: '#a1a1aa'}} tickFormatter={(v) => String(v).slice(5)} />
                  <YAxis tick={{fontSize: 10, fill: '#a1a1aa'}} width={30} />
                  <Tooltip
                    contentStyle={{fontSize: 12, borderRadius: 8, border: '1px solid #e4e4e7', background: '#fff'}}
                    labelFormatter={(v) => String(v)}
                  />
                  <Bar dataKey="success" stackId="a" fill={accentHex} radius={[2, 2, 0, 0]} />
                  <Bar dataKey="failed" stackId="a" fill="#ef4444" radius={[2, 2, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>

            {/* Status distribution donut */}
            <div className="rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
              <h3 className="mb-3 text-xs font-semibold text-zinc-500 dark:text-zinc-400">Status Distribution</h3>
              {stats.status_distribution.length > 0 ? (
                <div className="flex items-center gap-4">
                  <ResponsiveContainer width={140} height={140}>
                    <PieChart>
                      <Pie
                        data={stats.status_distribution}
                        dataKey="count"
                        nameKey="status"
                        cx="50%"
                        cy="50%"
                        innerRadius={35}
                        outerRadius={60}
                        strokeWidth={0}
                      >
                        {stats.status_distribution.map((entry) => (
                          <Cell key={entry.status} fill={entry.status === 'success' ? accentHex : (STATUS_COLORS[entry.status] ?? '#6b7280')} />
                        ))}
                      </Pie>
                    </PieChart>
                  </ResponsiveContainer>
                  <div className="space-y-1.5">
                    {stats.status_distribution.map((s) => (
                      <div key={s.status} className="flex items-center gap-2 text-[11px]">
                        <span className="size-2 rounded-full" style={{backgroundColor: s.status === 'success' ? accentHex : (STATUS_COLORS[s.status] ?? '#6b7280')}} />
                        <span className="text-zinc-600 dark:text-zinc-300">{s.status}</span>
                        <span className="font-mono text-zinc-400">{s.count}</span>
                      </div>
                    ))}
                  </div>
                </div>
              ) : (
                <p className="py-6 text-center text-xs text-zinc-400">No data</p>
              )}
            </div>
          </div>

          {/* Recent activity */}
          {stats.recent_activity.length > 0 && (
            <div className="rounded-2xl border border-zinc-200/80 bg-white/70 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
              <h3 className="border-b border-zinc-100 px-4 py-2.5 text-xs font-semibold text-zinc-500 dark:border-zinc-800/60 dark:text-zinc-400">
                Recent Activity
              </h3>
              {stats.recent_activity.map((item) => (
                <button
                  key={item.run_id}
                  type="button"
                  onClick={() => navigate('/logs')}
                  className="flex w-full items-center justify-between border-b border-zinc-100 px-4 py-2 text-left last:border-0 hover:bg-zinc-50/50 dark:border-zinc-800/60 dark:hover:bg-zinc-800/30"
                >
                  <div className="flex items-center gap-2">
                    <StatusChip status={item.status} />
                    <span className="text-xs text-zinc-700 dark:text-zinc-200">{item.task_slug}</span>
                  </div>
                  <span className="font-mono text-[10px] text-zinc-400 dark:text-zinc-500">
                    {new Date(item.at).toLocaleTimeString()}
                  </span>
                </button>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
