package hostservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/auditlog"
	"github.com/castrowithcee/qatlas-host/internal/host/hostproposal"
	"github.com/castrowithcee/qatlas-host/internal/proposals"
)

// proposalActions is the log action of each command of the proposal store.
var proposalActions = map[string]string{
	control.ProposalSubmitCommand:   auditlog.OperationProposalSubmit,
	control.ProposalWithdrawCommand: auditlog.OperationProposalWithdraw,
	control.ProposalListCommand:     auditlog.OperationProposalRead,
	control.ProposalShowCommand:     auditlog.OperationProposalRead,
	control.ProposalApproveCommand:  auditlog.OperationProposalApprove,
	control.ProposalRejectCommand:   auditlog.OperationProposalReject,
	control.ProposalResultCommand:   auditlog.OperationProposalResult,
}

// proposalCommands adds the commands of the store through add, each with the class the store gives it. The
// handlers learn the peer from their context.
func proposalCommands(add func(name, action string, class control.Class, h control.Handler) error, store *proposals.Store,
	commandsEnabled func() (bool, error)) error {
	for name, c := range store.Commands() {
		action, ok := proposalActions[name]
		if !ok {
			panic(fmt.Sprintf("hostservice: the proposal command %s has no log action", name))
		}
		handle := c.Handle
		if name == control.ProposalApproveCommand {
			handle = gateCommandApproval(handle, store.Commands()[control.ProposalShowCommand].Handle, commandsEnabled)
		}
		if err := add(name, action, c.Class, handle); err != nil {
			return err
		}
	}
	return nil
}

// normalizeProposal is the check of the kinds of the host service. For ProposalKindHostChange it decodes the
// content strictly, refuses a password by name, and checks the action, its arguments and keys with check, which
// reads nothing from the host and runs nothing. ProposalKindCommand is accepted only while commandsEnabled
// says the switch is on.
func normalizeProposal(check hostproposal.Checker, commandsEnabled func() (bool, error)) proposals.Normalizer {
	return func(kind string, content json.RawMessage) (any, *control.Error) {
		if kind == control.ProposalKindCommand {
			return normalizeCommandProposal(content, commandsEnabled)
		}
		if kind != control.ProposalKindHostChange {
			return nil, proposals.Invalid("the host service keeps proposals of the kinds %s and %s only, not %q",
				control.ProposalKindHostChange, control.ProposalKindCommand, proposals.Printable(kind))
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(content, &members); err != nil {
			return nil, proposals.Invalid("the content of a %s proposal must be a JSON object", control.ProposalKindHostChange)
		}
		if _, named := members["password"]; named {
			return nil, proposals.Invalid("a proposal cannot carry a password; the person who approves enters it")
		}
		var c control.ProposalHostChange
		if err := proposals.DecodeStrict(content, &c); err != nil {
			return nil, proposals.Invalid("the content of a %s proposal must be a JSON object with the fields action, args and keys only",
				control.ProposalKindHostChange)
		}
		keys, err := check.Check(c.Action, c.Args, c.Keys)
		if err != nil {
			return nil, proposals.Invalid("%s", proposals.Printable(err.Error()))
		}
		args := c.Args
		if args == nil {
			args = []string{}
		}
		return control.ProposalHostChange{Action: c.Action, Args: args, Keys: keys}, nil
	}
}

// normalizeCommandProposal checks a proposal of kind ProposalKindCommand. The switch is read first, so a refusal
// names it whatever the content is. The commands are only checked for their shape, never run or interpreted.
func normalizeCommandProposal(content json.RawMessage, enabled func() (bool, error)) (any, *control.Error) {
	on, err := enabled()
	if err != nil {
		return nil, control.NewError(control.CodeInternal, "internal error")
	}
	if !on {
		return nil, control.NewError(control.CodeCommandsDisabled,
			"proposals of commands are switched off; an administrator turns them on with 'sudo qhost proposal commands enable'")
	}
	var c control.ProposalCommand
	if err := proposals.DecodeStrict(content, &c); err != nil {
		return nil, proposals.Invalid("the content of a %s proposal must be a JSON object with the fields commands and reason only",
			control.ProposalKindCommand)
	}
	if n := len(c.Commands); n < 1 || n > control.ProposalCommandMax {
		return nil, proposals.Invalid("a %s proposal holds 1 to %d commands", control.ProposalKindCommand, control.ProposalCommandMax)
	}
	for _, line := range c.Commands {
		if !printableLine(line, control.ProposalCommandMaxChars) {
			return nil, proposals.Invalid("each command must be one non-empty line of printable characters, at most %d characters",
				control.ProposalCommandMaxChars)
		}
	}
	if !printableLine(c.Reason, control.ProposalReasonMaxChars) {
		return nil, proposals.Invalid("the reason is required and must be one line of printable characters, at most %d characters",
			control.ProposalReasonMaxChars)
	}
	return c, nil
}

// printableLine reports whether s is valid UTF-8 with printable characters only, so no line break, tab or
// control character, holds something besides spaces, and has at most max characters.
func printableLine(s string, max int) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > max {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsPrint(r) })
}

// gateCommandApproval refuses the approval of a proposal of kind ProposalKindCommand while the switch is off,
// so turning it off also stops a run that was reviewed before. Every other kind passes unchanged.
func gateCommandApproval(approve, show control.Handler, enabled func() (bool, error)) control.Handler {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var a control.ProposalApproveArgs
		if json.Unmarshal(raw, &a) == nil {
			// A proposal that cannot be shown is left to approve, which answers it in its own words.
			showArgs, _ := json.Marshal(control.ProposalIDArgs{ID: a.ID})
			if res, err := show(ctx, showArgs); err == nil {
				if shown, ok := res.(control.ProposalShowResult); ok && shown.Kind == control.ProposalKindCommand {
					on, err := enabled()
					if err != nil {
						return nil, control.NewError(control.CodeInternal, "internal error")
					}
					if !on {
						return nil, control.NewError(control.CodeCommandsDisabled,
							"proposals of commands are switched off; an administrator turns them on with 'sudo qhost proposal commands enable'")
					}
				}
			}
		}
		return approve(ctx, raw)
	}
}
