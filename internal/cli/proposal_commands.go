package cli

import (
	"encoding/json"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// commandProposalNotice is shown with every proposal of free commands.
const commandProposalNotice = "unchecked, no computed effect, no undo"

func newProposalCommandsCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	group := &cobra.Command{
		Use:   "commands",
		Short: "Turn proposals of free commands on or off, or show whether they are on",
		Long: "A proposal of the kind command carries shell commands and a reason for a change that no registered action\n" +
			"covers. It is accepted only while the switch is on; it is off until root turns it on. Turning it off keeps\n" +
			"the open proposals of this kind as they are. qhost never runs the commands by itself, and nothing checks\n" +
			"them. enable and disable need sudo; status needs no sudo for an administrator in the group qhost.",
	}
	set := func(use, short string, enabled bool) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			Args:  cobra.NoArgs,
			RunE: func(c *cobra.Command, _ []string) error {
				var res control.HostCommandsResult
				if err := hostCall(c, state, control.HostCommandsSetCommand, control.HostCommandsSetArgs{Enabled: enabled}, &res); err != nil {
					return err
				}
				return output.Encode(c.OutOrStdout(), program.Format(), commandsSwitchObject(res))
			},
		}
	}
	status := &cobra.Command{
		Use:   "status",
		Short: "Show whether proposals of free commands are accepted",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var res control.HostCommandsResult
			if err := hostCall(c, state, control.HostCommandsStatusCommand, nil, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), commandsSwitchObject(res))
		},
	}
	group.AddCommand(
		set("enable", "Accept proposals of free commands (needs root)", true),
		set("disable", "Stop accepting proposals of free commands; open ones stay (needs root)", false),
		status)
	return group
}

func commandsSwitchObject(res control.HostCommandsResult) output.Object {
	return output.Object{Fields: []output.Field{{Name: "enabled", Value: res.Enabled}}}
}

// commandProposalFailure names the switch when a submission is refused because it is off.
func commandProposalFailure(remote *control.Error) error {
	return &cli.Failure{Code: output.CodePermission, Message: shortText(remote.Message),
		NextStep:      "Ask an administrator to run 'sudo qhost proposal commands enable', then submit again.",
		AgentNextStep: "Do not turn the switch on yourself; ask the user to run it with sudo.", Cause: remote}
}

// commandProposalFields shows a proposal of free commands. The commands and the reason are text of the
// submitter: they are shown word for word, with every control character replaced, and marked as foreign text.
func commandProposalFields(format output.Format, res control.ProposalShowResult) []output.Field {
	fields := []output.Field{{Name: "notice", Value: commandProposalNotice}}
	var content control.ProposalCommand
	if err := json.Unmarshal(res.Content, &content); err != nil {
		return append(fields, output.Field{Name: "content_error", Value: "the content cannot be read"})
	}
	if structuredFormat(format) {
		return append(fields, output.Field{Name: "content_untrusted", Value: true})
	}
	for i, line := range content.Commands {
		fields = append(fields, output.Field{Name: "untrusted_command." + strconv.Itoa(i+1), Value: plainText(line)})
	}
	return append(fields, output.Field{Name: "untrusted_reason", Value: plainText(content.Reason)})
}
