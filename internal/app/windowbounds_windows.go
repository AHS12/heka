//go:build windows

package app

import "syscall"

// virtualDesktopBounds returns the bounding box of every monitor combined
// (the Windows "virtual screen"), used to detect windows stranded on a
// disconnected display. ok is false when the metrics are unavailable.
func virtualDesktopBounds() (bounds, bool) {
	const (
		smXVirtualScreen  = 76
		smYVirtualScreen  = 77
		smCXVirtualScreen = 78
		smCYVirtualScreen = 79
	)
	getMetrics := syscall.NewLazyDLL("user32.dll").NewProc("GetSystemMetrics")
	metric := func(index uintptr) int {
		v, _, _ := getMetrics.Call(index)
		return int(int32(v))
	}
	left, top := metric(smXVirtualScreen), metric(smYVirtualScreen)
	right, bottom := left+metric(smCXVirtualScreen), top+metric(smCYVirtualScreen)
	if right <= left || bottom <= top {
		return bounds{}, false
	}
	return bounds{left: left, top: top, right: right, bottom: bottom}, true
}
