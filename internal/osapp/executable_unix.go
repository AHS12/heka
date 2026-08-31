//go:build !windows

package osapp

import "os"

func ConsoleExecutable() (string, error) {
	return os.Executable()
}

func GUIExecutable() (string, error) {
	return os.Executable()
}
