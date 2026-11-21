package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/approval"
	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// effectLabel separates the fingerprint of an effect from every other hash of the same bytes.
const effectLabel = "qhost/proposal-effect/v1"

// shownLimit is how many characters of a proposed value are shown before the rest is only counted.
const shownLimit = 200

func newProposalApprove(program *cobracli.Program, state *notRunning, secrets *redact.Redactor) *cobra.Command {
	var o approveOptions
	var gate *cobracli.Gate
	c := &cobra.Command{
		Use:   "approve <id> [--url <ssh-url>] [--fingerprint <fingerprint> [--confirm]]",
		Short: "Review an open proposal with its effect and apply it after you confirm",
		Long: "approve shows the proposal and, below it, what applying it would change. For a source-add proposal it is\n" +
			"worked out with the same check as 'qhost source add'; for a stack-settings proposal it is the difference to\n" +
			"the settings that apply now, with every value in full and a backup command marked, and the settings are\n" +
			"written the way 'qhost stack settings set' writes them; for a window proposal it is the difference to the window\n" +
			"that applies now, and the window is set the way 'qhost window set' sets it; for a notify-channel proposal every field of\n" +
			"the channel is shown in full, you enter its secret parts as 'qhost notify add' asks for them (hidden at the\n" +
			"terminal, never kept on the proposal), and the channel is added the way 'qhost notify add' adds it. For a\n" +
			"source-add proposal you confirm the server's host key yourself (a proposal never does), add the deploy\n" +
			"key if the folder has none, and answer a last question. Right before anything is changed the proposal is\n" +
			"read again and the effect is worked out again; if either differs from what you saw, nothing is applied and\n" +
			"the new effect is shown. The change is then made the way 'qhost source add' makes it, and the result is\n" +
			"kept on the proposal. --url replaces the proposed address. Only a person at a terminal may approve:\n" +
			"there is no --yes, and --agent and input that is no terminal are refused. Run it with sudo. A proposal of\n" +
			"the host (kind host-change) is applied with 'sudo qhost host apply --proposal <id>' instead.\n\n" +
			"--fingerprint approves without a question instead, bound to the proposal as you reviewed it with 'qhost\n" +
			"proposal show': a proposal with another fingerprint is refused. Without --confirm the command shows the\n" +
			"proposal and its effect and stops with exit code 2, without asking, without reading standard input and\n" +
			"without taking the approval; with --confirm it applies the proposal after the same last look. The secret\n" +
			"parts of a notify-channel proposal then come on standard input only, as {\"secret\": \"...\", \"key\": \"...\"}\n" +
			"in the order 'qhost notify add' asks for them (key only for the signing key of a webhook). For a source-add\n" +
			"proposal no question is asked either: --host-key-fingerprint confirms the server's host key unless it is\n" +
			"pinned already, --expect-effect binds the run to the effect 'qhost source inspect' shows, and\n" +
			"--expect-state with --expect-commit to the comparison 'qhost source compare' shows, as for 'qhost source\n" +
			"add'; when the folder or the remote changed, nothing is applied and the command fails with state-changed.\n" +
			"A folder without deploy key is refused; create the key with 'qhost key generate' first. --agent is refused\n" +
			"here as well; a process without a terminal on standard input is accepted when it has a terminal of its own,\n" +
			"as when the terminal interface runs the command. The result is printed as an object: the fields of 'qhost\n" +
			"source add' for source-add, proposal, stack, changed and commit for stack-settings, proposal and the fields\n" +
			"of 'qhost window show' for window, and proposal, name, type, events and settings for notify-channel.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			// Without --confirm a run with --fingerprint shows the proposal and this error then ends it.
			o.unconfirmed = gate.Run(cobracli.Request{Changes: true}, func() error { return nil })
			return runProposalApprove(c, program, state, secrets, args[0], o)
		},
	}
	addEngineFlag(c, &o.engine, proposalEngineUsage, proposalEngineNote+" The approval is applied only in that engine, "+
		"and the command shows it; the proposal of a rootless engine is applied by the way 'qhost source add --engine' takes a "+
		"folder over, and an engine never changes silently: to use another one, reject the proposal and submit it again.")
	c.Flags().StringVar(&o.url, "url", "", "SSH address of the repository, replacing the proposed one")
	c.Flags().StringVar(&o.reviewed, "fingerprint", "", "fingerprint of the proposal as you reviewed it; approves without a question")
	c.Flags().StringVar(&o.hostKey, "host-key-fingerprint", "",
		"source-add with --fingerprint: expected SHA256 fingerprint of the server's host key")
	c.Flags().StringVar(&o.expectEffect, "expect-effect", "",
		"source-add with --fingerprint: effect fingerprint from 'source inspect'; refuses with state-changed when it differs now")
	remoteFlags(c, &o.expectState, &o.expectCommit)
	gate = program.AddGate(c, cobracli.Gates{Change: true})
	return c
}

func newProposalReject(program *cobracli.Program, state *notRunning) *cobra.Command {
	var reason, engine string
	c := &cobra.Command{
		Use:   "reject <id> [--reason <text>] [--engine <id>]",
		Short: "Reject an open proposal, with an optional reason the submitter can read",
		Long: "reject closes an open proposal as rejected, at the host service for a proposal of the host and at the deploy\n" +
			"service that stores it otherwise. The reason is kept at most 300 characters long and shows in 'qhost proposal show' for the\n" +
			"submitter. Nothing on the host changes. Only a person at a terminal may reject: --agent and a process\n" +
			"without a terminal are refused; a terminal of the process itself is enough, so the terminal interface may\n" +
			"run it with standard input that is no terminal. Run it with sudo. The result is the proposal as\n" +
			"'qhost proposal show' prints its fields, as JSON with --output json.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requirePerson(program, "rejecting a proposal", true); err != nil {
				return err
			}
			if err := routeProposal(c, state, engine, args[0]); err != nil {
				return err
			}
			var res control.ProposalInfo
			if err := proposalCallTo(c, state, hostProposalID(args[0]), control.ProposalRejectCommand,
				control.ProposalRejectArgs{ID: args[0], Reason: reason}, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: withProposalEngine(c, res.ID, proposalFields(program.Format(), res))})
		},
	}
	c.Flags().StringVar(&reason, "reason", "", "why the proposal is rejected")
	addEngineFlag(c, &engine, proposalEngineUsage, proposalEngineNote)
	return c
}

// requirePerson refuses everything but root at a terminal that is not driven as an agent. It runs before the
// service is asked anything. Standard input must be the terminal, unless controlling allows a terminal of the
// process itself, for a command that asks nothing at it.
func requirePerson(program *cobracli.Program, what string, controlling bool) error {
	if program.Agent() {
		return &cli.Failure{Code: output.CodePermission, Message: what + " is for a person at a terminal, not for an agent",
			NextStep: "Ask the person who runs the host to run 'sudo qhost proposal' with approve or reject."}
	}
	if !stdinIsTerminal() && (!controlling || !controllingTerminal()) {
		return &cli.Failure{Code: output.CodePermission, Message: what + " needs a terminal; there is no way around it",
			NextStep: "Run the command at a terminal with sudo."}
	}
	if folders.euid() != 0 {
		return &cli.Failure{Code: output.CodePermission, Message: what + " needs root; run with sudo",
			NextStep: "Run the same command with sudo."}
	}
	return nil
}

// proposalApprovers has one function per kind of proposal that can be approved. It gets the proposal as it was
// first read, open and of its kind, and does the review and the change of that kind.
var proposalApprovers = map[string]func(c *cobra.Command, program *cobracli.Program, state *notRunning, secrets *redact.Redactor, shown control.ProposalShowResult, o approveOptions) error{
	control.ProposalKindSourceAdd:     approveSourceAdd,
	control.ProposalKindStackSettings: approveStackSettings,
	control.ProposalKindWindow:        approveWindow,
	control.ProposalKindNotifyChannel: approveNotifyChannel,
}

func runProposalApprove(c *cobra.Command, program *cobracli.Program, state *notRunning, secrets *redact.Redactor, id string, o approveOptions) error {
	if err := o.check(); err != nil {
		return err
	}
	if err := requirePerson(program, "approving a proposal", !o.asks()); err != nil {
		return err
	}
	if hostProposalID(id) {
		var shown control.ProposalShowResult
		if err := proposalCallTo(c, state, true, control.ProposalShowCommand, control.ProposalIDArgs{ID: id}, &shown); err == nil &&
			shown.Kind == control.ProposalKindCommand {
			return runInstead(id)
		}
		return usageFailure("a proposal of the host is applied with 'sudo qhost host apply --proposal "+plainText(id)+"'",
			"Review it with 'qhost proposal show "+plainText(id)+"', then run 'sudo qhost host apply --proposal "+plainText(id)+" --confirm'.")
	}
	if err := routeProposal(c, state, o.engine, id); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.ErrOrStderr(), "Engine: %s\n", targetOf(c.Context()).id())
	shown, err := loadOpenProposal(c, state, id)
	if err != nil {
		return err
	}
	if !o.asks() && o.reviewed != shown.Fingerprint {
		return notReviewed(id)
	}
	approve, ok := proposalApprovers[shown.Kind]
	if !ok {
		return usageFailure("this version cannot apply a proposal of the kind "+shortText(shown.Kind),
			"Reject it with 'sudo qhost proposal reject "+id+"'.")
	}
	return approve(c, program, state, secrets, shown, o)
}

// loadOpenProposal reads a proposal and refuses one that is not open.
func loadOpenProposal(c *cobra.Command, state *notRunning, id string) (control.ProposalShowResult, error) {
	var res control.ProposalShowResult
	if err := proposalCall(c, state, control.ProposalShowCommand, control.ProposalIDArgs{ID: id}, &res); err != nil {
		return res, err
	}
	if res.State != control.ProposalOpen {
		return res, usageFailure("the proposal is "+plainText(res.State)+", not open",
			"List the proposals with 'qhost proposal list'.")
	}
	return res, nil
}

func approveSourceAdd(c *cobra.Command, program *cobracli.Program, state *notRunning, _ *redact.Redactor, first control.ProposalShowResult, o approveOptions) error {
	id, url := first.ID, o.url
	shown, content, err := decodeSourceProposal(first)
	if err != nil {
		return err
	}
	if url == "" {
		url = content.URL
	} else if err := gitrepo.ValidateURL(url); err != nil {
		return usageFailure(err.Error(), "Pass the SSH address of the repository.")
	}
	notice := c.ErrOrStderr()
	showProposal(notice, shown.ProposalInfo, content, url)
	rerun := "Run 'sudo qhost proposal approve " + id + " --url <ssh-url>'."
	if !o.asks() {
		return approveSourceAddReviewed(c, program, state, shown, content, url, rerun, o)
	}

	var seen bytes.Buffer
	both := io.MultiWriter(notice, &seen)
	sa, err := prepareSourceAdd(c, program, state, both, content.Path, url, "", rerun, nil)
	if err != nil {
		return err
	}
	warnOtherEngine(c.Context(), both, sa.res)
	effect, err := effectOf(sa, seen.String())
	if err != nil {
		return err
	}
	// The host key is confirmed here by the person; the proposal never carries one.
	fingerprint := ""
	switch {
	case sa.remote != nil:
		fingerprint = sa.remote.fingerprint
	case sa.res.Case == control.SourceCaseClone || sa.res.Case == control.SourceCaseAdopt:
		probe, err := trustHostKey(c, state, sa.repoURL, "")
		if err != nil {
			return err
		}
		fingerprint = probe.Fingerprint
	}
	if sa.repoURL != "" {
		if err := ensureDeployKey(c, state, content.Path); err != nil {
			return err
		}
	}
	approveQuestion := "Approve proposal " + shown.ID + " and apply these changes?"
	if warn := conflictConsequence(sa.res); warn != "" {
		approveQuestion = warn + " " + approveQuestion
	}
	if err := askApproval(c, approveQuestion); err != nil {
		return err
	}

	// Last look before anything changes: the proposal and its effect must be what was shown.
	now, _, err := loadSourceProposal(c, state, id)
	if err != nil {
		return err
	}
	if now.Fingerprint != shown.Fingerprint {
		return usageFailure("the proposal changed since it was shown; nothing was applied",
			"Run 'sudo qhost proposal approve "+id+"' again and review it.")
	}
	var fresh bytes.Buffer
	sa, err = prepareSourceAdd(c, program, state, &fresh, content.Path, url, fingerprint, rerun, nil)
	if err == nil {
		warnOtherEngine(c.Context(), &fresh, sa.res)
		var again string
		if again, err = effectOf(sa, fresh.String()); err == nil && again != effect {
			_, _ = fmt.Fprint(notice, "\nThe effect changed since it was shown. It is now:\n", fresh.String())
			return usageFailure("the effect of the proposal changed since it was shown; nothing was applied",
				"Run 'sudo qhost proposal approve "+id+"' again and review the new effect.")
		}
	}
	if err != nil {
		return err
	}

	return applySourceProposal(c, program, state, id, shown.Fingerprint, sa, fingerprint)
}

// warnOtherEngine warns when containers of the folder run in an engine other than the one whose deploy service
// stores the proposal. It names the consequence and the way to another engine, and blocks nothing.
func warnOtherEngine(ctx context.Context, w io.Writer, res control.SourceInspectResult) {
	others := suggestedEngines(res)
	if len(others) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "warning: containers of this folder run in the engine %s, but this proposal belongs to the %s engine; "+
		"deploying would create duplicate containers and take their ports twice. To use the engine %s instead, reject the "+
		"proposal and submit it again with --engine %s, or take the folder over by hand with 'sudo qhost source add %s --engine %s'.\n",
		strings.Join(others, ", "), targetOf(ctx).id(), others[0], others[0], shortText(res.Path), others[0])
}

// applySourceProposal takes the approval at the service, only for the fingerprint that was shown, adds the
// folder the way 'qhost source add' does and keeps the result on the proposal.
func applySourceProposal(c *cobra.Command, program *cobracli.Program, state *notRunning, id, proposal string, sa *sourceAdd, hostKey string) error {
	if err := proposalCall(c, state, control.ProposalApproveCommand,
		control.ProposalApproveArgs{ID: id, Fingerprint: proposal}, &control.ProposalInfo{}); err != nil {
		return err
	}
	applyErr := executeSourceAdd(c, program, state, sa.res, sa.repoURL, hostKey, sa.perms, sa.remote)
	result := control.ProposalResultArgs{ID: id, Fingerprint: proposal, Applied: applyErr == nil}
	if applyErr != nil {
		result.Message = failureText(applyErr)
	}
	if err := proposalCall(c, state, control.ProposalResultCommand, result, &control.ProposalInfo{}); err != nil {
		note := "the change was applied, but its result could not be kept on the proposal"
		if applyErr != nil {
			note = "the change failed, and its result could not be kept on the proposal"
		}
		return &cli.Failure{Code: output.CodeRuntime, Message: note, NextStep: "Run 'qhost proposal show " + id + "' to see its state.", Cause: err}
	}
	return applyErr
}

// loadSourceProposal reads a proposal of the kind source-add that is still open.
func loadSourceProposal(c *cobra.Command, state *notRunning, id string) (control.ProposalShowResult, control.ProposalSourceAdd, error) {
	res, err := loadOpenProposal(c, state, id)
	if err != nil {
		return res, control.ProposalSourceAdd{}, err
	}
	return decodeSourceProposal(res)
}

func decodeSourceProposal(res control.ProposalShowResult) (control.ProposalShowResult, control.ProposalSourceAdd, error) {
	var content control.ProposalSourceAdd
	id := res.ID
	if res.Kind != control.ProposalKindSourceAdd {
		return res, content, usageFailure("this version cannot apply a proposal of the kind "+shortText(res.Kind),
			"Reject it with 'sudo qhost proposal reject "+id+"'.")
	}
	if err := json.Unmarshal(res.Content, &content); err != nil {
		return res, content, usageFailure("the content of the proposal cannot be read", "Reject it with 'sudo qhost proposal reject "+id+"'.")
	}
	if !filepath.IsAbs(content.Path) || filepath.Clean(content.Path) != content.Path {
		return res, content, usageFailure("the proposed path is not an absolute, clean path", "Reject it with 'sudo qhost proposal reject "+id+"'.")
	}
	return res, content, nil
}

func showProposal(w io.Writer, p control.ProposalInfo, content control.ProposalSourceAdd, url string) {
	_, _ = fmt.Fprintf(w, "Proposal %s, kind %s, from %s, submitted %s, expires %s:\n",
		plainText(p.ID), shortText(p.Kind), shortText(p.SubmitterName), proposalTime(p.Submitted), proposalTime(p.Expires))
	_, _ = fmt.Fprintf(w, "  path: %s\n", shortText(content.Path))
	proposed := shortText(content.URL)
	if proposed == "" {
		proposed = "(none)"
	}
	_, _ = fmt.Fprintf(w, "  url:  %s\n", proposed)
	if url != content.URL {
		_, _ = fmt.Fprintf(w, "  You replace the proposed address with: %s\n", shortText(url))
	}
	_, _ = fmt.Fprintln(w, "Effect:")
}

// shortText makes untrusted text safe for a terminal and cuts what is longer than shownLimit characters.
func shortText(s string) string {
	s = plainText(s)
	if r := []rune(s); len(r) > shownLimit {
		return string(r[:shownLimit]) + fmt.Sprintf("... (%d more characters)", len(r)-shownLimit)
	}
	return s
}

// effectOf is the fingerprint of what applying the proposal would do: every line shown and the complete lists
// of files, of which the text shows only the first ones.
func effectOf(sa *sourceAdd, text string) (string, error) {
	var left []string
	for _, e := range sa.res.Excluded {
		left = append(left, e.Path+": "+e.Reason)
	}
	return approval.Fingerprint(effectLabel, struct {
		Text     string   `json:"text"`
		Question string   `json:"question"`
		Files    []string `json:"files"`
		Excluded []string `json:"excluded"`
		Engines  []string `json:"engines"`
	}{text, sa.question, sa.res.PlannedFiles, left, engineSignature(sa.res)})
}

// ensureDeployKey shows the deploy key of the folder, or offers to generate one and waits until the person has
// entered it in the git platform.
func ensureDeployKey(c *cobra.Command, state *notRunning, dir string) error {
	w := c.ErrOrStderr()
	key, found, err := findDeployKey(c, state, dir)
	switch {
	case err != nil:
		return err
	case found:
		_, _ = fmt.Fprintf(w, "Deploy key of the folder: %s\n", key.Fingerprint)
		return nil
	}
	if !askYes(c, "The folder has no deploy key yet. Generate one now?") {
		return &cli.Failure{Code: output.CodeConfirmationDeclined, Message: "no deploy key was made; nothing was applied",
			NextStep: "Run the command again, or create the key with 'sudo qhost key generate --dir " + dir + "'."}
	}
	if err := credentialCall(c, state, control.KeyGenerateCommand, control.KeyGenerateArgs{Dir: dir}, &key); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Deploy key %s:\n%s\nEnter this public key in the git platform for the repository first.\n",
		key.Fingerprint, strings.TrimSpace(key.PublicKey))
	return nil
}

// findDeployKey reads the deploy key of the folder; found is false when the folder has none.
func findDeployKey(c *cobra.Command, state *notRunning, dir string) (key control.KeyResult, found bool, err error) {
	err = credentialCall(c, state, control.KeyShowCommand, control.KeyShowArgs{Dir: dir}, &key)
	var f *cli.Failure
	var remote *control.Error
	switch {
	case err == nil:
		return key, true, nil
	case errors.As(err, &f) && errors.As(f.Cause, &remote) && remote.Code == control.CodeNoSuchKey:
		return key, false, nil
	}
	return key, false, err
}

func askApproval(c *cobra.Command, question string) error {
	if askYes(c, question) {
		return nil
	}
	return &cli.Failure{Code: output.CodeConfirmationDeclined, Message: "the proposal was not approved; nothing was changed",
		NextStep: "Run the command again to review it, or reject it with 'sudo qhost proposal reject <id>'."}
}

// askYes asks at the terminal; anything but y or yes, and an input that cannot be read, answers no.
func askYes(c *cobra.Command, question string) bool {
	_, _ = fmt.Fprint(c.ErrOrStderr(), question+" [y/N] ")
	line, _ := bufio.NewReader(stdinStream()).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// failureText is the message of a failure as the submitter may read it.
func failureText(err error) string {
	var f *cli.Failure
	if errors.As(err, &f) {
		return f.Message
	}
	return err.Error()
}
