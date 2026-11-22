package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/approval"
	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

// approveWindow reviews and applies a proposal of the kind window: it shows the difference to the window that
// applies now, asks, checks proposal and difference again and sets the window the way 'qhost window set' does:
// without --engine the host window, with --engine the own window of that engine.
func approveWindow(c *cobra.Command, program *cobracli.Program, state *notRunning, _ *redact.Redactor, first control.ProposalShowResult, o approveOptions) error {
	id := first.ID
	if err := o.onlyForSourceAdd(id); err != nil {
		return err
	}
	var content control.WindowSetArgs
	if err := json.Unmarshal(first.Content, &content); err != nil {
		return usageFailure("the content of the proposal cannot be read", "Reject it with 'sudo qhost proposal reject "+id+"'.")
	}
	notice := c.ErrOrStderr()
	_, _ = fmt.Fprintf(notice, "Proposal %s, kind %s, from %s, submitted %s, expires %s:\n",
		plainText(first.ID), shortText(first.Kind), shortText(first.SubmitterName), proposalTime(first.Submitted), proposalTime(first.Expires))
	shownEffect, err := windowEffect(c, state, content, o.engine)
	if err != nil {
		return err
	}
	_, _ = io.WriteString(notice, shownEffect)
	if err := o.decide(c, "Approve proposal "+first.ID+" and set this window?"); err != nil {
		return err
	}

	// Last look before anything changes: the proposal and its effect must be what was shown.
	now, err := loadOpenProposal(c, state, id)
	if err != nil {
		return err
	}
	if now.Fingerprint != first.Fingerprint {
		return usageFailure("the proposal changed since it was shown; nothing was applied",
			"Run 'sudo qhost proposal approve "+id+"' again and review it.")
	}
	again, err := windowEffect(c, state, content, o.engine)
	if err != nil {
		return err
	}
	if again != shownEffect {
		_, _ = fmt.Fprint(notice, "\nThe effect changed since it was shown. It is now:\n", again)
		return usageFailure("the effect of the proposal changed since it was shown; nothing was applied",
			"Run 'sudo qhost proposal approve "+id+"' again and review the new effect.")
	}

	if err := proposalCall(c, state, control.ProposalApproveCommand,
		control.ProposalApproveArgs{ID: id, Fingerprint: first.Fingerprint}, &control.ProposalInfo{}); err != nil {
		return err
	}
	var applied []output.Field
	var applyErr error
	if o.engine == "" {
		applied, applyErr = setHostWindow(content)
	} else {
		var res control.WindowResult
		if applyErr = credentialCall(c, state, control.WindowSetCommand, content, &res); applyErr == nil {
			applied = windowFields(res)
		}
	}
	result := control.ProposalResultArgs{ID: id, Fingerprint: first.Fingerprint, Applied: applyErr == nil}
	if applyErr != nil {
		result.Message = failureText(applyErr)
	}
	if err := proposalCall(c, state, control.ProposalResultCommand, result, &control.ProposalInfo{}); err != nil {
		note := "the window was set, but the result could not be kept on the proposal"
		if applyErr != nil {
			note = "the change failed, and its result could not be kept on the proposal"
		}
		return &cli.Failure{Code: output.CodeRuntime, Message: note, NextStep: "Run 'qhost proposal show " + id + "' to see its state.", Cause: err}
	}
	if applyErr != nil {
		return applyErr
	}
	fields := append([]output.Field{{Name: "proposal", Value: plainText(id)}}, applied...)
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
}

// windowEffect reads the window that applies now and renders the difference to the one the proposal would set.
// A part the proposal leaves out keeps its value, so it does not show.
// Without an engine that is the host window, read from its file; with one it is the window of that engine's
// deploy service, which the service answers.
func windowEffect(c *cobra.Command, state *notRunning, content control.WindowSetArgs, engine string) (string, error) {
	var cur control.WindowResult
	subject := "host window"
	if engine == "" {
		w, err := window.HostBase(windowHostFile(), stateDir())
		if err != nil {
			return "", &cli.Failure{Code: output.CodeRuntime, Message: "the host window cannot be read: " + err.Error(),
				NextStep: "Fix or move aside " + windowHostFile() + ", then approve the proposal again."}
		}
		cur = control.WindowResult{Days: w.Days.Names(), Start: window.FormatStart(w.Start),
			Duration: window.FormatDuration(w.Duration), Zone: w.Zone}
	} else {
		subject = "own window of the engine " + plainText(engine)
		if err := credentialCall(c, state, control.WindowShowCommand, nil, &cur); err != nil {
			return "", err
		}
	}
	type shape struct {
		Days     string `json:"days"`
		Start    string `json:"start"`
		Duration string `json:"duration"`
		Zone     string `json:"zone"`
	}
	before := shape{strings.Join(cur.Days, ","), cur.Start, cur.Duration, cur.Zone}
	after := before
	if content.Days != nil {
		after.Days = strings.Join(content.Days, ",")
	}
	if content.Start != "" {
		after.Start = content.Start
	}
	if content.Duration != "" {
		after.Duration = content.Duration
	}
	if content.Zone != "" {
		after.Zone = content.Zone
	}
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	changes, err := approval.Diff(b, a)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if len(changes) == 0 {
		out.WriteString("Effect: the " + subject + " equals the one that applies now; nothing would change.\n")
		return out.String(), nil
	}
	out.WriteString("Effect: the " + subject + " would change:\n")
	for _, ch := range changes {
		fmt.Fprintf(&out, "   %s\n", plainText(ch.Path))
		writeWrapped(&out, "      before: ", ch.Before)
		writeWrapped(&out, "      after:  ", ch.After)
	}
	return out.String(), nil
}
