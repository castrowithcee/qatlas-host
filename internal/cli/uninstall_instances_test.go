package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// addInstances sets up the instances of users below root with everything the setup and the service leave: the
// rule, the template, the channel, the state, the configuration and a drop-in directory. The users have a home
// with a rootless engine, which the removal must not touch.
func addInstances(t *testing.T, root string, users ...string) {
	t.Helper()
	setUpInstances(t, root, users...)
	file := func(p string) {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("invented"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	file(deploysetup.TemplatePath())
	for _, u := range users {
		inst := instanceOf(t, u)
		file(inst.ChannelDir() + "/control.sock")
		file(inst.StateDir() + "/credentials")
		file(inst.ConfigDir() + "/settings")
		file(inst.StackFoldersDropIn())
		file("home/" + u + "/.config/systemd/user/docker.service")
		file("home/" + u + "/data/keep.txt")
	}
}

func TestUninstallRemovesEveryInstanceAndKeepsUsersAndHomes(t *testing.T) {
	root, ran := useUninstallRoot(t, 0)
	addInstances(t, root, "alice", "bob")
	code, stdout, stderr := run(t, "uninstall", "--confirm", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, u := range []string{"alice", "bob"} {
		inst := instanceOf(t, u)
		for _, want := range []string{"systemctl disable --now " + inst.Unit(), "rm -r " + inst.ChannelDir(), "rm " + inst.TmpfilesConf(), "rm " + inst.StackFoldersDropIn(), inst.Unit()} {
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr lacks %q: %q", want, stderr)
			}
		}
		for _, p := range []string{inst.TmpfilesConf(), inst.ChannelDir(), inst.StackFoldersDropIn(), inst.DropInDir()} {
			if present(root, p) {
				t.Errorf("%s stays", p)
			}
		}
		for _, p := range []string{inst.StateDir() + "/credentials", inst.ConfigDir() + "/settings",
			"/home/" + u + "/.config/systemd/user/docker.service", "/home/" + u + "/data/keep.txt"} {
			if !present(root, p) {
				t.Errorf("%s was removed", p)
			}
		}
	}
	if present(root, deploysetup.TemplatePath()) {
		t.Error("the template stays")
	}
	if !strings.Contains(stdout, `"changed"`) || strings.Contains(strings.Join(*ran, "\n"), "userdel") {
		t.Errorf("stdout %q, ran %v", stdout, *ran)
	}
	// An instance is stopped before its rule goes, and the template goes after the last instance.
	text := stderr
	for _, u := range []string{"alice", "bob"} {
		inst := instanceOf(t, u)
		if strings.Index(text, "disable --now "+inst.Unit()) > strings.Index(text, "rm "+inst.TmpfilesConf()) ||
			strings.Index(text, "rm "+deploysetup.TemplatePath()) < strings.Index(text, "rm "+inst.TmpfilesConf()) {
			t.Errorf("order of the steps for %s: %q", u, text)
		}
	}
	*ran = nil
	code, stdout, _ = run(t, "uninstall", "--confirm", "--yes", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"changed":false`) && !strings.Contains(stdout, `"changed": false`) || len(*ran) != 0 {
		t.Fatalf("repeat: code %d, stdout %q, ran %v", code, stdout, *ran)
	}
}

func TestUninstallPurgeRemovesInstanceStateAndConfigurationOnly(t *testing.T) {
	root, ran := useUninstallRoot(t, 0)
	addInstances(t, root, "alice", "bob")
	code, _, stderr := run(t, "uninstall", "--purge", "--confirm", "--yes")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, u := range []string{"alice", "bob"} {
		inst := instanceOf(t, u)
		for _, p := range []string{inst.StateDir(), inst.ConfigDir()} {
			if present(root, p) {
				t.Errorf("%s stays", p)
			}
			if !strings.Contains(stderr, "rm -r "+p) {
				t.Errorf("stderr lacks the removal of %s: %q", p, stderr)
			}
		}
		for _, p := range []string{"/home/" + u + "/.config/systemd/user/docker.service", "/home/" + u + "/data/keep.txt"} {
			if !present(root, p) {
				t.Errorf("%s was removed", p)
			}
		}
	}
	if got := strings.Join(*ran, "\n"); strings.Contains(got, "userdel alice") || strings.Contains(got, "userdel bob") {
		t.Errorf("ran %v", *ran)
	}
}

func TestUninstallPurgeAfterAnUninstallRemovesInstanceStateAndConfiguration(t *testing.T) {
	root, _ := useUninstallRoot(t, 0)
	addInstances(t, root, "alice", "bob")
	if code, _, stderr := run(t, "uninstall", "--confirm", "--yes"); code != 0 {
		t.Fatalf("uninstall: code %d, stderr %q", code, stderr)
	}
	code, _, stderr := run(t, "uninstall", "--purge", "--confirm", "--yes")
	if code != 0 {
		t.Fatalf("purge: code %d, stderr %q", code, stderr)
	}
	for _, u := range []string{"alice", "bob"} {
		inst := instanceOf(t, u)
		for _, p := range []string{inst.StateDir(), inst.ConfigDir()} {
			if present(root, p) || !strings.Contains(stderr, "rm -r "+p) {
				t.Errorf("%s: present %v, stderr %q", p, present(root, p), stderr)
			}
		}
		if strings.Contains(stderr, "disable --now "+inst.Unit()) || !strings.Contains(stderr, inst.Unit()) || !strings.Contains(stderr, "removed earlier") {
			t.Errorf("the purge of %s: %q", u, stderr)
		}
		for _, p := range []string{"/home/" + u + "/.config/systemd/user/docker.service", "/home/" + u + "/data/keep.txt"} {
			if !present(root, p) {
				t.Errorf("%s was removed", p)
			}
		}
	}
}

func TestUninstallKeepsAnAdministratorsFileInAnInstanceDropIn(t *testing.T) {
	root, _ := useUninstallRoot(t, 0)
	addInstances(t, root, "alice")
	mine := filepath.Join(root, instanceOf(t, "alice").DropInDir(), "50-mine.conf")
	if err := os.WriteFile(mine, []byte("[Service]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run(t, "uninstall", "--confirm", "--yes"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(mine); err != nil {
		t.Errorf("the drop-in of the administrator was removed: %v", err)
	}
}

func TestUninstallPurgeRefusesWhenAStackFolderLiesInAnInstanceState(t *testing.T) {
	root, ran := useUninstallRoot(t, 0)
	addInstances(t, root, "alice")
	defer func() {
		if !present(root, instanceOf(t, "alice").StateDir()+"/credentials") {
			t.Error("the state was removed")
		}
	}()
	binding := `{"version":1,"url":"git@example.invalid:stacks.git","dir":"` + instanceOf(t, "alice").StateDir() + `/stacks"}`
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir(), "stacks-repo.json"), []byte(binding), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, "uninstall", "--purge", "--confirm", "--yes")
	if code == 0 || !strings.Contains(stderr, "stack folder") || len(*ran) != 0 {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
}
