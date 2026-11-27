package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/packaging"
)

// useOldDeployHost sets up a host that runs the deploy service in the setup of earlier releases below a temporary
// root. The runner removes the account like userdel would and records every command; chown records instead of
// changing owners.
func useOldDeployHost(t *testing.T, euid int) (root string, commands, chowns *[]string) {
	t.Helper()
	root = t.TempDir()
	commands, chowns = new([]string), new([]string)
	writeTestFile(t, filepath.Join(root, "etc/passwd"), "root:x:0:0::/root:/bin/bash\nqhost-deploy:x:996:996::/var/lib/qhost-deploy:/usr/sbin/nologin\n")
	writeTestFile(t, filepath.Join(root, "etc/group"), "root:x:0:\nqhost:x:997:\nqhost-deploy:x:996:\n")
	writeTestFile(t, filepath.Join(root, deploysetup.UnitPath()), string(packaging.DeployUnit))
	writeTestFile(t, filepath.Join(root, "etc/tmpfiles.d/qhost-deploy.conf"), "d /run/qhost-deploy 2750 qhost-deploy qhost -\n")
	writeTestFile(t, filepath.Join(root, "var/lib/qhost-deploy/runs/one.json"), "{}\n")
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "foreign"), "")
	if err := os.Symlink(outside, filepath.Join(root, "var/lib/qhost-deploy/link")); err != nil {
		t.Fatal(err)
	}
	old := deploySetup
	deploySetup = deploySetupHost{euid: func() int { return euid }, host: deploysetup.Host{Root: root,
		Chown: func(p string, _, _ int) error { *chowns = append(*chowns, p); return nil },
		Run: func(_ context.Context, name string, args ...string) error {
			line := name + " " + strings.Join(args, " ")
			*commands = append(*commands, line)
			if strings.HasPrefix(line, "systemd-tmpfiles") && strings.Contains(line, "fail") {
				return errors.New("failed")
			}
			if line == "userdel qhost-deploy" {
				writeTestFile(t, filepath.Join(root, "etc/passwd"), "root:x:0:0::/root:/bin/bash\n")
				writeTestFile(t, filepath.Join(root, "etc/group"), "root:x:0:\nqhost:x:997:\n")
			}
			return nil
		}}}
	t.Cleanup(func() { deploySetup = old })
	return root, commands, chowns
}

func TestDeployServicePrepareMovesTheOldSetupWithoutTouchingItsOwnUnitAndARepeatOnlyReportsDone(t *testing.T) {
	root, commands, chowns := useOldDeployHost(t, 0)
	code, _, stderr := run(t, "deploy-service", "prepare")
	if code != 0 || !strings.Contains(stderr, "transition deploy-service-as-root: applied\n") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, c := range *commands {
		if strings.HasPrefix(c, "systemctl") {
			t.Fatalf("the start step ran %q", c)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(root, "etc/tmpfiles.d/qhost-deploy.conf")); string(got) != "d /run/qhost-deploy 2750 root qhost -\n" {
		t.Fatalf("rule %q", got)
	}
	for _, c := range *chowns {
		if strings.Contains(c, "foreign") {
			t.Fatalf("a link was followed: %v", *chowns)
		}
	}
	n := len(*commands)
	code, _, stderr = run(t, "deploy-service", "prepare")
	if code != 0 || stderr != "transition deploy-service-as-root: already done\n" || len(*commands) != n {
		t.Fatalf("repeat: code %d, stderr %q, commands %v", code, stderr, (*commands)[n:])
	}
}

func TestDeployServicePrepareNeedsRootAndAFailureStopsTheStart(t *testing.T) {
	_, commands, _ := useOldDeployHost(t, 1000)
	if code, _, stderr := run(t, "deploy-service", "prepare"); code == 0 || !strings.Contains(stderr, "root") || len(*commands) != 0 {
		t.Fatalf("not root: code %d, stderr %q, commands %v", code, stderr, *commands)
	}
	deploySetup.euid = func() int { return 0 }
	deploySetup.host.Run = func(context.Context, string, ...string) error { return errors.New("exit status 1") }
	code, _, stderr := run(t, "deploy-service", "prepare")
	if code == 0 || !strings.Contains(stderr, "transition deploy-service-as-root") || !strings.Contains(stderr, "sudo qhost update migrate") {
		t.Fatalf("failure: code %d, stderr %q", code, stderr)
	}
}
