//go:build !windows

package ipc

// pipeSecurityDescriptor is never called on POSIX (unix sockets use file
// permissions instead of SDDL); it exists so the Windows-only seam compiles
// on every GOOS.
func pipeSecurityDescriptor() string {
	return "D:P(A;;GA;;;OW)"
}
