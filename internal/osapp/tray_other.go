//go:build !windows

package osapp

// trayIcon is the image bytes handed to systray.SetIcon. macOS (and Linux)
// menus want plain PNG bytes; the ICO conversion is Windows-only.
func trayIcon() []byte {
	return iconPNG
}
