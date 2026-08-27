package notify

import (
	"fmt"
	"os"

	"github.com/gen2brain/beeep"
)

// SoundPreset is a named notification sound configuration.
type SoundPreset string

const (
	SoundSystem  SoundPreset = "system"
	SoundSilent  SoundPreset = "silent"
	SoundChime   SoundPreset = "chime"
	SoundAlert   SoundPreset = "alert"
	SoundBell    SoundPreset = "bell"
)

// ValidPresets is the set of allowed sound preset values.
var ValidPresets = map[SoundPreset]bool{
	SoundSystem: true,
	SoundSilent: true,
	SoundChime:  true,
	SoundAlert:  true,
	SoundBell:   true,
}

// PresetLabels maps presets to human-readable labels for the UI.
var PresetLabels = map[SoundPreset]string{
	SoundSystem: "System default",
	SoundSilent: "Silent",
	SoundChime:  "Chime",
	SoundAlert:  "Alert",
	SoundBell:   "Bell",
}

// beepFallback tones per event type (freq Hz, duration ms).
var beepFallback = map[string][2]float64{
	"success": {880, 150},
	"failure": {220, 400},
	"timeout": {440, 300},
}

// PlaySound plays the notification sound for the given preset and event type.
// If the preset is "silent", it returns immediately. If the OS sound file is
// missing, it falls back to a beeep tone.
func PlaySound(preset string, eventType string) error {
	if preset == string(SoundSilent) {
		return nil
	}

	path := resolveSoundPath(preset, eventType)
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			return playWAV(path)
		}
	}

	// Fallback to beeep tone.
	tone, ok := beepFallback[eventType]
	if !ok {
		tone = [2]float64{440, 200}
	}
	return beeep.Beep(tone[0], int(tone[1]))
}

// DefaultPreset returns the default preset for an event type.
func DefaultPreset(eventType string) string {
	return string(SoundSystem)
}

// ValidatePreset checks if a preset string is valid.
func ValidatePreset(s string) error {
	if _, ok := ValidPresets[SoundPreset(s)]; !ok {
		return fmt.Errorf("invalid sound preset: %q (valid: system, silent, chime, alert, bell)", s)
	}
	return nil
}
