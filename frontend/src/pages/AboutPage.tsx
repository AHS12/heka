import {useState} from 'react'

const APP_VERSION = '0.1.0'

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
      onClick={handleCopy}
      className="ml-2 inline-flex items-center gap-1 rounded-md border border-zinc-200 bg-white/80 px-1.5 py-0.5 text-[10px] font-medium text-zinc-500 shadow-sm outline-none transition-all hover:border-accent hover:text-accent dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:border-accent dark:hover:text-accent"
    >
      {copied ? (
        <>
          <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
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
          Copy
        </>
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

# Register OS-level watchdog (auto-restart)
heka daemon watchdog install

# Check watchdog status
heka daemon watchdog status
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
    <div className="mx-auto max-w-2xl space-y-10">
      {/* Hero */}
      <section className="space-y-3">
        <div className="flex items-center gap-3">
          <img src="/appicon.png" alt="Heka" className="size-12 rounded-2xl shadow-sm" />
          <div>
            <h2 className="text-xl font-bold tracking-tight">Heka <span className="text-sm font-normal text-zinc-400 dark:text-zinc-500">v{APP_VERSION}</span></h2>
            <p className="text-sm text-zinc-500 dark:text-zinc-400">
              A local task runner & scheduler for programmers — with first-class AI agent support.
            </p>
          </div>
        </div>
        <p className="text-sm leading-relaxed text-zinc-600 dark:text-zinc-300">
          A persistent background daemon with a GUI management console and CLI clients.
          Close the GUI — Heka keeps running. Scripts decide what the work actually does;
          Heka schedules and executes.
        </p>
        <a
          href="https://github.com/AHS12/heka"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1.5 text-xs text-zinc-400 transition-colors hover:text-accent dark:text-zinc-500"
        >
          <svg className="size-3.5" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
          </svg>
          View source on GitHub
        </a>
      </section>

      {/* How It Works */}
      <section className="space-y-3">
        <div className="space-y-1">
          <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
            How It Works
          </h3>
          <p className="text-xs text-zinc-500 dark:text-zinc-400">
            Create and control tasks from anywhere. Heka's daemon keeps them running in the background.
          </p>
        </div>
        <div className="grid gap-3 sm:grid-cols-3">
          {[
            {
              icon: (
                <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <rect x="2" y="3" width="20" height="14" rx="2" />
                  <path d="M8 21h8M12 17v4" />
                </svg>
              ),
              title: 'GUI',
              desc: 'Visual task editor, schedule builder, and log viewer. Close it — the daemon keeps running.',
            },
            {
              icon: (
                <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <circle cx="12" cy="12" r="3" />
                  <path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4M4.22 19.78l2.83-2.83M16.95 7.05l2.83-2.83" />
                </svg>
              ),
              title: 'Daemon',
              desc: 'The core of Heka. Schedules tasks, executes processes, stores history, and keeps running in the background.',
              center: true,
            },
            {
              icon: (
                <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <polyline points="4 17 10 11 4 5" />
                  <line x1="12" y1="19" x2="20" y2="19" />
                </svg>
              ),
              title: 'CLI',
              desc: 'Everything from the terminal. Designed for programmers, scripts, and AI agents alike.',
            },
          ].map((item) => (
            <div
              key={item.title}
              className={`rounded-2xl border p-4 shadow-sm backdrop-blur-sm ${
                item.center
                  ? 'border-accent/30 bg-accent/5 dark:border-accent/20 dark:bg-accent/5'
                  : 'border-zinc-200/80 bg-white/70 dark:border-zinc-800 dark:bg-zinc-900/60'
              }`}
            >
              <div className={`mb-2 ${item.center ? 'text-accent' : 'text-zinc-400 dark:text-zinc-500'}`}>
                {item.icon}
              </div>
              <div className="mb-1 text-sm font-semibold text-zinc-800 dark:text-zinc-100">
                {item.title}
              </div>
              <p className="text-[11px] leading-relaxed text-zinc-500 dark:text-zinc-400">
                {item.desc}
              </p>
            </div>
          ))}
        </div>
      </section>

      {/* CLI Commands */}
      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
          CLI Reference
        </h3>
        <p className="text-xs text-zinc-500 dark:text-zinc-400">
          Every command supports <code className="rounded bg-zinc-100 px-1 py-0.5 font-mono text-[11px] dark:bg-zinc-800">--json</code> for machine-readable output. All commands go through the daemon via local IPC.
        </p>
        <div className="overflow-hidden rounded-2xl border border-zinc-200/80 bg-white/70 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
          {CLI_COMMANDS.map((section, i) => (
            <div key={section.title}>
              {i > 0 && <div className="border-t border-zinc-100 dark:border-zinc-800/60" />}
              <div className="bg-zinc-50/50 px-4 py-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-400 dark:bg-zinc-800/30 dark:text-zinc-500">
                {section.title}
              </div>
              {section.commands.map((cmd) => (
                <div
                  key={cmd.cmd}
                  className="flex items-center justify-between border-b border-zinc-100 px-4 py-2 last:border-0 dark:border-zinc-800/60"
                >
                  <code className="font-mono text-xs text-zinc-800 dark:text-zinc-100">
                    {cmd.cmd}
                  </code>
                  <span className="flex items-center">
                    <span className="text-[11px] text-zinc-400 dark:text-zinc-500">
                      {cmd.desc}
                    </span>
                    <CopyButton text={cmd.cmd} />
                  </span>
                </div>
              ))}
            </div>
          ))}
        </div>
      </section>

      {/* Agent Skill */}
      <section className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
            Agent Integration
          </h3>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleCopySkill}
              className="inline-flex items-center gap-1.5 rounded-full border border-zinc-200/80 bg-white/80 px-3 py-1 text-xs font-medium shadow-sm dark:border-zinc-700/60 dark:bg-zinc-900/70"
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
        <p className="text-xs text-zinc-500 dark:text-zinc-400">
          Download the agent skill file for your AI coding assistant. It teaches the agent how to use Heka's CLI to manage tasks, check status, and view logs — all with structured JSON output.
        </p>
        <div className="rounded-2xl border border-zinc-200/80 bg-white/70 p-4 shadow-sm backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-900/60">
          <pre className="overflow-auto rounded-xl border border-zinc-200 bg-zinc-50 p-3 font-mono text-[11px] leading-relaxed text-zinc-700 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-300">
            {`# Install the skill in your agent
# Copy heka-skill.md to your skills directory

# Then the agent can:
heka list --json                    # See all tasks
heka run my-task --json             # Trigger a task
heka status my-task --json          # Check result
heka logs my-task --json            # Read output
heka daemon status                  # Verify daemon`}
          </pre>
        </div>
      </section>

      {/* Security */}
      <section className="space-y-2">
        <h3 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
          Security
        </h3>
        <ul className="space-y-1 text-[11px] text-zinc-500 dark:text-zinc-400">
          <li className="flex items-start gap-2">
            <svg className="mt-0.5 size-3 shrink-0 text-emerald-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            Local-only IPC — no network exposure by default
          </li>
          <li className="flex items-start gap-2">
            <svg className="mt-0.5 size-3 shrink-0 text-emerald-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            Secrets encrypted at rest with AES-256-GCM
          </li>
          <li className="flex items-start gap-2">
            <svg className="mt-0.5 size-3 shrink-0 text-emerald-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            Task YAML is the canonical, portable definition
          </li>
          <li className="flex items-start gap-2">
            <svg className="mt-0.5 size-3 shrink-0 text-emerald-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            Per-task notification control — no surprise background behavior
          </li>
        </ul>
      </section>

      {/* Footer */}
      <div className="border-t border-zinc-200/80 pt-4 text-center text-[11px] text-zinc-400 dark:border-zinc-800 dark:text-zinc-500">
        Built with ❤️ by{' '}
        <a href="https://www.ahs12.xyz/" target="_blank" rel="noopener noreferrer" className="text-accent hover:underline">
          Azizul Hakim
        </a>
        {' '}· {new Date().getFullYear()} ·{' '}
        <a href="https://github.com/AHS12/heka" target="_blank" rel="noopener noreferrer" className="text-accent hover:underline">
          GitHub
        </a>
      </div>
    </div>
  )
}

const CLI_COMMANDS = [
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
    title: 'Daemon',
    commands: [
      {cmd: 'heka daemon start', desc: 'Start background daemon'},
      {cmd: 'heka daemon stop', desc: 'Stop gracefully'},
      {cmd: 'heka daemon status', desc: 'Health & uptime'},
      {cmd: 'heka daemon watch', desc: 'Watchdog loop'},
      {cmd: 'heka daemon watchdog install', desc: 'Register OS watchdog'},
      {cmd: 'heka daemon watchdog status', desc: 'Check watchdog state'},
    ],
  },
]
