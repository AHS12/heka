package db

import (
	"strings"
	"testing"
)

func TestSecretsEncryptedAtRest(t *testing.T) {
	d := openTest(t)
	value := "sk-real-secret-9f8a7b6c"

	if err := d.Secrets().Set("OPENROUTER_API_KEY", value); err != nil {
		t.Fatal(err)
	}

	// The raw row must not contain the plaintext anywhere.
	var stored string
	if err := d.sql.QueryRow(
		`SELECT value FROM secrets WHERE key = 'OPENROUTER_API_KEY'`,
	).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == value || strings.Contains(stored, "sk-real") {
		t.Fatalf("secret stored in plaintext: %q", stored)
	}

	// Round trip decrypts back to the original.
	got, ok, err := d.Secrets().Get("OPENROUTER_API_KEY")
	if err != nil || !ok || got != value {
		t.Fatalf("get = %q %v %v", got, ok, err)
	}
}

func TestSecretsLegacyPlaintextStillReadable(t *testing.T) {
	d := openTest(t)
	legacy := "pre-encryption-value"
	if _, err := d.sql.Exec(
		`INSERT INTO secrets (key, value) VALUES ('OLD_KEY', ?)`, legacy); err != nil {
		t.Fatal(err)
	}
	got, ok, err := d.Secrets().Get("OLD_KEY")
	if err != nil || !ok || got != legacy {
		t.Fatalf("legacy get = %q %v %v", got, ok, err)
	}
}

func TestSecretsKeyIsStablePerDataDir(t *testing.T) {
	// Two Open/Close cycles over the same dir must read each other's values.
	dir := t.TempDir()
	d1, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d1.Secrets().Set("K", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := d1.Close(); err != nil {
		t.Fatal(err)
	}

	d2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	got, ok, err := d2.Secrets().Get("K")
	if err != nil || !ok || got != "v1" {
		t.Fatalf("cross-open get = %q %v %v", got, ok, err)
	}
}

func TestSecretsNamesStayPlaintext(t *testing.T) {
	d := openTest(t)
	if err := d.Secrets().Set("SLACK_WEBHOOK_URL", "x"); err != nil {
		t.Fatal(err)
	}
	keys, err := d.Secrets().Keys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "SLACK_WEBHOOK_URL" {
		t.Fatalf("keys = %v", keys)
	}
}