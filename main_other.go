//go:build !darwin

package main

// interactiveTerminal is a darwin-only heuristic (SPEC-18 §3.1); other
// platforms always open the GUI for a bare `heka`.
func interactiveTerminal() bool { return false }
