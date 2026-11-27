package deploysetup

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const tmpfilesCreate = "systemd-tmpfiles --create /etc/tmpfiles.d/qhost-deploy.conf"

// setUpHost is a host that is set up as the current release does it.
func setUpHost(t *testing.T) *fakeHost {
	t.Helper()
	f := newFakeHost(t, baseGroups, basePasswd)
	if _, err := f.setUp(); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	return f
}

func planCommands(t *testing.T, f *fakeHost) []string {
	t.Helper()
	p, err := f.host.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return p.Commands()
}

func TestPlanOfASetUpHostIsEmpty(t *testing.T) {
	f := setUpHost(t)
	if got := planCommands(t, f); len(got) != 0 {
		t.Fatalf("plan %v", got)
	}
}

func TestPlanRepairsAChannelFolderWithTheWrongOwnerGroupOrModeUnderTheRightRule(t *testing.T) {
	wrong := map[string]channelState{
		"owner":  {uid: 996, gid: 997, mode: rightChannel.mode},
		"group":  {uid: 0, gid: 996, mode: rightChannel.mode},
		"mode":   {uid: 0, gid: 997, mode: os.ModeDir | 0o755},
		"setgid": {uid: 0, gid: 997, mode: os.ModeDir | 0o750},
	}
	for name, st := range wrong {
		t.Run(name, func(t *testing.T) {
			f := setUpHost(t)
			f.channel = st
			if got := planCommands(t, f); !slices.Equal(got, []string{tmpfilesCreate}) {
				t.Fatalf("plan %v", got)
			}
			if _, err := f.setUp(); err != nil {
				t.Fatal(err)
			}
			if got := planCommands(t, f); len(got) != 0 {
				t.Fatalf("plan after the repair %v", got)
			}
		})
	}
}

func TestPlanTreatsALinkInPlaceOfTheChannelFolderAsAWrongOne(t *testing.T) {
	f := setUpHost(t)
	dir := filepath.Join(f.root, "run/qhost-deploy")
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), dir); err != nil {
		t.Fatal(err)
	}
	if got := planCommands(t, f); !slices.Equal(got, []string{tmpfilesCreate}) {
		t.Fatalf("plan %v", got)
	}
}

func TestPlanRemovesTheAccountOfEarlierReleasesAfterTheRestartAndOnceEach(t *testing.T) {
	f, chowns := oldHost(t)
	f.channel = channelState{uid: 996, gid: 997, mode: rightChannel.mode}
	f.put("/run/qhost-deploy/.keep", "")
	plan, err := f.host.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"write /etc/tmpfiles.d/qhost-deploy.conf (mode 0644): d /run/qhost-deploy 2750 root qhost -",
		tmpfilesCreate,
		"mkdir /etc/qhost-deploy (root, mode 0755)",
		"chown /var/lib/qhost-deploy and its content to root, links not followed",
		"systemctl daemon-reload",
		"systemctl try-restart qhost-deploy",
		"userdel qhost-deploy",
		"groupdel qhost-deploy if userdel did not remove it",
	}
	if got := plan.Commands(); !slices.Equal(got, want) {
		t.Fatalf("plan\n%v\nwant\n%v", got, want)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(*chowns, "/var/lib/qhost-deploy/runs/one.json 0:0") || strings.Contains(f.read("/etc/passwd")+f.read("/etc/group"), "qhost-deploy") {
		t.Fatalf("chowns %v", *chowns)
	}
	if got := planCommands(t, f); len(got) != 0 {
		t.Fatalf("plan after the repair %v", got)
	}
}

func TestPlanRemovesAGroupThatIsLeftAlone(t *testing.T) {
	f := setUpHost(t)
	f.put("/etc/group", baseGroups+"qhost-deploy:x:996:\n")
	got := planCommands(t, f)
	if len(got) == 0 || got[len(got)-1] != "groupdel qhost-deploy if userdel did not remove it" {
		t.Fatalf("plan %v", got)
	}
	f.host.Run = func(_ context.Context, name string, args ...string) error {
		if name == "groupdel" {
			f.put("/etc/group", baseGroups)
		}
		return nil
	}
	p, _ := f.host.Plan(context.Background())
	if err := p.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := planCommands(t, f); len(got) != 0 {
		t.Fatalf("plan after the repair %v", got)
	}
}
