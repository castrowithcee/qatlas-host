package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
	"github.com/castrowithcee/qatlas-host/internal/host/hostproposal"
)

// controllingTerminal reports whether the process has a terminal of its own. Approving a proposal of the host
// needs one even when standard input carries the initial password, as when the terminal interface runs the
// command; tests replace it.
var controllingTerminal = func() bool {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// requireHostPerson refuses an agent and a process without a terminal. Root is checked before.
func requireHostPerson(program *cobracli.Program) error {
	const what = "applying a proposal of the host"
	if program.Agent() {
		return &cli.Failure{Code: output.CodePermission, Message: what + " is for a person at a terminal, not for an agent",
			NextStep: "Ask the person who runs the host to review the proposal and run 'sudo qhost host apply --proposal <id>'."}
	}
	if !stdinIsTerminal() && !controllingTerminal() {
		return &cli.Failure{Code: output.CodePermission, Message: what + " needs a terminal; there is no way around it",
			NextStep: "Run the command at a terminal with sudo."}
	}
	return nil
}

func stdinIsTerminal() bool {
	f, ok := stdinStream().(*os.File)
	return ok && isTerminal(f)
}

// loadHostProposal reads an open proposal of the kind host-change from the host service and decodes its content
// strictly.
func loadHostProposal(c *cobra.Command, state *notRunning, id string) (control.ProposalShowResult, control.ProposalHostChange, error) {
	var res control.ProposalShowResult
	var content control.ProposalHostChange
	if err := proposalCallTo(c, state, true, control.ProposalShowCommand, control.ProposalIDArgs{ID: id}, &res); err != nil {
		return res, content, err
	}
	if res.State != control.ProposalOpen {
		return res, content, usageFailure("the proposal is "+plainText(res.State)+", not open", "List the proposals with 'qhost proposal list'.")
	}
	reject := "Reject it with 'sudo qhost proposal reject " + plainText(id) + "'."
	if res.Kind != control.ProposalKindHostChange {
		return res, content, usageFailure("the proposal is of the kind "+shortText(res.Kind)+", not "+control.ProposalKindHostChange, reject)
	}
	dec := json.NewDecoder(bytes.NewReader(res.Content))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&content); err != nil {
		return res, content, usageFailure("the content of the proposal cannot be read", reject)
	}
	return res, content, nil
}

// applyProposal applies an open proposal of the host: the same check as when it was submitted, the sentence and
// the commands shown, the approval bound to the fingerprint that was shown, and the change made by the runner of
// 'host apply' with its journal, protection rules and report. The result is kept on the proposal.
//
// A non-nil unconfirmed (no --confirm) makes it a preview: the proposal is checked and shown, then that error ends
// the command before any question, the password on standard input, the journal or the approval.
func applyProposal(c *cobra.Command, program *cobracli.Program, state *notRunning, secrets *redact.Redactor, id, reviewed string, unconfirmed error) error {
	if err := requireHostPerson(program); err != nil {
		return err
	}
	if !hostProposalID(id) {
		return usageFailure("host apply --proposal takes a proposal of the host, whose id starts with "+control.HostProposalIDPrefix,
			"Approve any other proposal with 'sudo qhost proposal approve <id>'.")
	}
	again := "Review it with 'qhost proposal show " + plainText(id) + "' and run the command again."
	changed := func() error {
		return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true, NextStep: again,
			Message: "the proposal is not the one that was shown; nothing was applied"}
	}
	first, content, err := loadHostProposal(c, state, id)
	if err != nil {
		return err
	}
	if reviewed != "" && reviewed != first.Fingerprint {
		return changed()
	}
	check := hostproposal.New(changeEnv.registry())
	keys, err := check.Check(content.Action, content.Args, content.Keys)
	if err == nil && !slices.Equal(keys, content.Keys) {
		err = errors.New("its SSH keys are not in canonical form")
	}
	if err != nil {
		return usageFailure("the proposal does not pass the check of proposals: "+shortText(err.Error()),
			"Reject it with 'sudo qhost proposal reject "+plainText(id)+"'.")
	}
	password := ""
	if hostproposal.NeedsPassword(content.Action) && unconfirmed != nil {
		password = hostchange.DefaultPreviewSecret
	} else if hostproposal.NeedsPassword(content.Action) {
		if reviewed == "" {
			return usageFailure("the initial password comes on standard input, so there is no question here; pass --fingerprint",
				"Review the proposal with 'qhost proposal show "+plainText(id)+"', then pass its fingerprint with --fingerprint.")
		}
		if password, err = readProposalPassword(secrets); err != nil {
			return err
		}
	}
	plan, secret, err := check.Plan(content.Action, content.Args, keys, password)
	defer clear(secret)
	if err != nil {
		var arg *hostchange.ArgumentError
		if errors.As(err, &arg) && password != "" {
			return usageFailure(err.Error(), "Provide a valid initial password on standard input.")
		}
		return usageFailure("the proposal cannot be applied: "+shortText(err.Error()), "Reject it with 'sudo qhost proposal reject "+plainText(id)+"'.")
	}
	showHostProposal(c.ErrOrStderr(), first.ProposalInfo, content.Action, plan, keys)
	if unconfirmed != nil {
		return unconfirmed
	}
	if reviewed == "" {
		if !stdinIsTerminal() {
			return usageFailure("there is no terminal to ask; pass --fingerprint of the proposal you reviewed",
				"Review the proposal with 'qhost proposal show "+plainText(id)+"', then pass its fingerprint with --fingerprint.")
		}
		if err := askApproval(c, "Apply proposal "+plainText(id)+" to the host?"); err != nil {
			return err
		}
	}
	journal, err := changeEnv.openJournal()
	if err != nil {
		return changeFailed(fmt.Errorf("host apply: %w", err))
	}

	// Last look before anything changes: the proposal must be the one that was shown.
	now, _, err := loadHostProposal(c, state, id)
	if err != nil {
		return err
	}
	if now.Fingerprint != first.Fingerprint {
		return changed()
	}
	// The host service takes the approval before anything is applied, and only for the fingerprint that was shown.
	if err := proposalCallTo(c, state, true, control.ProposalApproveCommand,
		control.ProposalApproveArgs{ID: id, Fingerprint: first.Fingerprint}, &control.ProposalInfo{}); err != nil {
		return err
	}
	res, applyErr := changeEnv.runner(journal).Apply(c.Context(), content.Action, content.Args, secret)
	result := control.ProposalResultArgs{ID: id, Fingerprint: first.Fingerprint, Applied: applyErr == nil && res.Entry.Result == hostchange.ResultApplied}
	var refusal *hostchange.RefusedError
	switch {
	case errors.As(applyErr, &refusal):
		result.Message = "refused: " + refusal.Msg
	case applyErr != nil:
		result.Message = applyErr.Error()
	case !result.Applied:
		result.Message = res.Entry.Error
	}
	resultErr := proposalCallTo(c, state, true, control.ProposalResultCommand, result, &control.ProposalInfo{})

	extra := output.Field{Name: "proposal_id", Value: id}
	var reportErr error
	if applyErr != nil {
		if refused, ok := refusedChange(c, program, content.Action, applyErr, extra); ok {
			reportErr = refused
		} else {
			reportErr = changeFailed(fmt.Errorf("host apply: %w", applyErr))
		}
	} else {
		reportErr = finishChange(c, program, res, extra)
	}
	if resultErr != nil {
		note := "the change was applied, but its result could not be kept on the proposal"
		if !result.Applied {
			note = "the change did not stand, and its result could not be kept on the proposal"
		}
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s\n", note)
		if reportErr == nil {
			return &cli.Failure{Code: output.CodeRuntime, Message: note, Cause: resultErr,
				NextStep: "Run 'qhost proposal show " + plainText(id) + "' to see its state."}
		}
	}
	return reportErr
}

// readProposalPassword reads the initial password of user.create from standard input, as {"password": "..."}.
// The keys come from the proposal, so standard input may not name any.
func readProposalPassword(secrets *redact.Redactor) (string, error) {
	raw, err := readChangeSecret(stdinStream())
	if err != nil {
		return "", err
	}
	defer clear(raw)
	secrets.Add(string(raw))
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var in hostchange.CreateInput
	if err := dec.Decode(&in); err != nil {
		return "", usageFailure("standard input must hold the initial password as {\"password\": \"...\"}",
			"Provide the initial password as JSON on standard input.")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return "", usageFailure("standard input has data after the JSON object", "Provide the initial password as JSON on standard input.")
	}
	if in.Password != "" {
		secrets.Add(in.Password)
	}
	if len(in.Keys) > 0 {
		return "", usageFailure("the SSH keys come from the proposal; standard input holds the initial password only",
			"Provide only {\"password\": \"...\"} on standard input.")
	}
	return in.Password, nil
}

// showHostProposal prints what applying the proposal does, as the confirmation of a change by hand shows it.
func showHostProposal(w io.Writer, p control.ProposalInfo, action string, plan hostchange.Plan, keys []string) {
	_, _ = fmt.Fprintf(w, "Proposal %s, action %s, from %s, submitted %s, expires %s:\n",
		plainText(p.ID), shortText(action), shortText(p.SubmitterName), proposalTime(p.Submitted), proposalTime(p.Expires))
	_, _ = fmt.Fprintf(w, "  %s\n", plainText(plan.Sentence))
	for _, cmd := range plan.Commands {
		_, _ = fmt.Fprintf(w, "    %s\n", plainText(cmd.Display()))
	}
	for _, k := range keyDetails(keys) {
		_, _ = fmt.Fprintf(w, "  SSH key: %s\n", plainText(k))
	}
	_, _ = fmt.Fprintf(w, "  Fingerprint of the proposal: %s\n", plainText(p.Fingerprint))
}

// keyDetails names each key by type, fingerprint and comment.
func keyDetails(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, text := range keys {
		k, err := hostchange.ParseKey(text)
		if err != nil {
			out = append(out, "(not a valid key)")
			continue
		}
		line := k.Type + " " + k.Fingerprint
		if k.Comment != "" {
			line += " " + k.Comment
		}
		out = append(out, line)
	}
	return out
}

// hostChangeFields are the sentence, the commands and the keys of a proposal of the host for show. A proposal this
// version cannot plan shows why instead.
func hostChangeFields(program *cobracli.Program, res control.ProposalShowResult) []output.Field {
	format := program.Format()
	var content control.ProposalHostChange
	if err := json.Unmarshal(res.Content, &content); err != nil {
		return []output.Field{{Name: "plan_error", Value: "the content cannot be read"}}
	}
	plan, err := hostproposal.New(changeEnv.registry()).View(content.Action, content.Args, content.Keys)
	if err != nil {
		return []output.Field{{Name: "plan_error", Value: text(format, err.Error())}}
	}
	commands := make([]string, len(plan.Commands))
	for i, cmd := range plan.Commands {
		commands[i] = text(format, cmd.Display())
	}
	details := keyDetails(content.Keys)
	for i := range details {
		details[i] = text(format, details[i])
	}
	return []output.Field{
		{Name: "sentence", Value: text(format, plan.Sentence)},
		{Name: "commands", Value: listValueSep(program.Format(), commands, "; ")},
		{Name: "ssh_keys", Value: listValueSep(program.Format(), details, "; ")},
	}
}
