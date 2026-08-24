# SPEC-11 — Notifications & Secrets

Status: **Draft** · Depends on: SPEC-03 (secrets repo), SPEC-05 (env resolver), SPEC-07 (501 routes) · Master spec: §10, D7, D9, PRD §7.3, §14, §30

## Goal

Finish the notification surface: native desktop toasts **and** chat webhook channels (Slack, Discord, Pumble, Telegram, generic HTTP) with per-task policy, plus the local secret store that feeds both `${VAR}` resolution and webhook tokens — with the IPC routes from SPEC-07's 501 list finally lit up.

## Scope

**In:**
- `internal/notify`: beeep wrapper, per-task policy (`task.notify_on`), injectable send seam
- Executor callback so the daemon (not the executor) owns notifications
- Secrets IPC: `GET/PUT/DELETE /v1/secrets` (value never returned), daemon `EnvResolver` hookup (process env first, then secrets)
- Webhook channels in `internal/notify`: `slack`/`discord`/`telegram`/`generic` presets, async best-effort delivery (10 s timeout), `url`/`chat_id` refs resolved via the secret store
- Client additions: `ListSecrets/SetSecret/DeleteSecret`
- Dependencies: `github.com/gen2brain/beeep` (desktop)

**Out:** repeated-failure throttling/dedupe (v1 notifies per terminal group; documented as future), keychain storage (D9), CLI secret commands (GUI-only in v1), daemon-self-failure toasts (a dead daemon can't toast — that's the watchdog's job, SPEC-10), custom webhook body templates (generic sends `{"text": …}`), webhook delivery retry/dedupe.

## 1. Notifications

```go
// internal/notify
type Notifier struct { send func(title, msg string) error } // tests inject a fake

func New() *Notifier                       // send = beeep.Notify
func (n *Notifier) NotifyTaskResult(task *task.Task, finalStatus string) // fans out to desktop + configured webhooks
```

- **Policy** (PRD §30): notify for `success` / `failed` / `timed_out` only when `task.notify_on` includes that status. Empty `notify_on` → never. Titles: `<task name> — Success|Failed|Timed out`; body: duration + exit code when present.
- **Fire on group completion only** — the executor's new callback triggers it (see §3), so a retried group notifies once with its final status, not per attempt.
- **Never crashes anything**: beeep errors are logged by the daemon and swallowed (master §15 risk note: Windows WinRT toast without a packaged identity can be flaky — the daemon stays up either way).
- Daemon/scheduler liveness is deliberately *not* notified here (SPEC-10 owns that). Scheduler health is visible via `/v1/health`.
- Notification history: none in v1 (both channels are fire-once; the Runs view is the durable record).

### 1.1 Webhook channels (MVP — PRD §30, D13)

Configured **per task, inline in the task YAML** (portable, YAML-first). Shared endpoints reuse the same `${SECRET}` rather than duplicating tokens:

```yaml
notify_on: [failure, timeout]
notify:
  webhooks:
    - format: slack            # slack | discord | telegram | generic
      url: ${SLACK_WEBHOOK_URL}
    - format: telegram
      url: ${TELEGRAM_BOT_URL} # e.g. https://api.telegram.org/bot<token>/sendMessage
      chat_id: ${TELEGRAM_CHAT_ID}
```

| format | request | notes |
|---|---|---|
| `slack` | POST JSON `{"text": "<title>: <body>"}` | also covers **Pumble** (Slack-compatible webhooks) |
| `discord` | POST JSON `{"content": "<title>: <body>"}` | |
| `telegram` | POST form `chat_id` + `text` to the bot URL | Bot API `sendMessage` |
| `generic` | POST JSON `{"text": "<title>: <body>"}` | any URL — webhook.site, Teams-compatible sinks, etc. |

Rules pinned:

- **Tokens never in YAML.** `url` and `chat_id` are resolved through the same `EnvResolver` as task env (process env → secrets, §4). An unresolvable ref marks the channel misconfigured: logged once, skipped, task result unaffected.
- **Delivery**: one goroutine per webhook after the group result; 10 s timeout; failure → daemon log line. No retry, no dedup (MVP).
- **Config validation is SPEC-04's job** (unknown `format`, missing `url`, missing `chat_id` for telegram = task-validation errors at import/load time).

## 2. Secrets

- **Key rules**: `^[A-Za-z_][A-Za-z0-9_]*$` (must be a valid env var name — they're resolved into env). Invalid → 400 `bad_request`. Values: any string; body-size limited by the existing IPC 1 MiB cap.
- **Never returned**: `GET /v1/secrets` → `{"keys":["OPENROUTER_API_KEY"]}` — keys only. No value-read endpoint exists at all (PRD §14 masking). GUI shows `••••` for what it knows by key names.
- **Storage**: `secrets` table (SPEC-03), file perms already restricted (0600-equivalent). Plaintext at rest is the documented D9 trade-off; keychain is the future swap, and nothing else in the codebase depends on *where* values live (they only flow through `EnvResolver`).
- Delete is idempotent: missing key → still `{"ok":true}`.

### IPC contract (replacing 501s)

```text
GET    /v1/secrets            → {"keys":[...]}                    # masked surface
PUT    /v1/secrets/{key}      → {"ok":true}   body {"value":"…"}  # upsert
DELETE /v1/secrets/{key}      → {"ok":true}                       # idempotent
```

## 3. Executor callback seam

Executor gains one registration point (default: no-op) so run completion is observable without the executor knowing anything about notifications:

```go
type GroupResult struct {
    GroupID    string
    TaskSlug   string
    FinalStatus string   // success|failed|timed_out|cancelled
    Duration   time.Duration
    ExitCode   int
}

func (e *Executor) OnGroupFinished(fn func(GroupResult))
```

- Called once per group, after the final attempt row is written and the slug lock released.
- The daemon wires `notify.Notifier.NotifyTaskResult` (it looks up the `task` by slug for `notify_on`). SPEC-14's log/UI streams can register their own callbacks later without touching the executor.

## 4. EnvResolver hookup

The SPEC-05/06 seam gets its real production implementer in the daemon:

```go
// order: process env first, then secrets (master spec §7)
func envResolver(db *db.DB) executor.EnvResolver {
    return func(name string) (string, bool) {
        if v, ok := os.LookupEnv(name); ok { return v, true }
        return db.Secrets().Get(name)   // (value, bool)
    }
}
```

Task `environment` values that are literal strings pass through untouched; `${NAME}` refs resolve here — so a task can reference a secret that outlives a daemon restart and a `notify_on` value equal to any env var of the user's choice.

## 5. Testing

1. Notify policy table: `notify_on` subsets → notify/not for `success`/`failed`/`timed_out`/`cancelled`; empty → never.
2. Group completion: fake executor callback fires once for a 3-attempt failing group with final `failed`; beeep fake errors are swallowed, fake success recorded.
3. Secrets IPC e2e (SPEC-07 harness): set → keys list contains it (values absent from every body — assert on raw responses); invalid key → 400; delete → idempotent `ok`.
4. EnvResolver precedence: process env wins over secrets; secrets-only resolves; neither → not found.
5. Executor integration: helper-process task prints `$RESOLVED_SECRET` → resolved through the real resolver (task env `${RESOLVED_SECRET}`).
6. Webhook payload builders: slack/discord/telegram/generic shapes (incl. telegram `chat_id` form); fake HTTP server asserts method/path/body.
7. `${VAR}` in `url`/`chat_id` resolved via injected resolver; unresolvable ref → no send + single log entry.
8. Async isolation: slow (10 s+) fake webhook server → group result stays `success`/`failed` as-is, send error only logged.
9. Task validation rejects unknown `format`, missing `url`, missing telegram `chat_id` (SPEC-04 validator tests).
10. `make check` green (`-race`).

## 6. Files

```text
internal/notify/notify.go             # Notifier + policy eval + send seam
internal/notify/webhook.go            # preset builders (slack/discord/telegram/generic) + async sender
internal/executor/executor.go         # + OnGroupFinished + GroupResult
internal/ipc/secrets.go               # 3 handlers (replaces 501s)
internal/ipc/client.go                # + ListSecrets/SetSecret/DeleteSecret
internal/daemon/daemon.go             # envResolver + notify wiring
internal/notify/notify_test.go        # policy, swallow, group-result tests
internal/ipc/ipc_test.go              # + secrets e2e cases
go.mod                                # + github.com/gen2brain/beeep
```

## 7. Acceptance criteria

1. A task with `notify_on: [failure]` that fails → one native toast (manual check with the user on Windows; covered by fakes in CI).
2. Retried failing group → exactly one toast with final status.
3. Secrets: typed routes live, `GET` never leaks values, invalid keys rejected.
4. A task run with `${SOME_SECRET}` receives the secret in its environment (asserted via captured output).
5. Webhook e2e: task with a `slack`-format webhook pointed at a local fake server → notification posted on failure; nothing posted for statuses missing from `notify_on`.
6. One real chat channel verified manually with the user (user supplies a test Slack/Discord/Telegram webhook URL).
7. SPEC-07's secrets 501 tests updated to expect 200s.
8. `make check` green.

## DoD

1. Spec approved by user.
2. Criteria verified; `make check` green; one manual toast **and** one manual webhook confirmed with the user.
3. `internal/notify` has zero knowledge of IPC/DB; `internal/ipc` has zero knowledge of beeep — one-way wiring through the daemon.