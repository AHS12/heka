//go:build !windows

// GUI-already-running notice stub: the single-instance guard always passes
// off-Windows, so this never fires.
package app

// ShowGUIAlreadyRunning is a no-op on non-Windows platforms.
func ShowGUIAlreadyRunning() {}
