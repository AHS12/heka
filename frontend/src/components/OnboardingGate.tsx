// components/OnboardingGate.tsx — mounted once in the app shell. Decides what
// (if anything) opens at launch — the first-run tour or the What's New
// dialog — and consumes the one-shot triggers `heka dev …` drops into the
// data directory (polled while running; consumed at next launch otherwise).
import {useEffect} from 'react'
import {takeDevTrigger} from '../lib/api'
import {useOnboarding} from '../lib/onboarding'
import {AppTour} from './tour/AppTour'
import {WhatsNewModal} from './WhatsNewModal'

const POLL_MS = 4000

export function OnboardingGate() {
  const mode = useOnboarding((s) => s.mode)
  const autoOpen = useOnboarding((s) => s.autoOpen)
  const applyDevTrigger = useOnboarding((s) => s.applyDevTrigger)
  const finishTour = useOnboarding((s) => s.finishTour)

  useEffect(() => {
    let alive = true
    const consume = async () => {
      const trigger = await takeDevTrigger()
      if (!alive) return
      if (trigger) applyDevTrigger(trigger)
      else autoOpen()
    }
    void consume()
    const timer = setInterval(() => {
      void takeDevTrigger().then((trigger) => {
        if (alive && trigger) applyDevTrigger(trigger)
      })
    }, POLL_MS)
    return () => {
      alive = false
      clearInterval(timer)
    }
  }, [autoOpen, applyDevTrigger])

  if (mode === 'tour') return <AppTour onFinish={finishTour} />
  if (mode === 'whats-new') return <WhatsNewModal />
  return null
}
