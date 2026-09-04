import {describe, expect, it} from 'vitest'
import {crossProductWarning, cronToPattern, patternToCron, sortTimes} from './schedulePattern'

describe('patternToCron', () => {
  it('emits the example schedules from the builder shapes', () => {
    expect(patternToCron({kind: 'every', n: 15, unit: 'm'})).toBe('@every 15m')
    expect(patternToCron({kind: 'every', n: 2, unit: 'h'})).toBe('@every 2h')
    expect(patternToCron({kind: 'daily', times: [{hh: '09', mm: '00'}]})).toBe('0 9 * * *')
    expect(patternToCron({kind: 'daily', times: [{hh: '09', mm: '00'}, {hh: '17', mm: '00'}]})).toBe('0 9,17 * * *')
    expect(patternToCron({kind: 'weekly', days: [1, 2, 3, 4, 5], times: [{hh: '08', mm: '30'}]})).toBe('30 8 * * 1-5')
    expect(
      patternToCron({
        kind: 'monthly',
        days: [],
        ranges: [[23, 26]],
        months: [],
        times: [{hh: '09', mm: '00'}],
      })
    ).toBe('0 9 23-26 * *')
    expect(
      patternToCron({
        kind: 'monthly',
        days: [],
        ranges: [[23, 26]],
        months: [9],
        times: [{hh: '09', mm: '00'}],
      })
    ).toBe('0 9 23-26 9 *')
    expect(
      patternToCron({
        kind: 'monthly',
        days: [1, 15],
        ranges: [],
        months: [],
        times: [{hh: '09', mm: '00'}],
      })
    ).toBe('0 9 1,15 * *')
  })
})

describe('cronToPattern round-trips', () => {
  const cases: Array<[string, string]> = [
    ['0 9 * * *', 'daily'],
    ['0 9,17 * * *', 'daily'],
    ['30 8 * * 1-5', 'weekly'],
    ['0 9 * * 0,6', 'weekly'],
    ['0 9 23-26 * *', 'monthly'],
    ['0 9 23-26 9 *', 'monthly'],
    ['0 9 1,15 * *', 'monthly'],
    ['0 9 * 9 *', 'monthly'],
    ['15,45 9 15 * *', 'monthly'],
    ['@every 15m', 'every'],
    ['@every 2h', 'every'],
  ]

  for (const [expr, kind] of cases) {
    it(`round-trips "${expr}" through ${kind}`, () => {
      const pattern = cronToPattern(expr)
      expect(pattern.kind).toBe(kind)
      expect(patternToCron(pattern)).toBe(expr)
    })
  }

  it('maps fixed descriptors to their equivalent builder patterns', () => {
    expect(patternToCron(cronToPattern('@daily'))).toBe('0 0 * * *')
    expect(patternToCron(cronToPattern('@hourly'))).toBe('@every 1h')
    expect(patternToCron(cronToPattern('@weekly'))).toBe('0 0 * * 0')
    expect(patternToCron(cronToPattern('@monthly'))).toBe('0 0 1 * *')
    expect(patternToCron(cronToPattern('@yearly'))).toBe('0 0 1 1 *')
  })

  it('reconstructs times from minute/hour lists', () => {
    const pattern = cronToPattern('30 8,20 * * 1-5')
    expect(pattern.kind).toBe('weekly')
    if (pattern.kind === 'weekly') {
      expect(sortTimes(pattern.times)).toEqual([
        {hh: '08', mm: '30'},
        {hh: '20', mm: '30'},
      ])
      expect(pattern.days).toEqual([1, 2, 3, 4, 5])
    }
  })

  it('maps @daily to midnight', () => {
    const pattern = cronToPattern('@daily')
    expect(pattern.kind).toBe('daily')
    if (pattern.kind === 'daily') {
      expect(pattern.times).toEqual([{hh: '00', mm: '00'}])
    }
  })

  it('falls back to the raw cron pattern for foreign expressions', () => {
    for (const expr of [
      '*/15 * * * *',
      '0 12 * * MON',
      '0 9 23-26 * 1',
      '1/15 9 * * *',
      '0 0 9 23-26 * 30',
      '@every 1h30m',
      '0 9 * * 7',
      'garbage',
      '',
    ]) {
      const pattern = cronToPattern(expr)
      expect(pattern.kind, expr).toBe('cron')
    }
  })
})

describe('crossProductWarning', () => {
  it('is silent when times share an hour or a minute', () => {
    expect(
      crossProductWarning([
        {hh: '09', mm: '00'},
        {hh: '17', mm: '00'},
      ])
    ).toBeNull()
    expect(
      crossProductWarning([
        {hh: '09', mm: '00'},
        {hh: '09', mm: '30'},
      ])
    ).toBeNull()
    expect(crossProductWarning([{hh: '09', mm: '00'}])).toBeNull()
  })

  it('flags cross-product firing and lists the extras', () => {
    const warning = crossProductWarning([
      {hh: '09', mm: '05'},
      {hh: '10', mm: '42'},
    ])
    expect(warning).toContain('09:42')
    expect(warning).toContain('10:05')
  })
})
