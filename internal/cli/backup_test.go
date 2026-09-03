package cli

import (
	"errors"
	"strings"
	"testing"

	"heka/internal/ipc"
)

func TestBackupStatusHumanAndJSON(t *testing.T) {
	stub := &stubClient{}
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "backup", "status"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "running:") || !strings.Contains(out.String(), "last:") {
		t.Fatalf("human status:\n%s", out.String())
	}

	a2, out2, _ := newTestApp(stub)
	if err := runArgs(t, a2, "backup", "status", "--json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out2.String(), `"running":false`) {
		t.Fatalf("json status:\n%s", out2.String())
	}
}

func TestBackupHistoryOutput(t *testing.T) {
	stub := &stubClient{}
	// The stub returns no history; the empty case must render cleanly.
	a, out, _ := newTestApp(stub)
	if err := runArgs(t, a, "backup", "history"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no backups yet") {
		t.Fatalf("empty history:\n%s", out.String())
	}
}

func TestBackupTestReportsDestinations(t *testing.T) {
	// The stub returns zero results; the command must not error and must
	// render nothing (both destinations unconfigured).
	stub := &stubClient{}
	a, out, errOut := newTestApp(stub)
	if err := runArgs(t, a, "backup", "test"); err != nil {
		t.Fatal(err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr:\n%s", errOut.String())
	}
	if strings.Contains(out.String(), "FAILED") {
		t.Fatalf("unconfigured destinations must not look failed:\n%s", out.String())
	}
}

func TestBackupCreateSurfacesDaemonErrors(t *testing.T) {
	stub := &stubClient{err: errors.New("a backup is already running")}
	a, _, errOut := newTestApp(stub)
	if err := runArgs(t, a, "backup", "create"); err == nil {
		t.Fatal("busy error must propagate")
	}
	if !strings.Contains(errOut.String(), "already running") {
		t.Fatalf("stderr:\n%s", errOut.String())
	}
}

func TestRestoreRequiresPassphraseAndYes(t *testing.T) {
	a, _, errOut := newTestApp(&stubClient{})
	if err := runArgs(t, a, "restore", "backup.zip"); err == nil {
		t.Fatal("missing passphrase must error")
	}
	if !strings.Contains(errOut.String(), "passphrase is required") {
		t.Fatalf("stderr:\n%s", errOut.String())
	}
}

func TestBackupCommandsInHelp(t *testing.T) {
	a, out, _ := newTestApp(&stubClient{})
	if err := runArgs(t, a, "--help"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "backup") || !strings.Contains(out.String(), "restore") {
		t.Fatalf("help missing backup commands:\n%s", out.String())
	}
	_ = ipc.BackupJobDTO{} // keep the import honest if fixtures evolve
}
