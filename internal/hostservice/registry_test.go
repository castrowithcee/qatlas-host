package hostservice

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/proposals"
)

func testRegistry(t *testing.T) (*control.Registry, map[string]string) {
	t.Helper()
	store := proposals.New(t.TempDir(), "", nil, nil, nil)
	reg, actions, err := (&server{}).registry(store)
	if err != nil {
		t.Fatal(err)
	}
	return reg, actions
}

func TestEveryCommandIsClassifiedWithTheHostScope(t *testing.T) {
	reg, actions := testRegistry(t)
	want := map[string]control.Class{
		control.StatusCommand:             control.ClassRead,
		control.HostLogListCommand:        control.ClassRead,
		control.HostLogVerifyCommand:      control.ClassRead,
		control.ProposalListCommand:       control.ClassRead,
		control.ProposalShowCommand:       control.ClassRead,
		control.ProposalSubmitCommand:     control.ClassPropose,
		control.ProposalWithdrawCommand:   control.ClassPropose,
		control.HostRecordCommand:         control.ClassChange,
		control.HostCommandsSetCommand:    control.ClassChange,
		control.HostCommandsStatusCommand: control.ClassRead,
		control.HostUsageCommand:          control.ClassRead,
		control.ProposalApproveCommand:    control.ClassChange,
		control.ProposalRejectCommand:     control.ClassChange,
		control.ProposalResultCommand:     control.ClassChange,
	}
	entries := reg.Entries()
	if len(entries) != len(want) {
		t.Fatalf("%d commands registered, %d classified", len(entries), len(want))
	}
	for _, e := range entries {
		class, ok := want[e.Name]
		if !ok {
			t.Errorf("command %s is not classified", e.Name)
			continue
		}
		if e.Class != class || e.Scope != control.ScopeHost {
			t.Errorf("%s: class %s scope %s, want %s and host", e.Name, e.Class, e.Scope, class)
		}
		if actions[e.Name] == "" {
			t.Errorf("%s has no log action", e.Name)
		}
	}
}

// refusal reports the code of an access refusal, or "" when the request got past the access decision.
func refusal(err error) string {
	var ce *control.Error
	if errors.As(err, &ce) {
		switch ce.Code {
		case control.CodeNotRoot, control.CodeNotAdmin, control.CodeNoAccess:
			return ce.Code
		}
	}
	return ""
}

func TestAccessFollowsTheClassOfEachCommand(t *testing.T) {
	reg, _ := testRegistry(t)
	users := []struct {
		name       string
		uid        uint32
		refuseFrom func(control.Class) string
	}{
		{"root", 0, func(control.Class) string { return "" }},
		{"administrator", 4100, func(c control.Class) string {
			if c == control.ClassChange {
				return control.CodeNotRoot
			}
			return ""
		}},
		{"user without the administrator right", 4102, func(control.Class) string { return control.CodeNotAdmin }},
	}
	names := reg.Names()
	for _, u := range users {
		e := start(t, u.uid)
		for _, name := range names {
			cmd, _ := reg.Lookup(name)
			var out json.RawMessage
			err := e.do(name, json.RawMessage(`{}`), &out)
			if got, want := refusal(err), u.refuseFrom(cmd.Class); got != want {
				t.Errorf("%s runs %s (%s): refusal %q, want %q (err %v)", u.name, name, cmd.Class, got, want, err)
			}
		}
	}
	if !slices.IsSorted(names) {
		t.Fatal("names are not sorted")
	}
}
