package task

import (
	"strings"
	"testing"
)

const scriptYAML = `
version: 1
name: Daily AI Research
slug: daily-ai-research
type: script
runtime: powershell
script: ./scripts/daily-ai-research.ps1
working_directory: ./scripts
environment:
  OPENROUTER_API_KEY: ${OPENROUTER_API_KEY}
timeout: 300
retry:
  max_attempts: 3
  delay_seconds: 30
capture_output: true
notify_on: [failure, timeout]
notify:
  webhooks:
    - format: slack
      url: ${SLACK_WEBHOOK_URL}
    - format: telegram
      url: https://api.telegram.org/bot${TOKEN}/sendMessage
      chat_id: ${TELEGRAM_CHAT_ID}
`

const binaryYAML = `
version: 1
name: Database Backup
slug: database-backup
type: binary
command: ./bin/backup.exe
args: ["--database", "main"]
working_directory: ./bin
timeout: 600
`

func TestParseValid(t *testing.T) {
	task, err := Parse([]byte(scriptYAML))
	if err != nil {
		t.Fatal(err)
	}
	if task.Slug != "daily-ai-research" || task.Runtime != "powershell" {
		t.Fatalf("parsed: %+v", task)
	}
	if len(task.Notify.Webhooks) != 2 || task.Notify.Webhooks[1].ChatID != "${TELEGRAM_CHAT_ID}" {
		t.Fatalf("webhooks: %+v", task.Notify)
	}

	binary, err := Parse([]byte(binaryYAML))
	if err != nil {
		t.Fatal(err)
	}
	if binary.Type != "binary" || binary.Command != "./bin/backup.exe" {
		t.Fatalf("binary: %+v", binary)
	}
	if binary.Runtime != "" { // default applies to script only; binary keeps ""
		t.Fatalf("binary runtime = %q, want \"\"", binary.Runtime)
	}
}

func TestParseDefaults(t *testing.T) {
	// Omitted runtime / retry delay get schema defaults.
	task, err := Parse([]byte("version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\n"))
	if err != nil {
		t.Fatal(err)
	}
	if task.Runtime != "custom" {
		t.Errorf("runtime = %q, want custom", task.Runtime)
	}
}

// TestDeprecatedCaptureOutputAndPumbleAlias locks in the v0.8 normalization:
// v0.7 files carrying capture_output still parse (strict schema tolerated via
// the deprecated field) but the value is dropped from canonical export, and
// webhook format "pumble" — same Slack-style payload — normalizes to "slack".
func TestDeprecatedCaptureOutputAndPumbleAlias(t *testing.T) {
	src := strings.Replace(scriptYAML, "format: slack", "format: pumble", 1)
	parsed, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.CaptureOutput != nil {
		t.Error("capture_output should be dropped by normalization")
	}
	if parsed.Notify.Webhooks[0].Format != "slack" {
		t.Errorf("pumble format = %q, want slack", parsed.Notify.Webhooks[0].Format)
	}

	out, err := Export(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "capture_output") {
		t.Errorf("canonical export still emits capture_output:\n%s", out)
	}
	if strings.Contains(string(out), "pumble") {
		t.Errorf("canonical export still emits pumble:\n%s", out)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := map[string]struct {
		yaml string
		want string // substring expected in the error
	}{
		"unknown field": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\nbogus: 1\n",
			want: "invalid task YAML",
		},
		"bad version": {
			yaml: "version: 2\nname: X\nslug: x\nscript: s.sh\ntype: script\n",
			want: "version: must be 1",
		},
		"missing version": {
			yaml: "name: X\nslug: x\nscript: s.sh\ntype: script\n",
			want: "version: must be 1",
		},
		"missing name": {
			yaml: "version: 1\nslug: x\nscript: s.sh\ntype: script\n",
			want: "name: required",
		},
		"bad slug uppercase": {
			yaml: "version: 1\nname: X\nslug: Bad_Slug\nscript: s.sh\ntype: script\n",
			want: "slug: must match",
		},
		"bad type": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: workflow\n",
			want: `type: "workflow"`,
		},
		"bad runtime": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\nruntime: ruby\n",
			want: "runtime",
		},
		"neg timeout": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\ntimeout: -5\n",
			want: "timeout",
		},
		"neg retry delay": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\nretry:\n  max_attempts: 2\n  delay_seconds: -1\n",
			want: "retry.delay_seconds",
		},
		"missing script": {
			yaml: "version: 1\nname: X\nslug: x\ntype: script\n",
			want: "script: required",
		},
		"missing command": {
			yaml: "version: 1\nname: X\nslug: x\ntype: binary\n",
			want: "command: required",
		},
		"script on binary": {
			yaml: "version: 1\nname: X\nslug: x\ntype: binary\ncommand: ./b.exe\nscript: s.sh\n",
			want: "script: only valid for script tasks",
		},
		"bad env ref": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\nenvironment:\n  K: ${UNCLOSED\n",
			want: "environment.K",
		},
		"bad notify_on": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\nnotify_on: [nope]\n",
			want: "notify_on",
		},
		"webhook missing url": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\nnotify:\n  webhooks:\n    - format: slack\n",
			want: "url required",
		},
		"webhook bad format": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\nnotify:\n  webhooks:\n    - format: teams\n      url: http://x\n",
			want: "not slack|discord|telegram|generic",
		},
		"telegram missing chat_id": {
			yaml: "version: 1\nname: X\nslug: x\nscript: s.sh\ntype: script\nnotify:\n  webhooks:\n    - format: telegram\n      url: https://api.telegram.org/botX/sendMessage\n",
			want: "chat_id required",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestExportRoundTrip(t *testing.T) {
	for name, src := range map[string]string{
		"script": scriptYAML,
		"binary": binaryYAML,
	} {
		t.Run(name, func(t *testing.T) {
			original, err := Parse([]byte(src))
			if err != nil {
				t.Fatal(err)
			}
			out, err := Export(original)
			if err != nil {
				t.Fatal(err)
			}
			again, err := Parse(out)
			if err != nil {
				t.Fatalf("exported YAML does not parse: %v\n%s", err, out)
			}
			if again.Name != original.Name || again.Slug != original.Slug ||
				again.Type != original.Type || again.Timeout != original.Timeout {
				t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", again, original)
			}
			if len(again.Notify.Webhooks) != len(original.Notify.Webhooks) {
				t.Fatalf("webhooks lost in round trip: %+v", again.Notify)
			}
		})
	}
}

func TestImportRejectsBadVersion(t *testing.T) {
	_, err := Import([]byte("version: 9\nname: X\nslug: x\nscript: s.sh\ntype: script\n"))
	if err == nil || !strings.Contains(err.Error(), "version: must be 1") {
		t.Fatalf("err = %v, want version error", err)
	}
}

func TestResolveValue(t *testing.T) {
	resolve := func(name string) (string, bool) {
		if name == "TOKEN" {
			return "secret-value", true
		}
		return "", false
	}
	if v, err := ResolveValue("hello", resolve); err != nil || v != "hello" {
		t.Fatalf("literal: %q, %v", v, err)
	}
	if v, err := ResolveValue("${TOKEN}", resolve); err != nil || v != "secret-value" {
		t.Fatalf("ref: %q, %v", v, err)
	}
	if _, err := ResolveValue("${MISSING}", resolve); err == nil {
		t.Fatal("expected missing-var error")
	}
	if _, err := ResolveValue("${BAD", resolve); err == nil {
		t.Fatal("expected malformed-ref error")
	}
	if !IsEnvRef("${TOKEN}") || IsEnvRef("plain") {
		t.Fatal("IsEnvRef misbehaving")
	}
}

func TestResolveTemplate(t *testing.T) {
	resolve := func(name string) (string, bool) {
		if name == "TOKEN" {
			return "abc123", true
		}
		return "", false
	}
	if v, err := ResolveTemplate("hello", resolve); err != nil || v != "hello" {
		t.Fatalf("literal: %q, %v", v, err)
	}
	// Inline ref allowed (telegram-style bot URLs).
	got, err := ResolveTemplate("https://api.t.org/bot${TOKEN}/sendMessage", resolve)
	if err != nil || got != "https://api.t.org/botabc123/sendMessage" {
		t.Fatalf("inline: %q, %v", got, err)
	}
	// Multiple refs.
	if got, err := ResolveTemplate("a=${TOKEN};b=${TOKEN}", resolve); err != nil || got != "a=abc123;b=abc123" {
		t.Fatalf("multi: %q, %v", got, err)
	}
	if _, err := ResolveTemplate("x${MISSING}y", resolve); err == nil {
		t.Fatal("expected missing-var error")
	}
	if _, err := ResolveTemplate("x${BAD", resolve); err == nil {
		t.Fatal("expected malformed-ref error")
	}
}

func TestIndexableFullTask(t *testing.T) {
	task, err := Parse([]byte(scriptYAML))
	if err != nil {
		t.Fatal(err)
	}
	if task.Slug != "daily-ai-research" || task.Type != "script" || task.Runtime != "powershell" {
		t.Fatalf("parsed task: %+v", task)
	}
}
