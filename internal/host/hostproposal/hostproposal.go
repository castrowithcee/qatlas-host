// Package hostproposal is the check of proposals of changes of the host: which actions of the host administration
// may be proposed, and whether the arguments and keys of one fit the schema of its action. It builds plans only to
// check and show them and never reads, runs or changes anything on the host, so the host service checks a
// proposal without root, and the command that applies it checks it again with the same rules.
package hostproposal

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// viewPassword stands in for the initial password of user.create where a plan is built to check or show a
// proposal; it never reaches a command that runs.
const viewPassword = "proposal-view-only"

// notProposable are the actions a proposal may not name, with the reason. All are what the person decides alone.
var notProposable = map[string]string{
	hostchange.ActionUserSetPassword:  "a password is never part of a proposal",
	hostchange.ActionUserSetAdmin:     "administrator rights are given by a person, not proposed",
	hostchange.ActionFileWrite:        "file contents are never part of a proposal; agents get no access to files",
	hostchange.ActionFileCreateFolder: "files and folders are changed by a person; agents get no access to files",
	hostchange.ActionFileRename:       "files and folders are changed by a person; agents get no access to files",
	hostchange.ActionFileCopy:         "files and folders are changed by a person; agents get no access to files",
	hostchange.ActionFileRemove:       "files and folders are changed by a person; agents get no access to files",
}

// accessGroups are the groups whose members a proposal may not change: being in both is administrator access to
// the host and to qhost, which is what user.set-admin gives.
var accessGroups = []string{hostadmin.GroupAdmin, hostadmin.GroupQhost}

// Checker checks proposals against the actions of one registry.
type Checker struct{ r *hostchange.Registry }

// New returns the checker of the actions of r.
func New(r *hostchange.Registry) Checker { return Checker{r: r} }

// Default is the checker of the production actions. Building it reads nothing from the host.
func Default() Checker { return New(hostchange.Default()) }

// Check refuses a proposal of a change that may not be proposed or whose arguments or keys do not fit the
// schema of the action, and returns the keys in canonical form. The protection rules that depend on the host
// apply when the proposal is approved, not here.
func (c Checker) Check(action string, args, keys []string) ([]string, error) {
	if reason, ok := notProposable[action]; ok {
		return nil, fmt.Errorf("%s cannot be proposed: %s", action, reason)
	}
	if _, ok := c.r.Lookup(action); !ok {
		return nil, fmt.Errorf("unknown action %q; propose one of the actions that 'qhost help host-admin' lists", action)
	}
	switch {
	case action == hostchange.ActionUserCreate && len(args) > 1 && slices.Contains(args[1:], hostchange.OptionAdmin):
		return nil, errors.New("user.create cannot be proposed with the admin switch: administrator rights are given by a person")
	case action == hostchange.ActionGroupSetMembers && len(args) > 0 && slices.Contains(accessGroups, args[0]):
		return nil, fmt.Errorf("the members of %s cannot be proposed: membership in %s and %s is administrator access, given by a person",
			args[0], hostadmin.GroupAdmin, hostadmin.GroupQhost)
	}
	canonical := make([]string, 0, len(keys))
	for _, text := range keys {
		k, err := hostchange.ParseKey(text)
		if err != nil {
			return nil, err
		}
		canonical = append(canonical, k.Line)
	}
	if len(canonical) == 0 {
		canonical = nil
	}
	if _, _, err := c.Plan(action, args, canonical, viewPassword); err != nil {
		return nil, err
	}
	return canonical, nil
}

// View is the plan of a proposal as the person sees it before approving: the sentence and the commands, the
// initial password of user.create as its placeholder.
func (c Checker) View(action string, args, keys []string) (hostchange.Plan, error) {
	plan, _, err := c.Plan(action, args, keys, viewPassword)
	return plan, err
}

// Plan builds the plan of a proposal and the secret input of its action: for user.create the initial password
// the person entered and the proposed keys, for user.add-key the one proposed key. It reads nothing from the host.
func (c Checker) Plan(action string, args, keys []string, password string) (hostchange.Plan, []byte, error) {
	secret, err := c.secret(action, keys, password)
	if err != nil {
		return hostchange.Plan{}, nil, err
	}
	plan, err := c.r.Describe(action, args, secret)
	if err != nil {
		return hostchange.Plan{}, nil, fmt.Errorf("the arguments do not fit %s: %w", action, err)
	}
	return plan, secret, nil
}

func (c Checker) secret(action string, keys []string, password string) ([]byte, error) {
	switch action {
	case hostchange.ActionUserCreate:
		return json.Marshal(hostchange.CreateInput{Password: password, Keys: keys})
	case hostchange.ActionUserAddKey:
		if len(keys) != 1 {
			return nil, errors.New("user.add-key takes exactly one SSH key")
		}
		return []byte(keys[0]), nil
	}
	if len(keys) > 0 {
		return nil, fmt.Errorf("%s takes no SSH key", action)
	}
	if d, ok := c.r.Lookup(action); !ok || d.WantsSecret {
		return nil, fmt.Errorf("%s reads a secret input, which a proposal cannot carry", action)
	}
	return nil, nil
}

// NeedsPassword reports whether approving a proposal of action asks the person for the initial password.
func NeedsPassword(action string) bool { return action == hostchange.ActionUserCreate }
