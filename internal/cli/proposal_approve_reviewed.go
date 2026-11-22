package cli

import (
	"bytes"
	"encoding/json"
	"errors"
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

// approveOptions are the flags of 'proposal approve'. Without reviewed the person is asked at the terminal; with it
// the run is bound to the proposal as it was reviewed and asks nothing.
type approveOptions struct {
	url string
	// engine names the deploy service that stores the proposal, when several do.
	engine string
	// reviewed is the fingerprint of the proposal as the person reviewed it.
	reviewed string
	// unconfirmed is the error of the missing --confirm; nil when it was given.
	unconfirmed error
	// hostKey, expectEffect, expectState and expectCommit bind a source-add proposal to the state that was shown.
	hostKey, expectEffect, expectState, expectCommit string
	// include is the choice of unclear files of a source-add proposal.
	include []string
}

// asks says the person is asked at the terminal.
func (o approveOptions) asks() bool { return o.reviewed == "" }

// check refuses the flags that belong to --fingerprint without it. It runs before the service is asked anything.
func (o approveOptions) check() error {
	if o.hostKey != "" && !strings.HasPrefix(o.hostKey, "SHA256:") {
		return usageFailure("--host-key-fingerprint must look like SHA256:..., as ssh-keygen -l prints it",
			"Copy the fingerprint of the server's host key.")
	}
	if !o.asks() {
		return nil
	}
	fix := "Run 'sudo qhost proposal approve <id>' to be asked, or add --fingerprint of the proposal you reviewed."
	if o.unconfirmed == nil {
		return usageFailure("--confirm belongs to --fingerprint", fix)
	}
	if o.hostKey != "" || o.expectEffect != "" || o.expectState != "" || o.expectCommit != "" {
		return usageFailure("--host-key-fingerprint, --expect-effect, --expect-state and --expect-commit belong to --fingerprint", fix)
	}
	return nil
}

// onlyForSourceAdd refuses the flags that only a source-add proposal takes.
func (o approveOptions) onlyForSourceAdd(id string) error {
	if o.url != "" || o.hostKey != "" || o.expectEffect != "" || o.expectState != "" || o.expectCommit != "" || len(o.include) > 0 {
		return usageFailure("--url, --include, --host-key-fingerprint, --expect-effect, --expect-state and --expect-commit apply to proposals of the kind source-add only",
			"Run 'sudo qhost proposal approve "+id+"' without them.")
	}
	return nil
}

// decide asks at the terminal, or, bound to a reviewed fingerprint, ends a run without --confirm after the proposal
// and its effect were shown. With --confirm the reviewed fingerprint is the approval.
func (o approveOptions) decide(c *cobra.Command, question string) error {
	switch {
	case o.asks():
		return askApproval(c, question)
	case o.unconfirmed != nil:
		return o.unconfirmed
	}
	return nil
}

// notReviewed refuses a proposal whose fingerprint is not the one the person reviewed.
func notReviewed(id string) error {
	return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true,
		Message:  "the proposal is not the one that was shown; nothing was applied",
		NextStep: "Review it with 'qhost proposal show " + plainText(id) + "' and run the command again."}
}

// approveSourceAddReviewed applies a source-add proposal without a question. The host key is confirmed by
// --host-key-fingerprint or was pinned before, the folder and the remote must be in the state --expect-effect,
// --expect-state and --expect-commit name, and a folder without deploy key is refused rather than given one.
// Without --confirm it stops after showing the effect.
func approveSourceAddReviewed(c *cobra.Command, program *cobracli.Program, state *notRunning, shown control.ProposalShowResult,
	content control.ProposalSourceAdd, url, rerun string, o approveOptions) error {
	id, notice := shown.ID, c.ErrOrStderr()
	expect, err := expectedRemoteOf(o.expectState, o.expectCommit)
	if err != nil {
		return err
	}
	c.SetContext(withoutHostKeyQuestion(c.Context(), "--host-key-fingerprint"))
	var seen bytes.Buffer
	both := io.MultiWriter(notice, &seen)
	sa, err := prepareSourceAdd(c, program, state, both, content.Path, url, o.hostKey, rerun, expect, o.include, false)
	if err != nil {
		return err
	}
	warnOtherEngine(c.Context(), both, sa.res)
	if err := checkExpectedEffect(c.Context(), o.expectEffect, sa); err != nil {
		return err
	}
	effect, err := effectOf(sa, seen.String())
	if err != nil {
		return err
	}
	hostKey := ""
	switch {
	case sa.remote != nil:
		hostKey = sa.remote.fingerprint
	case sa.res.Case == control.SourceCaseClone || sa.res.Case == control.SourceCaseAdopt:
		probe, err := trustHostKey(c, state, sa.repoURL, o.hostKey)
		if err != nil {
			return err
		}
		hostKey = probe.Fingerprint
	}
	if sa.repoURL != "" {
		key, found, err := findDeployKey(c, state, content.Path)
		if err != nil {
			return err
		}
		if !found {
			return &cli.Failure{Code: output.CodeConfigMissing, Message: "the folder has no deploy key; nothing was applied",
				NextStep: "Create it with 'sudo qhost key generate --dir " + content.Path +
					"', enter its public key in the git platform, then run the command again."}
		}
		_, _ = fmt.Fprintf(notice, "Deploy key of the folder: %s\n", key.Fingerprint)
	}
	if o.unconfirmed != nil {
		if e, err := newSourceEffect(sa.res, sa.perms); err == nil {
			_, _ = fmt.Fprintf(notice, "Effect fingerprint of the folder: %s\n", e.fingerprint)
		}
		return o.unconfirmed
	}

	// Last look before anything changes: the proposal and its effect must be what was shown.
	now, _, err := loadSourceProposal(c, state, id)
	if err != nil {
		return err
	}
	if now.Fingerprint != shown.Fingerprint {
		return notReviewed(id)
	}
	var fresh bytes.Buffer
	sa, err = prepareSourceAdd(c, program, state, &fresh, content.Path, url, hostKey, rerun, expect, sa.include, false)
	if err != nil {
		return err
	}
	warnOtherEngine(c.Context(), &fresh, sa.res)
	again, err := effectOf(sa, fresh.String())
	if err != nil {
		return err
	}
	if again != effect {
		_, _ = fmt.Fprint(notice, "\nThe effect changed since it was shown. It is now:\n", fresh.String())
		return &cli.Failure{Code: output.Code(codeStateChanged),
			Message:  "the effect of the proposal changed since it was shown; nothing was applied",
			NextStep: "Review the proposal and the folder again, then run the command again."}
	}
	return applySourceProposal(c, program, state, id, shown.Fingerprint, sa, hostKey)
}

// channelSecrets are the secret parts of a notify channel as standard input holds them: secret is the first value
// 'qhost notify add' asks for, key the second (the signing key of a webhook).
type channelSecrets struct {
	Secret string `json:"secret"`
	Key    string `json:"key"`
}

// readSecretsJSON reads the secret parts of a channel from standard input as one JSON object, one value per
// prompt. Every value read is added to secrets before anything can fail on it.
func readSecretsJSON(prompts []string, secrets *redact.Redactor) ([]string, error) {
	if len(prompts) == 0 {
		return nil, nil
	}
	shape := `{"secret": "..."}`
	if len(prompts) == 2 {
		shape = `{"secret": "...", "key": "..."}`
	}
	fix := "Provide " + strings.Join(prompts, " and ") + " as " + shape + " on standard input."
	raw, err := readChangeSecret(stdinStream())
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	secrets.Add(string(raw))
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var in channelSecrets
	if err := dec.Decode(&in); err != nil {
		return nil, usageFailure("standard input must hold the secret parts as "+shape, fix)
	}
	for _, v := range []string{in.Secret, in.Key} {
		if v != "" {
			secrets.Add(v)
		}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, usageFailure("standard input has data after the JSON object", fix)
	}
	switch {
	case in.Secret == "" || (len(prompts) == 2 && in.Key == ""):
		return nil, usageFailure("standard input lacks a secret part: "+strings.Join(prompts, " and "), fix)
	case len(prompts) == 1 && in.Key != "":
		return nil, usageFailure("this channel takes no key; standard input holds "+prompts[0]+" only", fix)
	}
	return []string{in.Secret, in.Key}[:len(prompts)], nil
}
