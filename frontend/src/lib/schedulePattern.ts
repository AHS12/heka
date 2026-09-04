// Schedule pattern model: the builder's typed shapes and their cron
// emission / reverse-parsing. cronToPattern reconstructs builder state for
// editing and falls back to the raw 'cron' pattern for foreign expressions.

import {compressValues, expandSimpleField} from './cron'

export interface TimeValue {
  hh: string
  mm: string
}

export type SchedulePattern =
  | {kind: 'every'; n: number; unit: 'm' | 'h'}
  | {kind: 'daily'; times: TimeValue[]}
  | {kind: 'weekly'; days: number[]; times: TimeValue[]}
  | {
      kind: 'monthly'
      days: number[]
      ranges: Array<[number, number]>
      months: number[]
      times: TimeValue[]
    }
  | {kind: 'once'; runAt: string}
  | {kind: 'cron'; expr: string}

export type PatternKind = SchedulePattern['kind']

export function emptyPattern(kind: PatternKind): SchedulePattern {
  switch (kind) {
    case 'every':
      return {kind: 'every', n: 15, unit: 'm'}
    case 'daily':
      return {kind: 'daily', times: [{hh: '09', mm: '00'}]}
    case 'weekly':
      return {kind: 'weekly', days: [1], times: [{hh: '09', mm: '00'}]}
    case 'monthly':
      return {kind: 'monthly', days: [], ranges: [], months: [], times: [{hh: '09', mm: '00'}]}
    case 'once':
      return {kind: 'once', runAt: ''}
    case 'cron':
      return {kind: 'cron', expr: ''}
  }
}

export function patternToKind(pattern: SchedulePattern): 'recurring' | 'onetime' {
  return pattern.kind === 'once' ? 'onetime' : 'recurring'
}

export function patternToCron(pattern: SchedulePattern): string {
  switch (pattern.kind) {
    case 'every': {
      const n = Math.max(1, Math.floor(pattern.n))
      return `@every ${n}${pattern.unit}`
    }
    case 'daily':
      return `${minuteField(pattern.times)} ${hourField(pattern.times)} * * *`
    case 'weekly':
      return `${minuteField(pattern.times)} ${hourField(pattern.times)} * * ${compressValues(pattern.days, 0, 6)}`
    case 'monthly': {
      const domValues = [...pattern.days]
      for (const [a, b] of pattern.ranges) {
        for (let d = a; d <= b; d++) domValues.push(d)
      }
      const dom = domValues.length === 0 ? '*' : compressValues(domValues, 1, 31)
      const month = pattern.months.length === 0 ? '*' : compressValues(pattern.months, 1, 12)
      return `${minuteField(pattern.times)} ${hourField(pattern.times)} ${dom} ${month} *`
    }
    case 'once':
      return ''
    case 'cron':
      return pattern.expr.trim()
  }
}

function minuteField(times: TimeValue[]): string {
  return compressValues(times.map((t) => Number(t.mm)), 0, 59)
}

function hourField(times: TimeValue[]): string {
  return compressValues(times.map((t) => Number(t.hh)), 0, 23)
}

export function sortTimes(times: TimeValue[]): TimeValue[] {
  return [...times]
    .map((t) => ({hh: pad2(Number(t.hh)), mm: pad2(Number(t.mm))}))
    .sort((a, b) => a.hh.localeCompare(b.hh) || a.mm.localeCompare(b.mm))
}

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

// When times differ in both hour and minute, the emitted cron fires the cross
// product (9:05 + 10:42 also fires 9:42 and 10:05). Surface that honestly.
export function crossProductWarning(times: TimeValue[]): string | null {
  const hours = [...new Set(times.map((t) => pad2(Number(t.hh))))].sort()
  const minutes = [...new Set(times.map((t) => pad2(Number(t.mm))))].sort()
  if (times.length < 2 || hours.length < 2 || minutes.length < 2) return null
  const selected = new Set(times.map((t) => `${pad2(Number(t.hh))}:${pad2(Number(t.mm))}`))
  const extras: string[] = []
  for (const hh of hours) {
    for (const mm of minutes) {
      const key = `${hh}:${mm}`
      if (!selected.has(key)) extras.push(key)
    }
  }
  if (extras.length === 0) return null
  const shown = extras.slice(0, 4).join(', ')
  const more = extras.length > 4 ? ` and ${extras.length - 4} more` : ''
  return `Cron pairs every selected hour with every selected minute, so it also fires at ${shown}${more}.`
}

const EVERY_RE = /^@every (\d+)([mh])$/
const EVERY_COMPOUND_RE = /^@every \d+(?:\.\d+)?(?:ns|us|ms|s|m|h)(?:\d+(?:\.\d+)?(?:ns|us|ms|s|m|h))*$/

// Reconstruct a builder pattern from a cron expression. Anything the builder
// cannot represent (steps, names, seconds, both day fields restricted, large
// time sets) falls back to the raw 'cron' pattern.
export function cronToPattern(expr: string): SchedulePattern {
  const trimmed = expr.trim()

  const simpleEvery = trimmed.match(EVERY_RE)
  if (simpleEvery) {
    const n = Number(simpleEvery[1])
    if (n >= 1) return {kind: 'every', n, unit: simpleEvery[2] as 'm' | 'h'}
  }

  const fixed = mapFixedDescriptor(trimmed)
  if (fixed) return fixed

  const fields = trimmed.split(/\s+/)
  if (fields.length !== 5 && fields.length !== 6) {
    return {kind: 'cron', expr: trimmed}
  }
  if (fields.length === 6 && fields[0] !== '0') {
    return {kind: 'cron', expr: trimmed}
  }
  const [minute, hour, dom, month, dow] = fields.slice(-5)

  if (/@/.test(trimmed) || /[*?/]/.test(minute) || /[*?/]/.test(hour)) {
    return {kind: 'cron', expr: trimmed}
  }

  const minutes = expandSimpleField(minute, 0, 59)
  const hours = expandSimpleField(hour, 0, 23)
  if (!minutes || !hours) {
    return {kind: 'cron', expr: trimmed}
  }
  const times: TimeValue[] = []
  for (const hh of hours) {
    for (const mm of minutes) {
      times.push({hh: pad2(hh), mm: pad2(mm)})
    }
  }
  if (times.length === 0 || times.length > 8) {
    return {kind: 'cron', expr: trimmed}
  }

  const domStar = dom === '*' || dom === '?'
  const dowStar = dow === '*' || dow === '?'
  const monthStar = month === '*' || month === '?'

  if (domStar && dowStar) {
    if (monthStar) {
      return {kind: 'daily', times}
    }
    const months = expandSimpleField(month, 1, 12)
    if (!months) return {kind: 'cron', expr: trimmed}
    return {kind: 'monthly', days: [], ranges: [], months, times}
  }

  if (!domStar && dowStar) {
    const days = expandSimpleField(dom, 1, 31)
    if (!days) return {kind: 'cron', expr: trimmed}
    const months = monthStar ? [] : expandSimpleField(month, 1, 12)
    if (monthStar || months) {
      return {kind: 'monthly', days, ranges: [], months: months ?? [], times}
    }
    return {kind: 'cron', expr: trimmed}
  }

  if (domStar && !dowStar) {
    const days = expandSimpleField(dow, 0, 6)
    if (!days) return {kind: 'cron', expr: trimmed}
    return {kind: 'weekly', days, times}
  }

  return {kind: 'cron', expr: trimmed}
}

// Map fixed descriptors to their equivalent builder pattern for editing.
function mapFixedDescriptor(expr: string): SchedulePattern | null {
  switch (expr) {
    case '@daily':
    case '@midnight':
      return {kind: 'daily', times: [{hh: '00', mm: '00'}]}
    case '@hourly':
      return {kind: 'every', n: 1, unit: 'h'}
    case '@weekly':
      return {kind: 'weekly', days: [0], times: [{hh: '00', mm: '00'}]}
    case '@monthly':
      return {kind: 'monthly', days: [1], ranges: [], months: [], times: [{hh: '00', mm: '00'}]}
    case '@yearly':
    case '@annually':
      return {kind: 'monthly', days: [1], ranges: [], months: [1], times: [{hh: '00', mm: '00'}]}
    default:
      if (EVERY_COMPOUND_RE.test(expr)) return {kind: 'cron', expr}
      return null
  }
}
