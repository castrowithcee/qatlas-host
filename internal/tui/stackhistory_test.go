package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The invented diff of commit commitB; only the view of the diff shows its lines.
const historyDiff = "diff --git a/demo-stack/compose.yaml b/demo-stack/compose.yaml\nindex 1111111..2222222 100644\n" +
	"--- a/demo-stack/compose.yaml\n+++ b/demo-stack/compose.yaml\n@@ -1 +1 @@\n-image: example.invalid/app:1.4.2\n" +
	"+image: example.invalid/app:1.5.0\n"

// historyBackend is the stack folder of stackFilesBackend with two commits, as source.log lists them.
func historyBackend() *fakeBackend {
	b := stackFilesBackend()
	b.fakeHistory = fakeHistory{
		commitLogs: map[string]control.SourceLogResult{stackDir: {Commits: []control.SourceLogCommit{
			{ID: commitB, Author: "Ada Example", Date: "2026-11-02T09:30:00Z", Subject: "Update the app image", Paths: []control.SourceLogPath{
				{Path: composeFile, Kind: control.ChangeModified}, {Path: "demo-stack/notes.txt", Kind: control.ChangeNew}}},
			{ID: commitA, Author: "Ben Example", Date: "2026-10-30T16:05:00Z", Subject: "Add the demo stack", Paths: []control.SourceLogPath{
				{Path: composeFile, Kind: control.ChangeNew}, {Path: "demo-stack/old.txt", Kind: control.ChangeDeleted}}, PathsTruncated: true}}}},
		shows: map[string]string{commitB: historyDiff},
	}
	return b
}

// historyModel opens the history of the first stack folder from its row, as root or as the user ada (UID 1000).
func historyModel(t *testing.T, b *fakeBackend, root bool) (*Model, *fakeRoot) {
	t.Helper()
	m := newTestModel(t, i18n.English, b, wideWidth, 40)
	uid := 1000
	if root {
		uid = 0
	}
	m.euid, m.human = func() int { return uid }, func() int { return 1000 }
	f := &fakeRoot{root: root}
	m.root = f.hook()
	openHistoryOfRow(m)
	if m.shownHistory() == nil {
		t.Fatalf("the history of the folder row did not open:\n%s", m.render())
	}
	return m, f
}

// openHistoryOfRow opens the history of the first stack folder through the entry the key of the Stacks area will
// call.
func openHistoryOfRow(m *Model) {
	send(m, "up", "@")
	m.render()
}

// @ in the browser of the folder opens the history, and esc leads back to the browser.
func TestHistoryOpensFromTheBrowser(t *testing.T) {
	m := newTestModel(t, i18n.English, historyBackend(), wideWidth, 40)
	m.euid, m.mouseOn = func() int { return 1000 }, true
	fakeStackFiles(m)
	send(m, "up", "Q")
	contains(t, m, "@ history")
	send(m, "@")
	if m.shownHistory() == nil {
		t.Fatalf("@ in the browser did not open the history:\n%s", m.render())
	}
	send(m, "esc")
	if m.shownHistory() != nil || m.shownStackFiles() == nil {
		t.Fatalf("esc did not lead back to the browser:\n%s", m.render())
	}
}

// The list shows date, author and subject; enter shows the files with their kind, esc goes one level back and from
// the list to the folder row.
func TestHistoryListsCommitsAndFiles(t *testing.T) {
	m, _ := historyModel(t, historyBackend(), false)
	contains(t, m, "History of "+stackDir, "2026-11-02", "Ada Example", "Update the app image", "Add the demo stack")
	send(m, "enter")
	contains(t, m, "Files of commit "+short(commitB), composeFile, "modified", "demo-stack/notes.txt", "new")
	if strings.Contains(m.render(), "More files changed") {
		t.Fatalf("a complete file list says it is cut:\n%s", m.render())
	}
	send(m, "esc")
	send(m, "down", "enter")
	contains(t, m, "Files of commit "+short(commitA), "deleted", "More files changed than are listed.")
	send(m, "esc", "esc")
	if m.shownHistory() != nil || !m.stackRowsShown() {
		t.Fatalf("esc kept the history:\n%s", m.render())
	}
}

func TestHistoryCutIsSaid(t *testing.T) {
	b := historyBackend()
	r := b.commitLogs[stackDir]
	r.Truncated = true
	b.commitLogs[stackDir] = r
	m, _ := historyModel(t, b, false)
	contains(t, m, "The history is cut: older commits are not listed.")

	empty, _ := historyModel(t, historyBackend(), false)
	empty.sf.history.res, empty.sf.history.read.loaded = control.SourceLogResult{}, true
	empty.fillHistory()
	if strings.Contains(empty.render(), "history is cut") {
		t.Fatalf("an uncut history says it is cut:\n%s", empty.render())
	}
}

// Without root the diff comes through sudo after a question with the exact command, with path and commit; as root
// directly over the service.
func TestHistoryDiffAsksForSudoWithoutRoot(t *testing.T) {
	b := historyBackend()
	m, f := historyModel(t, b, false)
	f.reports = func([]string) string {
		return jsonOf(t, control.SourceShowResult{Path: stackDir, Commit: commitB, Diff: historyDiff})
	}
	send(m, "<")
	want := []string{"source", "show", "--output", "json", "--", stackDir, commitB}
	containsFlat(t, m, "Show the diff of commit "+short(commitB)+"?")
	showsCommand(t, m, want)
	if len(f.reqs) != 0 {
		t.Fatalf("something ran before y: %v", f.reqs)
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("commands %q, want %q", got, want)
	}
	contains(t, m, "-image: example.invalid/app:1.4.2", "+image: example.invalid/app:1.5.0")
	if b.calls["sourceshow"] != 0 || strings.Contains(m.failure+m.notice+m.result, "image:") {
		t.Fatalf("the service was called or a message quotes the diff: %v %q", b.calls, m.failure+m.notice+m.result)
	}
	send(m, "esc")
	if m.sf.history.diff != nil || m.shownHistory() == nil {
		t.Fatalf("esc did not lead back to the list:\n%s", m.render())
	}

	root, rf := historyModel(t, historyBackend(), true)
	send(root, "enter", "<")
	if root.modal != nil || len(rf.reqs) != 0 {
		t.Fatalf("root was asked or ran a command: %v\n%s", rf.reqs, root.render())
	}
	contains(t, root, "+image: example.invalid/app:1.5.0")
	send(root, "esc")
	contains(t, root, "Files of commit "+short(commitB))
}
