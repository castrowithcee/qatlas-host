package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	var url string
	c := &cobra.Command{
		Use:   "approve <id> [--url <ssh-url>]",
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
			"the host (kind host-change) is applied with 'sudo qhost host apply --proposal <id>' instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runProposalApprove(c, program, state, secrets, args[0], url)
		},
	}
	c.Flags().StringVar(&url, "url", "", "SSH address of the repository, replacing the proposed one")
	return c
}

func newProposalReject(program *cobracli.Program, state *notRunning) *cobra.Command {
	var reason string
	c := &cobra.Command{
		Use:   "reject <id> [--reason <text>]",
		Short: "Reject an open proposal, with an optional reason the submitter can read",
		Long: "reject closes an open proposal as rejected, at the host service for a proposal of the host and at the deploy\n" +
			"service otherwise. The reason is kept at most 300 characters long and shows in 'qhost proposal show' for the\n" +
			"submitter. Nothing on the host changes. Only a person at a terminal may reject: --agent and input that is\n" +
			"no terminal are refused. Run it with sudo.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requirePerson(program, "rejecting a proposal"); err != nil {
				return err
			}
			var res control.ProposalInfo
			if err := proposalCallTo(c, state, hostProposalID(args[0]), control.ProposalRejectCommand,
				control.ProposalRejectArgs{ID: args[0], Reason: reason}, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: proposalFields(program.Format(), res)})
		},
	}
	c.Flags().StringVar(&reason, "reason", "", "why the proposal is rejected")
	return c
}

// requirePerson refuses everything but root at a terminal that is not driven as an agent. It runs before the
// service is asked anything.
func requirePerson(program *cobracli.Program, what string) error {
	if program.Agent() {
		return &cli.Failure{Code: output.CodePermission, Message: what + " is for a person at a terminal, not for an agent",
			NextStep: "Ask the person who runs the host to run 'sudo qhost proposal' with approve or reject."}
	}
	if f, ok := stdinStream().(*os.File); !ok || !isTerminal(f) {
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
var proposalApprovers = map[string]func(c *cobra.Command, program *cobracli.Program, state *notRunning, secrets *redact.Redactor, shown control.ProposalShowResult, url string) error{
	control.ProposalKindSourceAdd:     approveSourceAdd,
	control.ProposalKindStackSettings: approveStackSettings,
	control.ProposalKindWindow:        approveWindow,
	control.ProposalKindNotifyChannel: approveNotifyChannel,
}

func runProposalApprove(c *cobra.Command, program *cobracli.Program, state *notRunning, secrets *redact.Redactor, id, url string) error {
	if err := requirePerson(program, "approving a proposal"); err != nil {
		return err
	}
	if hostProposalID(id) {
		return usageFailure("a proposal of the host is applied with 'sudo qhost host apply --proposal "+plainText(id)+"'",
			"Review it with 'qhost proposal show "+plainText(id)+"', then run 'sudo qhost host apply --proposal "+plainText(id)+" --confirm'.")
	}
	shown, err := loadOpenProposal(c, state, id)
	if err != nil {
		return err
	}
	approve, ok := proposalApprovers[shown.Kind]
	if !ok {
		return usageFailure("this version cannot apply a proposal of the kind "+shortText(shown.Kind),
			"Reject it with 'sudo qhost proposal reject "+id+"'.")
	}
	return approve(c, program, state, secrets, shown, url)
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

func approveSourceAdd(c *cobra.Command, program *cobracli.Program, state *notRunning, _ *redact.Redactor, first control.ProposalShowResult, url string) error {
	id := first.ID
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

	var seen bytes.Buffer
	sa, err := prepareSourceAdd(c, program, state, io.MultiWriter(notice, &seen), content.Path, url, "", rerun)
	if err != nil {
		return err
	}
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
	sa, err = prepareSourceAdd(c, program, state, &fresh, content.Path, url, fingerprint, rerun)
	if err == nil {
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

	// The service takes the approval before anything is applied, and only for the fingerprint that was shown.
	if err := proposalCall(c, state, control.ProposalApproveCommand,
		control.ProposalApproveArgs{ID: id, Fingerprint: shown.Fingerprint}, &control.ProposalInfo{}); err != nil {
		return err
	}
	applyErr := executeSourceAdd(c, program, state, sa.res, sa.repoURL, fingerprint, sa.perms, sa.remote)
	result := control.ProposalResultArgs{ID: id, Fingerprint: shown.Fingerprint, Applied: applyErr == nil}
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
	var key control.KeyResult
	err := credentialCall(c, state, control.KeyShowCommand, control.KeyShowArgs{Dir: dir}, &key)
	var f *cli.Failure
	var remote *control.Error
	switch {
	case err == nil:
		_, _ = fmt.Fprintf(w, "Deploy key of the folder: %s\n", key.Fingerprint)
		return nil
	case !errors.As(err, &f) || !errors.As(f.Cause, &remote) || remote.Code != control.CodeNoSuchKey:
		return err
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
