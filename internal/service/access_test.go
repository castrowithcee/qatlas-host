package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// An invented group database: example-admin is an administrator in the service group, example-primary the same
// through primary groups, example-reader is in the service group only, example-sudoer in the admin group only.
const (
	inventedGroups = "root:x:0:\n" +
		"sudo:x:27:example-admin,example-sudoer\n" +
		"example-primary:x:1500:\n" +
		"qhost:x:990:example-admin,example-reader,example-primary\n" +
		"sudo:x:28:example-reader\n" // a second entry of a name never counts
	inventedUsers = "root:x:0:0:root:/root:/bin/sh\n" +
		"example-admin:x:4100:4100::/nonexistent:/usr/sbin/nologin\n" +
		"example-primary:x:4101:27::/nonexistent:/usr/sbin/nologin\n" +
		"example-reader:x:4102:4102::/nonexistent:/usr/sbin/nologin\n" +
		"example-sudoer:x:4103:4103::/nonexistent:/usr/sbin/nologin\n" +
		"example-qhost:x:4104:990::/nonexistent:/usr/sbin/nologin\n" +
		"example-admin:x:4105:4105::/nonexistent:/usr/sbin/nologin\n" + // a second user of a name
		"example-empty:x:4106:::/nonexistent:/usr/sbin/nologin\n"
)

func writeDatabase(t *testing.T, groups, users string) (group, passwd string) {
	t.Helper()
	dir := t.TempDir()
	group, passwd = filepath.Join(dir, "group"), filepath.Join(dir, "passwd")
	if err := os.WriteFile(group, []byte(groups), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwd, []byte(users), 0o600); err != nil {
		t.Fatal(err)
	}
	return group, passwd
}

func TestAuthorize(t *testing.T) {
	group, passwd := writeDatabase(t, inventedGroups, inventedUsers)
	db := groupDB{group: group, passwd: passwd}
	code := func(err error) string {
		var ce *control.Error
		if err == nil {
			return ""
		}
		if !errors.As(err, &ce) {
			t.Fatalf("refusal %v has no fixed answer", err)
		}
		return ce.Code
	}
	for _, tc := range []struct {
		uid         uint32
		read, write string
	}{
		{0, "", ""},
		{4100, "", control.CodeNotRoot},
		{4101, "", control.CodeNotRoot},
		{4102, control.CodeNoAccess, control.CodeNoAccess},
		{4103, control.CodeNoAccess, control.CodeNoAccess},
		{4999, control.CodeNoAccess, control.CodeNoAccess},
	} {
		if got := code(db.authorize(tc.uid, classAccess(control.ClassRead))); got != tc.read {
			t.Errorf("read of %d = %q, want %q", tc.uid, got, tc.read)
		}
		if got := code(db.authorize(tc.uid, classAccess(control.ClassChange))); got != tc.write {
			t.Errorf("write of %d = %q, want %q", tc.uid, got, tc.write)
		}
		if got := code(db.authorize(tc.uid, classAccess(control.ClassPropose))); got != tc.read {
			t.Errorf("propose of %d = %q, want %q", tc.uid, got, tc.read)
		}
		if got := code(db.authorize(tc.uid, classAccess(0))); got != tc.write {
			t.Errorf("unmarked of %d = %q, want %q", tc.uid, got, tc.write)
		}
	}
	missing := groupDB{group: filepath.Join(t.TempDir(), "missing"), passwd: passwd}
	if got := code(missing.authorize(4100, classAccess(control.ClassRead))); got != control.CodeNoAccess {
		t.Fatalf("read without a database = %q", got)
	}
}

func TestRevokedAccessEndsAtTheNextRequestOfAnOpenConnection(t *testing.T) {
	for _, revoke := range []struct{ name, groups string }{
		{"admin group", strings.Replace(inventedGroups, "sudo:x:27:example-admin,", "sudo:x:27:", 1)},
		{"service group", strings.Replace(inventedGroups, "qhost:x:990:example-admin,", "qhost:x:990:", 1)},
	} {
		t.Run(revoke.name, func(t *testing.T) {
			group, passwd := writeDatabase(t, inventedGroups, inventedUsers)
			ep := runService(t, Config{Version: "x", GroupFile: group, PasswdFile: passwd,
				PeerCheck: func(net.Conn) (uint32, error) { return 4100, nil }})
			cl, err := client.Dial(context.Background(), ep)
			if err != nil {
				t.Fatal(err)
			}
			defer cl.Close()
			var st control.StatusResult
			if err := cl.Call(context.Background(), control.StatusCommand, nil, &st); err != nil || st.Version != "x" {
				t.Fatalf("status = %+v, %v", st, err)
			}
			// A refused write keeps the connection; the next read is answered.
			var ce *control.Error
			if err := cl.Call(context.Background(), control.WindowSetCommand, nil, nil); !errors.As(err, &ce) || ce.Code != control.CodeNotRoot {
				t.Fatalf("window.set = %v", err)
			}
			if err := cl.Call(context.Background(), control.StatusCommand, nil, nil); err != nil {
				t.Fatalf("status after a refused write = %v", err)
			}
			if err := os.WriteFile(group, []byte(revoke.groups), 0o600); err != nil {
				t.Fatal(err)
			}
			st = control.StatusResult{}
			err = cl.Call(context.Background(), control.StatusCommand, nil, &st)
			if !errors.As(err, &ce) || ce.Code != control.CodeNoAccess || st.Version != "" {
				t.Fatalf("status after the revocation = %+v, %v", st, err)
			}
		})
	}
}

func TestAPeerWithoutAccessLearnsNothing(t *testing.T) {
	group, passwd := writeDatabase(t, inventedGroups, inventedUsers)
	for _, uid := range []uint32{4102, 4103, 4999} {
		ep := runService(t, Config{Version: "x", GroupFile: group, PasswdFile: passwd,
			PeerCheck: func(net.Conn) (uint32, error) { return uid, nil }})
		for _, payload := range []string{`{"v":1,"command":"status"}`, `{"v":1,"command":"nope"}`, `{"v":1,"command":"deploy"}`} {
			resp := rawExchange(t, ep, payload+"\n")
			if resp.OK || resp.Result != nil || resp.Error == nil || resp.Error.Code != control.CodeNoAccess {
				t.Fatalf("user %d, %s = %+v", uid, payload, resp)
			}
		}
	}
}

func classAccess(class control.Class) control.Access {
	return control.Access{Class: class, Scope: control.ScopeDeploy}
}

// Every command the service registers says whether it reads, proposes or changes, and what it reaches. A command
// without the marks is refused by the registry, so the service would not start; the walk below states the
// contract for every name and lists the table.
func TestEveryCommandHasAClassAndAScope(t *testing.T) {
	var seen atomic.Pointer[control.Registry]
	runService(t, Config{Version: "x", PeerCheck: allowAll, registryHook: seen.Store})
	reg := seen.Load()
	if reg == nil || len(reg.Names()) == 0 {
		t.Fatal("the registry was not seen")
	}
	entries := reg.Entries()
	if len(entries) != len(reg.Names()) {
		t.Fatalf("%d entries for %d commands", len(entries), len(reg.Names()))
	}
	for _, e := range entries {
		c, _ := reg.Lookup(e.Name)
		if e.Class != c.Class || e.Scope != c.Scope {
			t.Errorf("entry %+v differs from its command", e)
		}
		if e.Class != control.ClassRead && e.Class != control.ClassPropose && e.Class != control.ClassChange {
			t.Errorf("command %s has class %v", e.Name, e.Class)
		}
		if e.Scope != control.ScopeDeploy && e.Scope != control.ScopeStack {
			t.Errorf("command %s has scope %v", e.Name, e.Scope)
		}
		if e.Scope == control.ScopeStack && c.Stack == nil {
			t.Errorf("stack command %s has no stack function", e.Name)
		}
		t.Logf("%-22s %-8v %v", e.Name, e.Class, e.Scope)
	}
}

// stackScoped lists the commands of scope stack with arguments that name the given stacks.
var stackScoped = map[string]func(stacks ...string) string{
	control.DeployCommand: one, control.ResumeCommand: one, control.PauseCommand: one, control.UnpauseCommand: one,
	control.PinCommand: one, control.UnpinCommand: one, control.StackStartCommand: one, control.StackStopCommand: one,
	control.StackRestartCommand: one, control.StackSettingsGetCommand: one, control.StackSettingsSetCommand: one,
	control.StackSettingsRemoveCommand: one, control.SecretSetCommand: one, control.SecretListCommand: one,
	control.SecretDeleteCommand: one, control.BackupCreateCommand: one, control.BackupListCommand: one,
	control.BackupRestoreCommand: one, control.UpdatesListCommand: one, control.ChangelogShowCommand: one,
	control.StacksCheckCommand: one, control.RunsCommand: one, control.UpdateRunCommand: many,
	control.ReturnCheckCommand: one, control.ReturnRunCommand: one,
	control.GrantAddCommand: one, control.GrantRemoveCommand: one,
}

func one(stacks ...string) string {
	if len(stacks) == 0 {
		return `{}`
	}
	b, _ := json.Marshal(map[string]any{"stack": stacks[0]})
	return string(b)
}

func many(stacks ...string) string {
	b, _ := json.Marshal(map[string]any{"stacks": stacks})
	return string(b)
}

func TestStackCommandsReadTheirStackFromTheArguments(t *testing.T) {
	var seen atomic.Pointer[control.Registry]
	runService(t, Config{Version: "x", PeerCheck: allowAll, registryHook: seen.Store})
	reg := seen.Load()
	for _, e := range reg.Entries() {
		if _, ok := stackScoped[e.Name]; ok != (e.Scope == control.ScopeStack) {
			t.Errorf("command %s has scope %v", e.Name, e.Scope)
		}
	}
	for name, args := range stackScoped {
		c, ok := reg.Lookup(name)
		if !ok {
			t.Errorf("command %s is not registered", name)
			continue
		}
		for _, tc := range []struct {
			what string
			raw  string
			want string
		}{
			{"one stack", args("demo-stack"), "demo-stack"},
			{"no argument", ``, ""},
			{"no stack", args(), ""},
			{"empty stack", args(""), ""},
			{"not an object", `"demo-stack"`, ""},
			{"null", `null`, ""},
			{"wrong type", `{"stack":5,"stacks":"x"}`, ""},
			{"broken", `{"stack":`, ""},
		} {
			got := c.AccessFor(json.RawMessage(tc.raw))
			wantScope := control.ScopeStack
			if tc.want == "" {
				wantScope = control.ScopeDeploy
			}
			if got.Stack != tc.want || got.Scope != wantScope {
				t.Errorf("%s with %s: %+v, want stack %q", name, tc.what, got, tc.want)
			}
		}
	}
	update, _ := reg.Lookup(control.UpdateRunCommand)
	for _, raw := range []string{many("a", "b"), many("a", "a"), many(), many("a", "")} {
		if got := update.AccessFor(json.RawMessage(raw)); got.Scope != control.ScopeDeploy || got.Stack != "" {
			t.Errorf("update.run with %s: %+v", raw, got)
		}
	}
	// The class does not depend on the scope: a refresh stays a change, also with a stack.
	list, _ := reg.Lookup(control.UpdatesListCommand)
	if got := list.AccessFor(json.RawMessage(`{"stack":"demo-stack","refresh":true}`)); got.Class != control.ClassChange || got.Stack != "demo-stack" {
		t.Errorf("refresh with a stack: %+v", got)
	}
}
