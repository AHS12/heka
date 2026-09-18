// Desktop toasts (SPEC-11, SPEC-17 §4.13). DesktopToast is the single
// entry point for native notifications: on darwin it posts through
// UNUserNotificationCenter first — macOS derives the icon from the
// notifying process's bundle, so toasts carry the Heka icon — and falls
// back to beeep (osascript) whenever the native path is unavailable.
package notify

import "github.com/gen2brain/beeep"

var (
	// nativeToast is the bundle-icon path (darwin only; nil elsewhere).
	nativeToast func(title, message string) bool
	// beepToast is the osascript floor so a failed native path never
	// drops the notification. Var for tests.
	beepToast = func(title, message string) error {
		return beeep.Notify(title, message, "")
	}
)

// DesktopToast posts a desktop notification: native first, beeep fallback.
// Both daemon call sites (task results, backup events) go through here so
// the fallback lives in one place.
func DesktopToast(title, message string) error {
	if nativeToast != nil && nativeToast(title, message) {
		return nil
	}
	return beepToast(title, message)
}
