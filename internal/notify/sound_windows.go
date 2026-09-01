//go:build windows

package notify

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	winmm          = syscall.NewLazyDLL("winmm.dll")
	mciSendStringW = winmm.NewProc("mciSendStringW")
)

// mciSendString sends a command string to the MCI interface.
func mciSendString(command string) error {
	commandPtr, err := syscall.UTF16PtrFromString(command)
	if err != nil {
		return err
	}
	ret, _, _ := mciSendStringW.Call(
		uintptr(unsafe.Pointer(commandPtr)),
		0, 0, 0,
	)
	if ret != 0 {
		return fmt.Errorf("MCI error: %d", ret)
	}
	return nil
}

// playWAV plays a WAV file using the Windows MCI interface.
// It opens the file, plays it, and waits for completion.
func playWAV(path string) error {
	// Use a unique alias per call to avoid conflicts.
	alias := fmt.Sprintf("heka_%d", time.Now().UnixNano())

	openCmd := fmt.Sprintf(`open "%s" type waveaudio alias %s`, path, alias)
	if err := mciSendString(openCmd); err != nil {
		return fmt.Errorf("MCI open: %w", err)
	}
	defer func() { _ = mciSendString(fmt.Sprintf("close %s", alias)) }()

	playCmd := fmt.Sprintf("play %s wait", alias)
	if err := mciSendString(playCmd); err != nil {
		return fmt.Errorf("MCI play: %w", err)
	}

	return nil
}

// resolveSoundPath maps a preset to a Windows system sound file.
// "system" uses the event-specific default; named presets map to distinct sounds.
func resolveSoundPath(preset, eventType string) string {
	mediaDir := `C:\Windows\Media`

	if preset == string(SoundSystem) {
		preset = eventType
	}

	switch preset {
	case "success":
		return mediaDir + `\Windows Notify System Generic.wav`
	case "failure":
		return mediaDir + `\Windows Exclamation.wav`
	case "timeout":
		return mediaDir + `\Windows Background.wav`
	case string(SoundChime):
		return mediaDir + `\Windows Notify System Generic.wav`
	case string(SoundAlert):
		return mediaDir + `\Windows Exclamation.wav`
	case string(SoundBell):
		return mediaDir + `\Windows Background.wav`
	default:
		return mediaDir + `\Windows Notify System Generic.wav`
	}
}
