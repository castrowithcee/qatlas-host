package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// approveNotifyChannel reviews and applies a proposal of the kind notify-channel: it shows every field of the
// channel in full, asks, reads the secret parts from the person the way 'qhost notify add' reads them (with
// --fingerprint as JSON on standard input instead), checks proposal and state again and adds the channel the way 'qhost notify add' does. The proposal never holds a
// secret part and the result kept on it never mentions one.
func approveNotifyChannel(c *cobra.Command, program *cobracli.Program, state *notRunning, secrets *redact.Redactor,
	first control.ProposalShowResult, o approveOptions) error {
	id := first.ID
	if err := o.onlyForSourceAdd(id); err != nil {
		return err
	}
	var content control.ProposalNotifyChannel
	if err := json.Unmarshal(first.Content, &content); err != nil {
		return usageFailure("the content of the proposal cannot be read", "Reject it with 'sudo qhost proposal reject "+id+"'.")
	}
	notice := c.ErrOrStderr()
	_, _ = fmt.Fprintf(notice, "Proposal %s, kind %s, from %s, submitted %s, expires %s:\n",
		plainText(first.ID), shortText(first.Kind), shortText(first.SubmitterName), proposalTime(first.Submitted), proposalTime(first.Expires))
	shownEffect, err := channelEffect(c, state, content)
	if err != nil {
		return err
	}
	_, _ = io.WriteString(notice, shownEffect)
	if err := o.decide(c, "Approve proposal "+first.ID+" and add this channel?"); err != nil {
		return err
	}

	// The person enters the secret parts; they exist only here and in the call that adds the channel.
	add := control.NotifyAddArgs{Name: content.Name, Type: content.Type, Events: content.Events, Host: content.Host,
		Port: content.Port, Security: content.Security, User: content.User, From: content.From, To: content.To,
		ChatID: content.ChatID, BaseURL: content.BaseURL}
	var values []string
	if o.asks() {
		values, err = readSecrets(c, add.Name, secretPrompts(add))
	} else {
		values, err = readSecretsJSON(secretPrompts(add), secrets)
	}
	if err != nil {
		return err
	}
	for _, v := range values {
		secrets.Add(v)
	}
	switch len(values) {
	case 2:
		add.Secret, add.Key = values[0], values[1]
	case 1:
		add.Secret = values[0]
	}

	// Last look before anything changes: the proposal and the state must be what was shown.
	now, err := loadOpenProposal(c, state, id)
	if err != nil {
		return err
	}
	if now.Fingerprint != first.Fingerprint {
		return usageFailure("the proposal changed since it was shown; nothing was applied",
			"Run 'sudo qhost proposal approve "+id+"' again and review it.")
	}
	again, err := channelEffect(c, state, content)
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
	var res control.NotifyChannelInfo
	applyErr := notifyCall(c, state, control.NotifyAddCommand, add, &res)
	result := control.ProposalResultArgs{ID: id, Fingerprint: first.Fingerprint, Applied: applyErr == nil}
	if applyErr != nil {
		result.Message = secrets.Apply(failureText(applyErr))
	}
	if err := proposalCall(c, state, control.ProposalResultCommand, result, &control.ProposalInfo{}); err != nil {
		note := "the channel was added, but the result could not be kept on the proposal"
		if applyErr != nil {
			note = "the change failed, and its result could not be kept on the proposal"
		}
		return &cli.Failure{Code: output.CodeRuntime, Message: note, NextStep: "Run 'qhost proposal show " + id + "' to see its state.", Cause: err}
	}
	if applyErr != nil {
		return applyErr
	}
	fields := append([]output.Field{{Name: "proposal", Value: plainText(id)}}, channelFields(res)...)
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
}

// channelEffect renders the channel in full and states that no channel of its name exists now; a name that is
// taken refuses the approval, as 'qhost notify add' would. It reads the channels and changes nothing.
func channelEffect(c *cobra.Command, state *notRunning, content control.ProposalNotifyChannel) (string, error) {
	var list control.NotifyListResult
	if err := notifyCall(c, state, control.NotifyListCommand, nil, &list); err != nil {
		return "", err
	}
	for _, ch := range list.Channels {
		if ch.Name == content.Name {
			return "", usageFailure("a channel named "+shortText(content.Name)+" exists now; nothing was applied",
				"Reject the proposal with 'sudo qhost proposal reject <id>', or remove the channel first with 'sudo qhost notify remove <name>'.")
		}
	}
	var out strings.Builder
	writeWrapped(&out, "  name:     ", content.Name)
	writeWrapped(&out, "  type:     ", content.Type)
	writeWrapped(&out, "  events:   ", strings.Join(content.Events, ","))
	field := func(name, value string) {
		if value != "" {
			writeWrapped(&out, fmt.Sprintf("  %-9s ", name+":"), value)
		}
	}
	field("host", content.Host)
	if content.Port != 0 {
		field("port", fmt.Sprint(content.Port))
	}
	field("security", content.Security)
	field("user", content.User)
	field("from", content.From)
	field("to", strings.Join(content.To, ","))
	field("chat_id", content.ChatID)
	field("base_url", content.BaseURL)
	out.WriteString("Effect: the channel would be added; no channel of this name exists now.\n" +
		"You enter its secret parts next; the proposal holds none.\n")
	return out.String(), nil
}
