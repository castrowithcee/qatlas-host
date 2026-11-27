package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/uninstall"
)

// useUninstallRoot makes an installed host below a temporary root, with a runner that only records its
// commands, and a state directory without bound folders. Nothing of the real host is involved.
func useUninstallRoot(t *testing.T, euid int) (string, *[]string) {
	t.Helper()
	root := t.TempDir()
	for _, p := range []string{
		"usr/local/bin/qhost", "usr/local/lib/systemd/system/qhost.service", "usr/local/lib/systemd/system/qhost-deploy.service",
		"usr/local/share/man/man1/qhost.1", "usr/local/share/man/man1/qhost-stack-list.1", "etc/tmpfiles.d/qhost-deploy.conf",
		"run/qhost-deploy/control.sock", "var/lib/qhost/a", "var/lib/qhost-deploy/b", "var/lib/qhost-host/journal/c",
		"srv/stacks/app/compose.yaml",
	} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("invented"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, content string) {
		if err := os.WriteFile(filepath.Join(root, p), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("etc/passwd", "root:x:0:0::/root:/bin/sh\nqhost:x:997:997::/x:/bin/false\nqhost-deploy:x:996:996::/x:/bin/false\n")
	write("etc/group", "root:x:0:\nqhost:x:997:\nqhost-deploy:x:996:\n")
	useStateDir(t)
	old := uninstaller
	var mu sync.Mutex
	var ran []string
	uninstaller = uninstallEnv{euid: func() int { return euid }, host: uninstall.Host{Root: root,
		Run: func(_ context.Context, name string, args ...string) error {
			mu.Lock()
			defer mu.Unlock()
			ran = append(ran, name+" "+strings.Join(args, " "))
			if file := map[string]string{"userdel": "etc/passwd", "groupdel": "etc/group"}[name]; file != "" {
				dropAccount(t, root, file, args[0])
			}
			return nil
		}}}
	t.Cleanup(func() { uninstaller = old })
	return root, &ran
}

// dropAccount deletes the line of an account from the user or group file, like userdel and groupdel.
func dropAccount(t *testing.T, root, file, name string) {
	t.Helper()
	full := filepath.Join(root, file)
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	var keep []string
	for _, l := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(l, name+":") {
			keep = append(keep, l)
		}
	}
	if err := os.WriteFile(full, []byte(strings.Join(keep, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// onlyDeployAccountRemoved reports whether no user or group is removed except qhost-deploy, which a removal
// without --purge deletes because the deploy service runs as root.
func onlyDeployAccountRemoved(ran []string) bool {
	for _, c := range ran {
		if (strings.HasPrefix(c, "userdel ") || strings.HasPrefix(c, "groupdel ")) && !strings.Contains(c, " qhost-deploy") {
			return false
		}
	}
	return true
}

func present(root, p string) bool {
	_, err := os.Lstat(filepath.Join(root, p))
	return err == nil
}

func TestUninstallNeedsRoot(t *testing.T) {
	root, ran := useUninstallRoot(t, 1000)
	code, _, stderr := run(t, "uninstall", "--confirm", "--yes")
	if code == 0 || !strings.Contains(stderr, "needs root") || len(*ran) != 0 || !present(root, "usr/local/bin/qhost") {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
}

func TestUninstallWithoutConfirmChangesNothing(t *testing.T) {
	root, ran := useUninstallRoot(t, 0)
	for _, args := range [][]string{{"uninstall"}, {"uninstall", "--yes"}, {"uninstall", "--purge"}} {
		code, stdout, stderr := run(t, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "--confirm") || len(*ran) != 0 || !present(root, "usr/local/bin/qhost") {
			t.Fatalf("%v: code %d, stdout %q, stderr %q, ran %v", args, code, stdout, stderr, *ran)
		}
		for _, want := range []string{"Remove qhost", "rm /usr/local/bin/qhost"} {
			if strings.Count(stderr, want) != 1 {
				t.Errorf("%v: stderr should show %q once: %q", args, want, stderr)
			}
		}
	}
}

func TestUninstallWithoutAnswerChangesNothing(t *testing.T) {
	root, ran := useUninstallRoot(t, 0)
	code, _, _ := run(t, "uninstall", "--confirm")
	if code == 0 || len(*ran) != 0 || !present(root, "usr/local/bin/qhost") {
		t.Fatalf("code %d, ran %v", code, *ran)
	}
}

func TestUninstallShowsStepsAndKeepsStateAndStacks(t *testing.T) {
	root, ran := useUninstallRoot(t, 0)
	code, stdout, stderr := run(t, "uninstall", "--confirm", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"Remove qhost", "stay", "systemctl disable --now qhost-deploy", "rm /usr/local/bin/qhost", "not touched"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q: %q", want, stderr)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got["changed"] != true || got["purged"] != false {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	for _, p := range []string{"usr/local/bin/qhost", "usr/local/lib/systemd/system/qhost.service", "usr/local/lib/systemd/system/qhost-deploy.service",
		"usr/local/share/man/man1/qhost.1", "usr/local/share/man/man1/qhost-stack-list.1", "etc/tmpfiles.d/qhost-deploy.conf", "run/qhost-deploy"} {
		if present(root, p) {
			t.Errorf("%s stays", p)
		}
	}
	for _, p := range []string{"var/lib/qhost/a", "var/lib/qhost-deploy/b", "var/lib/qhost-host/journal/c", "srv/stacks/app/compose.yaml"} {
		if !present(root, p) {
			t.Errorf("%s was removed", p)
		}
	}
	if !onlyDeployAccountRemoved(*ran) {
		t.Errorf("ran %v", *ran)
	}
	*ran = nil
	code, stdout, _ = run(t, "uninstall", "--confirm", "--yes", "--output", "json")
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil || got["changed"] != false || len(*ran) != 0 {
		t.Fatalf("repeat: code %d, stdout %q, ran %v", code, stdout, *ran)
	}
}

func TestUninstallPurgeRemovesStateUsersAndGroup(t *testing.T) {
	root, ran := useUninstallRoot(t, 0)
	code, stdout, stderr := run(t, "uninstall", "--purge", "--confirm", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"cannot be restored", "rm -r /var/lib/qhost-deploy", "userdel qhost", "groupdel qhost"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q: %q", want, stderr)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got["purged"] != true {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	for _, p := range []string{"var/lib/qhost", "var/lib/qhost-deploy", "var/lib/qhost-host"} {
		if present(root, p) {
			t.Errorf("%s stays", p)
		}
	}
	if !present(root, "srv/stacks/app/compose.yaml") {
		t.Error("the stack folder was touched")
	}
	if got := strings.Join(*ran, "\n"); !strings.Contains(got, "userdel qhost-deploy") || !strings.Contains(got, "groupdel qhost\n") && !strings.HasSuffix(got, "groupdel qhost") {
		t.Errorf("ran %v", *ran)
	}
}

func TestUninstallPurgeRefusesWhenAStackFolderLiesInTheState(t *testing.T) {
	root, ran := useUninstallRoot(t, 0)
	binding := `{"version":1,"url":"git@example.invalid:stacks.git","dir":"/var/lib/qhost-deploy/stacks"}`
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir(), "stacks-repo.json"), []byte(binding), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, "uninstall", "--purge", "--confirm", "--yes")
	if code == 0 || !strings.Contains(stderr, "stack folder") || len(*ran) != 0 || !present(root, "usr/local/bin/qhost") ||
		!present(root, "var/lib/qhost-deploy/b") {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
	// Without --purge the state is not touched, so the same binding does not stop the removal.
	if code, _, stderr := run(t, "uninstall", "--confirm", "--yes"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

// The manpages the program lists for removal are exactly the pages the release build writes.
func TestManpageNamesMatchTheGeneratedPages(t *testing.T) {
	dir := t.TempDir()
	if code, _, stderr := run(t, "manpage", "--dir", dir); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, e := range entries {
		want = append(want, e.Name())
	}
	root, _, _, err := newRoot()
	if err != nil {
		t.Fatal(err)
	}
	got := manpageNames(root)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("pages\n got %v\nwant %v", got, want)
	}
}
