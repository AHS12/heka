package notify

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"heka/internal/core/task"
)

// recorded collects desktop/log/posts under one mutex. Reads are snapshots so
// -race stays happy while delivery goroutines run.
type recorded struct {
	mu      sync.Mutex
	desktop []string
	posts   []string
	logged  []string
}

func (r *recorded) recordDesktop(title, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.desktop = append(r.desktop, title)
	return nil
}

func (r *recorded) recordLog(format string, _ ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logged = append(r.logged, format)
}

func (r *recorded) recordPost(url, contentType string, body []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.posts = append(r.posts, url+"|"+contentType+"|"+string(body))
}

func (r *recorded) desktopSends() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.desktop...)
}

// waitPosts blocks until n posts arrive or the deadline passes.
func (r *recorded) waitPosts(t *testing.T, n int, within time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if posts := r.postSnapshot(); len(posts) >= n {
			return posts
		}
		if time.Now().After(deadline) {
			t.Fatalf("want %d posts within %v, have %d", n, within, len(r.postSnapshot()))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *recorded) postSnapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.posts...)
}

func (r *recorded) loggedSnapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.logged...)
}

// testNotifier builds a Notifier wired to a recorder (thread-safe reads).
func testNotifier(t *testing.T) (*Notifier, *recorded) {
	t.Helper()
	rec := &recorded{}
	n := &Notifier{
		desktop: rec.recordDesktop,
		resolve: func(name string) (string, bool) {
			switch name {
			case "URL":
				return "https://hooks.example/slack", true
			case "CHAT":
				return "-100123", true
			}
			return "", false
		},
		log: rec.recordLog,
		post: func(_ context.Context, url, contentType string, body io.Reader) error {
			b, _ := io.ReadAll(body)
			rec.recordPost(url, contentType, b)
			return nil
		},
		soundResolver: func(eventType string) string {
			return string(SoundSilent)
		},
		timeout: time.Second,
	}
	return n, rec
}

func mkTask(notifyOn []string, webhooks ...task.Webhook) *task.Task {
	return &task.Task{
		Name: "Alpha", Slug: "alpha", NotifyOn: notifyOn, Notify: task.Notify{Webhooks: webhooks},
	}
}

func mkResult(t *task.Task, status string) TaskResult {
	return TaskResult{
		Task:     t,
		Status:   status,
		Trigger:  "manual",
		Duration: 5 * time.Second,
	}
}

func TestPolicyTable(t *testing.T) {
	for _, tt := range []struct {
		notifyOn []string
		status   string
		want     int
	}{
		{[]string{"failure"}, "failed", 1},
		{[]string{"failure"}, "success", 0},
		{[]string{"failure", "success"}, "timed_out", 0},
		{[]string{"failure,timeout"}, "timed_out", 0}, // typo list; no match
		{nil, "failed", 0},
		{[]string{"success"}, "cancelled", 0}, // cancelled is never notifyable
		{[]string{"success"}, "success", 1},
	} {
		n, rec := testNotifier(t)
		n.NotifyTaskResult(mkResult(mkTask(tt.notifyOn), tt.status))
		if got := len(rec.desktopSends()); got != tt.want {
			t.Fatalf("notify_on=%v status=%s: desktop sends = %d, want %d",
				tt.notifyOn, tt.status, got, tt.want)
		}
	}
}

func TestDesktopTitle(t *testing.T) {
	n, rec := testNotifier(t)
	n.NotifyTaskResult(mkResult(mkTask([]string{"failure"}), "failed"))
	sends := rec.desktopSends()
	if len(sends) != 1 || !strings.Contains(sends[0], "Alpha") ||
		!strings.Contains(sends[0], "Failed") {
		t.Fatalf("desktop title = %v", sends)
	}
}

func TestWebhookPayloads(t *testing.T) {
	webhooks := []task.Webhook{
		{Format: "slack", URL: "${URL}"},
		{Format: "discord", URL: "${URL}"},
		{Format: "telegram", URL: "${URL}", ChatID: "${CHAT}"},
		{Format: "generic", URL: "${URL}"},
	}
	n, rec := testNotifier(t)
	n.NotifyTaskResult(mkResult(mkTask([]string{"failure"}, webhooks...), "failed"))

	posts := rec.waitPosts(t, 4, 2*time.Second)

	// Delivery goroutines race, so classify instead of relying on order.
	textCount, contentCount, formCount := 0, 0, 0
	for _, p := range posts {
		if !strings.HasPrefix(p, "https://hooks.example/slack|") {
			t.Fatalf("unexpected post: %v", posts)
		}
		switch {
		case strings.Contains(p, `"content"`):
			contentCount++ // discord
		case strings.Contains(p, "application/x-www-form-urlencoded"):
			formCount++ // telegram
		default:
			textCount++ // slack + generic → 2
		}
	}
	if contentCount != 1 || formCount != 1 || textCount != 2 {
		t.Fatalf("payload classes = text:%d content:%d form:%d\n%v", textCount, contentCount, formCount, posts)
	}
	if !strings.Contains(strings.Join(posts, "\n"), "-100123") {
		t.Fatalf("telegram chat_id missing: %v", posts)
	}
}

func TestUnresolvableWebhookSkipped(t *testing.T) {
	webhooks := []task.Webhook{{Format: "slack", URL: "${MISSING}"}}
	n, rec := testNotifier(t)
	n.NotifyTaskResult(mkResult(mkTask([]string{"failure"}, webhooks...), "failed"))
	time.Sleep(100 * time.Millisecond)
	if len(rec.postSnapshot()) != 0 {
		t.Fatalf("unresolvable webhook was sent: %v", rec.postSnapshot())
	}
	if len(rec.loggedSnapshot()) == 0 {
		t.Fatal("unresolvable webhook was not logged")
	}
}

func TestSlowWebhookDoesNotBlockOrCrash(t *testing.T) {
	webhooks := []task.Webhook{{Format: "slack", URL: "${URL}"}}
	var mu sync.Mutex
	started := false
	n := &Notifier{
		desktop: func(string, string) error { return nil },
		resolve: func(name string) (string, bool) { return "https://slow", true },
		log:     func(string, ...any) {},
		post: func(ctx context.Context, _ string, _ string, _ io.Reader) error {
			mu.Lock()
			started = true
			mu.Unlock()
			<-ctx.Done() // hang past the timeout
			return errors.New("timeout")
		},
		soundResolver: func(eventType string) string {
			return string(SoundSilent)
		},
		timeout: 50 * time.Millisecond,
	}
	n.NotifyTaskResult(mkResult(mkTask([]string{"failure"}, webhooks...), "failed"))
	// Notify returns immediately even though the delivery hangs.
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		s := started
		mu.Unlock()
		if s {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("delivery never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // the goroutine must limp along, not crash
}

func TestCancelledStatusNeverNotifies(t *testing.T) {
	n, rec := testNotifier(t)
	n.NotifyTaskResult(mkResult(mkTask([]string{"failure", "success", "timeout"}), "cancelled"))
	if len(rec.desktopSends()) != 0 {
		t.Fatalf("cancelled must not notify: %v", rec.desktopSends())
	}
}

func TestBuildMessage(t *testing.T) {
	r := TaskResult{
		Task:     mkTask([]string{"failure"}),
		Status:   "failed",
		Trigger:  "schedule",
		Duration: 5*time.Second + 123*time.Millisecond,
		ExitCode: 1,
	}
	msg := buildMessage(r)
	if !strings.Contains(msg, "Trigger: schedule") {
		t.Fatalf("message missing trigger: %s", msg)
	}
	if !strings.Contains(msg, "Duration: 5.123s") {
		t.Fatalf("message missing duration: %s", msg)
	}
	if !strings.Contains(msg, "Exit code: 1") {
		t.Fatalf("message missing exit code: %s", msg)
	}
}

func TestBuildMessageSuccessNoExitCode(t *testing.T) {
	r := TaskResult{
		Task:     mkTask([]string{"success"}),
		Status:   "success",
		Trigger:  "manual",
		Duration: 2 * time.Second,
		ExitCode: 0,
	}
	msg := buildMessage(r)
	if strings.Contains(msg, "Exit code") {
		t.Fatalf("success message should not include exit code: %s", msg)
	}
}
