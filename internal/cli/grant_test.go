package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// startGrantService runs a deploy service with a bound stacks repository, a user database with one administrator
// and one user without access, and the caller as root. plain-stack has a backup and no check.
func startGrantService(t *testing.T) *repoEnv {
	t.Helper()
	dir := t.TempDir()
	passwd := filepath.Join(dir, "passwd")
	group := filepath.Join(dir, "group")
	if err := os.WriteFile(passwd, []byte("root:x:0:0::/root:/bin/sh\ndemo-admin:x:4100:4100::/home/demo-admin:/bin/sh\ndemo-user:x:4101:4101::/home/demo-user:/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(group, []byte("root:x:0:\nsudo:x:27:demo-admin\nqhost:x:990:demo-admin,demo-user\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldGroup, oldPasswd := testGroupFile, testPasswdFile
	testGroupFile, testPasswdFile = group, passwd
	t.Cleanup(func() { testGroupFile, testPasswdFile = oldGroup, oldPasswd })
	e := startSettingsService(t)
	useHostFixture(t, 0)
	settings := `{"version":1,"backup":{"paths":["data/db"]},"check":{"containers":false}}`
	if code, _, stderr := runWithStdin(t, settings, "stack", "settings", "set", "plain-stack", "--file", "-"); code != 0 {
		t.Fatalf("settings set: %d %s", code, stderr)
	}
	return e
}

func grantRows(t *testing.T, args ...string) []map[string]any {
	t.Helper()
	code, out, stderr := run(t, append([]string{"grant", "list", "--output", "json"}, args...)...)
	if code != 0 {
		t.Fatalf("grant list: %d %s", code, stderr)
	}
	return rowsOf(t, out)
}

func TestGrantAddWithoutConfirmShowsThePlanAndChangesNothing(t *testing.T) {
	startGrantService(t)
	code, out, stderr := run(t, "grant", "add", "demo-admin", "update.run", "plain-stack", "--expires", "2999-12-31")
	if code != 2 || out != "" {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out, stderr)
	}
	for _, want := range []string{"demo-admin", "uid 4100", "update.run", "plain-stack", "engine system", "after the end of 2999-12-31",
		"backup: set up", "check: not set up", "commits and pushes", "--confirm"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("plan lacks %q:\n%s", want, stderr)
		}
	}
	if rows := grantRows(t); len(rows) != 0 {
		t.Fatalf("a plan stored %v", rows)
	}
	// A stack without a backup shows that, and a restart carries no push note.
	_, _, stderr = run(t, "grant", "add", "demo-admin", "stack.restart", "demo-stack")
	if !strings.Contains(stderr, "backup: not set up") || strings.Contains(stderr, "commits and pushes") {
		t.Fatalf("plan:\n%s", stderr)
	}
}

func TestGrantAddListRemove(t *testing.T) {
	startGrantService(t)
	code, out, stderr := run(t, "grant", "add", "demo-admin", "stack.restart", "plain-stack", "--confirm", "--output", "json")
	if code != 0 {
		t.Fatalf("add: %d %s", code, stderr)
	}
	added := objectOf(t, out)
	id, _ := added["id"].(string)
	if !strings.HasPrefix(id, "g-") || added["replaced"] != false || added["backup"] != true || added["engine"] != "system" {
		t.Fatalf("add = %v", added)
	}
	// The same grant replaces the expiry and keeps the ID.
	code, out, _ = run(t, "grant", "add", "demo-admin", "stack.restart", "plain-stack", "--expires", "2999-12-31", "--confirm", "--output", "json")
	if again := objectOf(t, out); code != 0 || again["id"] != id || again["replaced"] != true || again["expires"] != "2999-12-31" {
		t.Fatalf("again: %d %v", code, again)
	}

	rows := grantRows(t)
	if len(rows) != 1 || rows[0]["id"] != id || rows[0]["state"] != "valid" || rows[0]["user"] != "demo-admin" {
		t.Fatalf("list = %v", rows)
	}
	if rows := grantRows(t, "--stack", "demo-stack"); len(rows) != 0 {
		t.Fatalf("filter by stack = %v", rows)
	}
	if rows := grantRows(t, "--user", "demo-admin", "--stack", "plain-stack"); len(rows) != 1 {
		t.Fatalf("filter by user = %v", rows)
	}
	if code, out, _ := run(t, "grant", "list"); code != 0 || !strings.Contains(out, "STATE") || !strings.Contains(out, id) {
		t.Fatalf("table: %d %q", code, out)
	}

	// Without --confirm remove names the grant and keeps it.
	code, out, stderr = run(t, "grant", "remove", id)
	if code != 2 || out != "" || !strings.Contains(stderr, id) || !strings.Contains(stderr, "demo-admin") || !strings.Contains(stderr, "plain-stack") {
		t.Fatalf("remove plan: %d %q %q", code, out, stderr)
	}
	if rows := grantRows(t); len(rows) != 1 {
		t.Fatalf("a plan removed the grant: %v", rows)
	}
	code, out, stderr = run(t, "grant", "remove", id, "--confirm", "--output", "json")
	if removed := objectOf(t, out); code != 0 || removed["id"] != id || removed["removed"] != true {
		t.Fatalf("remove: %d %q %s", code, out, stderr)
	}
	if rows := grantRows(t); len(rows) != 0 {
		t.Fatalf("list after remove = %v", rows)
	}
	for _, confirm := range [][]string{{"remove", id}, {"remove", id, "--confirm"}} {
		if code, _, stderr := run(t, append([]string{"grant"}, confirm...)...); code == 0 || !strings.Contains(stderr, "no grant has this ID") {
			t.Fatalf("%v: %d %q", confirm, code, stderr)
		}
	}
}

func TestGrantAddRefusals(t *testing.T) {
	startGrantService(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"demo-admin", "stack.stop", "plain-stack"}, "cannot be granted"},
		{[]string{"nobody", "stack.restart", "plain-stack"}, "no such user"},
		{[]string{"demo-user", "stack.restart", "plain-stack"}, "users with access"},
		{[]string{"demo-admin", "stack.restart", "missing-stack"}, "no such stack"},
	} {
		code, out, stderr := run(t, append([]string{"grant", "add"}, append(tc.args, "--confirm")...)...)
		if code == 0 || out != "" || !strings.Contains(stderr, tc.want) {
			t.Errorf("%v: code %d, stdout %q, stderr %q, want %q", tc.args, code, out, stderr, tc.want)
		}
	}
	if code, _, _ := run(t, "grant", "add", "demo-admin", "stack.restart"); code != 2 {
		t.Fatalf("two arguments: %d", code)
	}
}

func TestGrantListShowsAnInvalidGrantAndAnUnreadableFile(t *testing.T) {
	e := startGrantService(t)
	if code, _, stderr := run(t, "grant", "add", "demo-admin", "stack.restart", "plain-stack", "--confirm"); code != 0 {
		t.Fatalf("add: %d %s", code, stderr)
	}
	file := filepath.Join(e.state, "grants.json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// The user ID now belongs to another name.
	if err := os.WriteFile(testPasswdFile, []byte("root:x:0:0::/root:/bin/sh\nother-admin:x:4100:4100::/home/x:/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rows := grantRows(t); len(rows) != 1 || rows[0]["state"] != "user-changed" {
		t.Fatalf("list = %v", rows)
	}

	if err := os.WriteFile(file, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := run(t, "grant", "list", "--output", "json")
	if code != 1 || len(rowsOf(t, out)) != 0 || !strings.Contains(stderr, "grants file cannot be read") {
		t.Fatalf("unreadable: %d %q %q", code, out, stderr)
	}
	if got, _ := os.ReadFile(file); string(got) == string(data) || string(got) != "{broken" {
		t.Fatalf("the damaged file was changed: %q", got)
	}
}

func TestGrantAddAndRemoveNeedRootButListDoesNot(t *testing.T) {
	startGrantService(t)
	useHostFixture(t, 1000)
	for _, args := range [][]string{{"grant", "add", "demo-admin", "stack.restart", "plain-stack", "--confirm"}, {"grant", "remove", "g-00000000", "--confirm"}} {
		if code, out, stderr := run(t, args...); code != 1 || out != "" || !strings.Contains(stderr, "must run as root") {
			t.Errorf("%v: %d %q %q", args, code, out, stderr)
		}
	}
	if code, _, stderr := run(t, "grant", "list"); code != 0 {
		t.Fatalf("list: %d %s", code, stderr)
	}
}
