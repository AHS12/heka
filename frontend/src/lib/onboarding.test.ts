import {beforeEach, describe, expect, it} from 'vitest'
import {APP_VERSION} from './version'
import {useOnboarding} from './onboarding'

function resetStore() {
  localStorage.clear()
  useOnboarding.setState({
    tourCompleted: false,
    seenVersion: '',
    mode: 'none',
    force: false,
    scope: 'newer',
  })
}

beforeEach(resetStore)

describe('autoOpen', () => {
  it('shows the tour on a fresh install', () => {
    useOnboarding.getState().autoOpen()
    expect(useOnboarding.getState().mode).toBe('tour')
    expect(useOnboarding.getState().force).toBe(false)
  })

  it('shows What\'s New (releases since seen) when the app version moved past the seen version', () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: '0.8.1'})
    useOnboarding.getState().autoOpen()
    const s = useOnboarding.getState()
    expect(s.mode).toBe('whats-new')
    expect(s.scope).toBe('newer')
    expect(s.force).toBe(false)
  })

  it('stays quiet when the seen version is current', () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    useOnboarding.getState().autoOpen()
    expect(useOnboarding.getState().mode).toBe('none')
  })

  it('stays quiet on a downgrade instead of announcing "what\'s new"', () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: '9.9.9'})
    useOnboarding.getState().autoOpen()
    expect(useOnboarding.getState().mode).toBe('none')
    // The bogus newer stamp survives so a later real upgrade still compares right.
    expect(useOnboarding.getState().seenVersion).toBe('9.9.9')
  })

  it('stamps the seen version silently for installs that predate onboarding', () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: ''})
    useOnboarding.getState().autoOpen()
    const s = useOnboarding.getState()
    expect(s.mode).toBe('none')
    expect(s.seenVersion).toBe(APP_VERSION)
    expect(localStorage.getItem('heka-seen-version')).toBe(APP_VERSION)
  })
})

describe('tour lifecycle', () => {
  it('a fresh install chains tour → this release\'s What\'s New → seen stamp on dismiss', () => {
    useOnboarding.getState().autoOpen()
    expect(useOnboarding.getState().mode).toBe('tour')

    useOnboarding.getState().finishTour()
    let s = useOnboarding.getState()
    expect(s.tourCompleted).toBe(true)
    expect(s.mode).toBe('whats-new')
    expect(s.scope).toBe('latest')
    // Not stamped yet — the dialog's dismissal does that.
    expect(s.seenVersion).toBe('')

    useOnboarding.getState().dismissWhatsNew()
    s = useOnboarding.getState()
    expect(s.mode).toBe('none')
    expect(s.seenVersion).toBe(APP_VERSION)
    expect(localStorage.getItem('heka-seen-version')).toBe(APP_VERSION)
  })

  it('a forced tour finishes without opening What\'s New or touching the seen version', () => {
    useOnboarding.setState({tourCompleted: false, seenVersion: '0.8.1'})
    useOnboarding.getState().startTour(true)
    useOnboarding.getState().finishTour()
    const s = useOnboarding.getState()
    expect(s.tourCompleted).toBe(true)
    expect(s.mode).toBe('none')
    expect(s.seenVersion).toBe('0.8.1')
  })
})

describe('What\'s New dismissal', () => {
  it('persists the seen version on a natural dismiss', () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: '0.8.1'})
    useOnboarding.getState().showWhatsNew('newer', false)
    useOnboarding.getState().dismissWhatsNew()
    const s = useOnboarding.getState()
    expect(s.seenVersion).toBe(APP_VERSION)
    expect(s.mode).toBe('none')
    expect(localStorage.getItem('heka-seen-version')).toBe(APP_VERSION)
  })

  it('a force-mode dismiss persists nothing (pure inspection)', () => {
    localStorage.setItem('heka-seen-version', '0.8.1')
    useOnboarding.setState({tourCompleted: true, seenVersion: '0.8.1'})
    useOnboarding.getState().showWhatsNew('all', true)
    useOnboarding.getState().dismissWhatsNew()
    expect(useOnboarding.getState().seenVersion).toBe('0.8.1')
    expect(localStorage.getItem('heka-seen-version')).toBe('0.8.1')
  })
})

describe('applyDevTrigger', () => {
  it('whats-new forces the dialog open with every release', () => {
    useOnboarding.getState().applyDevTrigger({trigger: 'whats-new'})
    const s = useOnboarding.getState()
    expect(s.mode).toBe('whats-new')
    expect(s.force).toBe(true)
    expect(s.scope).toBe('all')
  })

  it('tour forces the tour open', () => {
    useOnboarding.getState().applyDevTrigger({trigger: 'tour'})
    const s = useOnboarding.getState()
    expect(s.mode).toBe('tour')
    expect(s.force).toBe(true)
  })

  it('reset clears both prefs and re-runs the natural decision (fresh install)', () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    useOnboarding.getState().applyDevTrigger({trigger: 'reset'})
    const s = useOnboarding.getState()
    expect(s.tourCompleted).toBe(false)
    expect(s.seenVersion).toBe('')
    expect(s.mode).toBe('tour')
    expect(localStorage.getItem('heka-tour-completed')).toBeNull()
    expect(localStorage.getItem('heka-seen-version')).toBeNull()
  })

  it('update-from stamps the seen version without opening anything', () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    useOnboarding.getState().applyDevTrigger({trigger: 'update-from', version: '0.7.0'})
    const s = useOnboarding.getState()
    expect(s.seenVersion).toBe('0.7.0')
    expect(s.mode).toBe('none')
  })

  it('update-from without a version is ignored', () => {
    useOnboarding.setState({tourCompleted: true, seenVersion: APP_VERSION})
    useOnboarding.getState().applyDevTrigger({trigger: 'update-from'})
    expect(useOnboarding.getState().seenVersion).toBe(APP_VERSION)
  })
})
