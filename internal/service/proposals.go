package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/backup"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/notifier"
	"github.com/castrowithcee/qatlas-host/internal/proposals"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

// proposalHooks holds what the normalization of the kinds needs from the running service.
type proposalHooks struct {
	log *slog.Logger
	// stackKnown tells whether a stack of that name is among those the service deploys; kinds that name a
	// stack are refused without it.
	stackKnown func(name string) bool
	// channelKnown tells whether a notification channel of that name exists, like the check of notify.add.
	channelKnown func(name string) (bool, error)
	// grants checks a proposal of kind grant like grant.add does; kinds of grants are refused without it.
	grants *grantService
}

// registerProposalCommands keeps the proposals of the service in a store below stateDir and registers its
// commands.
func registerProposalCommands(r *control.Registry, stateDir, passwd string, now func() time.Time, h *proposalHooks) error {
	return proposals.New(stateDir, passwd, now, h.log, h.normalize).Register(r)
}

var invalid = proposals.Invalid

// proposalNormalizers has one function per kind of proposal. Each checks the content of its kind with the
// rules of the hand-typed input and returns it in its canonical form, the one the fingerprint covers. It
// decodes strictly: a field the schema does not name is refused. It never runs, evaluates or expands the
// content, and looks at nothing but the content and, where the kind names a stack, whether the stack exists.
var proposalNormalizers = map[string]func(p *proposalHooks, content json.RawMessage) (any, *control.Error){
	control.ProposalKindSourceAdd:     (*proposalHooks).normalizeSourceAdd,
	control.ProposalKindStackSettings: (*proposalHooks).normalizeStackSettings,
	control.ProposalKindWindow:        (*proposalHooks).normalizeWindow,
	control.ProposalKindNotifyChannel: (*proposalHooks).normalizeNotifyChannel,
}

func (p *proposalHooks) normalize(ctx context.Context, kind string, content json.RawMessage) (any, *control.Error) {
	if kind == control.ProposalKindGrant {
		return p.normalizeGrant(ctx, content)
	}
	if n, ok := proposalNormalizers[kind]; ok {
		return n(p, content)
	}
	return nil, invalid("unknown proposal kind %q", printable(kind))
}

func (p *proposalHooks) normalizeSourceAdd(content json.RawMessage) (any, *control.Error) {
	var c control.ProposalSourceAdd
	if err := proposals.DecodeStrict(content, &c); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object with the fields path and url only", control.ProposalKindSourceAdd)
	}
	if err := checkDirShape(c.Path); err != nil {
		return nil, invalid("the path is not valid: %s", err)
	}
	if c.URL != "" {
		if err := gitrepo.ValidateURL(c.URL); err != nil {
			return nil, invalid("the url is not valid: %s", err)
		}
	}
	return c, nil
}

// normalizeStackSettings checks the settings with stacks.ParseSettings, the check of stack.settings.set, and
// stores them with every default written out, as stack.settings.get answers them. The schema has no secret
// member, so a secret field is an unknown field and refuses the proposal; nothing is dropped.
func (p *proposalHooks) normalizeStackSettings(content json.RawMessage) (any, *control.Error) {
	var c control.ProposalStackSettings
	if err := proposals.DecodeStrict(content, &c); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object with the fields stack and settings only, "+
			"and settings in the schema of 'qhost stack settings'", control.ProposalKindStackSettings)
	}
	if !backup.ValidStack(c.Stack) {
		return nil, invalid("the stack name is not valid")
	}
	data, err := json.Marshal(c.Settings)
	if err != nil {
		return nil, invalid("the settings cannot be encoded")
	}
	rules, err := stacks.ParseSettings(data)
	if err != nil {
		return nil, invalid("%s", printable(err.Error()))
	}
	if p.stackKnown == nil || !p.stackKnown(c.Stack) {
		return nil, control.NewError(control.CodeInvalid, "the stack is not known to the service")
	}
	canonical, err := settingsOf(rules)
	if err != nil {
		return nil, invalid("the settings cannot be encoded")
	}
	return control.ProposalStackSettings{Stack: c.Stack, Settings: canonical}, nil
}

// normalizeWindow checks the parts with the parsers of window.set and stores each in its canonical form. It
// writes nothing. The schema has no secret member, so any other field refuses the proposal.
func (p *proposalHooks) normalizeWindow(content json.RawMessage) (any, *control.Error) {
	var c control.WindowSetArgs
	if err := proposals.DecodeStrict(content, &c); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object with the fields days, start, duration and zone only", control.ProposalKindWindow)
	}
	if c.Days == nil && c.Start == "" && c.Duration == "" && c.Zone == "" {
		return nil, invalid("name at least one part of the window")
	}
	change, err := windowChange(c)
	if err != nil {
		var ie *window.InvalidError
		if errors.As(err, &ie) {
			return nil, invalid("%s", printable(ie.Text))
		}
		return nil, invalid("the window is not valid")
	}
	var out control.WindowSetArgs
	if change.Days != nil {
		out.Days = change.Days.Names()
	}
	if change.Start != nil {
		out.Start = window.FormatStart(*change.Start)
	}
	if change.Duration != nil {
		out.Duration = window.FormatDuration(*change.Duration)
	}
	if change.Zone != nil {
		out.Zone = *change.Zone
	}
	return out, nil
}

// normalizeNotifyChannel checks the channel with notifier.Validate without secrets, the check of notify.add
// for everything but them, and stores it with the defaults of its type written out. A member that carries a
// secret part is refused by name, before any other check, and every other member the schema does not name is
// refused too. A name that is taken is refused as notify.add refuses it. It reaches no network.
func (p *proposalHooks) normalizeNotifyChannel(content json.RawMessage) (any, *control.Error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(content, &members); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object", control.ProposalKindNotifyChannel)
	}
	for _, f := range control.ProposalNotifySecretFields {
		if _, named := members[f]; named {
			return nil, invalid("a proposal cannot carry a secret part of a channel; the person who approves enters it")
		}
	}
	var c control.ProposalNotifyChannel
	if err := proposals.DecodeStrict(content, &c); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object with the fields name, type, events, "+
			"host, port, security, user, from, to, chat_id and base_url only", control.ProposalKindNotifyChannel)
	}
	ch, err := notifier.Validate(notifier.Channel{Name: c.Name, Type: c.Type, Events: c.Events, Host: c.Host, Port: c.Port,
		Security: c.Security, User: c.User, From: c.From, To: c.To, ChatID: c.ChatID, BaseURL: c.BaseURL}, nil)
	if err != nil {
		var ie *notifier.InvalidError
		if errors.As(err, &ie) {
			return nil, invalid("%s", printable(ie.Text))
		}
		return nil, invalid("the channel is not valid")
	}
	if p.channelKnown != nil {
		known, err := p.channelKnown(ch.Name)
		if err != nil {
			p.log.Error("proposals: read the channels", "error", err)
			return nil, control.NewError(control.CodeInternal, "internal error")
		}
		if known {
			return nil, invalid("a channel of this name exists; remove it first")
		}
	}
	return control.ProposalNotifyChannel{Name: ch.Name, Type: ch.Type, Events: ch.Events, Host: ch.Host, Port: ch.Port,
		Security: ch.Security, User: ch.User, From: ch.From, To: ch.To, ChatID: ch.ChatID, BaseURL: ch.BaseURL}, nil
}
