//go:build windows

package osapp

import (
	"os"
	"path/filepath"
)

func ConsoleExecutable() (string, error) {
	return siblingExecutable("heka.exe")
}

func GUIExecutable() (string, error) {
	return siblingExecutable("heka-gui.exe")
}

func siblingExecutable(name string) (string, error) {
	current, err := os.Executable()
	if err != nil {
		return "", err
	}
	sibling := filepath.Join(filepath.Dir(current), name)
	if _, err := os.Stat(sibling); err == nil {
		return sibling, nil
	}
	return current, nil
}
