// AppTour tests: five chrome-only steps — no navigation hops, driver's built-in
// advance, themed popover, disabled spot interaction, completion callback.
// driver.js is stubbed in src/test/setup.ts.
import {describe, expect, it, vi, beforeEach} from 'vitest'
import {render, screen} from '@testing-library/react'
import {MemoryRouter, Routes, Route, useLocation} from 'react-router-dom'
import {driver} from 'driver.js'
import type {Config} from 'driver.js'
import {AppTour} from './AppTour'

function configFromLastCall(): Config {
  const calls = vi.mocked(driver).mock.calls
  if (calls.length === 0) throw new Error('driver() was never called')
  return calls[calls.length - 1][0] as Config
}

function PathProbe() {
  const {pathname} = useLocation()
  return <div data-testid="route-path">{pathname}</div>
}

beforeEach(() => {
  vi.mocked(driver).mockClear()
})

function renderTour(onFinish = vi.fn()) {
  render(
    // Start on a non-home page: the tour must run from wherever the user is.
    <MemoryRouter initialEntries={['/schedules']}>
      <Routes>
        <Route
          path="*"
          element={
            <>
              <PathProbe />
              <AppTour onFinish={onFinish} />
            </>
          }
        />
      </Routes>
    </MemoryRouter>
  )
  return onFinish
}

describe('AppTour', () => {
  it('runs five chrome-only steps with the themed popover and no interaction', () => {
    renderTour()
    const config = configFromLastCall()
    expect(config.steps).toHaveLength(5)
    expect(
      config.steps?.every((s) => !s.element || String(s.element).startsWith('[data-tour='))
    ).toBe(true)
    expect(config.popoverClass).toBe('heka-tour-popover')
    expect(config.disableActiveInteraction).toBe(true)
    // No cross-page hops: driver's own advance handles Next/Back.
    expect(config.onNextClick).toBeUndefined()
    expect(config.onPrevClick).toBeUndefined()
  })

  it('never navigates away from the page the tour started on', () => {
    renderTour()
    expect(screen.getByTestId('route-path')).toHaveTextContent('/schedules')
  })

  it('calls onFinish when the tour ends', () => {
    const onFinish = renderTour()
    const config = configFromLastCall()
    config.onDestroyed?.(
      undefined,
      undefined as never,
      {driver: {} as never, index: 4} as never
    )
    expect(onFinish).toHaveBeenCalledTimes(1)
  })
})
