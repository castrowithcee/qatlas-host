package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// approveGrant reviews and applies a proposal of the kind grant: it shows the grant for the submitter, asks,
// checks proposal and plan again and adds the grant the way 'qhost grant add' does, with the same checks in the
// service. The grant is for the user ID the proposal stores, never for another one.
func approveGrant(c *cobra.Command, program *cobracli.Program, state *notRunning, _ *redact.Redactor, first control.ProposalShowResult, o approveOptions) error {
	id := first.ID
	if err := o.onlyForSourceAdd(id); err != nil {
		return err
	}
	var content control.ProposalGrant
	if err := json.Unmarshal(first.Content, &content); err != nil {
		return usageFailure("the content of the proposal cannot be read", "Reject it with 'sudo qhost proposal reject "+id+"'.")
	}
	expires, err := grantExpiry(content.Expires, o.expires)
	if err != nil {
		return err
	}
	notice := c.ErrOrStderr()
	_, _ = fmt.Fprintf(notice, "Proposal %s, kind %s, from %s, submitted %s, expires %s:\n",
		plainText(first.ID), shortText(first.Kind), shortText(first.SubmitterName), proposalTime(first.Submitted), proposalTime(first.Expires))
	plan, err := grantPlan(c, state, content, expires)
	if err != nil {
		return err
	}
	shown := grantPlanText(c, content, expires, plan)
	_, _ = io.WriteString(notice, shown)
	if err := o.decide(c, "Approve proposal "+id+" and add this grant?"); err != nil {
		return err
	}

	// Last look before anything changes: the proposal and its plan must be what was shown.
	now, err := loadOpenProposal(c, state, id)
	if err != nil {
		return err
	}
	if now.Fingerprint != first.Fingerprint {
		return usageFailure("the proposal changed since it was shown; nothing was applied",
			"Run 'sudo qhost proposal approve "+id+"' again and review it.")
	}
	again, err := grantPlan(c, state, content, expires)
	if err != nil {
		return err
	}
	if text := grantPlanText(c, content, expires, again); text != shown {
		_, _ = fmt.Fprint(notice, "\nThe grant changed since it was shown. It is now:\n", text)
		return usageFailure("the effect of the proposal changed since it was shown; nothing was applied",
			"Run 'sudo qhost proposal approve "+id+"' again and review the new effect.")
	}
	return applyGrantProposal(c, program, state, first, content, expires)
}

// grantExpiry is the expiry the grant gets: the requested one, or the person's --expires. With a requested
// expiry --expires may only keep or shorten it; without one every date is allowed.
func grantExpiry(requested, flag string) (string, error) {
	if flag == "" {
		return requested, nil
	}
	if _, err := time.Parse(control.GrantExpiresLayout, flag); err != nil {
		return "", usageFailure("--expires is not a date of the form YYYY-MM-DD", "Pass the last day the grant holds, for example 2030-01-31.")
	}
	if requested != "" && flag > requested {
		return "", usageFailure("--expires is later than the requested expiry "+plainText(requested)+"; it may only keep or shorten it",
			"Pass a date up to "+plainText(requested)+".")
	}
	return flag, nil
}

// grantPlan asks the service for the plan of the grant, as 'qhost grant add' without --confirm does, and refuses
// what changed since the proposal: a name that belongs to another user ID now, and an equal grant that exists
// already, which the add would replace.
func grantPlan(c *cobra.Command, state *notRunning, content control.ProposalGrant, expires string) (control.GrantAddResult, error) {
	var res control.GrantAddResult
	req := control.GrantAddArgs{User: content.User, Action: content.Action, Stack: content.Stack, Expires: expires}
	if err := credentialCall(c, state, control.GrantAddCommand, req, &res); err != nil {
		return res, err
	}
	if res.Grant.UID != content.UID {
		return res, usageFailure("the name "+shortText(content.User)+" belongs to another user ID than when the proposal was submitted; nothing was applied",
			"Reject the proposal with 'sudo qhost proposal reject <id>'.")
	}
	if res.Replaced {
		return res, usageFailure("an equal grant exists now, "+plainText(res.Grant.ID)+"; the proposal would replace it, so nothing was applied",
			"Reject the proposal, or change the grant with 'sudo qhost grant add'.")
	}
	return res, nil
}

func grantPlanText(c *cobra.Command, content control.ProposalGrant, expires string, res control.GrantAddResult) string {
	var out strings.Builder
	showGrantPlan(&out, targetOf(c.Context()).id(), res)
	if expires != content.Expires {
		fmt.Fprintf(&out, "  You set the expiry to %s; the proposal asked for %s.\n", plainText(expires), requestedText(content.Expires))
	}
	fmt.Fprintf(&out, "  reason, text of the submitter: %q\n", shortText(content.Reason))
	return out.String()
}

func requestedText(expires string) string {
	if expires == "" {
		return "none"
	}
	return plainText(expires)
}

func applyGrantProposal(c *cobra.Command, program *cobracli.Program, state *notRunning, first control.ProposalShowResult, content control.ProposalGrant, expires string) error {
	id := first.ID
	if err := proposalCall(c, state, control.ProposalApproveCommand,
		control.ProposalApproveArgs{ID: id, Fingerprint: first.Fingerprint}, &control.ProposalInfo{}); err != nil {
		return err
	}
	req := control.GrantAddArgs{User: content.User, Action: content.Action, Stack: content.Stack, Expires: expires,
		SudoUID: uint32(changeEnv.sudoUID()), Confirm: true}
	var added control.GrantAddResult
	applyErr := credentialCall(c, state, control.GrantAddCommand, req, &added)
	result := control.ProposalResultArgs{ID: id, Fingerprint: first.Fingerprint, Applied: applyErr == nil}
	if applyErr != nil {
		result.Message = failureText(applyErr)
	}
	if err := proposalCall(c, state, control.ProposalResultCommand, result, &control.ProposalInfo{}); err != nil {
		note := "the grant was added, but the result could not be kept on the proposal"
		if applyErr != nil {
			note = "the grant could not be added, and its result could not be kept on the proposal"
		}
		return &cli.Failure{Code: output.CodeRuntime, Message: note, NextStep: "Run 'qhost proposal show " + id + "' to see its state.", Cause: err}
	}
	if applyErr != nil {
		return applyErr
	}
	g := added.Grant
	fields := []output.Field{
		{Name: "proposal", Value: plainText(id)}, {Name: "id", Value: g.ID}, {Name: "user", Value: plainText(g.User)},
		{Name: "action", Value: g.Action}, {Name: "stack", Value: plainText(g.Stack)}, {Name: "engine", Value: targetOf(c.Context()).id()},
		{Name: "expires", Value: g.Expires}, {Name: "replaced", Value: added.Replaced},
		{Name: "backup", Value: added.Backup}, {Name: "check", Value: added.Check},
	}
	if expires != content.Expires {
		fields = append(fields, output.Field{Name: "requested_expires", Value: content.Expires})
	}
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
}

// grantProposalFields shows a proposal of the kind grant. The reason is text of the submitter: it is shown with
// control characters replaced, in quotes and marked as foreign text. Backup and check come from the settings of
// the stack, which an administrator reads without sudo; a stack that cannot be read shows as unknown.
func grantProposalFields(c *cobra.Command, state *notRunning, format output.Format, res control.ProposalShowResult) []output.Field {
	var content control.ProposalGrant
	if err := json.Unmarshal(res.Content, &content); err != nil {
		return []output.Field{{Name: "content_error", Value: "the content cannot be read"}}
	}
	backup, check := "unknown", "unknown"
	var settings control.StackSettingsResult
	if err := credentialCall(c, state, control.StackSettingsGetCommand, control.StackSettingsArgs{Stack: content.Stack}, &settings); err == nil {
		backup, check = setUpText(false), setUpText(false)
		if s := settings.Settings; s != nil {
			backup = setUpText(s.Backup != nil)
			check = setUpText(s.Check != nil && (s.Check.Containers != nil && *s.Check.Containers || s.Check.HTTP != nil))
		}
	}
	fields := []output.Field{{Name: "backup", Value: backup}, {Name: "check", Value: check}}
	if structuredFormat(format) {
		return append(fields, output.Field{Name: "content_untrusted", Value: true})
	}
	return append(fields,
		output.Field{Name: "grant_user", Value: plainText(content.User)}, output.Field{Name: "grant_action", Value: plainText(content.Action)},
		output.Field{Name: "grant_stack", Value: plainText(content.Stack)}, output.Field{Name: "grant_expires", Value: requestedText(content.Expires)},
		output.Field{Name: "untrusted_reason", Value: `"` + plainText(content.Reason) + `"`})
}
