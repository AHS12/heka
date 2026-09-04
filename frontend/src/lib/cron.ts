// Cron engine for the schedule builder, mirroring the daemon's robfig/cron v3
// parser (SecondOptional | Minute | Hour | Dom | Month | Dow | Descriptor) so
// previews and validation match the engine exactly.

export interface CronField {
  values: Set<number>
  star: boolean
}

export type CronSpecSchedule = {
  kind: 'spec'
  second: CronField
  minute: CronField
  hour: CronField
  dom: CronField
  month: CronField
  dow: CronField
}

export type CronSchedule = CronSpecSchedule | {kind: 'every'; delaySec: number}

export type CronParseResult =
  | {ok: true; schedule: CronSchedule; tz?: string}
  | {ok: false; error: string}

interface Bounds {
  min: number
  max: number
  names: Map<string, number> | null
}

const MONTH_NAMES = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec']
const DAY_NAMES = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat']

const BOUNDS: Record<'second' | 'minute' | 'hour' | 'dom' | 'month' | 'dow', Bounds> = {
  second: {min: 0, max: 59, names: null},
  minute: {min: 0, max: 59, names: null},
  hour: {min: 0, max: 23, names: null},
  dom: {min: 1, max: 31, names: null},
  month: {min: 1, max: 12, names: new Map(MONTH_NAMES.map((n, i) => [n, i + 1]))},
  dow: {min: 0, max: 6, names: new Map(DAY_NAMES.map((n, i) => [n, i]))},
}

export const MONTH_LABELS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
export const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

const DESCRIPTOR_SPECS: Record<string, CronSchedule> = {
  '@yearly': descriptorSpec([0], [0], [0], [1], [1], null),
  '@annually': descriptorSpec([0], [0], [0], [1], [1], null),
  '@monthly': descriptorSpec([0], [0], [0], [1], null, null),
  '@weekly': descriptorSpec([0], [0], [0], null, null, [0]),
  '@daily': descriptorSpec([0], [0], [0], null, null, null),
  '@midnight': descriptorSpec([0], [0], [0], null, null, null),
  '@hourly': descriptorSpec([0], [0], null, null, null, null),
}

function descriptorSpec(
  seconds: number[] | null,
  minutes: number[] | null,
  hours: number[] | null,
  dom: number[] | null,
  month: number[] | null,
  dow: number[] | null
): CronSchedule {
  const field = (values: number[] | null, min: number, max: number): CronField =>
    values === null
      ? {values: fullRange(min, max), star: true}
      : {values: new Set(values), star: false}
  return {
    kind: 'spec',
    second: field(seconds, 0, 59),
    minute: field(minutes, 0, 59),
    hour: field(hours, 0, 23),
    dom: field(dom, 1, 31),
    month: field(month, 1, 12),
    dow: field(dow, 0, 6),
  }
}

function fullRange(min: number, max: number): Set<number> {
  const out = new Set<number>()
  for (let i = min; i <= max; i++) out.add(i)
  return out
}

function parseIntOrName(expr: string, bound: Bounds): number {
  if (bound.names) {
    const named = bound.names.get(expr.toLowerCase())
    if (named !== undefined) return named
  }
  if (!/^\d+$/.test(expr)) {
    throw new Error(`failed to parse int from ${expr}`)
  }
  return Number(expr)
}

function getRange(expr: string, bound: Bounds): CronField {
  const rangeAndStep = expr.split('/')
  if (rangeAndStep.length > 2) {
    throw new Error(`too many slashes: ${expr}`)
  }
  const lowAndHigh = rangeAndStep[0].split('-')
  if (lowAndHigh.length > 2) {
    throw new Error(`too many hyphens: ${expr}`)
  }
  const singleDigit = lowAndHigh.length === 1

  let start: number
  let end: number
  let star = false
  if (lowAndHigh[0] === '*' || lowAndHigh[0] === '?') {
    start = bound.min
    end = bound.max
    star = true
  } else {
    start = parseIntOrName(lowAndHigh[0], bound)
    end = lowAndHigh.length === 2 ? parseIntOrName(lowAndHigh[1], bound) : start
  }

  let step = 1
  if (rangeAndStep.length === 2) {
    step = parseIntOrName(rangeAndStep[1], bound)
    if (singleDigit) {
      end = bound.max
    }
    if (step > 1) {
      star = false
    }
  }

  if (start < bound.min) {
    throw new Error(`beginning of range (${start}) below minimum (${bound.min}): ${expr}`)
  }
  if (end > bound.max) {
    throw new Error(`end of range (${end}) above maximum (${bound.max}): ${expr}`)
  }
  if (start > end) {
    throw new Error(`beginning of range (${start}) beyond end of range (${end}): ${expr}`)
  }
  if (step === 0) {
    throw new Error(`step of range should be a positive number: ${expr}`)
  }

  const values = new Set<number>()
  for (let i = start; i <= end; i += step) values.add(i)
  return {values, star}
}

function getField(field: string, bound: Bounds): CronField {
  const segments = field.split(',').filter((s) => s !== '')
  if (segments.length === 0) {
    return {values: new Set(), star: false}
  }
  const merged: CronField = {values: new Set(), star: false}
  for (const segment of segments) {
    const part = getRange(segment, bound)
    for (const v of part.values) merged.values.add(v)
    if (part.star) merged.star = true
  }
  return merged
}

// Go time.ParseDuration: [+-]?(decimal+unit)+ with units ns|us|ms|s|m|h.
function parseGoDuration(text: string): number {
  if (!/^[-+]?((\d+(\.\d*)?|\.\d+)(ns|us|ms|s|m|h))+$/.test(text)) {
    throw new Error(`time: invalid duration "${text}"`)
  }
  const negative = text.startsWith('-')
  const body = text.replace(/^[-+]/, '')
  const unitNanos: Record<string, number> = {ns: 1, us: 1e3, ms: 1e6, s: 1e9, m: 6e10, h: 3.6e12}
  let nanos = 0
  const re = /(\d+(?:\.\d*)?|\.\d+)(ns|us|ms|s|m|h)/g
  let match: RegExpExecArray | null
  while ((match = re.exec(body)) !== null) {
    nanos += Number(match[1]) * unitNanos[match[2]]
  }
  return negative ? -nanos : nanos
}

function parseDescriptor(descriptor: string): CronSchedule {
  const fixed = DESCRIPTOR_SPECS[descriptor]
  if (fixed) return fixed

  const every = '@every '
  if (descriptor.startsWith(every)) {
    const durationText = descriptor.slice(every.length)
    let nanos: number
    try {
      nanos = parseGoDuration(durationText)
    } catch {
      throw new Error(`failed to parse duration ${descriptor}: time: invalid duration "${durationText}"`)
    }
    if (nanos < 1e9) nanos = 1e9
    return {kind: 'every', delaySec: Math.floor(nanos / 1e9)}
  }

  throw new Error(`unrecognized descriptor: ${descriptor}`)
}

function isValidTimeZone(name: string): boolean {
  try {
    new Intl.DateTimeFormat('en', {timeZone: name})
    return true
  } catch {
    return false
  }
}

function parseSpecBody(spec: string): CronSchedule {
  if (spec.startsWith('@')) {
    return parseDescriptor(spec)
  }

  const fields = spec.split(/\s+/)
  if (fields.length < 5 || fields.length > 6) {
    throw new Error(`expected 5 to 6 fields, found ${fields.length}: ${fields.join(' ')}`)
  }
  if (fields.length === 5) {
    fields.unshift('0')
  }

  return {
    kind: 'spec',
    second: getField(fields[0], BOUNDS.second),
    minute: getField(fields[1], BOUNDS.minute),
    hour: getField(fields[2], BOUNDS.hour),
    dom: getField(fields[3], BOUNDS.dom),
    month: getField(fields[4], BOUNDS.month),
    dow: getField(fields[5], BOUNDS.dow),
  }
}

export function parseCron(input: string): CronParseResult {
  let spec = input.trim()
  if (!spec) {
    return {ok: false, error: 'empty spec string'}
  }

  let tz: string | undefined
  if (spec.startsWith('TZ=') || spec.startsWith('CRON_TZ=')) {
    const spaceIndex = spec.indexOf(' ')
    if (spaceIndex === -1) {
      return {ok: false, error: 'missing timezone value after TZ=/'}
    }
    const eqIndex = spec.indexOf('=')
    tz = spec.slice(eqIndex + 1, spaceIndex)
    if (!isValidTimeZone(tz)) {
      return {ok: false, error: `provided bad location ${tz}: unknown time zone`}
    }
    spec = spec.slice(spaceIndex).trim()
    if (!spec) {
      return {ok: false, error: 'empty spec string'}
    }
  }

  try {
    const schedule = parseSpecBody(spec)
    return tz ? {ok: true, schedule, tz} : {ok: true, schedule}
  } catch (err) {
    return {ok: false, error: err instanceof Error ? err.message : String(err)}
  }
}

export function validateCron(expr: string): string | null {
  const parsed = parseCron(expr)
  return parsed.ok ? null : parsed.error
}

// dayMatches mirrors robfig spec.go: AND when either side is a star, OR when
// both day-of-month and day-of-week are restricted.
function dayMatches(s: CronSpecSchedule, t: Date): boolean {
  const domMatch = s.dom.values.has(t.getDate())
  const dowMatch = s.dow.values.has(t.getDay())
  if (s.dom.star || s.dow.star) {
    return domMatch && dowMatch
  }
  return domMatch || dowMatch
}

// nextSpecTime ports robfig SpecSchedule.Next exactly, including the `added`
// flag and the DST midnight correction. Returns null past the 5-year limit.
function nextSpecTime(s: CronSpecSchedule, from: Date): Date | null {
  let added = false
  const yearLimit = from.getFullYear() + 5
  let t = new Date(from.getTime() + (1000 - from.getMilliseconds()))

  wrap: for (;;) {
    if (t.getFullYear() > yearLimit) return null

    while (!s.month.values.has(t.getMonth() + 1)) {
      if (!added) {
        added = true
        t = new Date(t.getFullYear(), t.getMonth(), 1, 0, 0, 0, 0)
      }
      t = new Date(t.getFullYear(), t.getMonth() + 1, 1, 0, 0, 0, 0)
      if (t.getMonth() === 0) continue wrap
    }

    while (!dayMatches(s, t)) {
      if (!added) {
        added = true
        t = new Date(t.getFullYear(), t.getMonth(), t.getDate(), 0, 0, 0, 0)
      }
      t = new Date(t.getFullYear(), t.getMonth(), t.getDate() + 1, 0, 0, 0, 0)
      if (t.getHours() !== 0) {
        const h = t.getHours()
        t = new Date(t.getTime() + (h > 12 ? 24 - h : -h) * 3600_000)
      }
      if (t.getDate() === 1) continue wrap
    }

    while (!s.hour.values.has(t.getHours())) {
      if (!added) {
        added = true
        t = new Date(t.getFullYear(), t.getMonth(), t.getDate(), t.getHours(), 0, 0, 0)
      }
      t = new Date(t.getTime() + 3600_000)
      if (t.getHours() === 0) continue wrap
    }

    while (!s.minute.values.has(t.getMinutes())) {
      if (!added) {
        added = true
        t = new Date(Math.floor(t.getTime() / 60_000) * 60_000)
      }
      t = new Date(t.getTime() + 60_000)
      if (t.getMinutes() === 0) continue wrap
    }

    while (!s.second.values.has(t.getSeconds())) {
      if (!added) {
        added = true
        t = new Date(Math.floor(t.getTime() / 1000) * 1000)
      }
      t = new Date(t.getTime() + 1000)
      if (t.getSeconds() === 0) continue wrap
    }

    return t
  }
}

// Next runs by chaining, the same walk the daemon's reconcile uses
// (sched.Next(prev) until count is reached). Returns [] when the schedule
// never fires, or null when preview is unsupported (TZ-qualified specs).
export function nextRuns(expr: string, from: Date, count: number): Date[] | null {
  const parsed = parseCron(expr)
  if (!parsed.ok || parsed.tz) return null

  const out: Date[] = []
  if (parsed.schedule.kind === 'every') {
    const delayMs = parsed.schedule.delaySec * 1000
    let t = Math.floor(from.getTime() / 1000) * 1000 + delayMs
    for (let i = 0; i < count; i++) {
      out.push(new Date(t))
      t += delayMs
    }
    return out
  }

  let t: Date | null = from
  for (let i = 0; i < count; i++) {
    t = nextSpecTime(parsed.schedule, t as Date)
    if (!t) break
    out.push(t)
  }
  return out
}

// Compact expression for a set of field values; contiguous runs become ranges
// and a full range collapses to `*`.
export function compressValues(values: Iterable<number>, min: number, max: number): string {
  const sorted = [...new Set(values)].sort((a, b) => a - b)
  if (sorted.length === 0) return ''
  if (sorted.length === max - min + 1) return '*'
  const parts: string[] = []
  let runStart = sorted[0]
  let prev = sorted[0]
  const flush = () => {
    parts.push(runStart === prev ? `${runStart}` : `${runStart}-${prev}`)
  }
  for (let i = 1; i < sorted.length; i++) {
    if (sorted[i] === prev + 1) {
      prev = sorted[i]
      continue
    }
    flush()
    runStart = sorted[i]
    prev = sorted[i]
  }
  flush()
  return parts.join(',')
}

// Inverse of compressValues for plain number/range/list fields — null when the
// expression uses stars, steps, or names the builder cannot represent.
export function expandSimpleField(expr: string, min: number, max: number): number[] | null {
  if (expr === '*' || expr === '?') return null
  const out: number[] = []
  for (const segment of expr.split(',')) {
    if (!segment) return null
    if (/[*?/]/.test(segment)) return null
    const range = segment.split('-')
    if (range.length > 2) return null
    const nums: number[] = []
    for (const part of range) {
      if (!/^\d+$/.test(part)) return null
      const n = Number(part)
      if (n < min || n > max) return null
      nums.push(n)
    }
    if (range.length === 2) {
      if (nums[0] > nums[1]) return null
      for (let i = nums[0]; i <= nums[1]; i++) out.push(i)
    } else {
      out.push(nums[0])
    }
  }
  return [...new Set(out)].sort((a, b) => a - b)
}

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

function dayLabelsFor(values: number[]): string {
  return compressValues(values, 0, 6)
    .split(',')
    .map((v) => {
      if (v.includes('-')) {
        const [a, b] = v.split('-').map(Number)
        return `${DAY_LABELS[a]}\u2013${DAY_LABELS[b]}`
      }
      return DAY_LABELS[Number(v)]
    })
    .join(', ')
}

function monthLabelsFor(values: number[]): string {
  return compressValues(values, 1, 12)
    .split(',')
    .map((v) => {
      if (v.includes('-')) {
        const [a, b] = v.split('-').map(Number)
        return `${MONTH_LABELS[a - 1]}\u2013${MONTH_LABELS[b - 1]}`
      }
      return MONTH_LABELS[Number(v) - 1]
    })
    .join(', ')
}

function uniformStep(values: number[], min: number, max: number): number | null {
  if (values.length < 2) return null
  const step = values[1] - values[0]
  if (step <= 1) return null
  for (let i = 1; i < values.length; i++) {
    if (values[i] - values[i - 1] !== step) return null
  }
  if (values[0] !== min) return null
  const span = max - min + 1
  if ((values[values.length - 1] + step - min) % span !== 0) return null
  return step
}

function everyDelayLabel(delaySec: number): string {
  if (delaySec < 60) return `every ${delaySec} seconds`
  if (delaySec < 3600) {
    const m = Math.floor(delaySec / 60)
    const s = delaySec % 60
    return s ? `every ${m}m ${s}s` : `every ${m} minutes`
  }
  const h = Math.floor(delaySec / 3600)
  const m = Math.floor((delaySec % 3600) / 60)
  return m ? `every ${h}h ${m}m` : `every ${h} hour${h > 1 ? 's' : ''}`
}

const DESCRIPTOR_LABELS: Record<string, string> = {
  '@yearly': 'Every year on Jan 1 at 00:00',
  '@annually': 'Every year on Jan 1 at 00:00',
  '@monthly': 'Every month on day 1 at 00:00',
  '@weekly': 'Every week on Sun at 00:00',
  '@daily': 'Every day at 00:00',
  '@midnight': 'Every day at 00:00',
  '@hourly': 'Every hour at :00',
}

// Human-readable sentence for a cron expression; falls back to the raw text
// for shapes the summarizer does not model.
export function describeCron(expr: string): string {
  const trimmed = expr.trim()
  const parsed = parseCron(trimmed)
  if (!parsed.ok) return trimmed
  if (parsed.schedule.kind === 'every') {
    const label = everyDelayLabel(parsed.schedule.delaySec)
    return label.charAt(0).toUpperCase() + label.slice(1)
  }
  const descriptorLabel = DESCRIPTOR_LABELS[trimmed.toLowerCase()]
  if (descriptorLabel) return descriptorLabel
  const s = parsed.schedule

  const allOf = (f: CronField, min: number, max: number) =>
    f.star || (f.values.size === max - min + 1)

  if (allOf(s.dom, 1, 31) && allOf(s.dow, 0, 6) && allOf(s.month, 1, 12)) {
    if (s.hour.star && s.minute.star) {
      return s.second.star ? 'Every second' : 'Every minute'
    }
    if (s.hour.star && s.minute.values.size > 0) {
      const step = uniformStep([...s.minute.values], 0, 59)
      if (step) return `Every ${step} minutes`
    }
    if (s.minute.values.size === 1 && s.hour.star) {
      return `Every hour at :${pad2([...s.minute.values][0])}`
    }
    if (s.minute.values.size === 1 && !s.hour.star) {
      const step = uniformStep([...s.hour.values], 0, 23)
      if (step) return `Every ${step} hours at :${pad2([...s.minute.values][0])}`
    }
  }

  const times: string[] = []
  if (!s.hour.star && !s.minute.star) {
    const minutes = [...s.minute.values].sort((a, b) => a - b)
    const hours = [...s.hour.values].sort((a, b) => a - b)
    for (const h of hours) {
      for (const m of minutes) {
        times.push(`${pad2(h)}:${pad2(m)}`)
      }
    }
  }
  const timePart =
    times.length > 0 && times.length <= 6
      ? `At ${times.join(', ')}`
      : times.length > 6
        ? `At minute ${[...s.minute.values].sort((a, b) => a - b).join(', ')} past hour ${[...s.hour.values].sort((a, b) => a - b).join(', ')}`
        : s.hour.star && !s.minute.star
          ? `At :${pad2([...s.minute.values][0] ?? 0)} past every hour`
          : s.minute.star
            ? 'Every minute'
            : 'At unscheduled times'

  const domAll = allOf(s.dom, 1, 31)
  const dowAll = allOf(s.dow, 0, 6)
  let dayPart: string
  if (domAll && dowAll) {
    dayPart = 'every day'
  } else if (!domAll && dowAll) {
    const dayExpr = compressValues(s.dom.values, 1, 31)
    dayPart = /^[\d,-]+$/.test(dayExpr) && !dayExpr.includes(',') && dayExpr.includes('-')
      ? `on day ${dayExpr.replace('-', '\u2013')}`
      : `on day${dayExpr.includes(',') ? 's' : ''} ${dayExpr.split(',').map((p) => p.replace('-', '\u2013')).join(', ')}`
  } else if (domAll && !dowAll) {
    dayPart = `on ${dayLabelsFor([...s.dow.values])}`
  } else {
    dayPart = `on day ${compressValues(s.dom.values, 1, 31).split(',').map((p) => p.replace('-', '\u2013')).join(', ')} or ${dayLabelsFor([...s.dow.values])}`
  }

  const monthAll = allOf(s.month, 1, 12)
  const monthPart = monthAll ? '' : `, in ${monthLabelsFor([...s.month.values])}`

  const secondPart = s.second.star || (s.second.values.size === 1 && s.second.values.has(0))
    ? ''
    : ` (second ${[...s.second.values].sort((a, b) => a - b).join(', ')})`

  return `${timePart}, ${dayPart}${monthPart}${secondPart}`
}

// Raw field tokens for the advanced breakdown display. Null for descriptors.
export function cronFields(expr: string): {labels: Array<{name: string; value: string}>; hasSeconds: boolean} | null {
  const spec = expr.trim()
  if (!spec || spec.startsWith('@') || spec.startsWith('TZ=') || spec.startsWith('CRON_TZ=')) {
    return null
  }
  const fields = spec.split(/\s+/)
  if (fields.length < 5 || fields.length > 6) return null
  const hasSeconds = fields.length === 6
  if (fields.length === 5) {
    fields.unshift('0')
  }
  const all = [
    {name: 'Sec', value: fields[0]},
    {name: 'Min', value: fields[1]},
    {name: 'Hour', value: fields[2]},
    {name: 'Day', value: fields[3]},
    {name: 'Month', value: fields[4]},
    {name: 'Weekday', value: fields[5]},
  ]
  return {labels: hasSeconds ? all : all.slice(1), hasSeconds}
}
