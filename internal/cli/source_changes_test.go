package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func changesHandlers(seen *[]json.RawMessage) fakeHandlers {
	keep := func(raw json.RawMessage) { *seen = append(*seen, raw) }
	return fakeHandlers{
		control.SourceChangesCommand: func(raw json.RawMessage) any {
			keep(raw)
			return control.SourceChangesResult{Path: "/srv/stacks", Changes: []control.SourceChange{
				{Path: "web/.env", Kind: "modified", Stack: "web"}, {Path: "db/notes.txt", Kind: "new", Stack: "db"}}}
		},
		control.SourceDiffCommand: func(raw json.RawMessage) any {
			keep(raw)
			return control.SourceDiffResult{Path: "/srv/stacks", File: "web/.env", Diff: "-a\n+b\n", Truncated: true}
		},
		control.SourceDiscardCommand: func(raw json.RawMessage) any {
			keep(raw)
			return control.SourceDiscardResult{Path: "/srv/stacks", Discarded: []string{"web/.env", "db/notes.txt"}}
		},
		control.SourceCommitCommand: func(raw json.RawMessage) any {
			keep(raw)
			return control.SourceCommitResult{Path: "/srv/stacks", Commit: strings.Repeat("c", 40), Committed: []string{"web/.env", "db/old.txt"}}
		},
	}
}

func TestSourceChangesDiffAndDiscardCallTheService(t *testing.T) {
	var seen []json.RawMessage
	startFakeService(t, withStatus(changesHandlers(&seen), []string{"/srv/stacks"}, "web"))

	code, stdout, stderr := run(t, "source", "changes", "/srv/stacks", "--output", "json")
	var list struct {
		Path    string
		Changes []control.SourceChange
	}
	if err := json.Unmarshal([]byte(stdout), &list); code != 0 || err != nil || list.Path != "/srv/stacks" || len(list.Changes) != 2 ||
		list.Changes[1] != (control.SourceChange{Path: "db/notes.txt", Kind: "new", Stack: "db"}) {
		t.Fatalf("changes: code %d, stdout %q, stderr %q, err %v", code, stdout, stderr, err)
	}
	if string(seen[0]) != `{"path":"/srv/stacks"}` {
		t.Fatalf("changes arguments = %s", seen[0])
	}
	if code, stdout, _ := run(t, "source", "changes", "/srv/stacks"); code != 0 || !strings.Contains(stdout, "db/notes.txt") || !strings.Contains(stdout, "new") {
		t.Fatalf("plain changes: code %d, stdout %q", code, stdout)
	}

	code, stdout, stderr = run(t, "source", "diff", "/srv/stacks", "web/.env", "--output", "json")
	var diff struct {
		Diff      string
		Truncated bool
	}
	if err := json.Unmarshal([]byte(stdout), &diff); code != 0 || err != nil || diff.Diff != "-a\n+b\n" || !diff.Truncated {
		t.Fatalf("diff: code %d, stdout %q, stderr %q, err %v", code, stdout, stderr, err)
	}

	code, stdout, stderr = run(t, "source", "discard", "/srv/stacks", "web/.env", "db/notes.txt", "--output", "json")
	var gone struct{ Discarded []string }
	if err := json.Unmarshal([]byte(stdout), &gone); code != 0 || err != nil || len(gone.Discarded) != 2 {
		t.Fatalf("discard: code %d, stdout %q, stderr %q, err %v", code, stdout, stderr, err)
	}
	if last := seen[len(seen)-1]; string(last) != `{"path":"/srv/stacks","files":["web/.env","db/notes.txt"]}` {
		t.Fatalf("discard arguments = %s", last)
	}
}

func TestSourceChangesCommandsRefuseBadArgumentsBeforeCallingTheService(t *testing.T) {
	var seen []json.RawMessage
	svc := startFakeService(t, withStatus(changesHandlers(&seen), []string{"/srv/stacks"}, "web"))
	for _, args := range [][]string{{"source", "discard", "/srv/stacks"}, {"source", "discard"}, {"source", "changes"},
		{"source", "changes", "a", "b"}, {"source", "diff", "/srv/stacks"}, {"source", "changes", "web"},
		{"source", "changes", "/srv/stacks/../x"}, {"source", "discard", "web", ".env"}} {
		if code, _, stderr := run(t, args...); code == 0 || stderr == "" {
			t.Errorf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
	if len(seen) != 0 || svc.count(control.SourceDiscardCommand) != 0 {
		t.Fatalf("the service was called: %v", seen)
	}
}

func TestSourceChangesReachesTheInstanceThatHasTheFolderBound(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	var a, s []json.RawMessage
	system := startFakeService(t, withStatus(changesHandlers(&s), []string{"/srv/system"}, "web"))
	alice := serveFake(t, eps["alice"], withStatus(changesHandlers(&a), []string{"/srv/alice"}, "demo"))

	if code, _, stderr := run(t, "source", "discard", "/srv/alice", "demo/.env"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.SourceDiscardCommand) != 1 || system.count(control.SourceDiscardCommand) != 0 {
		t.Fatal("discard did not reach only the instance that has the folder bound")
	}
	if code, _, _ := run(t, "source", "changes", "/srv/unknown"); code == 0 || alice.count(control.SourceChangesCommand) != 0 ||
		system.count(control.SourceChangesCommand) != 0 {
		t.Fatal("a folder no engine has was asked for")
	}
}

const exampleHead = "0123456789abcdef0123456789abcdef01234567"

func TestSourceCommitCallsTheServiceAndPrintsTheCommit(t *testing.T) {
	var seen []json.RawMessage
	startFakeService(t, withStatus(changesHandlers(&seen), []string{"/srv/stacks"}, "web"))

	code, stdout, stderr := run(t, "source", "commit", "/srv/stacks", "web/.env", "db/old.txt", "--head", exampleHead, "--output", "json")
	var res struct {
		Path, Commit string
		Committed    []string
	}
	if err := json.Unmarshal([]byte(stdout), &res); code != 0 || err != nil || res.Commit != strings.Repeat("c", 40) || len(res.Committed) != 2 {
		t.Fatalf("commit: code %d, stdout %q, stderr %q, err %v", code, stdout, stderr, err)
	}
	if want := `{"path":"/srv/stacks","files":["web/.env","db/old.txt"],"head":"` + exampleHead + `"}`; string(seen[len(seen)-1]) != want {
		t.Fatalf("commit arguments = %s", seen[len(seen)-1])
	}
	if code, stdout, _ := run(t, "source", "commit", "/srv/stacks", "web/.env", "--head", exampleHead); code != 0 ||
		!strings.Contains(stdout, strings.Repeat("c", 40)) || !strings.Contains(stdout, "db/old.txt") {
		t.Fatalf("plain commit: code %d, stdout %q", code, stdout)
	}
}

func TestSourceCommitRefusesBadArgumentsBeforeCallingTheService(t *testing.T) {
	var seen []json.RawMessage
	svc := startFakeService(t, withStatus(changesHandlers(&seen), []string{"/srv/stacks"}, "web"))
	for _, args := range [][]string{{"source", "commit", "/srv/stacks", "--head", exampleHead}, {"source", "commit", "--head", exampleHead},
		{"source", "commit", "/srv/stacks", "web/.env"}, {"source", "commit", "web", ".env", "--head", exampleHead},
		{"source", "commit", "/srv/stacks/../x", ".env", "--head", exampleHead}} {
		if code, _, stderr := run(t, args...); code == 0 || stderr == "" {
			t.Errorf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
	if svc.count(control.SourceCommitCommand) != 0 {
		t.Fatalf("the service was called: %v", seen)
	}
}

func TestSourceCommitReachesTheInstanceThatHasTheFolderBound(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	var a, s []json.RawMessage
	system := startFakeService(t, withStatus(changesHandlers(&s), []string{"/srv/system"}, "web"))
	alice := serveFake(t, eps["alice"], withStatus(changesHandlers(&a), []string{"/srv/alice"}, "demo"))

	if code, _, stderr := run(t, "source", "commit", "/srv/alice", "demo/.env", "--head", exampleHead); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.SourceCommitCommand) != 1 || system.count(control.SourceCommitCommand) != 0 {
		t.Fatal("commit did not reach only the instance that has the folder bound")
	}
}
