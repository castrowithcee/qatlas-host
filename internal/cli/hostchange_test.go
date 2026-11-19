package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

type changeWorld struct {
	values  map[string]string
	calls   []string
	stdin   string
	failSet string
}

func (w *changeWorld) Run(_ context.Context, c hostchange.Command) ([]byte, error) {
	w.calls = append(w.calls, c.Display())
	w.stdin = string(c.Secret)
	if c.Argv[0] == "demo-set" {
		if c.Argv[2] == w.failSet {
			return []byte("simulated"), errors.New("exit status 1")
		}
		w.values[c.Argv[1]] = c.Argv[2]
	}
	return nil, nil
}

type changeFixture struct {
	w       *changeWorld
	dir     string
	audited []hostchange.AuditEntry
	auditEr error
}

// newChangeFixture replaces the change environment with an invented one: demo actions, a recording executor, a
// journal in a temporary directory and a recorder that keeps what it is handed. The caller is root unless the
// test says otherwise; no real account, folder or journal is touched.
func newChangeFixture(t *testing.T, euid int) *changeFixture {
	t.Helper()
	useHostFixture(t, euid)
	f := &changeFixture{w: &changeWorld{values: map[string]string{"alpha": "old"}}}
	reg := hostchange.NewRegistry()
	register := func(d hostchange.Definition) {
		if err := reg.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	register(hostchange.Definition{
		ID: "demo.set",
		Plan: func(args []string, _ []byte) (hostchange.Plan, error) {
			if len(args) != 2 {
				return hostchange.Plan{}, hostchange.Argf("demo.set needs a key and a value")
			}
			return hostchange.Plan{
				Sentence: "Set " + args[0] + " to " + args[1] + ".",
				Commands: []hostchange.Command{{Argv: []string{"demo-set", args[0], args[1]}}},
				Capture: func(context.Context) (json.RawMessage, error) {
					return json.Marshal([]string{args[0], f.w.values[args[0]]})
				},
			}, nil
		},
		Inverse: func(prev json.RawMessage) (hostchange.Plan, error) {
			var kv []string
			if err := json.Unmarshal(prev, &kv); err != nil {
				return hostchange.Plan{}, err
			}
			return hostchange.Plan{Sentence: "Put it back.", Commands: []hostchange.Command{{Argv: []string{"demo-set", kv[0], kv[1]}}}}, nil
		},
	})
	register(hostchange.Definition{
		ID: "demo.login", WantsSecret: true,
		Plan: func(_ []string, secret []byte) (hostchange.Plan, error) {
			return hostchange.Plan{
				Sentence: "Set the login secret.", NotReversible: "the old one is not kept",
				Commands: []hostchange.Command{{Argv: []string{"demo-login", "user1"}, Secret: secret}},
			}, nil
		},
	})
	register(hostchange.Definition{
		ID: "demo.guard",
		Plan: func([]string, []byte) (hostchange.Plan, error) {
			return hostchange.Plan{
				Sentence: "Guarded change.", NotReversible: "demo", Commands: []hostchange.Command{{Argv: []string{"demo-set", "guard", "x"}}},
				Capture: func(context.Context) (json.RawMessage, error) {
					return nil, hostchange.Refusef("demo-user is the last administrator")
				},
			}, nil
		},
	})
	tick := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	f.dir = filepath.Join(t.TempDir(), "qhost-host", "journal")
	old := changeEnv
	t.Cleanup(func() { changeEnv = old })
	changeEnv = changeEnvironment{
		registry: func() *hostchange.Registry { return reg }, exec: f.w, journalDir: f.dir, ownerUID: os.Getuid(),
		now:     func() time.Time { tick = tick.Add(time.Second); return tick },
		sudoUID: func() int { return 1000 },
		record: func(a hostchange.AuditEntry) error {
			f.audited = append(f.audited, a)
			return f.auditEr
		},
	}
	return f
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not JSON: %q: %v", s, err)
	}
	return m
}

func TestHostApplyRunsTheActionAndReportsIt(t *testing.T) {
	f := newChangeFixture(t, 0)
	code, out, errOut := run(t, "host", "apply", "demo.set", "alpha", "new", "--confirm", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, errOut)
	}
	m := decode(t, out)
	if m["result"] != "applied" || m["action"] != "demo.set" || m["sentence"] != "Set alpha to new." ||
		m["check"] != "none" || m["restored"] != false || m["reversible"] != true || m["warning"] != "" || m["journal_id"] == "" {
		t.Fatalf("%v", m)
	}
	if strings.Join(f.w.calls, "") != "demo-set alpha new" || len(f.audited) != 1 || f.audited[0].SudoUID != 1000 {
		t.Fatalf("executed %v, audit %+v", f.w.calls, f.audited)
	}
}

func TestHostChangeNeedsRootAndConfirm(t *testing.T) {
	f := newChangeFixture(t, 1000)
	for _, args := range [][]string{
		{"host", "apply", "demo.set", "alpha", "new", "--confirm"}, {"host", "undo", "x", "--confirm"}, {"host", "journal"},
	} {
		code, out, errOut := run(t, args...)
		if code == 0 || out != "" || !strings.Contains(errOut, "must run as root") {
			t.Errorf("%v: code %d out %q err %q", args, code, out, errOut)
		}
	}
	hostEnv.euid = func() int { return 0 }
	code, out, errOut := run(t, "host", "apply", "demo.set", "alpha", "new")
	if code != 2 || out != "" || !strings.Contains(errOut, "--confirm") {
		t.Errorf("missing --confirm: code %d out %q err %q", code, out, errOut)
	}
	if code, _, _ = run(t, "host", "undo", "20300102T030405.000000Z-aaaaaa"); code != 2 {
		t.Errorf("undo without --confirm: code %d", code)
	}
	if len(f.w.calls) != 0 || f.w.values["alpha"] != "old" {
		t.Errorf("nothing may run: %v", f.w.calls)
	}
}

func TestHostApplyReportsARefusalWithItsReasonAndChangesNothing(t *testing.T) {
	f := newChangeFixture(t, 0)
	code, out, errOut := run(t, "host", "apply", "demo.guard", "--confirm", "--output", "json")
	if code != 1 || out == "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	m := decode(t, out)
	if m["result"] != "refused" || m["error"] != "demo-user is the last administrator" || m["journal_id"] != "" {
		t.Errorf("%v", m)
	}
	if !strings.Contains(errOut, "change-refused") || len(f.w.calls) != 0 || len(f.audited) != 0 {
		t.Errorf("err %q, executed %v, audited %v", errOut, f.w.calls, f.audited)
	}
	files, _ := os.ReadDir(f.dir)
	for _, de := range files {
		if strings.HasSuffix(de.Name(), ".json") {
			t.Errorf("a refusal wrote the journal entry %s", de.Name())
		}
	}
}

func TestHostApplyRefusesBadActionAndArguments(t *testing.T) {
	f := newChangeFixture(t, 0)
	for _, args := range [][]string{
		{"host", "apply", "--confirm"}, {"host", "apply", "demo.unknown", "--confirm"}, {"host", "apply", "demo.set", "alpha", "--confirm"},
	} {
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("%v: code %d", args, code)
		}
	}
	if len(f.w.calls) != 0 {
		t.Errorf("executed %v", f.w.calls)
	}
}

func TestHostApplySecretComesFromStdinOnly(t *testing.T) {
	f := newChangeFixture(t, 0)
	const marker = "demo-secret-42"
	code, out, errOut := runWithStdin(t, marker+"\n", "host", "apply", "demo.login", "--confirm", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d %q", code, errOut)
	}
	m := decode(t, out)
	if f.w.stdin != marker || m["commands"].([]any)[0] != "demo-login user1 < <password>" || m["reversible"] != false {
		t.Errorf("stdin %q, report %v", f.w.stdin, m)
	}
	files, _ := os.ReadDir(f.dir)
	all := out + errOut + strings.Join(f.w.calls, "")
	for _, de := range files {
		b, _ := os.ReadFile(filepath.Join(f.dir, de.Name()))
		all += string(b)
	}
	if strings.Contains(all, marker) {
		t.Error("the secret leaked into output, argv or journal")
	}
	if code, _, _ := runWithStdin(t, "", "host", "apply", "demo.login", "--confirm"); code != 2 {
		t.Errorf("empty stdin: code %d", code)
	}
}

func TestHostFailedChangeReportsAndExitsNonZero(t *testing.T) {
	f := newChangeFixture(t, 0)
	f.w.failSet = "new"
	code, out, errOut := run(t, "host", "apply", "demo.set", "alpha", "new", "--confirm", "--output", "json")
	m := decode(t, out)
	if code != 1 || m["result"] != "failed" || m["restored"] != false || !strings.Contains(m["error"].(string), "command 1") ||
		!strings.Contains(errOut, "did not succeed") || f.w.values["alpha"] != "old" {
		t.Fatalf("code %d %v err %q", code, m, errOut)
	}
}

func TestHostAuditFailureWarnsButTheChangeStands(t *testing.T) {
	f := newChangeFixture(t, 0)
	f.auditEr = errors.New("the host service is not running")
	code, out, errOut := run(t, "host", "apply", "demo.set", "alpha", "new", "--confirm", "--output", "json")
	m := decode(t, out)
	if code != 0 || m["result"] != "applied" || !strings.Contains(m["warning"].(string), "audit entry is missing") ||
		!strings.Contains(errOut, "warning: ") || f.w.values["alpha"] != "new" {
		t.Fatalf("code %d %v err %q", code, m, errOut)
	}
}

func TestHostJournalAndUndo(t *testing.T) {
	f := newChangeFixture(t, 0)
	_, out, _ := run(t, "host", "apply", "demo.set", "alpha", "new", "--confirm", "--output", "json")
	id := decode(t, out)["journal_id"].(string)

	code, out, errOut := run(t, "host", "journal", "--output", "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); code != 0 || err != nil || len(rows) != 1 || rows[0]["id"] != id ||
		rows[0]["result"] != "applied" || rows[0]["reversible"] != true || rows[0]["sudo_uid"] != 1000.0 ||
		rows[0]["undo_sentence"] != "Put it back." {
		t.Fatalf("code %d %q %s %v", code, errOut, out, err)
	}
	if cmds, _ := rows[0]["undo_commands"].([]any); len(cmds) != 1 || cmds[0] != "demo-set alpha old" {
		t.Errorf("undo commands %v", rows[0]["undo_commands"])
	}

	code, out, errOut = run(t, "host", "undo", id, "--confirm", "--output", "json")
	m := decode(t, out)
	if code != 0 || m["result"] != "applied" || m["reversible"] != false || f.w.values["alpha"] != "old" {
		t.Fatalf("code %d %q %v %v", code, errOut, m, f.w.values)
	}
	if len(f.audited) != 2 || f.audited[1].UndoOf != id {
		t.Errorf("audit %+v", f.audited)
	}
	_, out, _ = run(t, "host", "journal", "--output", "json")
	rows = nil
	_ = json.Unmarshal([]byte(out), &rows)
	if len(rows) != 2 || rows[1]["result"] != "undone" || rows[1]["reversible"] != false || rows[1]["undo_sentence"] != "" {
		t.Fatalf("%v", rows)
	}
	if code, _, _ := run(t, "host", "undo", id, "--confirm"); code == 0 {
		t.Error("a second undo must fail")
	}
	if code, _, _ := run(t, "host", "undo", "../../etc/passwd", "--confirm"); code == 0 {
		t.Error("an invalid id must fail")
	}
}

func TestSudoUIDIsReadStrictly(t *testing.T) {
	for value, want := range map[string]int{
		"1000": 1000, "0": 0, "": 0, "+1000": 0, "-5": 0, " 1000": 0, "1000 ": 0, "01000": 0, "1e3": 0,
		"abc": 0, "99999999999": 0, "2147483648": 0,
	} {
		t.Setenv("SUDO_UID", value)
		if got := sudoUID(); got != want {
			t.Errorf("SUDO_UID %q = %d, want %d", value, got, want)
		}
	}
}

func TestHostApplyTakesItsArgumentsAfterADoubleDash(t *testing.T) {
	f := newChangeFixture(t, 0)
	code, out, errOut := run(t, "host", "apply", "--confirm", "--output", "json", "--", "demo.set", "alpha", "-new")
	if code != 0 {
		t.Fatalf("code %d %q", code, errOut)
	}
	id := decode(t, out)["journal_id"].(string)
	if code, _, errOut = run(t, "host", "undo", "--confirm", "--output", "json", "--", id); code != 0 || f.w.values["alpha"] != "old" {
		t.Fatalf("undo code %d %q %v", code, errOut, f.w.values)
	}
}

func TestHostApplyRefusesASystemPathWithoutTheAcknowledgement(t *testing.T) {
	f := newChangeFixture(t, 0)
	reg := changeEnv.registry()
	if err := hostchange.RegisterFolderActions(reg, hostchange.FolderHost{Accounts: hostadmin.NewHost(false)}); err != nil {
		t.Fatal(err)
	}
	args := hostchange.FolderArgs(hostchange.FolderRequest{
		Path: "/etc/demo-app", Owner: "demo-user", Group: "demo-group",
		UserAccess: hostadmin.AccessReadWrite, GroupAccess: hostadmin.AccessRead, OtherAccess: hostadmin.AccessNothing,
	})
	cmd := append([]string{"host", "apply", "--confirm", "--output", "json", "--", hostchange.ActionFolderSetPermissions}, args...)
	code, out, errOut := run(t, cmd...)
	m := decode(t, out)
	if code != 1 || m["result"] != "refused" || len(f.w.calls) != 0 || len(f.audited) != 0 {
		t.Fatalf("code %d err %q %v", code, errOut, m)
	}
}

func TestHostReportsReadInEveryFormat(t *testing.T) {
	newChangeFixture(t, 0)
	for _, format := range []string{"table", "compact"} {
		code, out, errOut := run(t, "host", "apply", "demo.set", "alpha", "new", "--confirm", "--output", format)
		if code != 0 || !strings.Contains(out, "demo-set alpha new") {
			t.Fatalf("%s apply: code %d out %q err %q", format, code, out, errOut)
		}
		if code, out, errOut = run(t, "host", "journal", "--output", format); code != 0 || !strings.Contains(out, "demo-set alpha old") {
			t.Fatalf("%s journal: code %d out %q err %q", format, code, out, errOut)
		}
		if code, out, errOut = run(t, "host", "groups", "--output", format); code != 0 || !strings.Contains(out, "demo-admin") {
			t.Fatalf("%s groups: code %d out %q err %q", format, code, out, errOut)
		}
	}
}
