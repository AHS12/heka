// lib/onboarding.ts — the guided tour and the What's New dialog. Pure UI
// prefs, persisted to localStorage with the same pattern as theme.ts /
// accent.ts (heka-* keys, plain zustand store, no persist middleware). The
// daemon is never involved: seen-version compares against the build-time
// APP_VERSION constant.
import {create} from 'zustand'
import {APP_VERSION} from './version'
import {compareVersions} from './changelog'
import type {DevTrigger} from './api'

const TOUR_KEY = 'heka-tour-completed'
const SEEN_KEY = 'heka-seen-version'

export type OnboardingMode = 'none' | 'tour' | 'whats-new'

/** Which slice of the changelog the dialog shows: everything since the seen
 *  version (update flow), just the current release (fresh install, right
 *  after the tour), or the whole file (force mode: dev trigger / About). */
export type WhatsNewScope = 'newer' | 'latest' | 'all'

interface OnboardingState {
  tourCompleted: boolean
  seenVersion: string
  /** Which onboarding surface is open right now. */
  mode: OnboardingMode
  /** Opened via `heka dev …` or the About page — dismissal persists nothing
   *  (pure inspection). */
  force: boolean
  scope: WhatsNewScope
  /** The natural launch decision: first-run tour, else What's New on update. */
  autoOpen: () => void
  /** Applies a trigger dropped by `heka dev whats-new|tour|reset|update-from`. */
  applyDevTrigger: (t: DevTrigger) => void
  startTour: (force?: boolean) => void
  showWhatsNew: (scope: WhatsNewScope, force?: boolean) => void
  /** Tour finished: mark done; on a natural first run hand straight over to
   *  the What's New dialog for this release, which stamps seen-version on
   *  dismissal. */
  finishTour: () => void
  /** What's New dismissed: stamp seen-version unless in force mode. */
  dismissWhatsNew: () => void
  /** `heka dev update-from <v>` — pretend the last seen version was v. */
  markSeen: (v: string) => void
  /** `heka dev reset` — clear both prefs and re-run the natural decision. */
  reset: () => void
}

export const useOnboarding = create<OnboardingState>((set, get) => ({
  tourCompleted: localStorage.getItem(TOUR_KEY) === '1',
  seenVersion: localStorage.getItem(SEEN_KEY) ?? '',
  mode: 'none',
  force: false,
  scope: 'newer',

  autoOpen: () => {
    const {tourCompleted, seenVersion} = get()
    if (!tourCompleted) {
      set({mode: 'tour', force: false, scope: 'newer'})
      return
    }
    if (seenVersion && compareVersions(APP_VERSION, seenVersion) > 0) {
      set({mode: 'whats-new', force: false, scope: 'newer'})
      return
    }
    if (!seenVersion) {
      // Tour completed but never stamped (pre-onboarding install): stamp
      // silently so a future upgrade triggers What's New from now on.
      localStorage.setItem(SEEN_KEY, APP_VERSION)
      set({seenVersion: APP_VERSION})
    }
  },

  applyDevTrigger: (t) => {
    switch (t.trigger) {
      case 'whats-new':
        get().showWhatsNew('all', true)
        break
      case 'tour':
        get().startTour(true)
        break
      case 'reset':
        get().reset()
        break
      case 'update-from':
        if (t.version) get().markSeen(t.version)
        break
    }
  },

  startTour: (force = false) => set({mode: 'tour', force, scope: 'newer'}),
  showWhatsNew: (scope, force = false) => set({mode: 'whats-new', scope, force}),

  finishTour: () => {
    localStorage.setItem(TOUR_KEY, '1')
    if (!get().force) {
      // First install: the tour hands over to this release's notes; the
      // seen-version stamp happens when that dialog is dismissed.
      set({tourCompleted: true, mode: 'whats-new', scope: 'latest', force: false})
    } else {
      set({tourCompleted: true, mode: 'none', force: false})
    }
  },

  dismissWhatsNew: () => {
    if (!get().force) {
      localStorage.setItem(SEEN_KEY, APP_VERSION)
      set({seenVersion: APP_VERSION, mode: 'none', force: false})
    } else {
      set({mode: 'none', force: false})
    }
  },

  markSeen: (v) => {
    localStorage.setItem(SEEN_KEY, v)
    set({seenVersion: v, mode: 'none', force: false})
  },

  reset: () => {
    localStorage.removeItem(TOUR_KEY)
    localStorage.removeItem(SEEN_KEY)
    set({tourCompleted: false, seenVersion: '', mode: 'none', force: false, scope: 'newer'})
    get().autoOpen()
  },
}))
