// Package cli implements the Heka command-line client. The full command set
// arrives in SPEC-08; for now it only provides the Stub entry point.
package cli

import (
	"fmt"
	"os"
)

// Stub is the placeholder for CLI commands. It always fails so scripts notice
// the command is not implemented yet.
func Stub(command string) {
	fmt.Fprintf(os.Stderr, "heka: command %q is not implemented yet (planned in SPEC-08)\n", command)
	os.Exit(1)
}
