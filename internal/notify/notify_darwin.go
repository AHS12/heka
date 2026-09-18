//go:build darwin

package notify

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications
#include <stdlib.h>
#include "notify_darwin.h"
*/
import "C"

import "unsafe"

func init() {
	nativeToast = nativeToastImpl
}

// nativeToastImpl posts via UNUserNotificationCenter — the toast carries
// the Heka bundle icon. Returns false whenever the native path is
// unavailable (no NSApp under HEKA_NO_TRAY, permission denied, system
// rejection) so the caller falls back to beeep.
func nativeToastImpl(title, message string) bool {
	if C.notifyAvailable() == 0 {
		return false
	}
	ct, cm := C.CString(title), C.CString(message)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cm))
	return C.notifyPost(ct, cm) == 0
}