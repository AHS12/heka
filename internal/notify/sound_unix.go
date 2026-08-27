//go:build !windows && !darwin

package notify

import (
	"os/exec"
)

// playWAV plays a sound file on Linux using paplay (PulseAudio) or aplay (ALSA).
func playWAV(path string) error {
	// Try paplay first (PulseAudio, most common on desktop Linux).
	if err := exec.Command("paplay", path).Run(); err == nil {
		return nil
	}
	// Fall back to aplay (ALSA).
	return exec.Command("aplay", "-q", path).Run()
}

// resolveSoundPath maps a preset to a Linux sound file.
// Paths follow the XDG/freedesktop sound theme convention.
func resolveSoundPath(preset, eventType string) string {
	soundsDir := "/usr/share/sounds"

	if preset == string(SoundSystem) {
		preset = eventType
	}

	switch preset {
	case "success":
		return soundsDir + "/stereo/bell.oga"
	case "failure":
		return soundsDir + "/stereo/bell-red.oga"
	case "timeout":
		return soundsDir + "/stereo/message-new-instant.oga"
	case string(SoundChime):
		return soundsDir + "/stereo/display-notification.oga"
	case string(SoundAlert):
		return soundsDir + "/stereo/alarm-clock-elapsed.oga"
	case string(SoundBell):
		return soundsDir + "/stereo/bell.oga"
	default:
		return soundsDir + "/stereo/display-notification.oga"
	}
}
