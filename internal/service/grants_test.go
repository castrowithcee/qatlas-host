package service_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
	"github.com/castrowithcee/qatlas-host/internal/service/servicetest"
)

func grantHarness(t *testing.T) (*harness, *peers) {
	t.Helper()
	p := &peers{}
	var seen atomic.Pointer[control.Registry]
	h := boundWith(t, &fakeDeployer{}, map[string]string{servicetest.Settings("plain-stack"): backupRules}, withPeers(t, p, &seen))
	return h, p
}

func grantsFile(h *harness) string { return filepath.Join(h.state, "grants.json") }

func listGrants(t *testing.T, h *harness, args control.GrantListArgs) control.GrantListResult {
	t.Helper()
	var res control.GrantListResult
	if err := h.call(control.GrantListCommand, args, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestGrantAddPlansFirstThenStoresAndListsIt(t *testing.T) {
	h, _ := grantHarness(t)
	args := control.GrantAddArgs{User: "example-admin", Action: "stack.restart", Stack: "plain-stack", Expires: "2999-12-31", SudoUID: 1000}

	var plan control.GrantAddResult
	if err := h.call(control.GrantAddCommand, args, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Applied || plan.Replaced || plan.Grant.ID != "" || plan.Grant.UID != 4100 || plan.Grant.CreatedBy != 1000 || !plan.Settings || !plan.Backup || !plan.Check {
		t.Fatalf("plan = %+v", plan)
	}
	if _, err := os.Stat(grantsFile(h)); !os.IsNotExist(err) {
		t.Fatalf("a plan wrote the grants file: %v", err)
	}
	var bare control.GrantAddResult
	other := args
	other.Stack = "demo-stack"
	if err := h.call(control.GrantAddCommand, other, &bare); err != nil || bare.Backup {
		t.Fatalf("plan of a stack without a backup = %+v, %v", bare, err)
	}

	args.Confirm = true
	var added control.GrantAddResult
	if err := h.call(control.GrantAddCommand, args, &added); err != nil {
		t.Fatal(err)
	}
	if !added.Applied || !regexp.MustCompile(`^g-[0-9a-f]{8}$`).MatchString(added.Grant.ID) || added.Grant.User != "example-admin" ||
		added.Grant.Expires != "2999-12-31" || added.Grant.CreatedBy != 1000 || added.Grant.CreatedAt.IsZero() {
		t.Fatalf("added = %+v", added)
	}
	info, err := os.Stat(grantsFile(h))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file = %v, %v", info, err)
	}
	list := listGrants(t, h, control.GrantListArgs{})
	if list.Problem != "" || len(list.Grants) != 1 || list.Grants[0].Grant != added.Grant || list.Grants[0].Invalid != "" {
		t.Fatalf("list = %+v", list)
	}

	// The same grant is not stored twice: a new expiry replaces the old one and the ID stays. Without a
	// sudo user the origin is 0, and the first one is kept.
	again := args
	again.Expires, again.SudoUID = "", 0
	var replaced control.GrantAddResult
	if err := h.call(control.GrantAddCommand, again, &replaced); err != nil {
		t.Fatal(err)
	}
	if !replaced.Replaced || replaced.Grant.ID != added.Grant.ID || replaced.Grant.Expires != "" || replaced.Grant.CreatedBy != 1000 {
		t.Fatalf("replaced = %+v", replaced)
	}
	if list := listGrants(t, h, control.GrantListArgs{}); len(list.Grants) != 1 || list.Grants[0].Expires != "" {
		t.Fatalf("list after replace = %+v", list)
	}

	var unset control.GrantAddResult
	noSudo := control.GrantAddArgs{User: "example-admin", Action: "update.check", Stack: "plain-stack", Confirm: true}
	if err := h.call(control.GrantAddCommand, noSudo, &unset); err != nil || unset.Grant.CreatedBy != 0 {
		t.Fatalf("add without sudo user = %+v, %v", unset, err)
	}
	if got := listGrants(t, h, control.GrantListArgs{Stack: "demo-stack"}); len(got.Grants) != 0 {
		t.Fatalf("filter by stack = %+v", got)
	}
	if got := listGrants(t, h, control.GrantListArgs{User: "example-admin", Stack: "plain-stack"}); len(got.Grants) != 2 {
		t.Fatalf("filter by user and stack = %+v", got)
	}

	var removed control.GrantRemoveResult
	if err := h.call(control.GrantRemoveCommand, control.GrantRemoveArgs{ID: added.Grant.ID}, &removed); err != nil || removed.Grant.ID != added.Grant.ID {
		t.Fatalf("remove = %+v, %v", removed, err)
	}
	if list := listGrants(t, h, control.GrantListArgs{}); len(list.Grants) != 1 || list.Grants[0].Action != "update.check" {
		t.Fatalf("list after remove = %+v", list)
	}
	wantCode(t, h.call(control.GrantRemoveCommand, control.GrantRemoveArgs{ID: added.Grant.ID}, nil), control.CodeNoSuchGrant)
	// A named stack must be the stack of the grant.
	kept := listGrants(t, h, control.GrantListArgs{}).Grants[0]
	wantCode(t, h.call(control.GrantRemoveCommand, control.GrantRemoveArgs{ID: kept.ID, Stack: "demo-stack"}, nil), control.CodeNoSuchGrant)
	if err := h.call(control.GrantRemoveCommand, control.GrantRemoveArgs{ID: kept.ID, Stack: "plain-stack"}, nil); err != nil {
		t.Fatal(err)
	}
	if list := listGrants(t, h, control.GrantListArgs{}); len(list.Grants) != 0 {
		t.Fatalf("list after the second remove = %+v", list)
	}
	wantCode(t, h.call(control.GrantRemoveCommand, control.GrantRemoveArgs{ID: "g-ffffffff"}, nil), control.CodeNoSuchGrant)
	wantCode(t, h.call(control.GrantRemoveCommand, map[string]any{"id": "g-ffffffff", "extra": 1}, nil), control.CodeBadRequest)
}

func TestGrantAddRefusesWhatCannotBeGrantedWithItsOwnError(t *testing.T) {
	h, _ := grantHarness(t)
	good := control.GrantAddArgs{User: "example-admin", Action: "update.run", Stack: "plain-stack", Confirm: true}
	for _, tc := range []struct {
		name string
		edit func(*control.GrantAddArgs)
		code string
	}{
		{"unknown action", func(a *control.GrantAddArgs) { a.Action = "stack.stop" }, control.CodeGrantAction},
		{"an action of the grants themselves", func(a *control.GrantAddArgs) { a.Action = "grant.add" }, control.CodeGrantAction},
		{"no action", func(a *control.GrantAddArgs) { a.Action = "" }, control.CodeGrantAction},
		{"unknown user", func(a *control.GrantAddArgs) { a.User = "example-nobody" }, control.CodeNoSuchUser},
		{"user without access", func(a *control.GrantAddArgs) { a.User = "example-reader" }, control.CodeUserNoAccess},
		{"root", func(a *control.GrantAddArgs) { a.User = "root" }, control.CodeNoSuchUser},
		{"unknown stack", func(a *control.GrantAddArgs) { a.Stack = "no-such-stack" }, control.CodeNoSuchStack},
		{"invalid stack name", func(a *control.GrantAddArgs) { a.Stack = "../x" }, control.CodeInvalid},
		{"expiry not a date", func(a *control.GrantAddArgs) { a.Expires = "tomorrow" }, control.CodeInvalid},
		{"expiry in the past", func(a *control.GrantAddArgs) { a.Expires = "2000-01-01" }, control.CodeInvalid},
	} {
		a := good
		tc.edit(&a)
		wantCode(t, h.call(control.GrantAddCommand, a, nil), tc.code)
	}
	if _, err := os.Stat(grantsFile(h)); !os.IsNotExist(err) {
		t.Fatalf("a refused request wrote the grants file: %v", err)
	}
	if err := h.call(control.GrantAddCommand, good, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGrantCommandsThatChangeNeedRoot(t *testing.T) {
	h, p := grantHarness(t)
	p.as(exampleAdmin)
	add := control.GrantAddArgs{User: "example-admin", Action: "stack.restart", Stack: "plain-stack", Confirm: true}
	wantCode(t, h.call(control.GrantAddCommand, add, nil), control.CodeNotRoot)
	wantCode(t, h.call(control.GrantRemoveCommand, control.GrantRemoveArgs{ID: "g-ffffffff"}, nil), control.CodeNotRoot)
	if res := listGrants(t, h, control.GrantListArgs{}); len(res.Grants) != 0 {
		t.Fatalf("list = %+v", res)
	}
	p.as(exampleReader)
	wantCode(t, h.call(control.GrantListCommand, nil, nil), control.CodeNoAccess)
	p.as(0)
	if _, err := os.Stat(grantsFile(h)); !os.IsNotExist(err) {
		t.Fatalf("a refused request wrote the grants file: %v", err)
	}
}

const (
	grantRow = `{"id":"%s","user":"%s","uid":%d,"action":"%s","stack":"%s"%s,"created_at":"2030-01-01T00:00:00Z","created_by":0}`
)

func TestGrantListNamesWhyAGrantIsInvalid(t *testing.T) {
	h, _ := grantHarness(t)
	row := func(id, user string, uid int, action, stack, expires string) string {
		if expires != "" {
			expires = `,"expires":"` + expires + `"`
		}
		return fmt.Sprintf(grantRow, id, user, uid, action, stack, expires)
	}
	doc := `{"schema":1,"grants":[` +
		row("g-00000001", "example-admin", 4100, "stack.restart", "plain-stack", "") + "," +
		row("g-00000002", "example-admin", 4100, "update.check", "plain-stack", "2020-01-01") + "," +
		row("g-00000003", "example-admin", 4100, "update.run", "gone-stack", "") + "," +
		row("g-00000004", "example-old-name", 4100, "container.restart", "plain-stack", "") + "," +
		row("g-00000005", "example-admin", 4100, "stack.restart", "demo-stack", "2999-12-31") + `]}`
	formatnumtest.Seed(t, grantsFile(h), doc)

	res := listGrants(t, h, control.GrantListArgs{})
	got := map[string]string{}
	for _, g := range res.Grants {
		got[g.ID] = g.Invalid
	}
	want := map[string]string{
		"g-00000001": "", "g-00000002": control.GrantExpired, "g-00000003": control.GrantStackMissing,
		"g-00000004": control.GrantUserChanged, "g-00000005": "",
	}
	if res.Problem != "" || len(got) != len(want) {
		t.Fatalf("list = %+v", res)
	}
	for id, reason := range want {
		if got[id] != reason {
			t.Errorf("%s: invalid = %q, want %q", id, got[id], reason)
		}
	}
	if by := listGrants(t, h, control.GrantListArgs{User: "example-old-name"}); len(by.Grants) != 1 || by.Grants[0].ID != "g-00000004" {
		t.Fatalf("filter by user = %+v", by)
	}
	// An invalid grant is replaced like any other: adding it again with a new expiry makes it valid.
	var renewed control.GrantAddResult
	err := h.call(control.GrantAddCommand, control.GrantAddArgs{User: "example-admin", Action: "update.check", Stack: "plain-stack",
		Expires: "2999-01-01", Confirm: true}, &renewed)
	if err != nil || !renewed.Replaced || renewed.Grant.ID != "g-00000002" {
		t.Fatalf("renew = %+v, %v", renewed, err)
	}
	for _, g := range listGrants(t, h, control.GrantListArgs{}).Grants {
		if g.ID == "g-00000002" && g.Invalid != "" {
			t.Fatalf("renewed grant is %s", g.Invalid)
		}
	}
}

func TestDamagedGrantsFileStaysAsItIsAndIsReported(t *testing.T) {
	h, _ := grantHarness(t)
	for name, doc := range map[string]string{
		"not json":  `{"schema":1,"grants":[`,
		"too new":   `{"schema":2,"grants":[],"added_later":true}`,
		"no number": `{"grants":[]}`,
	} {
		formatnumtest.Seed(t, grantsFile(h), doc)
		res := listGrants(t, h, control.GrantListArgs{})
		if res.Problem == "" || len(res.Grants) != 0 {
			t.Errorf("%s: list = %+v, want a problem and no grant", name, res)
		}
		wantCode(t, h.call(control.GrantAddCommand,
			control.GrantAddArgs{User: "example-admin", Action: "stack.restart", Stack: "plain-stack", Confirm: true}, nil), control.CodeGrantsUnreadable)
		wantCode(t, h.call(control.GrantAddCommand,
			control.GrantAddArgs{User: "example-admin", Action: "stack.restart", Stack: "plain-stack"}, nil), control.CodeGrantsUnreadable)
		wantCode(t, h.call(control.GrantRemoveCommand, control.GrantRemoveArgs{ID: "g-00000001"}, nil), control.CodeGrantsUnreadable)
		formatnumtest.Unchanged(t, grantsFile(h), doc)
	}
}

func TestGrantCommandsAreChangesOfScopeStackExceptList(t *testing.T) {
	p := &peers{}
	var seen atomic.Pointer[control.Registry]
	boundWith(t, &fakeDeployer{}, nil, withPeers(t, p, &seen))
	reg := seen.Load()
	for command, want := range map[string]struct {
		class control.Class
		scope control.Scope
	}{
		control.GrantAddCommand:    {control.ClassChange, control.ScopeStack},
		control.GrantRemoveCommand: {control.ClassChange, control.ScopeStack},
		control.GrantListCommand:   {control.ClassRead, control.ScopeDeploy},
	} {
		c, ok := reg.Lookup(command)
		if !ok || c.Class != want.class || c.Scope != want.scope {
			t.Errorf("%s = %+v, %v", command, c, ok)
		}
	}
	// grant.remove needs only the ID, so a request with the ID alone is one of scope deploy.
	rm, _ := reg.Lookup(control.GrantRemoveCommand)
	if a := rm.AccessFor([]byte(`{"id":"g-00000001"}`)); a.Scope != control.ScopeDeploy || a.Class != control.ClassChange {
		t.Errorf("grant.remove access = %+v", a)
	}
	add, _ := reg.Lookup(control.GrantAddCommand)
	if a := add.AccessFor([]byte(`{"stack":"plain-stack"}`)); a.Scope != control.ScopeStack || a.Stack != "plain-stack" {
		t.Errorf("grant.add access = %+v", a)
	}
	if slices.Contains(control.GrantActions, control.GrantAddCommand) || slices.Contains(control.GrantActions, control.GrantRemoveCommand) {
		t.Error("a grant can name the grant commands")
	}
}
