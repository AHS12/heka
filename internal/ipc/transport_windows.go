//go:build windows

package ipc

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// fallbackPipeSD is the owner-only descriptor used when the current user's
// SID cannot be resolved. Better than the permissive default (which grants
// read to Everyone), but see pipeSecurityDescriptor for its flaw.
const fallbackPipeSD = "D:P(A;;GA;;;OW)"

// pipeSecurityDescriptor builds the pipe's SDDL at runtime: SYSTEM and
// Administrators keep full control, and the running user's SID is granted
// generic access so every process of that user — elevated or not — can reach
// the endpoint.
//
// A static owner-only descriptor (D:P(A;;GA;;;OW)) cannot express this:
// objects created by an elevated process are owned by BUILTIN\Administrators
// (the elevated token's default owner), so the same user's non-elevated
// CLI/GUI/watchdog get ERROR_ACCESS_DENIED on dial and bind — which reads as
// "daemon is not running".
func pipeSecurityDescriptor() string {
	sid, err := currentUserSID()
	if err != nil {
		return fallbackPipeSD
	}
	return fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;%s)", sid)
}

// currentUserSID resolves the process token's user SID. The current-process
// pseudo-token needs no close and is valid for TOKEN_QUERY.
func currentUserSID() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}
