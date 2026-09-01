//go:build darwin

package notify

import "os/exec"

// playWAV plays a sound file on macOS using afplay (ships with every macOS).
func playWAV(path string) error {
	cmd := exec.Command("afplay", path)
	return cmd.Run()
}

// resolveSoundPath maps a preset to a macOS system sound file.
func resolveSoundPath(preset, eventType string) string {
	soundsDir := "/System/Library/Sounds"

	if preset == string(SoundSystem) {
		preset = eventType
	}

	switch preset {
	case "success":
		return soundsDir + "/Hero.aiff"
	case "failure":
		return soundsDir + "/Basso.aiff"
	case "timeout":
		return soundsDir + "/Ping.aiff"
	case string(SoundChime):
		return soundsDir + "/Glass.aiff"
	case string(SoundAlert):
		return soundsDir + "/Sosumi.aiff"
	case string(SoundBell):
		return soundsDir + "/Ping.aiff"
	default:
		return soundsDir + "/Glass.aiff"
	}
}
