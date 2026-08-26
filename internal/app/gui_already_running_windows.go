//go:build windows

// GUI-already-running notice: shown by the single-instance guard before any
// Wails window exists, so it is a plain Win32 message box (no owner window).
package app

import "golang.org/x/sys/windows"

// ShowGUIAlreadyRunning tells the user a GUI instance is already open.
func ShowGUIAlreadyRunning() {
	title, _ := windows.UTF16PtrFromString("Heka")
	text, _ := windows.UTF16PtrFromString("Heka GUI is already running.")
	flags := uint32(windows.MB_OK | windows.MB_ICONINFORMATION | windows.MB_SETFOREGROUND | windows.MB_TOPMOST)
	_, _ = windows.MessageBox(0, text, title, flags)
}
