import {useState} from 'react'
import {Tabs} from '@heroui/react'
import type {Run} from '../../lib/api'

function formatDuration(ms?: number) {
  if (!ms) return '—'
  if (ms < 1000) return `${ms}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  return `${m}m ${Math.round(s % 60)}s`
}

function formatTime(iso?: string) {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

export function OutputViewer({run}: {run: Run}) {
  const [tab, setTab] = useState<'stdout' | 'stderr'>('stdout')
  const [copied, setCopied] = useState(false)
  const output = tab === 'stdout' ? run.stdout : run.stderr

  const handleCopy = async () => {
    if (!output) return
    try {
      await navigator.clipboard.writeText(output)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard API not available (e.g. insecure context)
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <Tabs
          selectedKey={tab}
          onSelectionChange={(key) => setTab(key as 'stdout' | 'stderr')}
        >
          <Tabs.ListContainer>
            <Tabs.List aria-label="Output stream">
              <Tabs.Tab id="stdout">
                STDOUT
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="stderr">
                STDERR
                <Tabs.Indicator />
              </Tabs.Tab>
            </Tabs.List>
          </Tabs.ListContainer>
        </Tabs>
        <button
          type="button"
          onClick={handleCopy}
          className="rounded-lg px-2 py-1 text-xs text-zinc-500 outline-none hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800"
        >
          {copied ? 'Copied!' : 'Copy'}
        </button>
      </div>
      <pre className="max-h-96 overflow-auto rounded-xl border border-zinc-200 bg-zinc-50 p-3 font-mono text-xs text-zinc-800 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-200">
        {output || (
          <span className="text-zinc-400 dark:text-zinc-500">No output</span>
        )}
      </pre>
    </div>
  )
}

export function AttemptList({runs}: {runs: Run[]}) {
  if (runs.length <= 1) return null

  return (
    <div className="space-y-2">
      <h4 className="text-xs font-semibold text-zinc-500 dark:text-zinc-400">
        Attempts ({runs.length})
      </h4>
      <div className="overflow-hidden rounded-xl border border-zinc-200 dark:border-zinc-800">
        <table className="w-full text-left text-xs">
          <thead>
            <tr className="border-b border-zinc-200 bg-zinc-50 text-zinc-500 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-400">
              <th className="px-3 py-1.5 font-medium">#</th>
              <th className="px-3 py-1.5 font-medium">Status</th>
              <th className="px-3 py-1.5 font-medium">Duration</th>
              <th className="px-3 py-1.5 font-medium">Exit</th>
              <th className="px-3 py-1.5 font-medium">Started</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((r) => (
              <tr key={r.run_id} className="border-b border-zinc-100 last:border-0 dark:border-zinc-800/60">
                <td className="px-3 py-1.5 font-mono text-zinc-600 dark:text-zinc-300">
                  {r.attempt + 1}
                </td>
                <td className="px-3 py-1.5">
                  <StatusInline status={r.status} />
                </td>
                <td className="px-3 py-1.5 text-zinc-500 dark:text-zinc-400">
                  {formatDuration(r.duration_ms)}
                </td>
                <td className="px-3 py-1.5 font-mono text-zinc-600 dark:text-zinc-300">
                  {r.exit_code ?? '—'}
                </td>
                <td className="px-3 py-1.5 text-zinc-500 dark:text-zinc-400">
                  {formatTime(r.started_at)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function StatusInline({status}: {status: string}) {
  const colors: Record<string, string> = {
    success: 'text-emerald-600 dark:text-emerald-400',
    failed: 'text-red-600 dark:text-red-400',
    running: 'text-blue-600 dark:text-blue-400',
    timed_out: 'text-amber-600 dark:text-amber-400',
  }
  return (
    <span className={`font-medium ${colors[status] ?? 'text-zinc-500 dark:text-zinc-400'}`}>
      {status.replace(/_/g, ' ')}
    </span>
  )
}
