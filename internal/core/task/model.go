// Package task models the canonical, portable Heka task definition (SPEC-04).
//
// The YAML schema v1 is the single source of truth (PRD §10, §11.4). This
// package never touches the database — the daemon derives index rows from
// tasks and stores them via the db package (one-way dependency).
//
// Structs carry both YAML and JSON tags: YAML is the canonical file format,
// JSON is the IPC wire format (SPEC-07).
package task

// Task is the YAML schema v1 model. Field order defines canonical export
// order; the schema is versioned via Version.
type Task struct {
	Version          int               `yaml:"version" json:"version"`
	Name             string            `yaml:"name" json:"name"`
	Slug             string            `yaml:"slug" json:"slug"`
	Type             string            `yaml:"type" json:"type"` // "script" | "binary"
	Runtime          string            `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Script           string            `yaml:"script,omitempty" json:"script,omitempty"`
	Command          string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args             []string          `yaml:"args,omitempty" json:"args,omitempty"`
	WorkingDirectory string            `yaml:"working_directory,omitempty" json:"working_directory,omitempty"`
	OutputDir       string            `yaml:"output_dir,omitempty" json:"output_dir,omitempty"`
	Environment     map[string]string `yaml:"environment,omitempty" json:"environment,omitempty"`
	Timeout          int               `yaml:"timeout" json:"timeout"`
	Retry            Retry             `yaml:"retry,omitempty" json:"retry,omitempty"`
	// Deprecated: no behavioral effect (the executor always records
	// status/exit/duration; output is capped in the runs row). Kept in the
	// struct so v0.7 task files still parse; applyDefaults drops the value
	// so canonical export omits it.
	CaptureOutput *bool `yaml:"capture_output,omitempty" json:"capture_output,omitempty"`
	NotifyOn         []string          `yaml:"notify_on,omitempty" json:"notify_on,omitempty"`
	Notify           Notify            `yaml:"notify,omitempty" json:"notify,omitempty"`
}

// Retry is the linear retry policy (PRD §29, SPEC-05 §5).
type Retry struct {
	MaxAttempts  int `yaml:"max_attempts" json:"max_attempts"`   // total executions, 0 = no retry
	DelaySeconds int `yaml:"delay_seconds" json:"delay_seconds"` // between attempts, default 30
}

// Notify groups the notification channels (SPEC-11 webhooks).
type Notify struct {
	Webhooks []Webhook `yaml:"webhooks,omitempty" json:"webhooks,omitempty"`
}

// Webhook is one outgoing webhook channel (PRD §30, D13).
type Webhook struct {
	Format string `yaml:"format" json:"format"`                       // slack | discord | telegram | generic
	URL    string `yaml:"url" json:"url"`                             // may contain ${VAR} refs
	ChatID string `yaml:"chat_id,omitempty" json:"chat_id,omitempty"` // required for telegram
}
