package admins

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestInDatabase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		groups string
		uid    uint32
		want   bool
	}{
		{"member of both", inventedGroups, 4100, true},
		{"admin group as primary group", inventedGroups, 4101, true},
		{"service group only", inventedGroups, 4102, false},
		{"admin group only", inventedGroups, 4103, false},
		{"service group as primary group only", inventedGroups, 4104, false},
		{"second user of a member's name", inventedGroups, 4105, true},
		{"unknown user", inventedGroups, 4999, false},
		{"root is no member", inventedGroups, 0, false},
		{"no service group", "sudo:x:27:example-admin\n", 4100, false},
		{"no admin group", "qhost:x:990:example-admin\n", 4100, false},
		{"malformed lines", "sudo:x\nqhost\n", 4100, false},
		{"primary service group and admin member", "sudo:x:27:example-qhost\nqhost:x:990:\n", 4104, true},
		{"empty group id never matches", "sudo:x::example-empty\nqhost:x::\n", 4106, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := InDatabase(strings.NewReader(tc.groups), strings.NewReader(inventedUsers), tc.uid)
			if err != nil || got != tc.want {
				t.Fatalf("admin(%d) = %v, %v; want %v", tc.uid, got, err, tc.want)
			}
		})
	}
	long := "qhost:x:990:" + strings.Repeat("x", maxLine) + "\n"
	if ok, err := InDatabase(strings.NewReader(long), strings.NewReader(inventedUsers), 4100); ok || err == nil {
		t.Fatalf("an overlong line = %v, %v", ok, err)
	}
}

func TestDatabaseReadsTheFilesOnEveryCall(t *testing.T) {
	dir := t.TempDir()
	db := Database{Group: filepath.Join(dir, "group"), Passwd: filepath.Join(dir, "passwd")}
	if ok, err := db.Admin(4100); ok || err == nil {
		t.Fatalf("a missing database = %v, %v", ok, err)
	}
	if err := os.WriteFile(db.Passwd, []byte(inventedUsers), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db.Group, []byte(inventedGroups), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := db.Admin(4100); !ok || err != nil {
		t.Fatalf("admin = %v, %v", ok, err)
	}
	if err := os.WriteFile(db.Group, []byte(strings.Replace(inventedGroups, "sudo:x:27:example-admin,", "sudo:x:27:", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := db.Admin(4100); ok || err != nil {
		t.Fatalf("admin after the revocation = %v, %v", ok, err)
	}
}

func TestAuthorizeDecidesByClass(t *testing.T) {
	for name, tc := range map[string]struct {
		uid   uint32
		admin bool
		class control.Class
		want  string
	}{
		"root reads":         {0, false, control.ClassRead, ""},
		"root proposes":      {0, false, control.ClassPropose, ""},
		"root changes":       {0, false, control.ClassChange, ""},
		"admin reads":        {4100, true, control.ClassRead, ""},
		"admin proposes":     {4100, true, control.ClassPropose, ""},
		"admin changes":      {4100, true, control.ClassChange, control.CodeNotRoot},
		"admin, unmarked":    {4100, true, 0, control.CodeNotRoot},
		"other user reads":   {4102, false, control.ClassRead, control.CodeNoAccess},
		"other user changes": {4102, false, control.ClassChange, control.CodeNoAccess},
	} {
		t.Run(name, func(t *testing.T) {
			err := Authorize(tc.uid, tc.admin, control.Access{Class: tc.class, Scope: control.ScopeHost})
			var ce *control.Error
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("err = %v", err)
			case tc.want != "" && (!errors.As(err, &ce) || ce.Code != tc.want):
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}
