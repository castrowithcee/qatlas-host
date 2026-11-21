package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newProposalCommand(program *cobracli.Program, state *notRunning, secrets *redact.Redactor) *cobra.Command {
	group := &cobra.Command{
		Use:   "proposal",
		Short: "Submit, list, show, withdraw, approve and reject proposals of changes to the setup and the host",
		Long: "A proposal asks for a change of the setup, for example a stack folder, or of the host, for example a user.\n" +
			"The service that owns its kind checks and stores it and changes nothing: the host service keeps the kind\n" +
			"host-change, a deploy service every other kind. A host has one deploy service for the system engine and\n" +
			"one for each rootless engine, and a proposal belongs to the one it was submitted to; list shows the\n" +
			"proposals of all services with their engine. Every command runs\n" +
			"without sudo for an administrator in the group qhost; a proposal can be withdrawn by its submitter or by\n" +
			"root. approve and reject are for root at a terminal; a proposal of the host is approved with\n" +
			"'sudo qhost host apply --proposal <id>'. See 'qhost help proposals'.",
	}
	group.AddCommand(newProposalSubmit(program, state), newProposalList(program, state),
		newProposalShow(program, state), newProposalWithdraw(program, state),
		newProposalApprove(program, state, secrets), newProposalReject(program, state),
		newProposalCommandsCommand(program, state), newProposalRunCommand(program, state, secrets))
	return group
}

func newProposalSubmit(program *cobracli.Program, state *notRunning) *cobra.Command {
	var kind, file, engine string
	c := &cobra.Command{
		Use:   "submit --kind source-add|stack-settings|window|notify-channel|host-change|command [--engine <id>] [--file <path>|-]",
		Short: "Submit a proposal read as JSON from a file or standard input",
		Long: "submit sends the content of a proposal to the service of its kind, which checks and stores it, and prints its id. The\n" +
			"content is a JSON object in the schema of the kind, read from --file or, without it or with '-', from\n" +
			"standard input; the file is read as the calling user, at most a little over 64 KiB. The kind source-add\n" +
			"takes {\"path\": \"/absolute/folder\", \"url\": \"ssh://...\"}, url optional. The kind stack-settings takes\n" +
			"{\"stack\": \"name\", \"settings\": {\"version\": 1, ...}}, with the settings of 'qhost stack settings'. The kind window takes\n" +
			"{\"days\": [\"mon\"], \"start\": \"03:00\", \"duration\": \"2h\", \"zone\": \"UTC\"}, at least one part. Nothing\n" +
			"on the host changes. The kind notify-channel takes the fields of 'qhost notify add' except its secret parts, which a\n" +
			"proposal may not name: {\"name\": \"ops\", \"type\": \"webhook\", \"events\": [\"failed\"]}. The kind host-change goes to the\n" +
			"host service and takes one action of 'qhost help host-admin' with its arguments and, for user.create and\n" +
			"user.add-key, public SSH keys: {\"action\": \"user.create\", \"args\": [\"anna\", \"group=dev\"], \"keys\": [\"ssh-ed25519 ...\"]}.\n" +
			"It never holds a password; user.set-password, user.set-admin, user.create with admin and member changes of\n" +
			"the groups sudo and qhost are refused. The kind command goes to the host service as well and takes\n" +
			"{\"commands\": [\"touch /srv/example/ready\"], \"reason\": \"why\"}: one to 10 commands, each one printable line of at\n" +
			"most 1000 characters, and a reason of at most 1000 characters. It is accepted only while an administrator has\n" +
			"turned it on with 'sudo qhost proposal commands enable'. See 'qhost help proposals'.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if kind == "" {
				return usageFailure("--kind is required", "Run 'qhost proposal submit --kind source-add --file <path>'.")
			}
			content, err := readProposalContent(file)
			if err != nil {
				return err
			}
			host := hostKind(kind)
			if host && engine != "" {
				return usageFailure("--engine does not apply to the kind "+kind+", which the host service keeps",
					"Run the command without --engine.")
			}
			if !host {
				if err := routeInstance(c, engine); err != nil {
					return err
				}
			}
			var res control.ProposalInfo
			if err := proposalCallTo(c, state, host, control.ProposalSubmitCommand,
				control.ProposalSubmitArgs{Kind: kind, Content: content}, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: withProposalEngine(c, res.ID, proposalFields(program.Format(), res))})
		},
	}
	c.Flags().StringVar(&kind, "kind", "", "kind of the proposal: source-add, stack-settings, window, notify-channel, host-change or command")
	c.Flags().StringVar(&file, "file", "", "file with the content as JSON, or - for standard input (default)")
	addEngineFlag(c, &engine, "Docker engine whose deploy service stores the proposal: "+engineIDs+" "+instanceDefault,
		"The proposal goes to "+instanceRule+" The proposal itself carries no engine: the deploy service that stored it is "+
			"the one that applies it. The kinds host-change and command go to the host service and take no --engine.")
	return c
}

func newProposalList(program *cobracli.Program, state *notRunning) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the proposals of all services, newest first",
		Long: "list shows every proposal of the host service and of every deploy service with its id, kind, service, engine,\n" +
			"state, submitter and times. The engine is that of the deploy service that stores the proposal; a proposal of\n" +
			"the host has none. An open proposal past its time shows as expired. A service that is not running or not\n" +
			"set up is named on standard error and the proposals of the others are listed; it fails only when none answers.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			listed, err := listProposals(c, state)
			if err != nil {
				return err
			}
			format := program.Format()
			rows := make([]output.Row, 0, len(listed))
			for _, p := range listed {
				rows = append(rows, output.Row{"id": text(format, p.ID), "kind": text(format, p.Kind), "service": p.service,
					"engine": p.engine, "state": text(format, p.State), "submitter": text(format, p.SubmitterName),
					"submitted": proposalTime(p.Submitted), "expires": proposalTime(p.Expires)})
			}
			return output.Encode(c.OutOrStdout(), format, output.Collection{
				Columns: []string{"id", "kind", "service", "engine", "state", "submitter", "submitted", "expires"}, Rows: rows})
		},
	}
}

// The names of the services as list shows them.
const (
	serviceHost   = "host"
	serviceDeploy = "deploy"
)

// listedProposal is one proposal of list with the service that keeps it and, for a deploy service, its engine.
type listedProposal struct {
	control.ProposalInfo
	service string
	engine  string
}

// listProposals asks the host service and every deploy service for their proposals and merges them, the newest
// first. A service that is not running or not set up is named on standard error; only when none answers is it
// an error.
func listProposals(c *cobra.Command, state *notRunning) ([]listedProposal, error) {
	targets, err := folders.engineTargets()
	if err != nil {
		return nil, err
	}
	type source struct {
		svc    string
		target engineTarget
		unset  bool
	}
	var sources []source
	for _, t := range targets {
		sources = append(sources, source{svc: serviceDeploy, target: t})
	}
	if len(targets) == 0 {
		sources = append(sources, source{svc: serviceDeploy, unset: true})
	}
	sources = append(sources, source{svc: serviceHost})
	var all []listedProposal
	var missing []error
	for _, src := range sources {
		var res control.ProposalListResult
		var err error
		engine := ""
		switch {
		case src.svc == serviceHost:
			if err = call(c.Context(), hostEndpoint(), control.ProposalListCommand, nil, &res); err != nil {
				err = hostFailure(err)
			}
		case src.unset:
			err = notSetUp(client.ErrNotRunning)
		default:
			engine = src.target.id()
			if err = call(c.Context(), src.target.endpoint(), control.ProposalListCommand, nil, &res); err != nil {
				err = targetFailure(withEngine(c.Context(), src.target), err)
			}
		}
		if err != nil {
			if !errors.Is(err, client.ErrNotRunning) {
				return nil, proposalFailure(err)
			}
			missing = append(missing, err)
			continue
		}
		for _, p := range res.Proposals {
			all = append(all, listedProposal{ProposalInfo: p, service: src.svc, engine: engine})
		}
		who := "the " + src.svc + " service"
		if engine != "" {
			who = "the deploy service of the " + engine + " engine"
		}
		for _, u := range res.Unreadable {
			_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s has a proposal that cannot be read: %s\n", who, u.Error)
		}
	}
	if len(missing) == len(sources) {
		state.seen = true
		who := "the deploy service"
		if len(targets) > 1 {
			who = "any deploy service"
		}
		return nil, &cli.Failure{Code: output.CodeUnreachable, Message: "neither the host service nor " + who + " answers",
			NextStep: "Start the host service with 'sudo systemctl start qhost', then run the command again.", Cause: missing[0]}
	}
	for _, err := range missing {
		var f *cli.Failure
		if errors.As(err, &f) {
			_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s; its proposals are not listed\n", f.Message)
		}
	}
	// Each service answers newest first; the stable merge keeps its order among proposals of the same time.
	sort.SliceStable(all, func(i, j int) bool { return all[i].Submitted.After(all[j].Submitted) })
	return all, nil
}

// withProposalEngine adds the engine the proposal belongs to, which is that of the deploy service the command
// reached; a proposal of the host has none.
func withProposalEngine(c *cobra.Command, id string, fields []output.Field) []output.Field {
	if hostProposalID(id) {
		return fields
	}
	return append(fields, output.Field{Name: "engine", Value: targetOf(c.Context()).id()})
}

// locateProposal finds the deploy service that stores the proposal. A named engine is taken as it is. On a host
// with one deploy service nothing is asked. Otherwise every service is asked for the proposal: one that holds it
// is the target, two stop the command, and a service that could not be asked is a warning, or an error when the
// proposal is in none of the others, so that a proposal is never applied by another service than its own.
func (h folderHost) locateProposal(ctx context.Context, warn io.Writer, engine, id string) (engineTarget, error) {
	if engine != "" {
		return h.namedEngine(engine)
	}
	targets, err := h.engineTargets()
	if err != nil {
		return engineTarget{}, err
	}
	if len(targets) <= 1 {
		return firstTarget(targets), nil
	}
	errs := make([]error, len(targets))
	held := make([]bool, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Go(func() {
			var res control.ProposalShowResult
			err := callFor(ctx, peerLimit, t.endpoint(), control.ProposalShowCommand, control.ProposalIDArgs{ID: id}, &res)
			var remote *control.Error
			switch {
			case err == nil:
				held[i] = true
			case errors.As(err, &remote) && remote.Code == control.CodeNotFound:
			default:
				errs[i] = err
			}
		})
	}
	wg.Wait()
	var found []engineTarget
	var unchecked, answers []engineAnswer
	for i, t := range targets {
		answers = append(answers, engineAnswer{target: t, err: errs[i]})
		switch {
		case errs[i] != nil:
			unchecked = append(unchecked, engineAnswer{target: t, err: errs[i]})
		case held[i]:
			found = append(found, t)
		}
	}
	name := shortText(id)
	switch {
	case len(found) > 1:
		return engineTarget{}, usageFailure(fmt.Sprintf("the proposal %s exists in the engines %s", name, targetIDs(found)),
			"Choose one with --engine, for example --engine "+found[0].id()+".")
	case len(found) == 0 && len(unchecked) == len(answers):
		return engineTarget{}, targetFailure(withEngine(ctx, unchecked[0].target), unchecked[0].err)
	case len(found) == 0 && len(unchecked) > 0:
		return engineTarget{}, &cli.Failure{Code: output.CodeUnreachable,
			Message: fmt.Sprintf("the proposal %s is in none of the engines that answered; %s", name, uncheckedList(unchecked)),
			NextStep: "Start that deploy service and run the command again, or name its engine with --engine " +
				unchecked[0].engine() + ".", Cause: unchecked[0].err}
	case len(found) == 0:
		return engineTarget{}, &cli.Failure{Code: output.CodeNotFound,
			Message:  fmt.Sprintf("no such proposal: %s is in none of the engines %s", name, answerIDs(answers)),
			NextStep: "List the proposals with 'qhost proposal list'."}
	}
	for _, u := range unchecked {
		_, _ = fmt.Fprintln(warn, "warning: "+uncheckedText(u))
	}
	return found[0], nil
}

// routeProposal makes the deploy service that stores the proposal the target of every later call of the
// command. A proposal of the host needs no routing.
func routeProposal(c *cobra.Command, state *notRunning, engine, id string) error {
	if hostProposalID(id) {
		if engine != "" {
			return usageFailure("--engine does not apply to a proposal of the host", "Run the command without --engine.")
		}
		return nil
	}
	t, err := folders.locateProposal(c.Context(), c.ErrOrStderr(), engine, id)
	if err != nil {
		if errors.Is(err, client.ErrNotRunning) {
			state.seen = true
		}
		return err
	}
	c.SetContext(withEngine(c.Context(), t))
	return nil
}

// hostProposalID reports whether id names a proposal of the host service.
func hostProposalID(id string) bool { return strings.HasPrefix(id, control.HostProposalIDPrefix) }

// hostKind reports whether the host service keeps proposals of kind.
func hostKind(kind string) bool {
	return kind == control.ProposalKindHostChange || kind == control.ProposalKindCommand
}

func newProposalShow(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	c := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one proposal with its content",
		Long: "show prints a proposal with its fingerprint, the submitter and the content as it was submitted. For a proposal\n" +
			"of the host it also prints the sentence and the commands of the change, as 'sudo qhost host apply --proposal'\n" +
			"would run them, and the type, fingerprint and comment of each proposed SSH key. For a proposal of the kind\n" +
			"command it prints the commands and the reason word for word as text of the submitter, with control\n" +
			"characters replaced, and the notice: unchecked, no computed effect, no undo.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := routeProposal(c, state, engine, args[0]); err != nil {
				return err
			}
			var res control.ProposalShowResult
			if err := proposalCallTo(c, state, hostProposalID(args[0]), control.ProposalShowCommand, control.ProposalIDArgs{ID: args[0]}, &res); err != nil {
				return err
			}
			format := program.Format()
			fields := withProposalEngine(c, res.ID, proposalFields(format, res.ProposalInfo))
			if res.Kind == control.ProposalKindHostChange {
				fields = append(fields, hostChangeFields(program, res)...)
			}
			if res.Kind == control.ProposalKindCommand {
				fields = append(fields, commandProposalFields(format, res)...)
			}
			var content map[string]any
			if err := json.Unmarshal(res.Content, &content); err != nil {
				content = map[string]any{}
			}
			if structuredFormat(format) {
				fields = append(fields, output.Field{Name: "content", Value: content})
			} else if res.Kind != control.ProposalKindCommand {
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
	addEngineFlag(c, &engine, proposalEngineUsage, proposalEngineNote)
	return c
}

// The usage of --engine of the commands that name a proposal, and the note in their help.
const (
	proposalEngineUsage = "Docker engine whose deploy service stores the proposal: " + engineIDs + "; needed only when several engines have a proposal of that id"
	proposalEngineNote  = "The command looks for the proposal in the deploy services of all engines and reaches the one that stores it. " +
		"When several do, it stops and names them; " + engineChoice + " chooses one. A deploy service that cannot be " +
		"asked is named in a warning, or in the error when none holds the proposal. A proposal of the host needs no --engine."
)

func newProposalWithdraw(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	c := &cobra.Command{
		Use:   "withdraw <id>",
		Short: "Withdraw an open proposal",
		Long:  "withdraw closes an open proposal of your own as withdrawn; root may withdraw any. It changes nothing else.",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := routeProposal(c, state, engine, args[0]); err != nil {
				return err
			}
			var res control.ProposalInfo
			if err := proposalCallTo(c, state, hostProposalID(args[0]), control.ProposalWithdrawCommand, control.ProposalIDArgs{ID: args[0]}, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: withProposalEngine(c, res.ID, proposalFields(program.Format(), res))})
		},
	}
	addEngineFlag(c, &engine, proposalEngineUsage, proposalEngineNote)
	return c
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

// proposalCall asks the deploy service; see proposalCallTo.
func proposalCall(c *cobra.Command, state *notRunning, command string, args, result any) error {
	return proposalCallTo(c, state, false, command, args, result)
}

// proposalCallTo asks the host service when host is set and the deploy service otherwise, and maps the codes of
// the proposal commands; any other failure stays as the shared mapping made it.
func proposalCallTo(c *cobra.Command, state *notRunning, host bool, command string, args, result any) error {
	var err error
	if host {
		err = hostCall(c, state, command, args, result)
	} else {
		err = credentialCall(c, state, command, args, result)
	}
	if err == nil {
		return nil
	}
	return proposalFailure(err)
}

// proposalFailure maps the codes of the proposal commands in a failure of the shared mapping.
func proposalFailure(err error) error {
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
		case control.CodeCommandsDisabled:
			return commandProposalFailure(remote)
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
