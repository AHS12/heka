// components/WhatsNewModal.tsx — the "What's New" dialog. Renders every
// release newer than the stored seen-version (newest first) from the
// embedded CHANGELOG.md; `heka dev whats-new` opens it in force mode with
// all releases and dismisses without persisting anything. Works entirely
// offline — the daemon is never consulted; the full changelog lives online.
import {useEffect, useState} from 'react'
import {Modal} from '@heroui/react'
import {AppDialog, dialogHeaderCls, dialogBodyCls, dialogFooterCls} from './AppDialog'
import {Markdown} from './Markdown'
import {pillBtn, primaryBtn} from './controls'
import {getChangelog, openURL} from '../lib/api'
import {compareVersions, parseChangelog, type Release} from '../lib/changelog'
import {useOnboarding, type WhatsNewScope} from '../lib/onboarding'
import {APP_VERSION} from '../lib/version'

/** The project website — the "View full changelog" target. */
export const HEKA_CHANGELOG_URL = 'https://heka.ahs12.xyz/changelog/'

const SCOPE_SUBTITLES: Record<WhatsNewScope, string> = {
  newer: 'Everything since you were last here',
  latest: 'What shipped in this release',
  all: 'All releases, newest first',
}

export function WhatsNewModal() {
  const mode = useOnboarding((s) => s.mode)
  const force = useOnboarding((s) => s.force)
  const scope = useOnboarding((s) => s.scope)
  const seenVersion = useOnboarding((s) => s.seenVersion)
  const dismissWhatsNew = useOnboarding((s) => s.dismissWhatsNew)
  const [releases, setReleases] = useState<Release[]>([])
  const open = mode === 'whats-new'

  useEffect(() => {
    if (!open) return
    let alive = true
    void getChangelog().then((md) => {
      if (!alive) return
      const all = parseChangelog(md)
      let list: Release[]
      if (scope === 'all') {
        list = all
      } else if (scope === 'latest') {
        const current = all.filter((r) => r.version === APP_VERSION)
        list = current.length > 0 ? current : all.slice(0, 1)
      } else {
        list = all.filter((r) => compareVersions(r.version, seenVersion) > 0)
      }
      setReleases(list)
    })
    return () => {
      alive = false
    }
  }, [open, force, scope, seenVersion])

  return (
    <AppDialog
      isOpen={open}
      onOpenChange={(o) => !o && dismissWhatsNew()}
      size="lg"
      dialogClassName="max-w-2xl"
    >
      <Modal.Header className={dialogHeaderCls}>
        <div className="min-w-0">
          <Modal.Heading className="text-lg font-semibold">
            What&rsquo;s new in Heka
          </Modal.Heading>
          <p className="mt-0.5 text-xs text-foreground/55">{SCOPE_SUBTITLES[scope]}</p>
        </div>
        <Modal.CloseTrigger aria-label="Close What's new dialog" />
      </Modal.Header>
      <Modal.Body className={`${dialogBodyCls} space-y-5`}>
        {releases.map((release) => (
          <article key={release.version} aria-label={`Version ${release.version}`}>
            <div className="mb-1.5 flex items-center gap-2">
              <span className="rounded-full bg-accent/10 px-2 py-0.5 text-[11px] font-semibold text-accent">
                v{release.version}
              </span>
              <span className="text-[11px] text-foreground/45">{release.date}</span>
            </div>
            {release.summary && <Markdown text={release.summary} />}
            {release.body && <Markdown text={release.body} />}
          </article>
        ))}
        {releases.length === 0 && (
          <p className="py-4 text-center text-sm text-foreground/55">
            No release notes found.
          </p>
        )}
      </Modal.Body>
      <Modal.Footer className={dialogFooterCls}>
        <button
          type="button"
          className={pillBtn}
          onClick={() => void openURL(HEKA_CHANGELOG_URL)}
        >
          View full changelog
        </button>
        <button type="button" className={primaryBtn} onClick={dismissWhatsNew}>
          Got it
        </button>
      </Modal.Footer>
    </AppDialog>
  )
}
