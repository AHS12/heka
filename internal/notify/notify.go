// Package notify delivers task-completion notifications (SPEC-11): native
// desktop toasts and outgoing webhooks (Slack/Discord/Telegram/generic, D13).
//
// Delivery is async and best-effort — a broken webhook or flaky toast never
// affects task state or the daemon. The package knows nothing about IPC, the
// DB, or scheduling; the daemon wires it to executor group completion.
package notify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"heka/internal/core/task"
)

// defaultTimeout bounds a single webhook delivery.
const defaultTimeout = 10 * time.Second

// Resolver looks up a ${VAR} for webhook url/chat_id substitution. The
// daemon provides the same resolver the executor uses (env → secrets).
type Resolver func(name string) (string, bool)

// TaskResult carries the information needed for a notification.
type TaskResult struct {
	Task     *task.Task
	Status   string // "success" | "failed" | "timed_out" | "cancelled"
	Trigger  string // "manual" | "schedule" | "retry"
	Duration time.Duration
	ExitCode int
}

// Notifier dispatches to channels. All seams are injectable for tests.
type Notifier struct {
	desktop       func(title, message string) error
	resolve       Resolver
	log           func(format string, args ...any)
	post          func(ctx context.Context, url, contentType string, body io.Reader) error
	soundResolver func(eventType string) string // returns preset for event type
	timeout       time.Duration
}

// Option configures a Notifier.
type Option func(*Notifier)

// WithDesktop swaps the native toast sender (default: beeep.Notify).
func WithDesktop(fn func(title, message string) error) Option {
	return func(n *Notifier) { n.desktop = fn }
}

// WithResolver sets ${VAR} lookup (default: os environment).
func WithResolver(r Resolver) Option {
	return func(n *Notifier) { n.resolve = r }
}

// WithLogger sets the error sink (default: discard).
func WithLogger(fn func(format string, args ...any)) Option {
	return func(n *Notifier) { n.log = fn }
}

// WithPost swaps the HTTP delivery (default: http.Client.Post).
func WithPost(fn func(ctx context.Context, url, contentType string, body io.Reader) error) Option {
	return func(n *Notifier) { n.post = fn }
}

// WithTimeout bounds each webhook delivery.
func WithTimeout(d time.Duration) Option {
	return func(n *Notifier) { n.timeout = d }
}

// WithSoundResolver sets the function that maps event types to sound presets.
func WithSoundResolver(fn func(eventType string) string) Option {
	return func(n *Notifier) { n.soundResolver = fn }
}

// New builds a Notifier with production defaults.
func New(opts ...Option) *Notifier {
	n := &Notifier{
		desktop: func(title, message string) error {
			// Production default: no-op; daemon wires beeep.Notify.
			return nil
		},
		resolve: func(name string) (string, bool) {
			return os.LookupEnv(name)
		},
		log:  func(string, ...any) {},
		post: defaultPost,
		soundResolver: func(eventType string) string {
			return string(SoundSystem)
		},
		timeout: defaultTimeout,
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

// NotifyTaskResult fans out to every enabled channel when notify_on allows
// (PRD §30, SPEC-11 §1). Fires once per group completion.
//
// The task schema names events `success|failure|timeout` while executor
// statuses are `success|failed|timed_out`; eventName bridges the two so the
// policy compares apples to apples.
func (n *Notifier) NotifyTaskResult(r TaskResult) {
	if !allows(r.Task.NotifyOn, eventName(r.Status)) {
		return
	}
	event := eventName(r.Status)
	title := fmt.Sprintf("%s — %s", r.Task.Name, displayStatus(r.Status))
	message := buildMessage(r)

	// Play notification sound.
	preset := n.soundResolver(event)
	if err := PlaySound(preset, event); err != nil {
		n.log("heka: sound for %s: %v", r.Task.Slug, err)
	}

	// Desktop toast.
	if n.desktop != nil {
		if err := n.desktop(title, message); err != nil {
			n.log("heka: desktop notification for %s: %v", r.Task.Slug, err)
		}
	}

	for _, wb := range r.Task.Notify.Webhooks {
		n.deliverWebhook(wb, message)
	}
}

// buildMessage constructs a richer notification message.
func buildMessage(r TaskResult) string {
	parts := []string{}

	parts = append(parts, fmt.Sprintf("Trigger: %s", r.Trigger))

	if r.Duration > 0 {
		parts = append(parts, fmt.Sprintf("Duration: %s", r.Duration.Round(time.Millisecond)))
	}

	if r.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("Exit code: %d", r.ExitCode))
	}

	return strings.Join(parts, " • ")
}

// eventName maps a terminal status onto the notify_on vocabulary.
func eventName(status string) string {
	switch status {
	case "failed":
		return "failure"
	case "timed_out":
		return "timeout"
	}
	return status
}

// deliverWebhook resolves the destination and dispatches asynchronously.
func (n *Notifier) deliverWebhook(wb task.Webhook, message string) {
	dest, err := task.ResolveTemplate(wb.URL, n.resolve)
	if err != nil {
		n.log("heka: webhook %s: %v", wb.Format, err)
		return
	}
	chatID := ""
	if wb.Format == "telegram" {
		chatID, err = task.ResolveTemplate(wb.ChatID, n.resolve)
		if err != nil {
			n.log("heka: webhook telegram chat_id: %v", err)
			return
		}
	}
	go n.deliver(wb.Format, dest, chatID, message)
}

// deliver builds the format-specific payload and posts it (SPEC-11 §1.1),
// bounded by the timeout. Errors are logged, never raised.
func (n *Notifier) deliver(format, dest, chatID, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	var contentType string
	var body io.Reader

	switch format {
	case "telegram":
		values := url.Values{}
		values.Set("chat_id", chatID)
		values.Set("text", message)
		contentType = "application/x-www-form-urlencoded"
		body = strings.NewReader(values.Encode())
	case "discord":
		contentType = "application/json"
		body = bytes.NewBufferString(fmt.Sprintf(`{"content":%q}`, message))
	default: // slack, generic
		contentType = "application/json"
		body = bytes.NewBufferString(fmt.Sprintf(`{"text":%q}`, message))
	}

	if err := n.post(ctx, dest, contentType, body); err != nil {
		n.log("heka: webhook %s to %s: %v", format, dest, err)
	}
}

// allows reports whether the notify_on list contains the terminal status.
func allows(notifyOn []string, status string) bool {
	for _, e := range notifyOn {
		if e == status {
			return true
		}
	}
	return false
}

func displayStatus(s string) string {
	switch s {
	case "success":
		return "Success"
	case "failed":
		return "Failed"
	case "timed_out":
		return "Timed out"
	case "cancelled":
		return "Cancelled"
	}
	return s
}

// defaultPost is the production HTTP client path.
func defaultPost(ctx context.Context, dest, contentType string, body io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded %s", resp.Status)
	}
	return nil
}
