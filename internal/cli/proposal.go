package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newProposalCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	group := &cobra.Command{
		Use:   "proposal",
		Short: "Submit, list, show, withdraw, approve and reject proposals of changes to the setup",
		Long: "A proposal asks for a change of the setup, for example a stack folder. The service checks and stores it and\n" +
			"changes nothing on the host. Every command runs without sudo for an administrator in the group qhost;\n" +
			"a proposal can be withdrawn by its submitter or by root. approve and reject are for root at a terminal.\n" +
			"See 'qhost help proposals'.",
	}
	group.AddCommand(newProposalSubmit(program, state), newProposalList(program, state),
		newProposalShow(program, state), newProposalWithdraw(program, state),
		newProposalApprove(program, state), newProposalReject(program, state))
	return group
}

func newProposalSubmit(program *cobracli.Program, state *notRunning) *cobra.Command {
	var kind, file string
	c := &cobra.Command{
		Use:   "submit --kind source-add [--file <path>|-]",
		Short: "Submit a proposal read as JSON from a file or standard input",
		Long: "submit sends the content of a proposal to the service, which checks and stores it, and prints its id. The\n" +
			"content is a JSON object in the schema of the kind, read from --file or, without it or with '-', from\n" +
			"standard input; the file is read as the calling user, at most a little over 64 KiB. The kind source-add\n" +
			"takes {\"path\": \"/absolute/folder\", \"url\": \"ssh://...\"}, url optional. Nothing on the host changes. See\n" +
			"'qhost help proposals'.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if kind == "" {
				return usageFailure("--kind is required", "Run 'qhost proposal submit --kind source-add --file <path>'.")
			}
			content, err := readProposalContent(file)
			if err != nil {
				return err
			}
			var res control.ProposalInfo
			if err := proposalCall(c, state, control.ProposalSubmitCommand,
				control.ProposalSubmitArgs{Kind: kind, Content: content}, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: proposalFields(program.Format(), res)})
		},
	}
	c.Flags().StringVar(&kind, "kind", "", "kind of the proposal: source-add")
	c.Flags().StringVar(&file, "file", "", "file with the content as JSON, or - for standard input (default)")
	return c
}

func newProposalList(program *cobracli.Program, state *notRunning) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the proposals, newest first",
		Long:  "list shows every proposal with its id, kind, state, submitter and times. An open proposal past its time shows as expired.",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var res control.ProposalListResult
			if err := proposalCall(c, state, control.ProposalListCommand, nil, &res); err != nil {
				return err
			}
			format := program.Format()
			rows := make([]output.Row, 0, len(res.Proposals))
			for _, p := range res.Proposals {
				rows = append(rows, output.Row{"id": text(format, p.ID), "kind": text(format, p.Kind), "state": text(format, p.State),
					"submitter": text(format, p.SubmitterName), "submitted": proposalTime(p.Submitted), "expires": proposalTime(p.Expires)})
			}
			return output.Encode(c.OutOrStdout(), format, output.Collection{
				Columns: []string{"id", "kind", "state", "submitter", "submitted", "expires"}, Rows: rows})
		},
	}
}

func newProposalShow(program *cobracli.Program, state *notRunning) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show one proposal with its content",
		Long:  "show prints a proposal with its fingerprint, the submitter and the content as it was submitted.",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var res control.ProposalShowResult
			if err := proposalCall(c, state, control.ProposalShowCommand, control.ProposalIDArgs{ID: args[0]}, &res); err != nil {
				return err
			}
			format := program.Format()
			fields := proposalFields(format, res.ProposalInfo)
			var content map[string]any
			if err := json.Unmarshal(res.Content, &content); err != nil {
				content = map[string]any{}
			}
			if structuredFormat(format) {
				fields = append(fields, output.Field{Name: "content", Value: content})
			} else {
				keys := make([]string, 0, len(content))
				for k := range content {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					v, ok := content[k].(string)
					if !ok {
						raw, _ := json.Marshal(content[k])
						v = string(raw)
					}
					fields = append(fields, output.Field{Name: "content." + plainText(k), Value: plainText(v)})
				}
			}
			return output.Encode(c.OutOrStdout(), format, output.Object{Fields: fields})
		},
	}
}

func newProposalWithdraw(program *cobracli.Program, state *notRunning) *cobra.Command {
	return &cobra.Command{
		Use:   "withdraw <id>",
		Short: "Withdraw an open proposal",
		Long:  "withdraw closes an open proposal of your own as withdrawn; root may withdraw any. It changes nothing else.",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var res control.ProposalInfo
			if err := proposalCall(c, state, control.ProposalWithdrawCommand, control.ProposalIDArgs{ID: args[0]}, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: proposalFields(program.Format(), res)})
		},
	}
}

// readProposalContent reads the content as the calling user from a file or standard input. It reads a little
// more than the limit, so the size check stays with the service while the client never reads without bound.
func readProposalContent(file string) (json.RawMessage, error) {
	var in io.Reader
	if file == "" || file == "-" {
		in = stdinStream()
	} else {
		f, err := os.Open(file)
		if err != nil {
			return nil, usageFailure("cannot read the content file: "+plainText(err.Error()), "Pass a readable file, or '-' for standard input.")
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	raw, err := io.ReadAll(io.LimitReader(in, control.ProposalMaxBytes+1))
	if err != nil {
		return nil, &cli.Failure{Code: output.CodeRuntime, Message: "cannot read the content: " + plainText(err.Error()),
			NextStep: "Run the command again.", Cause: err}
	}
	if len(raw) == 0 {
		return nil, usageFailure("the content is empty", "Pass the content as a JSON object.")
	}
	// A cut-off oversize content is not valid JSON; it is refused here with the same reason as the service gives.
	if !json.Valid(raw) {
		if len(raw) > control.ProposalMaxBytes {
			return nil, proposalTooLarge(nil)
		}
		return nil, usageFailure("the content is not valid JSON", "Pass the content as a JSON object.")
	}
	return json.RawMessage(raw), nil
}

func proposalTooLarge(cause error) error {
	return &cli.Failure{Code: output.CodeInvalidRequest, Message: "a proposal may hold at most 64 KiB", Usage: true,
		NextStep: "Shorten the content.", Cause: cause}
}

// proposalCall maps the codes of the proposal commands; any other failure stays as the shared mapping made it.
func proposalCall(c *cobra.Command, state *notRunning, command string, args, result any) error {
	err := credentialCall(c, state, command, args, result)
	if err == nil {
		return nil
	}
	var f *cli.Failure
	if !errors.As(err, &f) {
		return err
	}
	if errors.Is(f.Cause, control.ErrTooLarge) {
		return proposalTooLarge(f.Cause)
	}
	var remote *control.Error
	if errors.As(f.Cause, &remote) {
		switch remote.Code {
		case control.CodeTooLarge:
			return proposalTooLarge(remote)
		case control.CodeProposalLimit:
			return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message, Usage: true,
				NextStep: "Withdraw a proposal with 'qhost proposal withdraw <id>', then submit again.", Cause: remote}
		case control.CodeNotSubmitter:
			return &cli.Failure{Code: output.CodePermission, Message: remote.Message,
				NextStep: "Only the submitter or root may withdraw it.", Cause: remote}
		case control.CodeProposalChanged:
			return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message + "; nothing was applied", Usage: true,
				NextStep: "Show the proposal with 'qhost proposal show <id>' and review it again.", Cause: remote}
		case control.CodeNotFound:
			return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
				NextStep: "List the proposals with 'qhost proposal list'.", Cause: remote}
		}
	}
	return err
}

func proposalFields(format output.Format, p control.ProposalInfo) []output.Field {
	fields := []output.Field{
		{Name: "id", Value: text(format, p.ID)}, {Name: "kind", Value: text(format, p.Kind)},
		{Name: "state", Value: text(format, p.State)}, {Name: "fingerprint", Value: text(format, p.Fingerprint)},
		{Name: "submitter", Value: text(format, p.SubmitterName)}, {Name: "submitter_uid", Value: p.SubmitterUID},
		{Name: "submitted", Value: proposalTime(p.Submitted)}, {Name: "expires", Value: proposalTime(p.Expires)},
	}
	if p.Message != "" {
		fields = append(fields, output.Field{Name: "message", Value: text(format, p.Message)})
	}
	return fields
}

func proposalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// text keeps a value of the submitter intact in structured output, where encoding escapes it, and makes it
// safe for a terminal otherwise.
func text(format output.Format, s string) string {
	if structuredFormat(format) {
		return s
	}
	return plainText(s)
}

// plainText replaces every control character, line and paragraph separator, bidirectional control and invalid
// byte of untrusted text by U+FFFD, so it cannot move the cursor or reorder what a terminal shows.
func plainText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n <= 1, unicode.IsControl(r), unicode.Is(unicode.Zl, r), unicode.Is(unicode.Zp, r), isBidi(r):
			b.WriteRune(utf8.RuneError)
		default:
			b.WriteRune(r)
		}
		i += n
	}
	return b.String()
}

func isBidi(r rune) bool {
	return r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069 || r == 0x200e || r == 0x200f || r == 0x061c
}
