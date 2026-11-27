package deploysetup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/packaging"
)

const oldPasswd = basePasswd + "qhost-deploy:x:996:996::/var/lib/qhost-deploy:/usr/sbin/nologin\n"
const oldGroups = "root:x:0:\ndocker:x:998:alice,qhost-deploy\nqhost:x:997:\nqhost-deploy:x:996:\n"

// oldHost is a host with the deploy service in the setup of earlier releases; the runner removes the user and
// group from the database like userdel would, and Chown records instead of changing owners.
func oldHost(t *testing.T) (*fakeHost, *[]string) {
	t.Helper()
	f := newFakeHost(t, oldGroups, oldPasswd)
	f.put(UnitPath(), string(packaging.DeployUnit))
	f.put("/etc/tmpfiles.d/qhost-deploy.conf", "d /run/qhost-deploy 2750 qhost-deploy qhost -\n")
	f.put("/var/lib/qhost-deploy/runs/one.json", "{}\n")
	var chowns []string
	f.host.Chown = func(p string, uid, gid int) error {
		rel, _ := filepath.Rel(f.root, p)
		chowns = append(chowns, "/"+filepath.ToSlash(rel)+" "+itoa(uid)+":"+itoa(gid))
		return nil
	}
	f.host.Run = func(ctx context.Context, name string, args ...string) error {
		line := name + " " + strings.Join(args, " ")
		if f.failOn != "" && strings.HasPrefix(line, f.failOn) {
			return errors.New("failed")
		}
		switch line {
		case "userdel qhost-deploy":
			f.put("/etc/passwd", basePasswd)
			f.put("/etc/group", "root:x:0:\ndocker:x:998:alice\nqhost:x:997:\n")
		case "groupdel qhost-deploy":
			t.Fatal("groupdel ran although userdel removed the group")
		}
		f.calls = append(f.calls, line)
		return nil
	}
	return f, &chowns
}

func moveToRoot(f *fakeHost) (string, error) {
	var report bytes.Buffer
	err := f.host.MoveToRoot(context.Background(), &report)
	return report.String(), err
}

func TestMoveToRootConvertsTheOldSetupInOrderAndARepeatChangesNothing(t *testing.T) {
	f, chowns := oldHost(t)
	if old, err := f.host.HasAccountSetup(); err != nil || !old {
		t.Fatalf("old setup not found: %v %v", old, err)
	}
	report, err := moveToRoot(f)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"systemd-tmpfiles --create /etc/tmpfiles.d/qhost-deploy.conf",
		"systemctl daemon-reload",
		"systemctl try-restart qhost-deploy",
		"userdel qhost-deploy",
	}
	if !slices.Equal(f.calls, want) {
		t.Fatalf("commands:\n%v\nwant\n%v", f.calls, want)
	}
	if got := f.read("/etc/tmpfiles.d/qhost-deploy.conf"); got != "d /run/qhost-deploy 2750 root qhost -\n" {
		t.Fatalf("rule: %q", got)
	}
	if wantOwn := []string{"/var/lib/qhost-deploy 0:0", "/var/lib/qhost-deploy/runs 0:0", "/var/lib/qhost-deploy/runs/one.json 0:0"}; !slices.Equal(*chowns, wantOwn) {
		t.Fatalf("chowns %v, want %v", *chowns, wantOwn)
	}
	if strings.Count(report, "\n") != 7 || !strings.Contains(report, "deploy service to root: userdel qhost-deploy\n") {
		t.Fatalf("report %q", report)
	}
	if old, err := f.host.HasAccountSetup(); err != nil || old {
		t.Fatalf("still old after the move: %v %v", old, err)
	}
	if got := f.read("/etc/passwd") + f.read("/etc/group"); strings.Contains(got, "qhost-deploy") {
		t.Fatalf("account database: %q", got)
	}
}

func TestMoveToRootRemovesAGroupThatUserdelLeaves(t *testing.T) {
	f, _ := oldHost(t)
	f.put("/etc/passwd", basePasswd) // the user is gone, the group stays
	if old, err := f.host.HasAccountSetup(); err != nil || !old {
		t.Fatalf("a leftover group is not found: %v %v", old, err)
	}
	f.host.Run = func(_ context.Context, name string, args ...string) error {
		f.calls = append(f.calls, name+" "+strings.Join(args, " "))
		return nil
	}
	if _, err := moveToRoot(f); err != nil {
		t.Fatal(err)
	}
	if f.calls[len(f.calls)-1] != "groupdel qhost-deploy" || slices.Contains(f.calls, "userdel qhost-deploy") {
		t.Fatalf("commands %v", f.calls)
	}
}

func TestMoveToRootAfterAFailureStopsThereAndARepeatReachesTheTarget(t *testing.T) {
	f, chowns := oldHost(t)
	f.failOn = "systemctl try-restart"
	_, err := moveToRoot(f)
	if err == nil || !strings.Contains(err.Error(), "systemctl try-restart qhost-deploy") {
		t.Fatalf("error %v", err)
	}
	if slices.Contains(f.calls, "userdel qhost-deploy") || !strings.Contains(f.read("/etc/passwd"), "qhost-deploy") {
		t.Fatalf("the account went although the restart failed: %v", f.calls)
	}
	if old, err := f.host.HasAccountSetup(); err != nil || !old {
		t.Fatal("a half-done move is not repeated")
	}
	f.failOn, f.calls, *chowns = "", nil, nil
	if _, err := moveToRoot(f); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.calls, "userdel qhost-deploy") || len(*chowns) != 3 {
		t.Fatalf("commands %v, chowns %v", f.calls, *chowns)
	}
	if old, _ := f.host.HasAccountSetup(); old || strings.Contains(f.read("/etc/passwd"), "qhost-deploy") {
		t.Fatal("the repeat did not reach the target")
	}
}

func TestMoveToRootChangesNothingWhenAStepFailsEarly(t *testing.T) {
	f, chowns := oldHost(t)
	f.failOn = "systemd-tmpfiles"
	if _, err := moveToRoot(f); err == nil {
		t.Fatal("no error")
	}
	if len(*chowns) != 0 || len(f.calls) != 0 {
		t.Fatalf("later steps ran: %v %v", f.calls, *chowns)
	}
}

func TestMoveToRootDoesNotFollowLinksInTheState(t *testing.T) {
	f, chowns := oldHost(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "foreign"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.root, "var/lib/qhost-deploy/link")); err != nil {
		t.Fatal(err)
	}
	if _, err := moveToRoot(f); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(*chowns, "/var/lib/qhost-deploy/link 0:0") {
		t.Fatalf("the link itself is not handed over: %v", *chowns)
	}
	for _, c := range *chowns {
		if strings.Contains(c, "foreign") {
			t.Fatalf("a linked folder was entered: %v", *chowns)
		}
	}
}

func TestMoveToRootGivesTheSystemMarkerFolderToTheReceiverGroupOnlyWhereItIsSetUp(t *testing.T) {
	f, _ := oldHost(t)
	if _, err := moveToRoot(f); err != nil {
		t.Fatal(err)
	}
	if f.host.exists(HookTmpfilesConf) {
		t.Fatal("a host without the receiver got a receiver rule")
	}

	f, _ = oldHost(t)
	f.put("/etc/group", oldGroups+"qhost-hook:x:995:\n")
	f.put("/etc/passwd", oldPasswd+"qhost-hook:x:995:995::/nonexistent:/usr/sbin/nologin\n")
	f.put(HookUnitPath(), string(packaging.HookUnit))
	f.put(HookTmpfilesConf, "d /run/qhost-hook 0755 qhost-hook qhost-hook -\nd /run/qhost-hook/system 0770 qhost-hook qhost-deploy -\n")
	if _, err := moveToRoot(f); err != nil {
		t.Fatal(err)
	}
	if got := f.read(HookTmpfilesConf); !strings.Contains(got, "d /run/qhost-hook/system 0770 qhost-hook qhost-hook -\n") || strings.Contains(got, "qhost-deploy") {
		t.Fatalf("receiver rule: %q", got)
	}
	if !slices.Contains(f.calls, "systemd-tmpfiles --create "+HookTmpfilesConf) {
		t.Fatalf("the rule was not applied: %v", f.calls)
	}
}

func TestMoveToRootLeavesFreshAndMissingSetupsAlone(t *testing.T) {
	fresh := newFakeHost(t, baseGroups, basePasswd)
	fresh.put(UnitPath(), string(packaging.DeployUnit))
	none := newFakeHost(t, oldGroups, oldPasswd) // an account, but the service was never set up
	for name, f := range map[string]*fakeHost{"fresh": fresh, "none": none} {
		if old, err := f.host.HasAccountSetup(); err != nil || old {
			t.Fatalf("%s: old %v, %v", name, old, err)
		}
	}
	if old, err := (Host{Root: t.TempDir()}).HasAccountSetup(); err != nil || old {
		t.Fatalf("empty root: old %v, %v", old, err)
	}
}

func TestMoveToRootRefusesAUnitThatStillRunsAsTheAccount(t *testing.T) {
	f, chowns := oldHost(t)
	f.put(UnitPath(), "[Service]\nUser=qhost-deploy\nGroup=qhost-deploy\n")
	if _, err := moveToRoot(f); err == nil {
		t.Fatal("no error")
	}
	if len(f.calls) != 0 || len(*chowns) != 0 || !strings.Contains(f.read("/etc/passwd"), "qhost-deploy") {
		t.Fatalf("changed: %v %v", f.calls, *chowns)
	}
}
