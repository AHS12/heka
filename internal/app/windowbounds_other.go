//go:build !windows

package app

// virtualDesktopBounds has no portable implementation outside Windows; the
// off-screen check is skipped (valid=false) and the saved position is
// trusted as-is.
func virtualDesktopBounds() (bounds, bool) { return bounds{}, false }
