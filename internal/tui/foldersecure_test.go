package tui

import (
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
)

const gitUnsafeReason = ".git is owned by qhost, not by root"

// unsafeFolderBackend is the folders host with the finding git-unsafe on the clone.
func unsafeFolderBackend() *fakeBackend {
	b := foldersBackend()
	b.status.Engine.Sources[0].GitUnsafe = gitUnsafeReason
	b.host = &fakeFolders{}
	return b
}

func TestFolderWithUnsafeGitShowsTheReason(t *testing.T) {
	m := folderModel(t, unsafeFolderBackend())
	onFolder(t, m, stackDir)
	// The row is cut at the width of its column; the detail has the whole reason.
	contains(t, m, "git not run: .git is owned by", "Git is not run in this folder: "+gitUnsafeReason, "secure")
}

// Without root the action asks with the command and, on y, runs `sudo qhost source secure` with the path as an
// argument of its own; the status is read again after it.
func TestSecureWithoutRootRunsTheCommandAndReadsTheStatusAgain(t *testing.T) {
	b := unsafeFolderBackend()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	f.report = `{"path":"` + stackDir + `","changed":4}`
	onFolder(t, m, stackDir)
	before := b.called("status")
	send(m, "&")
	says(t, m, "Make the .git of "+stackDir+" root-only?", "sudo qhost source secure --yes --output json -- "+stackDir)
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	wantCommands(t, m, f, []string{"source", "secure", "--yes", "--output", "json", "--", stackDir})
	if len(b.host.calls) != 0 || b.called("fetch") != 0 {
		t.Fatalf("the host or the service was used directly: %v", b.host.calls)
	}
	if b.called("status") <= before {
		t.Fatal("the status was not read again")
	}
	contains(t, m, "The .git of "+stackDir+" is root-only now.")
}

// As root the host side secures the folder directly, and the status is read again.
func TestSecureAsRootCallsTheHostSideAndReadsTheStatusAgain(t *testing.T) {
	b := unsafeFolderBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, 40)
	m.euid = func() int { return 0 }
	onFolder(t, m, stackDir)
	before := b.called("status")
	send(m, "&", "y")
	if !slices.Equal(b.host.calls, []string{"secure:" + stackDir}) {
		t.Fatalf("host calls %v", b.host.calls)
	}
	if b.called("status") <= before {
		t.Fatal("the status was not read again")
	}
	contains(t, m, "The .git of "+stackDir+" is root-only now.")
}

func TestSecureOnAFolderWithoutTheFindingOnlySays(t *testing.T) {
	b := foldersBackend()
	b.host = &fakeFolders{}
	m := newTestModel(t, i18n.English, b, wideWidth, 40)
	m.euid = func() int { return 0 }
	onFolder(t, m, stackDir)
	send(m, "&")
	contains(t, m, "needs no securing")
	if len(b.host.calls) != 0 {
		t.Fatalf("host calls %v", b.host.calls)
	}
}

func TestGoldenStacksFolderGitUnsafe(t *testing.T) {
	goldenScreens(t, "stacks.folder.gitunsafe", settingsRows, func() *fakeBackend {
		b := foldersBackend()
		b.status.Engine.Sources[0].GitUnsafe = gitUnsafeReason
		return b
	}, seq(toFolders, keys("up"))...)
}
