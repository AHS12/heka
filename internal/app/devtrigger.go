// Dev-trigger plumbing: `heka dev whats-new|tour|reset|update-from` drops a
// one-shot JSON file into the data directory and the GUI consumes it through
// App.TakeDevTrigger (polled while running, on mount otherwise). Pure
// GUI-local state — the daemon never reads or writes the file, mirroring the
// window-state.json precedent.
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// onboardingTriggerFile is the trigger file name inside the data directory.
const onboardingTriggerFile = "onboarding-trigger.json"

// OnboardingTriggerPath returns the trigger file path inside a data directory.
func OnboardingTriggerPath(dataDir string) string {
	return filepath.Join(dataDir, onboardingTriggerFile)
}

// DevTrigger is a one-shot command the CLI leaves for the GUI.
type DevTrigger struct {
	Trigger string `json:"trigger"` // whats-new | tour | reset | update-from
	Version string `json:"version,omitempty"`
}

// validTriggers is the set the GUI acts on; anything else is consumed and
// ignored.
var validTriggers = map[string]bool{
	"whats-new":   true,
	"tour":        true,
	"reset":       true,
	"update-from": true,
}

// WriteDevTrigger drops a trigger for the GUI to consume (CLI side). Last
// write wins — fine for a dev aid.
func WriteDevTrigger(path string, t DevTrigger) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// TakeDevTrigger consumes the pending dev trigger, if any. The GUI calls this
// at startup and polls it; every failure case degrades to "nothing pending"
// so a dev aid never disturbs normal operation.
func (a *App) TakeDevTrigger() DevTrigger {
	dir := a.DataDir()
	if dir == "" {
		return DevTrigger{}
	}
	return TakeDevTriggerFile(OnboardingTriggerPath(dir))
}

// TakeDevTriggerFile atomically consumes the trigger file: it is renamed
// first so a concurrent CLI write can never be observed half-written.
// Malformed or unknown content is consumed and ignored.
func TakeDevTriggerFile(path string) DevTrigger {
	tmp := path + ".consuming"
	if err := os.Rename(path, tmp); err != nil {
		return DevTrigger{} // nothing pending
	}
	defer os.Remove(tmp) // best effort; read below may already have failed
	data, err := os.ReadFile(tmp)
	if err != nil {
		return DevTrigger{}
	}
	var t DevTrigger
	if json.Unmarshal(data, &t) != nil || !validTriggers[t.Trigger] {
		return DevTrigger{}
	}
	return t
}
