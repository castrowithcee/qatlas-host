package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/grants"
	"github.com/castrowithcee/qatlas-host/internal/proposals"
)

// normalizeGrant checks a proposal of kind grant with the rules of grant.add, for the user of the peer: the
// user is never read from the content, and the schema has no member for one. It changes nothing.
func (p *proposalHooks) normalizeGrant(ctx context.Context, content json.RawMessage) (any, *control.Error) {
	if p.grants == nil {
		return nil, control.NewError(control.CodeInternal, "internal error")
	}
	var c control.ProposalGrantRequest
	if err := proposals.DecodeStrict(content, &c); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object with the fields action, stack, expires and reason only",
			control.ProposalKindGrant)
	}
	if strings.TrimSpace(c.Reason) == "" || utf8.RuneCountInString(c.Reason) > control.ProposalGrantReasonMaxChars {
		return nil, invalid("the reason is required and has at most %d characters", control.ProposalGrantReasonMaxChars)
	}
	uid, ok := control.PeerUID(ctx)
	if !ok {
		return nil, control.NewError(control.CodeInternal, "internal error")
	}
	g := p.grants
	if err := g.checkShape(c.Action, c.Stack, c.Expires, g.time()); err != nil {
		return nil, err
	}
	name, ok := g.nameOf(uid)
	if !ok {
		return nil, control.NewError(control.CodeNoSuchUser, "the host has no such user")
	}
	if !g.stackKnown(c.Stack) {
		return nil, control.NewError(control.CodeNoSuchStack, "this instance has no such stack")
	}
	list, err := g.load()
	if err != nil {
		var ce *control.Error
		if errors.As(err, &ce) {
			return nil, ce
		}
		return nil, control.NewError(control.CodeInternal, "internal error")
	}
	for _, x := range list {
		if x.UID == uid && x.Action == c.Action && x.Stack == c.Stack &&
			grants.Invalid(x, g.time(), g.stackKnown, g.nameOf) == "" {
			return nil, invalid("a valid grant of this action for this user and stack exists already")
		}
	}
	return control.ProposalGrant{User: name, UID: uid, Action: c.Action, Stack: c.Stack, Expires: c.Expires, Reason: c.Reason}, nil
}
