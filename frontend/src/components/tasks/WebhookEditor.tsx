// components/tasks/WebhookEditor.tsx (SPEC-13 §4) — notification webhook rows
// with per-platform fields, setup guides, and secret-vault integration.
// Each format gets its own fields (not a one-size-fits-all URL box) so the
// user knows exactly what to paste and where to get it.
import {RemoveRow, SelectField, pillBtn} from '../controls'
import {SecretValue} from './SecretValue'
import type {WebhookDraft, WebhookFormat} from '../../lib/taskForm'

const FORMATS: WebhookFormat[] = ['slack', 'discord', 'telegram', 'generic']

// Pumble incoming webhooks accept Slack-style payloads, so one format value
// ("slack") serves both — the dropdown and guides just name both platforms.
const FORMAT_LABELS: Record<WebhookFormat, string> = {
  slack: 'Slack / Pumble',
  discord: 'Discord',
  telegram: 'Telegram',
  generic: 'Generic',
}

const GUIDES: Record<WebhookFormat, {links: Array<{label: string; url: string}>; hint: string}> = {
  slack: {
    links: [
      {
        label: 'Slack Incoming Webhook',
        url: 'https://api.slack.com/messaging/webhooks',
      },
      {
        label: 'Pumble Webhook',
        url: 'https://www.zoho.com/pumble/help/integrations/webhooks.html',
      },
    ],
    hint: 'Slack: create an app → Incoming Webhooks → Add to Slack → copy the webhook URL. Pumble: Settings → Integrations → Incoming Webhooks → copy the URL — both accept the same payload.',
  },
  discord: {
    links: [
      {
        label: 'Discord Webhook',
        url: 'https://support.discord.com/hc/en-us/articles/228383668',
      },
    ],
    hint: 'Server Settings → Integrations → Webhooks → New Webhook → Copy Webhook URL.',
  },
  telegram: {
    links: [
      {
        label: 'Telegram Bot',
        url: 'https://core.telegram.org/bots#creating-a-new-bot',
      },
    ],
    hint: 'Create a bot via @BotFather → copy the bot token. Send a message to your bot, then visit https://api.telegram.org/bot<TOKEN>/getUpdates to find your chat_id.',
  },
  generic: {
    links: [],
    hint: 'A JSON POST will be sent to the URL below. Use ${WEBHOOK_URL} or ${SECRET} references to keep the URL out of the YAML.',
  },
}

export function WebhookEditor({
  rows,
  onChange,
}: {
  rows: WebhookDraft[]
  onChange: (rows: WebhookDraft[]) => void
}) {
  const set = (i: number, next: WebhookDraft) => {
    onChange(rows.map((row, idx) => (idx === i ? next : row)))
  }
  return (
    <div className="space-y-3">
      {rows.length === 0 && (
        <p className="text-xs text-foreground/50">
          No webhook notifications configured.
        </p>
      )}
      {rows.map((row, i) => {
        const guide = GUIDES[row.format]
        return (
          <div
            key={i}
            className="relative space-y-2 rounded-xl border border-border/80 bg-surface-secondary/50 px-4 py-3 pr-10"
          >
            {/* Close button pinned top-right */}
            <RemoveRow
              label={`Remove webhook ${i + 1}`}
              onClick={() => onChange(rows.filter((_, idx) => idx !== i))}
              className="absolute right-2 top-2"
            />

            {/* Format selector */}
            <SelectField
              aria-label={`Webhook ${i + 1} format`}
              value={row.format}
              onChange={(next) =>
                set(i, {...row, format: next as WebhookFormat})
              }
              items={FORMATS.map((f) => ({
                id: f,
                label: FORMAT_LABELS[f],
              }))}
            />

            {/* Guide links */}
            {guide.links.map((link) => (
              <a
                key={link.url}
                href={link.url}
                target="_blank"
                rel="noopener noreferrer"
                className="mr-3 inline-flex items-center gap-1 text-xs text-foreground/50 underline-offset-2 hover:text-foreground/75 hover:underline"
              >
                How to create a {link.label} ↗
              </a>
            ))}

            {/* Platform-specific fields */}
            {row.format === 'telegram' ? (
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <span className="mb-1 block text-[11px] font-medium text-foreground/50">
                    Bot Token
                  </span>
                  <SecretValue
                    ariaLabel={`Webhook ${i + 1} bot token`}
                    value={row.url}
                    onChange={(url) => set(i, {...row, url})}
                  />
                </div>
                <div>
                  <span className="mb-1 block text-[11px] font-medium text-foreground/50">
                    Chat ID
                  </span>
                  <SecretValue
                    ariaLabel={`Webhook ${i + 1} chat id`}
                    value={row.chatId}
                    onChange={(chatId) => set(i, {...row, chatId})}
                  />
                </div>
              </div>
            ) : (
              <div>
                <span className="mb-1 block text-[11px] font-medium text-foreground/50">
                  Webhook URL
                </span>
                <SecretValue
                  ariaLabel={`Webhook ${i + 1} URL`}
                  value={row.url}
                  onChange={(url) => set(i, {...row, url})}
                />
              </div>
            )}

            {/* Hint */}
            <p className="text-[11px] leading-relaxed text-foreground/50">
              {guide.hint}
            </p>
          </div>
        )
      })}
      <button
        type="button"
        onClick={() =>
          onChange([...rows, {format: 'slack', url: '', chatId: ''}])
        }
        className={pillBtn}
      >
        + Webhook
      </button>
    </div>
  )
}