package service

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/backup"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/grants"
)

// grantInspectWait bounds the lookup of a container before a grant decides on it.
const grantInspectWait = 10 * time.Second

type grantKey struct{}

// withGrant returns ctx carrying the grant a request runs under.
func withGrant(ctx context.Context, g control.Grant) context.Context {
	return context.WithValue(ctx, grantKey{}, g)
}

// grantOf returns the grant the request a handler serves runs under; false for a request that needs none.
func grantOf(ctx context.Context) (control.Grant, bool) {
	g, ok := ctx.Value(grantKey{}).(control.Grant)
	return g, ok
}

// cover is the grantCheck of the service. The request must have the one narrow form of a grantable action, and
// the grants file, read now, must hold a valid grant of the user for that action and stack. An unreadable file
// or user database, arguments that do not decode strictly and a container without a stack of this instance
// all mean no grant.
func (g *grantService) cover(ctx context.Context, uid uint32, command string, raw json.RawMessage) (control.Grant, bool) {
	action, stack, ok := g.narrow(ctx, command, raw)
	if !ok || !backup.ValidStack(stack) {
		return control.Grant{}, false
	}
	list, err := g.store.List()
	if err != nil {
		g.log.Warn("the grants file cannot be read; no grant counts", "error", printable(err.Error()))
		return control.Grant{}, false
	}
	now := g.now()
	for _, x := range list {
		if x.UID == uid && x.Action == action && x.Stack == stack && grants.Invalid(x, now, g.stackKnown, g.nameOf) == "" {
			return x, true
		}
	}
	return control.Grant{}, false
}

// narrow returns the grantable action and the stack of a request that has exactly the form a grant of that
// action covers, decoded at least as strictly as its handler decodes it; ok is false for every other request.
func (g *grantService) narrow(ctx context.Context, command string, raw json.RawMessage) (action, stack string, ok bool) {
	switch command {
	case control.StackRestartCommand:
		var a control.StackActionArgs
		if !strictArgs(raw, &a, "stack") {
			return "", "", false
		}
		return control.GrantActionStackRestart, a.Stack, true
	case control.UpdatesListCommand:
		var a control.UpdatesArgs
		if !strictArgs(raw, &a, "stack", "refresh", "changelog") || !a.Refresh || a.Changelog {
			return "", "", false
		}
		return control.GrantActionUpdateCheck, a.Stack, true
	case control.UpdateRunCommand:
		var a control.UpdateRunArgs
		if !strictArgs(raw, &a, "stacks", "version", "variable") || len(a.Stacks) != 1 || a.Version != "" || a.Variable != "" {
			return "", "", false
		}
		return control.GrantActionUpdateRun, a.Stacks[0], true
	case control.ContainerRestartCommand:
		var a control.ContainerActionArgs
		if !strictArgs(raw, &a, "name", "grace_seconds") || a.Name == "" || a.GraceSeconds != 0 || g.containerStack == nil {
			return "", "", false
		}
		ctx, cancel := context.WithTimeout(ctx, grantInspectWait)
		defer cancel()
		stack, ok := g.containerStack(ctx, a.Name)
		return control.GrantActionContainerRestart, stack, ok
	}
	return "", "", false
}

// strictArgs decodes raw into a as its handler does and also requires one JSON object whose members are among
// names, each spelled exactly so and present once: encoding/json would take another letter case or a repeated
// member too, and the last one would win.
func strictArgs(raw json.RawMessage, a any, names ...string) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for dec.More() {
		t, err := dec.Token()
		name, ok := t.(string)
		if err != nil || !ok || seen[name] || !slices.Contains(names, name) {
			return false
		}
		seen[name] = true
		var value json.RawMessage
		if dec.Decode(&value) != nil {
			return false
		}
	}
	return decodeArgs(raw, a) == nil
}
