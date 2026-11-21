package hostservice

import (
	"encoding/json"
	"fmt"

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
func proposalCommands(add func(name, action string, class control.Class, h control.Handler) error, store *proposals.Store) error {
	for name, c := range store.Commands() {
		action, ok := proposalActions[name]
		if !ok {
			panic(fmt.Sprintf("hostservice: the proposal command %s has no log action", name))
		}
		if err := add(name, action, c.Class, c.Handle); err != nil {
			return err
		}
	}
	return nil
}

// normalizeProposal is the check of the only kind of the host service, ProposalKindHostChange. It decodes the
// content strictly, refuses a password by name, and checks the action, its arguments and keys with check, which
// reads nothing from the host and runs nothing.
func normalizeProposal(check hostproposal.Checker) proposals.Normalizer {
	return func(kind string, content json.RawMessage) (any, *control.Error) {
		if kind != control.ProposalKindHostChange {
			return nil, proposals.Invalid("the host service keeps proposals of the kind %s only, not %q",
				control.ProposalKindHostChange, proposals.Printable(kind))
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
