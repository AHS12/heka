//go:build darwin

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// interactiveTerminal reports whether the process was launched from an
// interactive terminal (SPEC-18 §3.1): TERM is set and both stdio streams
// are ttys. LaunchServices/Dock/Finder launches have no TERM and no TTY,
// so they still open the GUI.
func interactiveTerminal() bool {
	if os.Getenv("TERM") == "" {
		return false
	}
	return isTTY(os.Stdin) && isTTY(os.Stdout)
}

func isTTY(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TIOCGETA)
	return err == nil
}
