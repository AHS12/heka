// components/tour/AppTour.tsx — the guided tour (driver.js). Five steps, all
// spotlighting the always-visible TopNav chrome, so the tour runs wherever
// the user happens to be and never navigates away. Popover styling lives in
// main.css (.heka-tour-popover) as high-contrast chrome legible on every
// theme variant.
import {useEffect, useRef} from 'react'
import {driver} from 'driver.js'
import 'driver.js/dist/driver.css'
import type {Config, DriveStep} from 'driver.js'

const STEPS: DriveStep[] = [
  {
    popover: {
      title: 'Welcome to Heka',
      description:
        'A local task runner and scheduler for programmers. Everything runs on ' +
        'your machine — this tour takes a minute and shows you around.',
    },
  },
  {
    element: '[data-tour="nav"]',
    popover: {
      title: 'Everything a click away',
      description:
        'Home is your dashboard. Tasks holds your task list and editor, ' +
        'Schedules the cron builder, and Logs every run with its captured output.',
    },
  },
  {
    element: '[data-tour="daemon"]',
    popover: {
      title: 'Daemon status at a glance',
      description:
        'Green means the background daemon is healthy and scheduling. Hover ' +
        'the dot for version, uptime, and scheduler state; if it ever turns ' +
        'red, the banner on the page can start it for you.',
    },
  },
  {
    element: '[data-tour="theme"]',
    popover: {
      title: 'Make it yours',
      description:
        'Flip light/dark here. Six theme variants, accent colors, and motion ' +
        'controls live in Settings → Appearance.',
    },
  },
  {
    element: '[data-tour="settings"]',
    popover: {
      title: 'Settings — and you’re all set',
      description:
        'Appearance, data locations, automatic backups, startup and watchdog ' +
        'toggles, retention, notification sounds, and the secrets vault. ' +
        'That’s the tour — head to Tasks → “+ New task” to create your first one.',
    },
  },
]

export function AppTour({onFinish}: {onFinish: () => void}) {
  // Keep the callback in a ref so a parent re-render can never restart the tour.
  const onFinishRef = useRef(onFinish)
  onFinishRef.current = onFinish

  useEffect(() => {
    const config: Config = {
      overlayColor: 'rgba(0, 0, 0, 0.65)',
      showProgress: true,
      progressText: '{{current}} of {{total}}',
      nextBtnText: 'Next',
      prevBtnText: 'Back',
      doneBtnText: 'Finish',
      popoverClass: 'heka-tour-popover',
      // The spotlight must not turn into a stray click target mid-tour.
      disableActiveInteraction: true,
      steps: STEPS,
      onDestroyed: () => onFinishRef.current(),
    }
    const tour = driver(config)
    tour.drive()
    return () => tour.destroy()
  }, [])

  return null
}
