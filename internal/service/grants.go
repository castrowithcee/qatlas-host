package service

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/backup"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/grants"
)

// userDirectory is what the grant commands ask the user database. admins.Database is the real one; its answers
// are read from the files on every call.
type userDirectory interface {
	UID(name string) (uid uint32, ok bool, err error)
	Name(uid uint32) (name string, ok bool, err error)
	Admin(uid uint32) (bool, error)
}

// grantService serves the grant commands and decides, by cover, whether a grant lets a request run.
type grantService struct {
	store *grants.Store
	users userDirectory
	now   func() time.Time
	// stackKnown tells whether the instance deploys a stack of that name.
	stackKnown func(name string) bool
	// stackSetup tells whether the stack has settings at all, a backup and a health check.
	stackSetup func(ctx context.Context, name string) (settings, backup, check bool)
	// containerStack returns the stack of this instance a container belongs to; nil knows none.
	containerStack func(ctx context.Context, ref string) (string, bool)
	log            *slog.Logger
}

// registerGrantCommands registers grant.add, grant.remove and grant.list. None of them is offered as a grant
// itself: add and remove need root, and the actions a grant can name do not include them.
func registerGrantCommands(r *control.Registry, g *grantService) error {
	// grant.remove takes the stack as an optional argument: the ID alone names the grant, and a request of
	// scope stack without a stack is one of scope deploy (see Command.AccessFor).
	commands := map[string]control.Command{
		control.GrantAddCommand:    inStack(changing(g.handleAdd), stackField),
		control.GrantRemoveCommand: inStack(changing(g.handleRemove), stackField),
		control.GrantListCommand:   reading(g.handleList),
	}
	for name, c := range commands {
		if err := r.Register(name, c); err != nil {
			return err
		}
	}
	return nil
}

func (g *grantService) time() time.Time { return g.now().UTC().Truncate(time.Second) }

// load reads the grants; a file that cannot be read is answered, never changed.
func (g *grantService) load() ([]control.Grant, error) {
	list, err := g.store.List()
	if err != nil {
		g.log.Warn("the grants file cannot be read; no grant counts", "error", printable(err.Error()))
		return nil, control.NewError(control.CodeGrantsUnreadable, "the grants file cannot be read and is left unchanged; no grant counts until it is dealt with")
	}
	return list, nil
}

func (g *grantService) handleAdd(ctx context.Context, raw json.RawMessage) (any, error) {
	var a control.GrantAddArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if !slices.Contains(control.GrantActions, a.Action) {
		return nil, control.NewError(control.CodeGrantAction,
			"the action cannot be granted; grantable are "+strings.Join(control.GrantActions, ", "))
	}
	if !backup.ValidStack(a.Stack) {
		return nil, control.NewError(control.CodeInvalid, "the stack name is not valid")
	}
	now := g.time()
	if a.Expires != "" {
		day, err := grants.ParseExpires(a.Expires)
		if err != nil {
			return nil, control.NewError(control.CodeInvalid, "the expiry is not a date of the form YYYY-MM-DD")
		}
		if !now.Before(day.AddDate(0, 0, 1)) {
			return nil, control.NewError(control.CodeInvalid, "the expiry date is in the past")
		}
	}
	uid, name, err := g.grantee(a.User)
	if err != nil {
		return nil, err
	}
	if !g.stackKnown(a.Stack) {
		return nil, control.NewError(control.CodeNoSuchStack, "this instance has no such stack")
	}
	list, err := g.load()
	if err != nil {
		return nil, err
	}
	want := control.Grant{User: name, UID: uid, Action: a.Action, Stack: a.Stack, Expires: a.Expires, CreatedAt: now, CreatedBy: a.SudoUID}
	res := control.GrantAddResult{Grant: want}
	if i := slices.IndexFunc(list, func(x control.Grant) bool {
		return x.UID == uid && x.Action == a.Action && x.Stack == a.Stack
	}); i >= 0 {
		res.Replaced = true
		res.Grant = list[i]
		res.Grant.User, res.Grant.Expires = name, a.Expires
	} else if len(list) >= control.GrantMax {
		return nil, control.NewError(control.CodeGrantLimit, "the instance has the most grants it may have; remove one first")
	}
	res.Settings, res.Backup, res.Check = g.stackSetup(ctx, a.Stack)
	if !a.Confirm {
		return res, nil
	}
	stored, replaced, err := g.store.Upsert(want)
	switch {
	case errors.Is(err, grants.ErrFull):
		return nil, control.NewError(control.CodeGrantLimit, "the instance has the most grants it may have; remove one first")
	case err != nil:
		return nil, err
	}
	res.Grant, res.Replaced, res.Applied = stored, replaced, true
	g.log.Info("grant added", "id", stored.ID, "user", printable(name), "action", stored.Action, "target", printable(stored.Stack),
		"expires", stored.Expires, "replaced", replaced, "by", a.SudoUID)
	return res, nil
}

// grantee resolves a user name to its ID and checks the user has access to the channel, as an administrator in
// the service group. Any doubt about the user database means no access.
func (g *grantService) grantee(user string) (uint32, string, error) {
	if user == "" || len(user) > 256 {
		return 0, "", control.NewError(control.CodeInvalid, "the user name is not valid")
	}
	uid, ok, err := g.users.UID(user)
	switch {
	case err != nil:
		g.log.Error("the user database cannot be read", "error", printable(err.Error()))
		return 0, "", control.NewError(control.CodeInternal, "internal error")
	case !ok:
		return 0, "", control.NewError(control.CodeNoSuchUser, "the host has no such user")
	}
	if admin, err := g.users.Admin(uid); err != nil || !admin {
		return 0, "", control.NewError(control.CodeUserNoAccess, "grants are for users with access to the service only")
	}
	return uid, user, nil
}

func (g *grantService) handleRemove(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.GrantRemoveArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	list, err := g.load()
	if err != nil {
		return nil, err
	}
	// A stack the caller names must be the stack of the grant, or the request was made for another one.
	if a.Stack != "" && !slices.ContainsFunc(list, func(x control.Grant) bool { return x.ID == a.ID && x.Stack == a.Stack }) {
		return nil, control.NewError(control.CodeNoSuchGrant, "no grant has this ID")
	}
	gone, err := g.store.Remove(a.ID)
	switch {
	case errors.Is(err, grants.ErrNotFound):
		return nil, control.NewError(control.CodeNoSuchGrant, "no grant has this ID")
	case err != nil:
		return nil, err
	}
	g.log.Info("grant removed", "id", gone.ID, "user", printable(gone.User), "action", gone.Action, "target", printable(gone.Stack))
	return control.GrantRemoveResult{Grant: gone}, nil
}

func (g *grantService) handleList(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.GrantListArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	res := control.GrantListResult{Grants: []control.GrantInfo{}}
	list, err := g.store.List()
	if err != nil {
		g.log.Warn("the grants file cannot be read; no grant counts", "error", printable(err.Error()))
		res.Problem = printable(err.Error())
		return res, nil
	}
	now := g.now()
	for _, x := range list {
		if a.Stack != "" && x.Stack != a.Stack || a.User != "" && x.User != a.User {
			continue
		}
		info := control.GrantInfo{Grant: x, Invalid: grants.Invalid(x, now, g.stackKnown, g.nameOf)}
		info.User, info.Stack = printable(x.User), printable(x.Stack)
		res.Grants = append(res.Grants, info)
	}
	slices.SortFunc(res.Grants, func(x, y control.GrantInfo) int {
		return cmp.Or(strings.Compare(x.Stack, y.Stack), strings.Compare(x.User, y.User), strings.Compare(x.Action, y.Action))
	})
	return res, nil
}

// nameOf returns the name the user uid has today; an unreadable user database names nobody.
func (g *grantService) nameOf(uid uint32) (string, bool) {
	name, ok, err := g.users.Name(uid)
	return name, ok && err == nil
}

// stackSetup reads the settings of a stack for the plan of a grant. A stack whose settings cannot be read has
// none for this purpose: the plan then shows no backup and no check, which is the cautious answer.
func (s *sourceSet) stackSetup(ctx context.Context, name string) (settings, backup, check bool) {
	ctx, cancel := context.WithTimeout(ctx, control.StackSettingsTimeout)
	defer cancel()
	rules, err := s.StackSettings(ctx, name)
	if err != nil || rules == nil {
		return false, false, false
	}
	return true, rules.Backup != nil, rules.Check.Containers || rules.Check.HTTP != nil
}
