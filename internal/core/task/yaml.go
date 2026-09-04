package task

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse decodes canonical task YAML strictly (unknown fields are errors),
// applies defaults, and validates. It is the shared codec for LoadFile,
// Import, and the GUI's YAML tab.
func Parse(data []byte) (Task, error) {
	var t Task
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil {
		return Task{}, fmt.Errorf("invalid task YAML: %w", err)
	}
	applyDefaults(&t)
	if errs := validate(&t); len(errs) > 0 {
		return Task{}, fmt.Errorf("invalid task: %s", strings.Join(errs, "; "))
	}
	return t, nil
}

// ValidateYAML returns the per-problem list for a YAML document without
// producing a Task: schema errors surface as one item, field-validation
// errors as one item each (SPEC-13 §1, the editor's Form↔YAML handoff).
func ValidateYAML(data []byte) []string {
	var t Task
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil {
		return []string{"invalid task YAML: " + err.Error()}
	}
	applyDefaults(&t)
	return validate(&t)
}

// Import is Parse — an alias for the portability story (PRD §27): bytes in,
// validated task out.
func Import(data []byte) (Task, error) {
	return Parse(data)
}

// Export renders the canonical normalized YAML. The output must round-trip:
// Parse(Export(t)) == t.
func Export(t Task) ([]byte, error) {
	applyDefaults(&t)
	if errs := validate(&t); len(errs) > 0 {
		return nil, fmt.Errorf("invalid task: %s", strings.Join(errs, "; "))
	}
	out, err := yaml.Marshal(&t)
	if err != nil {
		return nil, fmt.Errorf("export task: %w", err)
	}
	return out, nil
}

// applyDefaults fills schema defaults before validation/marshal:
// runtime=custom (script), retry.delay_seconds=30. Deprecated/alias values
// are normalized away: capture_output (v0.7 field with no behavioral effect)
// is dropped from canonical output, and webhook format "pumble" — whose
// incoming webhooks accept Slack-style payloads — becomes "slack".
func applyDefaults(t *Task) {
	if t.Type == "script" && t.Runtime == "" {
		t.Runtime = "custom"
	}
	t.CaptureOutput = nil
	for i := range t.Notify.Webhooks {
		if t.Notify.Webhooks[i].Format == "pumble" {
			t.Notify.Webhooks[i].Format = "slack"
		}
	}
	if t.Retry.MaxAttempts > 0 && t.Retry.DelaySeconds == 0 {
		t.Retry.DelaySeconds = 30
	}
}

// IsEnvRef reports whether s is a single ${NAME} reference. Literals are
// passed through as-is by ResolveValue.
func IsEnvRef(s string) bool {
	return envRefRe.MatchString(s)
}

// ResolveValue resolves a single value: if it is a ${NAME} reference, look
// the name up via resolve; otherwise return it unchanged. Missing names
// produce an error (callers decide how to treat the channel/task).
func ResolveValue(s string, resolve func(name string) (string, bool)) (string, error) {
	if !strings.Contains(s, "${") {
		return s, nil
	}
	m := envRefRe.FindStringSubmatch(s)
	if m == nil {
		return "", fmt.Errorf("malformed ${VAR} reference: %q", s)
	}
	if v, ok := resolve(m[1]); ok {
		return v, nil
	}
	return "", fmt.Errorf("environment variable %q not found", m[1])
}

// ResolveTemplate substitutes every ${NAME} reference in s. Inline refs are
// allowed — used for webhook urls/chat_ids (SPEC-11). Any reference missing
// from the resolver is an error.
func ResolveTemplate(s string, resolve func(name string) (string, bool)) (string, error) {
	if !strings.Contains(s, "${") {
		return s, nil
	}
	if rest := malformedRef(s); rest != "" {
		return "", fmt.Errorf("malformed ${VAR} reference: %q", rest)
	}
	var err error
	out := refScan.ReplaceAllStringFunc(s, func(ref string) string {
		if err != nil {
			return ref
		}
		m := envRefRe.FindStringSubmatch(ref)
		if m == nil {
			err = fmt.Errorf("malformed ${VAR} reference: %q", ref)
			return ref
		}
		if v, ok := resolve(m[1]); ok {
			return v
		}
		err = fmt.Errorf("environment variable %q not found", m[1])
		return ref
	})
	if err != nil {
		return "", err
	}
	return out, nil
}
