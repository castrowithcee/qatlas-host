package cli

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/tools"
	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/release/releasetest"
)

func invoke(t *testing.T, name, args string) (string, error) {
	t.Helper()
	out, err := tools.Invoke(context.Background(), name, json.RawMessage(args))
	return string(out), err
}

// sameAsCommand checks that the call answers exactly what the command line prints with --output json.
func sameAsCommand(t *testing.T, name, args string, command ...string) {
	t.Helper()
	got, err := invoke(t, name, args)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	code, out, errOut := run(t, append(command, "--output", "json")...)
	if code != 0 {
		t.Fatalf("%v: code %d, stderr %q", command, code, errOut)
	}
	if got != strings.TrimSpace(out) {
		t.Errorf("%s = %s\n%v prints %s", name, got, command, out)
	}
}

func TestEveryLocalReadEntryHasACallAndNoOtherDoes(t *testing.T) {
	for _, e := range tools.All() {
		want := e.Exec == tools.ExecLocal && e.Class == tools.ClassRead
		if (e.Call != nil) != want {
			t.Errorf("%s: has call %v, want %v", e.Name, e.Call != nil, want)
		}
	}
}

func TestCallsAnswerWhatTheCommandsPrint(t *testing.T) {
	useHostFixture(t, 0)
	dir := hostFileTree(t)
	sameAsCommand(t, "host.users.list", `{}`, "host", "users")
	sameAsCommand(t, "host.users.show", `{"name":"demo-admin"}`, "host", "user", "demo-admin")
	sameAsCommand(t, "host.groups.list", `{}`, "host", "groups")
	sameAsCommand(t, "host.groups.show", `{"name":"demo-user"}`, "host", "group", "demo-user")
	sameAsCommand(t, "host.folder.show", `{"path":`+quote(dir)+`}`, "host", "folder", dir)
	sameAsCommand(t, "host.file.read", `{"path":`+quote(dir)+`}`, "host", "file", dir)
	file := filepath.Join(dir, "demo.conf")
	sameAsCommand(t, "host.file.read", `{"path":`+quote(file)+`}`, "host", "file", file)
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestCallDiagnoseWritesThePackageLikeTheCommand(t *testing.T) {
	home := useHome(t)
	startPlainService(t)
	fromCall, err := invoke(t, "host.diagnose", `{"out":`+quote(filepath.Join(home, "call.tar.gz"))+`}`)
	if err != nil {
		t.Fatal(err)
	}
	fromCommand := diagnoseJSON(t, "--out", filepath.Join(home, "command.tar.gz"))
	var got map[string]any
	if err := json.Unmarshal([]byte(fromCall), &got); err != nil {
		t.Fatal(err)
	}
	if got["path"] != filepath.Join(home, "call.tar.gz") {
		t.Fatalf("path = %v", got["path"])
	}
	got["path"], fromCommand["path"] = nil, nil
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(fromCommand)
	if string(a) != string(b) {
		t.Errorf("call %s, command %s", a, b)
	}
	if files := readArchive(t, filepath.Join(home, "call.tar.gz")); files["version.txt"] == "" {
		t.Error("the package is empty")
	}
}

func TestCallUpdateStatusAnswersWhatTheCommandPrints(t *testing.T) {
	signer := releasetest.NewSigner(t)
	useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	selfUpdate.euid = func() int { return 1000 }
	sameAsCommand(t, "host.update.status", `{}`, "update", "status")
}

func TestCallsKeepTheRequirementsOfTheCommands(t *testing.T) {
	calls := map[string]string{
		"host.users.list": `{}`, "host.users.show": `{"name":"demo-user"}`, "host.groups.list": `{}`,
		"host.groups.show": `{"name":"sudo"}`, "host.folder.show": `{"path":"/"}`, "host.file.read": `{"path":"/"}`,
	}
	// Not an administrator: nothing is answered.
	useHostFixture(t, 1001)
	for name, args := range calls {
		if _, err := invoke(t, name, args); !isPermission(err) {
			t.Errorf("%s as a plain user: %v", name, err)
		}
	}
	// Administrator without root: everything but the file read.
	useHostFixture(t, 1000)
	for name, args := range calls {
		_, err := invoke(t, name, args)
		if name == "host.file.read" {
			if !isPermission(err) || !strings.Contains(err.Error(), "must run as root") {
				t.Errorf("%s as an administrator: %v", name, err)
			}
		} else if err != nil {
			t.Errorf("%s as an administrator: %v", name, err)
		}
	}
}

func isPermission(err error) bool {
	var f *cli.Failure
	return errors.As(err, &f) && f.Code == output.CodePermission
}

func TestInvokeRefusesWhatTheTableOnlyDescribesAndBadArguments(t *testing.T) {
	useHostFixture(t, 0)
	for _, name := range []string{"host.apply", "host.undo", "host.uninstall", "host.update.install", "source.grant", "status", "stack.start"} {
		if _, err := invoke(t, name, `{}`); !errors.Is(err, tools.ErrNotCallable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := invoke(t, "no.such.tool", `{}`); !errors.Is(err, tools.ErrUnknownTool) {
		t.Errorf("unknown: %v", err)
	}
	for _, args := range []string{`{}`, `{"name":1}`, `{"name":"a","extra":1}`, `[]`, `nonsense`} {
		if _, err := invoke(t, "host.users.show", args); !errors.Is(err, tools.ErrInvalidArguments) {
			t.Errorf("%s: %v", args, err)
		}
	}
	if _, err := invoke(t, "host.users.list", ""); err != nil {
		t.Errorf("empty arguments: %v", err)
	}
}
