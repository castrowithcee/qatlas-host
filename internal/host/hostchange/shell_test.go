package hostchange

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/redact"
)

func TestSystemShellRunsWithAFixedEnvironment(t *testing.T) {
	t.Setenv("DEMO_CALLER_VALUE", "leaked")
	res := SystemShell{}.RunShell(context.Background(), `echo "$DEMO_CALLER_VALUE|$LANG|$HOME"; cat; echo err >&2; exit 3`, 10*time.Second)
	if !res.Ran || res.ExitCode != 3 || res.TimedOut || res.Truncated {
		t.Fatalf("%+v", res)
	}
	// Standard input is closed (cat ends at once), stderr joins stdout, and nothing comes from the caller.
	if res.Output != "|C|/root\nerr\n" {
		t.Fatalf("output %q", res.Output)
	}
}

func TestSystemShellKillsTheWholeGroupWhenTimeIsUp(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "late")
	start := time.Now()
	res := SystemShell{}.RunShell(context.Background(), "(sleep 3; touch '"+marker+"') & sleep 30", 300*time.Millisecond)
	if !res.TimedOut || res.ExitCode != -1 || res.Succeeded() || time.Since(start) > 10*time.Second {
		t.Fatalf("%+v after %v", res, time.Since(start))
	}
	time.Sleep(3500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a child of the command outlived the time limit")
	}
}

func TestSystemShellCutsTheOutputAtTheLimit(t *testing.T) {
	res := SystemShell{}.RunShell(context.Background(), "head -c 200000 /dev/zero | tr '\\000' a", 30*time.Second)
	if res.ExitCode != 0 || !res.Truncated || len(res.Output) != MaxShellOutput {
		t.Fatalf("exit %d, truncated %v, %d bytes", res.ExitCode, res.Truncated, len(res.Output))
	}
	small := SystemShell{}.RunShell(context.Background(), "echo ok", 30*time.Second)
	if small.Truncated || small.Output != "ok\n" {
		t.Fatalf("%+v", small)
	}
}

type scriptedShell struct {
	ran   []string
	exits map[string]int
	out   map[string]string
}

func (s *scriptedShell) RunShell(_ context.Context, c string, _ time.Duration) ShellResult {
	s.ran = append(s.ran, c)
	return ShellResult{Command: c, Ran: true, ExitCode: s.exits[c], Output: s.out[c]}
}

func TestRunCommandsKeepsExitCodesAndHidesSecrets(t *testing.T) {
	f := newFixture(t, false)
	secret := "demo-secret-" + time.Now().Format("150405.000000")
	var secrets redact.Redactor
	secrets.Add(secret)
	var audited []AuditEntry
	f.runner.Record = func(a AuditEntry) error { audited = append(audited, a); return nil }
	sh := &scriptedShell{exits: map[string]int{"two": 4}, out: map[string]string{"one": "token " + secret}}
	res, results, err := f.runner.RunCommands(context.Background(), sh, CommandRun{
		ProposalID: "h-0123456789abcdef", Commands: []string{"one", "two", "three " + secret}, Timeout: time.Second, SudoUID: 1000,
		SubmitterUID: 4100, SubmitterName: "demo-submitter",
	}, secrets.Apply)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(sh.ran, "|") != "one|two" || results[2].Ran || strings.Contains(results[0].Output, secret) || !strings.Contains(results[0].Output, "token") {
		t.Fatalf("ran %v, results %+v", sh.ran, results)
	}
	e, err := f.journal.Get(res.Entry.ID)
	if err != nil || e.Result != ResultFailed || e.Reversible || len(e.Exits) != 2 || e.Exits[1] != 4 || len(e.Commands) != 3 ||
		e.SudoUID != 1000 || e.SubmitterUID != 4100 || e.SubmitterName != "demo-submitter" || strings.Contains(strings.Join(e.Commands, " "), secret) {
		t.Fatalf("entry %+v, %v", e, err)
	}
	if len(audited) != 1 || len(audited[0].Commands) != 0 || audited[0].ProposalID != "h-0123456789abcdef" || audited[0].Result != ResultFailed {
		t.Fatalf("audit %+v", audited)
	}
	if _, err := f.runner.Undo(context.Background(), e.ID); err == nil || !strings.Contains(err.Error(), "cannot be undone") {
		t.Fatalf("undo: %v", err)
	}
	if _, err := f.runner.PreviewUndo(e.ID); err == nil {
		t.Fatal("the preview of an undo must refuse too")
	}
}
