package service

import (
	"context"
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
		if got := code(db.authorize(tc.uid, control.AccessRead)); got != tc.read {
			t.Errorf("read of %d = %q, want %q", tc.uid, got, tc.read)
		}
		if got := code(db.authorize(tc.uid, control.AccessWrite)); got != tc.write {
			t.Errorf("write of %d = %q, want %q", tc.uid, got, tc.write)
		}
		if got := code(db.authorize(tc.uid, control.AccessPropose)); got != tc.read {
			t.Errorf("propose of %d = %q, want %q", tc.uid, got, tc.read)
		}
		if got := code(db.authorize(tc.uid, 0)); got != tc.write {
			t.Errorf("unmarked of %d = %q, want %q", tc.uid, got, tc.write)
		}
	}
	missing := groupDB{group: filepath.Join(t.TempDir(), "missing"), passwd: passwd}
	if got := code(missing.authorize(4100, control.AccessRead)); got != control.CodeNoAccess {
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

// Every command the service registers says whether it reads, writes or proposes. A command without the mark is refused
// by the registry, so the service would not start; the walk below states the contract for every name.
func TestEveryCommandIsMarkedAsReadWriteOrPropose(t *testing.T) {
	var seen atomic.Pointer[control.Registry]
	runService(t, Config{Version: "x", PeerCheck: allowAll, registryHook: seen.Store})
	reg := seen.Load()
	if reg == nil || len(reg.Names()) == 0 {
		t.Fatal("the registry was not seen")
	}
	for _, name := range reg.Names() {
		c, _ := reg.Lookup(name)
		if c.Access != control.AccessRead && c.Access != control.AccessWrite && c.Access != control.AccessPropose {
			t.Errorf("command %s is %v", name, c.Access)
		}
		t.Logf("%-22s %v", name, c.Access)
	}
}
