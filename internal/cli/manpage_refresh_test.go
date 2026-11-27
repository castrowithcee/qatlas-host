package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const setupPage = "qhost-deploy-service-setup.1"

// useMigrateHost lets 'update migrate' run as root below a temporary root with one invented transition, and
// keeps the manpage work files in a temporary folder.
func useMigrateHost(t *testing.T) string {
	t.Helper()
	useTestTransition(t, nil)
	t.Setenv("TMPDIR", t.TempDir())
	root := t.TempDir()
	old := selfUpdate
	selfUpdate = selfUpdateHost{euid: func() int { return 0 }, root: root}
	t.Cleanup(func() { selfUpdate = old })
	return root
}

func manDirOf(root string) string { return filepath.Join(root, "usr/local/share/man/man1") }

func TestMigrateWritesTheDifferingManpagesAndARepeatWritesNothing(t *testing.T) {
	root := useMigrateHost(t)
	man := manDirOf(root)
	writeTestFile(t, filepath.Join(man, setupPage), ".TH OLD\nstale text\n")
	if code, _, stderr := run(t, "update", "migrate"); code != 0 || !strings.Contains(stderr, "manpages: updated ") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	got, _ := os.ReadFile(filepath.Join(man, setupPage))
	if strings.Contains(string(got), "stale") || !strings.Contains(string(got), ".TH") {
		t.Fatalf("page not replaced: %q", got)
	}
	if _, err := os.Stat(filepath.Join(man, "qhost.1")); err != nil {
		t.Fatalf("a page the host lacked was not written: %v", err)
	}
	// A different date in the header alone is no difference.
	data, _ := os.ReadFile(filepath.Join(man, "qhost.1"))
	dated := regexp.MustCompile(`(?m)^\.TH .*$`).ReplaceAllString(string(data), `.TH "QHOST" "1" "Jan 2001" "" ""`)
	if dated == string(data) {
		t.Fatal("the page has no header line")
	}
	writeTestFile(t, filepath.Join(man, "qhost.1"), dated)
	if code, _, stderr := run(t, "update", "migrate"); code != 0 || strings.Contains(stderr, "manpages:") || strings.Contains(stderr, "warning") {
		t.Fatalf("repeat: code %d, stderr %q", code, stderr)
	}
	if again, _ := os.ReadFile(filepath.Join(man, "qhost.1")); string(again) != dated {
		t.Fatal("a page that differs only in its header date was rewritten")
	}
}

func TestMigrateLeavesALinkInPlaceOfAPageAndAMissingFolderAlone(t *testing.T) {
	root := useMigrateHost(t)
	if code, _, stderr := run(t, "update", "migrate"); code != 0 || stderr != "transition invented-marker: applied\n" {
		t.Fatalf("no man1: code %d, stderr %q", code, stderr)
	}
	if _, err := os.Lstat(manDirOf(root)); !os.IsNotExist(err) {
		t.Fatalf("the manpage folder was created: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "foreign")
	writeTestFile(t, outside, "foreign\n")
	if err := os.MkdirAll(manDirOf(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(manDirOf(root), setupPage)); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, "update", "migrate")
	if code != 0 || !strings.Contains(stderr, "warning") || !strings.Contains(stderr, setupPage) {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if got, _ := os.ReadFile(outside); string(got) != "foreign\n" {
		t.Fatalf("the link target was written: %q", got)
	}
	if info, err := os.Lstat(filepath.Join(manDirOf(root), setupPage)); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was replaced: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manDirOf(root), "qhost.1")); err != nil {
		t.Fatalf("the other pages were not written: %v", err)
	}
}

func TestDeployServicePrepareRenewsManpagesAndAWriteFailureDoesNotStopIt(t *testing.T) {
	root, _, _ := useOldDeployHost(t, 0)
	t.Setenv("TMPDIR", t.TempDir())
	man := manDirOf(root)
	writeTestFile(t, filepath.Join(man, setupPage), ".TH OLD\nstale text\n")
	if code, _, stderr := run(t, "deploy-service", "prepare"); code != 0 || !strings.Contains(stderr, "manpages: updated ") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if got, _ := os.ReadFile(filepath.Join(man, setupPage)); strings.Contains(string(got), "stale") {
		t.Fatalf("page not replaced: %q", got)
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write a read-only folder")
	}
	writeTestFile(t, filepath.Join(man, setupPage), ".TH OLD\nstale text\n")
	if err := os.Chmod(man, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(man, 0o755) })
	code, _, stderr := run(t, "deploy-service", "prepare")
	if code != 0 || !strings.Contains(stderr, "warning: the manpages are not all up to date") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}
