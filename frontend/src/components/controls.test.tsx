// TimePickerField tests: the "HH:MM" ↔ Time conversion helpers and a smoke
// render of the segmented HeroUI TimeField wrapper (jsdom renders the
// datefield segments; no native dropdown is involved).
import {describe, expect, it} from 'vitest'
import {render, screen} from '@testing-library/react'
import {Time} from '@internationalized/date'
import {TimePickerField, parseHHMM, timeToHHMM} from './controls'

describe('parseHHMM', () => {
  it('parses valid 24h times', () => {
    expect(parseHHMM('03:00')).toEqual(new Time(3, 0))
    expect(parseHHMM('23:59')).toEqual(new Time(23, 59))
    expect(parseHHMM('9:05')).toEqual(new Time(9, 5))
  })

  it('rejects invalid or empty input', () => {
    expect(parseHHMM('')).toBeNull()
    expect(parseHHMM(null)).toBeNull()
    expect(parseHHMM('25:00')).toBeNull()
    expect(parseHHMM('12:99')).toBeNull()
    expect(parseHHMM('not-a-time')).toBeNull()
  })
})

describe('timeToHHMM', () => {
  it('zero-pads hour and minute', () => {
    expect(timeToHHMM(new Time(3, 0))).toBe('03:00')
    expect(timeToHHMM(new Time(23, 59))).toBe('23:59')
  })

  it('returns null for null', () => {
    expect(timeToHHMM(null)).toBeNull()
  })
})

describe('TimePickerField', () => {
  it('renders a labelled segmented group for the initial value', () => {
    render(<TimePickerField aria-label="Backup time of day" value="03:00" onChange={() => {}} />)
    // react-aria labels both the outer group and the inner input group with
    // the same aria-label — query with *AllBy and assert the group exists.
    const groups = screen.getAllByLabelText('Backup time of day')
    expect(groups.length).toBeGreaterThan(0)
    // Segmented entry (hour spinbutton), not a native dropdown.
    expect(document.querySelector('[data-type="hour"]')).toBeTruthy()
    // The hidden mirror carries the value for form submission.
    const mirror = document.querySelector('input[hidden]') as HTMLInputElement | null
    expect(mirror?.value).toBe('03:00:00')
  })
})
