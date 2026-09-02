package ipc

import "testing"

func TestPipeNameOverride(t *testing.T) {
	t.Setenv("HEKA_PIPE_NAME", "heka-custom")
	if got := pipeName(); got != "heka-custom" {
		t.Fatalf("pipeName = %q, want override honored", got)
	}
}

func TestPipeNameFromUsername(t *testing.T) {
	t.Setenv("HEKA_PIPE_NAME", "")
	t.Setenv("USERNAME", "AHS12")
	if got := pipeName(); got != "heka-AHS12" {
		t.Fatalf("pipeName = %q, want heka-AHS12", got)
	}
}

func TestSanitizeUser(t *testing.T) {
	// os/user on Windows can return DOMAIN\user; backslashes are illegal in
	// pipe names and the bare form must match the USERNAME env form.
	cases := map[string]string{
		"AHS12":          "AHS12",
		`AHS12-PC\AHS12`: "AHS12",
		`domain/user`:    "user",
		"":               "",
		`a\b\c`:          "c",
	}
	for in, want := range cases {
		if got := sanitizeUser(in); got != want {
			t.Fatalf("sanitizeUser(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPipeNameValidWithoutUsername(t *testing.T) {
	t.Setenv("HEKA_PIPE_NAME", "")
	t.Setenv("USERNAME", "")
	// Whatever the fallback resolves to, it must be a legal pipe name (no
	// backslash): the pipe is only usable if every process derives the same
	// name, and NPFS rejects backslashes outright.
	if got := pipeName(); containsBackslash(got) {
		t.Fatalf("pipeName = %q contains a backslash", got)
	}
}

func containsBackslash(s string) bool {
	for _, r := range s {
		if r == '\\' {
			return true
		}
	}
	return false
}
