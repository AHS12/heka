//go:build windows

package osapp

// trayIcon is the image bytes handed to systray.SetIcon. Windows systray
// requires ICO; the 32x32 PNG-compressed entry is built from the embedded
// icon.png.
func trayIcon() []byte {
	icoBytes, err := pngToICO(iconPNG)
	if err != nil {
		return iconPNG
	}
	return icoBytes
}
