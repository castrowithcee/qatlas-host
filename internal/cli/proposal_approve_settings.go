package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/approval"
	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/backup"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// wrapWidth is the width at which a shown value is broken into lines. A value is never cut.
const wrapWidth = 76

// approveStackSettings reviews and applies a proposal of the kind stack-settings: it shows the difference to
// the settings that apply now, asks, checks proposal and difference again and writes the settings the way
// 'qhost stack settings set' does.
func approveStackSettings(c *cobra.Command, program *cobracli.Program, state *notRunning, _ *redact.Redactor, first control.ProposalShowResult, url string) error {
	id := first.ID
	if url != "" {
		return usageFailure("--url applies to proposals of the kind source-add only", "Run 'sudo qhost proposal approve "+id+"' without --url.")
	}
	content, err := decodeSettingsProposal(first)
	if err != nil {
		return err
	}
	notice := c.ErrOrStderr()
	_, _ = fmt.Fprintf(notice, "Proposal %s, kind %s, from %s, submitted %s, expires %s:\n",
		plainText(first.ID), shortText(first.Kind), shortText(first.SubmitterName), proposalTime(first.Submitted), proposalTime(first.Expires))
	shownEffect, err := settingsEffect(c, state, content)
	if err != nil {
		return err
	}
	_, _ = io.WriteString(notice, shownEffect)
	if err := askApproval(c, "Approve proposal "+first.ID+" and apply these settings?"); err != nil {
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
	again, err := settingsEffect(c, state, content)
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
	var res control.StackSettingsSetResult
	applyErr := remoteCall(c, state, control.StackSettingsTimeout, control.StackSettingsSetCommand,
		control.StackSettingsSetArgs{Stack: content.Stack, Settings: content.Settings}, &res)
	result := control.ProposalResultArgs{ID: id, Fingerprint: first.Fingerprint, Applied: applyErr == nil}
	if applyErr != nil {
		result.Message = failureText(applyErr)
	}
	if err := proposalCall(c, state, control.ProposalResultCommand, result, &control.ProposalInfo{}); err != nil {
		note := "the settings were applied, but the result could not be kept on the proposal"
		if applyErr != nil {
			note = "the change failed, and its result could not be kept on the proposal"
		}
		return &cli.Failure{Code: output.CodeRuntime, Message: note, NextStep: "Run 'qhost proposal show " + id + "' to see its state.", Cause: err}
	}
	if applyErr != nil {
		return applyErr
	}
	fields := []output.Field{{Name: "proposal", Value: plainText(id)}, {Name: "stack", Value: res.Stack}, {Name: "changed", Value: res.Changed}}
	if res.Commit != "" {
		fields = append(fields, output.Field{Name: "commit", Value: res.Commit})
	}
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
}

func decodeSettingsProposal(res control.ProposalShowResult) (control.ProposalStackSettings, error) {
	var content control.ProposalStackSettings
	if err := json.Unmarshal(res.Content, &content); err != nil || !backup.ValidStack(content.Stack) {
		return content, usageFailure("the content of the proposal cannot be read", "Reject it with 'sudo qhost proposal reject "+res.ID+"'.")
	}
	return content, nil
}

// settingsEffect reads the settings that apply now and renders the difference to the proposed ones. A stack
// that does not exist refuses the approval.
func settingsEffect(c *cobra.Command, state *notRunning, content control.ProposalStackSettings) (string, error) {
	var cur control.StackSettingsResult
	if err := credentialCall(c, state, control.StackSettingsGetCommand, control.StackSettingsArgs{Stack: content.Stack}, &cur); err != nil {
		var f *cli.Failure
		var remote *control.Error
		if errors.As(err, &f) && errors.As(f.Cause, &remote) && remote.Code == control.CodeNoSuchStack {
			return "", usageFailure("the stack "+shortText(content.Stack)+" does not exist; nothing was applied",
				"Reject the proposal with 'sudo qhost proposal reject <id>'.")
		}
		return "", err
	}
	var before []byte
	if cur.Settings != nil {
		before, _ = json.Marshal(cur.Settings)
	}
	after, err := json.Marshal(content.Settings)
	if err != nil {
		return "", err
	}
	changes, err := approval.Diff(before, after)
	if err != nil {
		return "", err
	}
	return renderSettingsDiff(content.Stack, cur.Settings != nil, changes), nil
}

// renderSettingsDiff shows every changed value in full, broken into lines and never cut. A backup command is
// marked, because it runs on the host later.
func renderSettingsDiff(stack string, had bool, changes []approval.Change) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  stack: %s\n", plainText(stack))
	switch {
	case len(changes) == 0:
		b.WriteString("Effect: the settings equal those that apply now; nothing would change.\n")
		return b.String()
	case !had:
		b.WriteString("Effect: the stack has no settings yet; these would be set:\n")
	default:
		b.WriteString("Effect: these settings would change:\n")
	}
	command := false
	for _, ch := range changes {
		mark := "  "
		if isBackupCommand(ch.Path) {
			mark, command = "!!", true
		}
		fmt.Fprintf(&b, "%s %s\n", mark, plainText(ch.Path))
		writeWrapped(&b, "      before: ", ch.Before)
		writeWrapped(&b, "      after:  ", ch.After)
	}
	if command {
		b.WriteString("\n!! The backup command runs on the host later, before each update. Read every word of it above.\n")
	}
	return b.String()
}

func isBackupCommand(path string) bool {
	return path == "backup.command" || strings.HasPrefix(path, "backup.command[")
}

// writeWrapped writes value after prefix, continued on further lines of at most wrapWidth characters.
func writeWrapped(b *strings.Builder, prefix, value string) {
	if value == "" {
		value = "(none)"
	}
	runes := []rune(plainText(value))
	pad := strings.Repeat(" ", len(prefix))
	for i := 0; i == 0 || i < len(runes); i += wrapWidth {
		if i > 0 {
			b.WriteString(pad)
		} else {
			b.WriteString(prefix)
		}
		b.WriteString(string(runes[i:min(i+wrapWidth, len(runes))]))
		b.WriteByte('\n')
	}
}
