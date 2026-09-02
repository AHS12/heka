import {useState} from 'react'

import {openURL} from '../lib/api'
import {APP_VERSION} from '../lib/version'

function CopyButton({text}: {text: string}) {
  const [copied, setCopied] = useState(false)
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {}
  }
  return (
    <button
      type="button"
      aria-label={copied ? 'Copied' : 'Copy command'}
      onClick={handleCopy}
      className={`inline-flex size-6 shrink-0 items-center justify-center rounded-md border outline-none transition-all ${
        copied
          ? 'border-emerald-300 text-emerald-500 dark:border-emerald-500/40'
          : 'border-transparent text-zinc-300 hover:border-zinc-200 hover:text-accent dark:text-zinc-600 dark:hover:border-zinc-700'
      }`}
    >
      {copied ? (
        <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
          <polyline points="20 6 9 17 4 12" />
        </svg>
      ) : (
        <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <rect x="9" y="9" width="13" height="13" rx="2" />
          <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
        </svg>
      )}
    </button>
  )
}

const SKILL_CONTENT = `---
name: heka
description: "Use the Heka CLI to manage tasks, schedules, and view logs. Heka is a local task runner and scheduler for programmers."
argument-hint: "command and arguments, for example: heka run my-task"
---

# Heka CLI Skill

## Description
Use the Heka CLI to interact with the Heka task runner and scheduler daemon. All commands communicate with the running daemon via local IPC. Every command supports \`--json\` for machine-readable output.

## When to Use
Use this skill when you need to:
- List, run, or inspect tasks
- Check task status or view logs
- Enable or disable tasks
- Manage the Heka daemon
- Query schedules

## Instructions

### Task Management

\`\`\`bash
# List all registered tasks
heka list --json

# Run a task immediately
heka run <slug> --json

# Check task status and last run
heka status <slug> --json

# View latest run output
heka logs <slug> --json

# Enable a task
heka enable <slug>

# Disable a task
heka disable <slug>
\`\`\`

### Daemon Management

\`\`\`bash
# Start the daemon in the background
heka daemon start

# Stop the daemon gracefully
heka daemon stop

# Show daemon health and uptime
heka daemon status

# Register the daemon to start with the OS
heka daemon startup on

# Remove OS startup registration
heka daemon startup off

# Show startup registration state
heka daemon startup status

# Show watchdog installation state (toggle it in Settings → Reliability)
heka daemon watchdog status
\`\`\`

### Schedules

\`\`\`bash
# List all schedules with last/next run times
heka schedules list --json

# Fire missed recurring runs immediately (per missed_policy)
heka schedules reconcile --json

# List missed/skipped schedule runs for debugging
heka schedules missed --json
heka schedules missed --since 24h --task my-task --json
\`\`\`

### JSON Output

All commands support \`--json\` for structured output. Example:

\`\`\`bash
heka run my-task --json
\`\`\`

\`\`\`json
{
  "success": true,
  "slug": "my-task",
  "run_id": "01J...",
  "status": "queued"
}
\`\`\`

### Task Definitions

Tasks are YAML files in \`~/.heka/tasks/\`. Example:

\`\`\`yaml
version: 1
name: Daily Research
slug: daily-research
type: script
runtime: powershell
script: ./scripts/research.ps1
timeout: 300
capture_output: true
\`\`\`

## Code Style & Conventions
- Always use \`--json\` flag for machine-parseable output
- Use task slugs (not names) in commands
- Check daemon status before running commands
- Handle \`daemon_not_running\` errors by suggesting \`heka daemon start\`
`

export function AboutPage() {
  const [skillCopied, setSkillCopied] = useState(false)

  const handleDownloadSkill = () => {
    const blob = new Blob([SKILL_CONTENT], {type: 'text/markdown'})
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'heka-skill.md'
    a.click()
    URL.revokeObjectURL(url)
  }

  const handleCopySkill = async () => {
    try {
      await navigator.clipboard.writeText(SKILL_CONTENT)
      setSkillCopied(true)
      setTimeout(() => setSkillCopied(false), 1500)
    } catch {}
  }

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      {/* Hero + How It Works */}
      <section className="overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/70 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
        <div className="flex flex-wrap items-center gap-4 p-5">
          <img src="/appicon.png" alt="Heka" className="size-14 rounded-2xl shadow-sm ring-1 ring-zinc-950/5 dark:ring-white/10" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h2 className="text-lg font-bold tracking-tight text-zinc-900 dark:text-zinc-50">Heka</h2>
              <span className="rounded-full bg-accent/10 px-2 py-0.5 text-[11px] font-semibold text-accent">
                v{APP_VERSION}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-zinc-500 dark:text-zinc-400">
              A local task runner & scheduler for programmers — with first-class AI agent support.
            </p>
          </div>
          <button
            type="button"
            onClick={() => void openURL('https://github.com/AHS12/heka')}
            className="inline-flex items-center gap-1.5 rounded-full border border-zinc-200/80 bg-white/80 px-3 py-1.5 text-xs font-medium text-zinc-600 shadow-sm transition-colors hover:border-accent hover:text-accent dark:border-zinc-700/60 dark:bg-zinc-900/70 dark:text-zinc-300 dark:hover:border-accent dark:hover:text-accent"
          >
            <svg className="size-3.5" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
            </svg>
            GitHub
          </button>
        </div>
        <div className="grid border-t border-zinc-100 sm:grid-cols-3 dark:border-zinc-800/60">
          {HOW_IT_WORKS.map((item, i) => (
            <div
              key={item.title}
              className={`flex items-start gap-3 p-4 ${
                i > 0 ? 'border-t border-zinc-100 sm:border-t-0 sm:border-l dark:border-zinc-800/60' : ''
              }`}
            >
              <div className={`mt-0.5 shrink-0 ${i === 1 ? 'text-accent' : 'text-zinc-400 dark:text-zinc-500'}`}>
                {item.icon}
              </div>
              <div className="min-w-0">
                <div className="text-xs font-semibold text-zinc-800 dark:text-zinc-100">{item.title}</div>
                <p className="mt-0.5 text-[11px] leading-relaxed text-zinc-500 dark:text-zinc-400">{item.desc}</p>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* Security + Agent Integration */}
      <div className="grid items-start gap-4 lg:grid-cols-2">
        {/* Security */}
        <section className="space-y-2">
          <div className="px-1">
            <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Security</h3>
          </div>
          <div className="rounded-2xl border border-zinc-200/80 bg-white/70 p-2 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
            <ul>
              {SECURITY_FEATURES.map((item) => (
                <li
                  key={item}
                  className="flex items-start gap-2.5 rounded-lg px-2.5 py-1.5 transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800/40"
                >
                  <svg className="mt-0.5 size-3.5 shrink-0 text-emerald-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                  <span className="text-xs leading-relaxed text-zinc-600 dark:text-zinc-300">{item}</span>
                </li>
              ))}
            </ul>
          </div>
        </section>

        {/* Agent Integration */}
        <section className="space-y-2">
          <div className="flex items-center justify-between gap-2 px-1">
            <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Agent Integration</h3>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={handleCopySkill}
                className="inline-flex items-center gap-1.5 rounded-full border border-zinc-200/80 bg-white/80 px-3 py-1 text-xs font-medium text-zinc-600 shadow-sm transition-colors hover:border-accent hover:text-accent dark:border-zinc-700/60 dark:bg-zinc-900/70 dark:text-zinc-300 dark:hover:border-accent dark:hover:text-accent"
              >
                {skillCopied ? (
                  <>
                    <svg className="size-3 text-emerald-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                      <polyline points="20 6 9 17 4 12" />
                    </svg>
                    Copied
                  </>
                ) : (
                  <>
                    <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <rect x="9" y="9" width="13" height="13" rx="2" />
                      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                    </svg>
                    Copy skill
                  </>
                )}
              </button>
              <button
                type="button"
                onClick={handleDownloadSkill}
                className="inline-flex items-center gap-1.5 rounded-full bg-accent px-3 py-1 text-xs font-medium text-accent-contrast shadow-sm outline-none transition-opacity hover:opacity-90"
              >
                <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                  <polyline points="7 10 12 15 17 10" />
                  <line x1="12" y1="15" x2="12" y2="3" />
                </svg>
                Download .md
              </button>
            </div>
          </div>
          <div className="overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/70 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
            <div className="flex items-center gap-1.5 border-b border-zinc-100 px-3.5 py-2.5 dark:border-zinc-800/60">
              <span className="size-2 rounded-full bg-zinc-300 dark:bg-zinc-600" />
              <span className="size-2 rounded-full bg-zinc-300 dark:bg-zinc-600" />
              <span className="size-2 rounded-full bg-zinc-300 dark:bg-zinc-600" />
              <span className="ml-2 font-mono text-[10px] text-zinc-400 dark:text-zinc-500">heka-skill.md</span>
            </div>
            <pre className="overflow-auto p-3.5 font-mono text-[11px] leading-relaxed text-zinc-700 dark:text-zinc-300">
              {`$ heka list --json               # See all tasks
$ heka run my-task --json        # Trigger a task
$ heka status my-task --json     # Check result
$ heka logs my-task --json       # Read output
$ heka schedules missed --json   # Debug missed runs
$ heka daemon status             # Verify daemon`}
            </pre>
          </div>
          <p className="px-1 text-[11px] leading-relaxed text-zinc-500 dark:text-zinc-400">
            Copy or download the skill file into your AI assistant's skills directory. It teaches the agent every
            command, the{' '}
            <code className="rounded bg-zinc-100 px-1 py-0.5 font-mono text-[10px] dark:bg-zinc-800">--json</code>{' '}
            convention, and daemon error handling — structured output end-to-end.
          </p>
        </section>
      </div>

      {/* CLI Reference — double table */}
      <section className="space-y-2">
        <div className="flex items-baseline justify-between gap-2 px-1">
          <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">CLI Reference</h3>
          <p className="text-[11px] text-zinc-400 dark:text-zinc-500">
            every command accepts{' '}
            <code className="rounded bg-zinc-100 px-1 py-0.5 font-mono text-[10px] dark:bg-zinc-800">--json</code>
          </p>
        </div>
        <div className="grid items-start gap-4 md:grid-cols-2">
          {CLI_COMMAND_COLUMNS.map((columns, ci) => (
            <div
              key={ci}
              className="overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/70 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60"
            >
              {columns.map((section) => (
                <div key={section.title} className="border-b border-zinc-100 p-3 last:border-0 dark:border-zinc-800/60">
                  <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-zinc-400 dark:text-zinc-500">
                    {section.title}
                  </div>
                  {section.commands.map((cmd) => (
                    <div
                      key={cmd.cmd}
                      className="group flex items-center justify-between gap-3 rounded-lg px-1.5 py-1 transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800/40"
                    >
                      <code className="truncate font-mono text-xs text-zinc-800 dark:text-zinc-100">{cmd.cmd}</code>
                      <span className="flex min-w-0 items-center gap-1.5">
                        <span className="truncate text-right text-[11px] text-zinc-400 dark:text-zinc-500">{cmd.desc}</span>
                        <CopyButton text={cmd.cmd} />
                      </span>
                    </div>
                  ))}
                </div>
              ))}
            </div>
          ))}
        </div>
      </section>

      {/* Footer */}
      <div className="border-t border-zinc-200/80 pt-4 text-center text-[11px] text-zinc-400 dark:border-zinc-800 dark:text-zinc-500">
        Built with ❤️ by{' '}
        <button
          type="button"
          onClick={() => void openURL('https://www.ahs12.xyz/')}
          className="text-accent hover:underline"
        >
          Azizul Hakim
        </button>
        {' '}· {new Date().getFullYear()} ·{' '}
        <button
          type="button"
          onClick={() => void openURL('https://github.com/AHS12/heka')}
          className="text-accent hover:underline"
        >
          GitHub
        </button>
      </div>
    </div>
  )
}

const HOW_IT_WORKS = [
  {
    icon: (
      <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="2" y="3" width="20" height="14" rx="2" />
        <path d="M8 21h8M12 17v4" />
      </svg>
    ),
    title: 'GUI',
    desc: 'Visual task editor, schedule builder, and log viewer. Close it — the daemon keeps running.',
  },
  {
    icon: (
      <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <circle cx="12" cy="12" r="3" />
        <path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4M4.22 19.78l2.83-2.83M16.95 7.05l2.83-2.83" />
      </svg>
    ),
    title: 'Daemon',
    desc: 'The core of Heka. Schedules tasks, executes processes, stores history, and keeps running in the background.',
  },
  {
    icon: (
      <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <polyline points="4 17 10 11 4 5" />
        <line x1="12" y1="19" x2="20" y2="19" />
      </svg>
    ),
    title: 'CLI',
    desc: 'Everything from the terminal. Designed for programmers, scripts, and AI agents alike.',
  },
]

const SECURITY_FEATURES = [
  'Local-only IPC — no network exposure by default',
  'Secrets encrypted at rest with AES-256-GCM',
  'Secret values never cross the wire — only key names are exposed',
  'Task YAML is the canonical, portable definition',
  'Per-task notification control — no surprise background behavior',
]

const CLI_COMMAND_COLUMNS = [
  [
    {
      title: 'Tasks',
      commands: [
        {cmd: 'heka list', desc: 'List all tasks'},
        {cmd: 'heka run <slug>', desc: 'Run a task now'},
        {cmd: 'heka status <slug>', desc: 'Task status & last run'},
        {cmd: 'heka logs <slug>', desc: 'Latest run output'},
        {cmd: 'heka enable <slug>', desc: 'Enable a task'},
        {cmd: 'heka disable <slug>', desc: 'Disable a task'},
      ],
    },
    {
      title: 'Schedules',
      commands: [
        {cmd: 'heka schedules list', desc: 'List schedules & next runs'},
        {cmd: 'heka schedules reconcile', desc: 'Fire missed runs now'},
        {cmd: 'heka schedules missed', desc: 'List missed/skipped runs'},
      ],
    },
  ],
  [
    {
      title: 'Daemon',
      commands: [
        {cmd: 'heka daemon start', desc: 'Start background daemon'},
        {cmd: 'heka daemon stop', desc: 'Stop gracefully'},
        {cmd: 'heka daemon status', desc: 'Health & uptime'},
        {cmd: 'heka daemon startup on', desc: 'Start with system'},
        {cmd: 'heka daemon startup off', desc: 'Remove from startup'},
        {cmd: 'heka daemon startup status', desc: 'Check startup state'},
        {cmd: 'heka daemon watchdog status', desc: 'Check watchdog state'},
      ],
    },
    {
      title: 'Info',
      commands: [
        {cmd: 'heka --version', desc: 'Print version'},
        {cmd: 'heka --help', desc: 'Show help'},
      ],
    },
  ],
]
