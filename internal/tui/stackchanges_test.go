package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The invented diff of demo-stack/.env; only the view of the diff shows its lines.
const (
	envDiff = "diff --git a/demo-stack/.env b/demo-stack/.env\nindex 1111111..2222222 100644\n--- a/demo-stack/.env\n" +
		"+++ b/demo-stack/.env\n@@ -1 +1 @@\n-APP_VERSION=1.4.2\n+APP_VERSION=1.5.0\n"
	envFile     = "demo-stack/.env"
	composeFile = "demo-stack/compose.yaml"
)

// changesBackend is the stack folder of stackFilesBackend with two uncommitted changes, as the service lists them:
// the folder holds a secrets.env too, which the service never lists.
func changesBackend() *fakeBackend {
	b := stackFilesBackend()
	b.changes = map[string][]control.SourceChange{stackDir: {
		{Path: envFile, Kind: control.ChangeModified, Stack: "demo-stack"},
		{Path: composeFile, Kind: control.ChangeModified, Stack: "demo-stack"}}}
	b.diffs = map[string]string{envFile: envDiff}
	return b
}

// changesModel opens the changes of the first stack folder from its row, as root or as the user ada (UID 1000)
// without root, whose commands the fake hands to sudo.
func changesModel(t *testing.T, b *fakeBackend, root bool) (*Model, *fakeRoot) {
	t.Helper()
	m := newTestModel(t, i18n.English, b, wideWidth, 40)
	uid := 1000
	if root {
		uid = 0
	}
	m.euid, m.human = func() int { return uid }, func() int { return 1000 }
	f := &fakeRoot{root: root}
	m.root = f.hook()
	send(m, "up")
	contains(t, m, "changes")
	send(m, "=")
	if m.shownChanges() == nil {
		t.Fatalf("= on the folder row did not open its changes:\n%s", m.render())
	}
	return m, f
}

// The list is the one of the service: the two changed files, never secrets.env, which the service leaves out.
func TestChangesListTheFilesOfTheService(t *testing.T) {
	m, _ := changesModel(t, changesBackend(), false)
	contains(t, m, "Uncommitted changes of "+stackDir, envFile, composeFile, "demo-stack")
	var rows []string
	for _, l := range strings.Split(m.render(), "\n") {
		if strings.Contains(l, "modified") {
			rows = append(rows, l)
		}
		if strings.Contains(l, "secrets.env") && strings.Contains(l, "demo-stack/") {
			t.Fatalf("secrets.env is listed:\n%s", m.render())
		}
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows, want 2:\n%s", len(rows), m.render())
	}
	send(m, "esc")
	if m.shownChanges() != nil || !m.stackRowsShown() {
		t.Fatalf("esc kept the changes:\n%s", m.render())
	}
	// A folder without changes has no view of them.
	send(m, "home", "=")
	if m.shownChanges() != nil || !strings.Contains(m.notice, "has no uncommitted changes") {
		t.Fatalf("= on a folder without changes: %q\n%s", m.notice, m.render())
	}
}

// Without root the diff comes through sudo after a question with the exact command, as root directly; it shows
// only in the text view.
func TestChangesDiffAsksForSudoWithoutRoot(t *testing.T) {
	b := changesBackend()
	m, f := changesModel(t, b, false)
	f.reports = func([]string) string {
		return jsonOf(t, control.SourceDiffResult{Path: stackDir, File: envFile, Diff: envDiff})
	}
	send(m, "enter")
	want := []string{"source", "diff", "--output", "json", "--", stackDir, envFile}
	containsFlat(t, m, "Show the diff of "+envFile+"?")
	showsCommand(t, m, want)
	if len(f.reqs) != 0 {
		t.Fatalf("something ran before y: %v", f.reqs)
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("commands %q, want %q", got, want)
	}
	contains(t, m, "-APP_VERSION=1.4.2", "+APP_VERSION=1.5.0", "@@ -1 +1 @@")
	if b.calls["sourcediff"] != 0 || strings.Contains(m.failure+m.notice+m.result, "APP_VERSION") {
		t.Fatalf("the service was called or a message quotes the diff: %v %q", b.calls, m.failure+m.notice+m.result)
	}
	send(m, "esc")
	if m.sf.changes.diff != nil || m.shownChanges() == nil {
		t.Fatalf("esc did not lead back to the list:\n%s", m.render())
	}

	root, rf := changesModel(t, changesBackend(), true)
	send(root, "enter")
	if root.modal != nil || len(rf.reqs) != 0 {
		t.Fatalf("root was asked or ran a command: %v\n%s", rf.reqs, root.render())
	}
	contains(t, root, "+APP_VERSION=1.5.0")
}

func TestDiffSpansMarkAddedAndRemovedLines(t *testing.T) {
	spans := diffSpans(envDiff + `\ No newline at end of file` + "\n")
	roles := make([]kittui.TextRole, len(spans))
	for i, s := range spans {
		if len(s) == 1 {
			roles[i] = s[0].Role
		}
	}
	want := []kittui.TextRole{kittui.TextRoleKey, kittui.TextRoleKey, kittui.TextRoleKey, kittui.TextRoleKey,
		kittui.TextRoleComment, kittui.TextRoleKeyword, kittui.TextRoleString, kittui.TextRoleComment}
	if !slices.Equal(roles, want) {
		t.Fatalf("roles %v, want %v", roles, want)
	}
}

// Without root the commit runs the exact command with every listed file and the head read with them; its result
// offers the runs. As root the service gets the same files, the head of the last refresh where the list of folders
// cannot be read, and the person.
func TestCommitRunsTheExactCommand(t *testing.T) {
	listed := changesBackend()
	listed.folderList = control.SourceListResult{Sources: []control.SourceStatus{{Path: stackDir, Head: commitB}}}
	m, f := changesModel(t, listed, false)
	f.reports = func([]string) string {
		return jsonOf(t, control.SourceCommitResult{Path: stackDir, Commit: commitB, Committed: []string{envFile, composeFile}})
	}
	send(m, ">")
	want := []string{"source", "commit", "--head", commitB, "--output", "json", "--", stackDir, envFile, composeFile}
	containsFlat(t, m, "Commit 2 files of "+stackDir+"?", "pushes the commit")
	showsCommand(t, m, want)
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("commands %q, want %q", got, want)
	}
	containsFlat(t, m, "Committed 2 files as "+short(commitB)+".", "show runs")
	send(m, "l")
	if m.area != areaRuns || m.modal != nil {
		t.Fatalf("l did not open the runs: area %s\n%s", m.area, m.render())
	}

	b := changesBackend()
	b.errs = map[string]error{"sourcelist:": errors.New("invented failure")}
	root, _ := changesModel(t, b, true)
	send(root, ">", "y")
	wantArgs := control.SourceCommitArgs{Path: stackDir, Files: []string{envFile, composeFile}, Head: commitA, SudoUID: 1000}
	if len(b.commits) != 1 || b.commits[0].Path != wantArgs.Path || !slices.Equal(b.commits[0].Files, wantArgs.Files) ||
		b.commits[0].Head != wantArgs.Head || b.commits[0].SudoUID != wantArgs.SudoUID {
		t.Fatalf("commits %+v, want %+v", b.commits, wantArgs)
	}
	containsFlat(t, root, "Committed 2 files")
	send(root, "esc")
	contains(t, root, "No uncommitted changes.")
}

// Every refusal of a commit says in words why nothing was committed, from the service as from the command.
func TestCommitRefusalsAreShownInWords(t *testing.T) {
	b := changesBackend()
	m, _ := changesModel(t, b, true)
	for code, words := range map[string]string{
		control.CodeSecrets:      "would push secret-like files",
		control.CodePushRejected: "the remote refused the push",
		control.CodeHeadMoved:    "the folder moved on since the list was read",
		control.CodeNotChanged:   "the list no longer matches the changes",
		control.CodeUnreadable:   "a stack of the files no longer loads",
		control.CodeRunActive:    "a run of a stack of the files is under way",
	} {
		b.errs = map[string]error{"sourcecommit:" + stackDir: &control.Error{Code: code, Message: "invented reason " + code}}
		send(m, ">", "y")
		containsFlat(t, m, "Commit refused", "Nothing was committed", words, "invented reason "+code)
		send(m, "enter")
		if m.modal != nil || m.shownChanges() == nil {
			t.Fatalf("%s: the refusal did not lead back to the list:\n%s", code, m.render())
		}
	}

	sudo, f := changesModel(t, changesBackend(), false)
	f.runErr = errors.New("exit status 1")
	for code, words := range map[string]string{"secrets-found": "would push secret-like files", "config-invalid": "no longer loads"} {
		f.stderr = "qhost: " + code + ": invented reason of the command\n"
		send(sudo, ">", "y")
		containsFlat(t, sudo, "Nothing was committed", words, "invented reason of the command")
		send(sudo, "enter")
	}
}

// A discard asks for one file at a time with the exact command; a new file is named as deleted.
func TestDiscardAsksForEachFile(t *testing.T) {
	added := control.SourceChange{Path: "demo-stack/notes.txt", Kind: control.ChangeNew, Stack: "demo-stack"}
	b := changesBackend()
	b.changes[stackDir] = append(b.changes[stackDir], added)
	m, f := changesModel(t, b, false)
	send(m, "d")
	containsFlat(t, m, "Discard the changes of "+envFile+"?", "cannot be brought back")
	showsCommand(t, m, []string{"source", "discard", "--yes", "--output", "json", "--", stackDir, envFile})
	send(m, "n", "end", "d")
	containsFlat(t, m, "Delete the new file "+added.Path+"?", "deleting it cannot be undone")
	f.reports = func([]string) string {
		return jsonOf(t, control.SourceDiscardResult{Path: stackDir, Discarded: []string{added.Path}})
	}
	send(m, "y")
	want := []string{"source", "discard", "--yes", "--output", "json", "--", stackDir, added.Path}
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("commands %q, want %q", got, want)
	}
	if !strings.Contains(m.result, "Deleted the new file "+added.Path) {
		t.Fatalf("result %q", m.result)
	}

	rb := changesBackend()
	root, _ := changesModel(t, rb, true)
	send(root, "d", "y")
	if len(rb.discards) != 1 || rb.discards[0].Path != stackDir || !slices.Equal(rb.discards[0].Files, []string{envFile}) {
		t.Fatalf("discards %+v", rb.discards)
	}
	if got := root.sf.changes.res.Changes; len(got) != 1 || got[0].Path != composeFile {
		t.Fatalf("listed %+v after the discard:\n%s", got, root.render())
	}
}

// The note of the browser over the folder opens the changes on a click, and esc leads back to the browser; a click
// on a button acts as its key.
func TestChangesOpenFromTheBrowserAndByClick(t *testing.T) {
	m := newTestModel(t, i18n.English, changesBackend(), wideWidth, 40)
	m.euid, m.mouseOn = func() int { return 1000 }, true
	fakeStackFiles(m)
	send(m, "up", "Q")
	contains(t, m, "= changes")
	clickOn(t, m, "This folder has uncommitted changes")
	if m.shownChanges() == nil {
		t.Fatalf("a click on the note in the browser did not open the changes:\n%s", m.render())
	}
	clickOn(t, m, "[ commit ]")
	containsFlat(t, m, "Commit 2 files of "+stackDir+"?")
	send(m, "n", "esc")
	if m.shownChanges() != nil || m.shownStackFiles() == nil {
		t.Fatalf("esc did not lead back to the browser:\n%s", m.render())
	}
}
