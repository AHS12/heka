import {describe, expect, it} from 'vitest'
import {fireEvent, render, screen} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {CronPreview} from './CronPreview'
import {
  ScheduleForm,
  draftFromSchedule,
  draftToPayload,
  emptyScheduleDraft,
  validateScheduleDraft,
} from './ScheduleForm'
import type {ScheduleDraft} from './ScheduleForm'
import {emptyPattern} from '../../lib/schedulePattern'
import type {TaskSummary} from '../../lib/api'

const tasks: TaskSummary[] = [{slug: 'backup', name: 'Backup', type: 'script', runtime: 'powershell', enabled: true, updated_at: ''}]

function renderForm(draft = emptyScheduleDraft(), onChange = (_next: ScheduleDraft) => {}) {
  return render(
    <ScheduleForm draft={draft} onChange={onChange} tasks={tasks} errors={[]} />
  )
}

describe('ScheduleForm pattern picker', () => {
  it('shows the six pattern sections', () => {
    renderForm()
    for (const id of ['every', 'daily', 'weekly', 'monthly', 'once', 'cron']) {
      expect(screen.getByTestId(`pattern-option-${id}`)).toBeTruthy()
    }
  })

  it('switches panels when a pattern is chosen', async () => {
    const user = userEvent.setup()
    let current = emptyScheduleDraft()
    const onChange = (next: ScheduleDraft) => {
      current = next
      rerender()
    }
    const view = renderForm(current, onChange)
    const rerender = () =>
      view.rerender(
        <ScheduleForm draft={current} onChange={onChange} tasks={tasks} errors={[]} />
      )

    await user.click(screen.getByTestId('pattern-option-weekly'))
    expect(screen.getByRole('button', {name: 'Mon'})).toBeTruthy()

    await user.click(screen.getByTestId('pattern-option-monthly'))
    expect(screen.getByRole('button', {name: 'Day 23'})).toBeTruthy()

    await user.click(screen.getByTestId('pattern-option-cron'))
    expect(screen.getByLabelText('Cron expression')).toBeTruthy()

    expect(current.pattern.kind).toBe('cron')
  })
})

describe('ScheduleForm monthly builder', () => {
  it('emits a day-of-month range through the range adder', async () => {
    const user = userEvent.setup()
    let current: ReturnType<typeof emptyScheduleDraft> = {
      ...emptyScheduleDraft(),
      pattern: emptyPattern('monthly'),
    }
    const onChange = (next: ScheduleDraft) => {
      current = next
      rerender()
    }
    const view = render(
      <ScheduleForm draft={current} onChange={onChange} tasks={tasks} errors={[]} />
    )
    const rerender = () =>
      view.rerender(
        <ScheduleForm draft={current} onChange={onChange} tasks={tasks} errors={[]} />
      )

    await user.click(screen.getByTestId('add-day-range'))

    expect(screen.getByTestId('range-chip-1-7')).toBeTruthy()
    expect(screen.getByTestId('cron-preview-expression')).toHaveTextContent('0 9 1-7 * *')
  })

  it('emits a single selected day', async () => {
    const user = userEvent.setup()
    let current: ReturnType<typeof emptyScheduleDraft> = {
      ...emptyScheduleDraft(),
      pattern: emptyPattern('monthly'),
    }
    const onChange = (next: ScheduleDraft) => {
      current = next
      rerender()
    }
    const view = render(
      <ScheduleForm draft={current} onChange={onChange} tasks={tasks} errors={[]} />
    )
    const rerender = () =>
      view.rerender(
        <ScheduleForm draft={current} onChange={onChange} tasks={tasks} errors={[]} />
      )

    await user.click(screen.getByRole('button', {name: 'Day 23'}))
    expect(screen.getByTestId('cron-preview-expression')).toHaveTextContent('0 9 23 * *')
  })

  it('marks day removal through the chip', async () => {
    const user = userEvent.setup()
    let current: ReturnType<typeof emptyScheduleDraft> = {
      ...emptyScheduleDraft(),
      pattern: emptyPattern('monthly'),
    }
    const onChange = (next: ScheduleDraft) => {
      current = next
      rerender()
    }
    const view = render(
      <ScheduleForm draft={current} onChange={onChange} tasks={tasks} errors={[]} />
    )
    const rerender = () =>
      view.rerender(
        <ScheduleForm draft={current} onChange={onChange} tasks={tasks} errors={[]} />
      )

    await user.click(screen.getByRole('button', {name: 'Day 23'}))
    await user.click(screen.getByRole('button', {name: 'Day 23'}))
    expect(screen.getByTestId('cron-preview-expression')).toHaveTextContent('0 9 * * *')
  })
})

describe('validateScheduleDraft', () => {
  it('requires slug and task', () => {
    const errors = validateScheduleDraft(emptyScheduleDraft())
    expect(errors.some((e) => e.startsWith('slug:'))).toBe(true)
    expect(errors.some((e) => e.startsWith('task:'))).toBe(true)
  })

  it('accepts a complete monthly draft', () => {
    const draft = {
      slug: 'september-window',
      taskSlug: 'backup',
      missedPolicy: 'skip',
      pattern: {...emptyPattern('monthly'), ranges: [[23, 26] as [number, number]], months: [9]},
    }
    expect(validateScheduleDraft(draft)).toEqual([])
    const payload = draftToPayload(draft)
    expect(payload.kind).toBe('recurring')
    expect(payload.cron).toBe('0 9 23-26 9 *')
  })

  it('requires days or months for monthly patterns', () => {
    const draft = {
      slug: 'x',
      taskSlug: 'backup',
      missedPolicy: 'skip',
      pattern: emptyPattern('monthly'),
    }
    expect(validateScheduleDraft(draft).some((e) => e.startsWith('cron:'))).toBe(true)
  })

  it('requires a day for weekly patterns', () => {
    const draft = {
      slug: 'x',
      taskSlug: 'backup',
      missedPolicy: 'skip',
      pattern: {...emptyPattern('weekly'), days: []},
    }
    expect(validateScheduleDraft(draft).some((e) => e.includes('day of the week'))).toBe(true)
  })

  it('rejects past one-time runs', () => {
    const draft = {
      slug: 'x',
      taskSlug: 'backup',
      missedPolicy: 'skip',
      pattern: {kind: 'once' as const, runAt: '2020-01-01T09:00'},
    }
    expect(validateScheduleDraft(draft).some((e) => e.startsWith('run_at:'))).toBe(true)
  })

  it('rejects invalid raw cron', () => {
    const draft = {
      slug: 'x',
      taskSlug: 'backup',
      missedPolicy: 'skip',
      pattern: {kind: 'cron' as const, expr: '0 9 * * 7'},
    }
    expect(validateScheduleDraft(draft).some((e) => e.startsWith('cron:'))).toBe(true)
  })
})

describe('draftFromSchedule (edit prefill)', () => {
  it('reverse-parses a builder-shaped cron into its pattern', () => {
    const draft = draftFromSchedule({
      id: '1',
      slug: 'september-window',
      task_slug: 'backup',
      kind: 'recurring',
      cron: '0 9 23-26 9 *',
      enabled: true,
      missed_policy: 'run_now',
      skipped_count: 0,
      missed_count: 0,
    })
    expect(draft.pattern.kind).toBe('monthly')
    expect(draft.missedPolicy).toBe('run_now')
    expect(draftToPayload(draft).cron).toBe('0 9 23-26 9 *')
  })

  it('keeps foreign crons editable in the cron pattern', () => {
    const draft = draftFromSchedule({
      id: '1',
      slug: 'weird',
      task_slug: 'backup',
      kind: 'recurring',
      cron: '*/15 9-17 * * 1-5',
      enabled: true,
      missed_policy: 'skip',
      skipped_count: 0,
      missed_count: 0,
    })
    expect(draft.pattern.kind).toBe('cron')
    expect(draftToPayload(draft).cron).toBe('*/15 9-17 * * 1-5')
  })

  it('prefills one-time schedules', () => {
    const draft = draftFromSchedule({
      id: '1',
      slug: 'once',
      task_slug: 'backup',
      kind: 'onetime',
      run_at: '2030-01-01T09:00:00Z',
      enabled: true,
      missed_policy: 'skip',
      skipped_count: 0,
      missed_count: 0,
    })
    expect(draft.pattern.kind).toBe('once')
    expect(draftToPayload(draft).kind).toBe('onetime')
  })
})

describe('CronPreview', () => {
  it('shows the description and upcoming runs for a daily pattern', () => {
    render(<CronPreview pattern={{kind: 'daily', times: [{hh: '09', mm: '00'}]}} />)
    expect(screen.getByTestId('cron-preview-description')).toHaveTextContent('At 09:00, every day')
    expect(screen.getByTestId('cron-preview-runs').children.length).toBe(5)
  })

  it('flags cross-product firing for mismatched times', () => {
    render(
      <CronPreview
        pattern={{kind: 'daily', times: [{hh: '09', mm: '05'}, {hh: '10', mm: '42'}]}}
      />
    )
    expect(screen.getByTestId('cron-preview-warning')).toBeTruthy()
  })

  it('shows an error for invalid cron expressions', () => {
    render(<CronPreview pattern={{kind: 'cron', expr: '0 9 * * 7'}} />)
    expect(screen.getByTestId('cron-preview-description').textContent).toContain('7')
  })
})
