package task

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	slugRe       = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	envRefRe     = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
	refScan      = regexp.MustCompile(`\$\{[^}]*\}`)
	runtimes     = map[string]bool{"powershell": true, "python": true, "node": true, "bash": true, "custom": true}
	webhookFmts  = map[string]bool{"slack": true, "discord": true, "telegram": true, "generic": true}
	notifyEvents = map[string]bool{"success": true, "failure": true, "timeout": true}
)

// validate applies the SPEC-04 rule table and returns human-readable,
// field-labelled errors. It assumes defaults have been applied.
//
// Interpreter existence is deliberately NOT a validation rule — a task stays
// importable even when its runtime is missing on this machine (SPEC-04 §3);
// the executor flags that at run time.
func validate(t *Task) []string {
	var errs []string

	if t.Version != 1 {
		errs = append(errs, fmt.Sprintf("version: must be 1 (got %d)", t.Version))
	}
	if name := strings.TrimSpace(t.Name); name == "" {
		errs = append(errs, "name: required")
	}
	if t.Slug == "" {
		errs = append(errs, "slug: required")
	} else if !slugRe.MatchString(t.Slug) {
		errs = append(errs, "slug: must match lowercase slug format (a-z, 0-9, dashes)")
	}

	switch t.Type {
	case "script":
		if t.Runtime != "" && !runtimes[t.Runtime] {
			errs = append(errs, fmt.Sprintf("runtime: %q is not a supported runtime", t.Runtime))
		}
		if t.Command != "" {
			errs = append(errs, "command: only valid for binary tasks")
		}
		if strings.TrimSpace(t.Script) == "" {
			errs = append(errs, "script: required for script tasks")
		}
	case "binary":
		if t.Runtime != "" {
			errs = append(errs, "runtime: only valid for script tasks")
		}
		if t.Script != "" {
			errs = append(errs, "script: only valid for script tasks")
		}
		if strings.TrimSpace(t.Command) == "" {
			errs = append(errs, "command: required for binary tasks")
		}
	default:
		errs = append(errs, fmt.Sprintf("type: %q must be \"script\" or \"binary\"", t.Type))
	}

	if t.Timeout < 0 {
		errs = append(errs, fmt.Sprintf("timeout: must be >= 0 (got %d)", t.Timeout))
	}
	if t.Retry.MaxAttempts < 0 {
		errs = append(errs, fmt.Sprintf("retry.max_attempts: must be >= 0 (got %d)", t.Retry.MaxAttempts))
	}
	if t.Retry.DelaySeconds < 0 {
		errs = append(errs, fmt.Sprintf("retry.delay_seconds: must be >= 0 (got %d)", t.Retry.DelaySeconds))
	}

	for k, v := range t.Environment {
		if strings.Contains(v, "${") && !envRefRe.MatchString(v) {
			errs = append(errs, fmt.Sprintf("environment.%s: malformed ${VAR} reference", k))
		}
	}

	for _, e := range t.NotifyOn {
		if !notifyEvents[e] {
			errs = append(errs, fmt.Sprintf("notify_on: %q is not success|failure|timeout", e))
		}
	}

	for i, wb := range t.Notify.Webhooks {
		label := fmt.Sprintf("notify.webhooks[%d]", i)
		if wb.Format == "" {
			errs = append(errs, label+": format required")
		} else if !webhookFmts[wb.Format] {
			errs = append(errs, fmt.Sprintf("%s.format: %q is not slack|discord|telegram|generic", label, wb.Format))
		}
		if strings.TrimSpace(wb.URL) == "" {
			errs = append(errs, label+": url required")
		} else if malformedRef(wb.URL) != "" {
			errs = append(errs, label+".url: malformed ${VAR} reference")
		}
		if wb.Format == "telegram" {
			if strings.TrimSpace(wb.ChatID) == "" {
				errs = append(errs, label+": chat_id required for telegram")
			} else if malformedRef(wb.ChatID) != "" {
				errs = append(errs, label+".chat_id: malformed ${VAR} reference")
			}
		}
	}

	return errs
}

// malformedRef reports whether s contains an incomplete ${ reference. Inline
// refs ("https://api.telegram.org/bot${TOKEN}/sendMessage") are legal: every
// "${" must be closed by "}". Webhook urls/chat_ids use this template
// semantics; environment values use the stricter whole-value rule above.
func malformedRef(s string) string {
	rest := refScan.ReplaceAllString(s, "")
	if strings.Contains(rest, "${") {
		return rest
	}
	return ""
}
