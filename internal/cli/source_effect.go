package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/approval"
	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// sourceEffectLabel separates the fingerprint of the effect of source add from every other hash.
const sourceEffectLabel = "qhost/source-add-effect/v1"

// codeStateChanged is the stable error code of a command that refuses because the state it was told to expect
// is not the state it found. The command has changed nothing then.
const codeStateChanged = "state-changed"

// sourceEffect is what source add would do to the folder, and the fingerprint of it. source inspect shows it
// and source add --expect-effect compares it, so both are calculated here and nowhere else.
type sourceEffect struct {
	// missing says the folder does not exist yet.
	missing     bool
	perms       *permPlan
	fingerprint string
}

// permissionsChanged is the number of paths the permission plan changes.
func (e *sourceEffect) permissionsChanged() int {
	if e.perms == nil {
		return 0
	}
	return e.perms.changed()
}

// newSourceEffect works out the effect of adding the folder of res with the permission plan perms. It changes
// nothing. The address, the stacks, the conflicts and the engines are not part of it: containers come and go,
// and the address is entered later and changes only those.
func newSourceEffect(res control.SourceInspectResult, perms *permPlan) (*sourceEffect, error) {
	missing, err := folders.missing(res.Path)
	if err != nil {
		return nil, err
	}
	left := []string{}
	for _, e := range res.Excluded {
		left = append(left, e.Path+": "+e.Reason)
	}
	files := append([]string{}, res.PlannedFiles...)
	var included []string
	for _, e := range res.Included {
		included = append(included, e.Path+": "+e.Reason)
	}
	summary, changed := []string{}, 0
	if perms != nil {
		summary, changed = perms.summary(), perms.changed()
	}
	fp, err := approval.Fingerprint(sourceEffectLabel, struct {
		Case     string   `json:"case"`
		Missing  bool     `json:"missing"`
		Files    []string `json:"files"`
		Excluded []string `json:"excluded"`
		// Included is the choice of unclear files; the effect of a folder without one stays what it was.
		Included []string `json:"included,omitempty"`
		Changed  int      `json:"changed"`
		Summary  []string `json:"summary"`
	}{res.Case, missing, files, left, included, changed, summary})
	if err != nil {
		return nil, folderFailure("cannot fingerprint the effect of the folder", err)
	}
	return &sourceEffect{missing: missing, perms: perms, fingerprint: fp}, nil
}

// inspectEffect plans the permissions of the folder of res like source add and returns the effect. A rejected
// folder has none.
func inspectEffect(ctx context.Context, res control.SourceInspectResult) (*sourceEffect, error) {
	if res.Case == control.SourceCaseReject {
		return nil, nil
	}
	perms, err := planAccess(ctx, res)
	if err != nil {
		return nil, err
	}
	return newSourceEffect(res, perms)
}

// permissionsOutput is the plan of the permission changes as JSON and TOON show it.
type permissionsOutput struct {
	Changed int      `json:"changed"`
	Summary []string `json:"summary"`
}

// effectFields are the fields source inspect adds as root.
func effectFields(format output.Format, e *sourceEffect) []output.Field {
	fields := []output.Field{{Name: "missing", Value: e.missing}}
	if e.perms != nil && e.permissionsChanged() > 0 {
		if structuredFormat(format) {
			fields = append(fields, output.Field{Name: "permissions",
				Value: structured(format, permissionsOutput{Changed: e.perms.changed(), Summary: e.perms.summary()})})
		} else {
			fields = append(fields, output.Field{Name: "permissions_changed", Value: e.perms.changed()})
			for _, l := range e.perms.summary() {
				fields = append(fields, output.Field{Name: "permission", Value: strings.TrimSpace(l)})
			}
		}
	}
	return append(fields, output.Field{Name: "effect", Value: e.fingerprint})
}

// checkExpectedEffect refuses, before anything changes, when the effect of adding the folder is not the one
// the operator was shown. An empty expected effect asks for no check.
//
// The choice of unclear files is part of the effect, because it changes the files and what is left out: the
// effect of 'source inspect' with the same --include is the one that matches, and an effect worked out with
// another choice never passes. A folder of files that joins a remote is added with the plan of the comparison, which
// may hold more files, but the unclear files of both plans are the same ones.
//
// The effect is the one source inspect shows. A folder of files that joins a remote is added with the plan of the
// comparison, which may hold more files; those rest on the remote commit, which --expect-state and
// --expect-commit bind, so the effect is still checked against the inspection.
func checkExpectedEffect(ctx context.Context, expected string, sa *sourceAdd) error {
	if expected == "" {
		return nil
	}
	res, perms := sa.inspected, sa.perms
	if sa.remote != nil {
		var err error
		if perms, err = planAccess(ctx, res); err != nil {
			return err
		}
	}
	now, err := newSourceEffect(res, perms)
	if err != nil {
		return err
	}
	if now.fingerprint == expected {
		return nil
	}
	return &cli.Failure{Code: output.Code(codeStateChanged),
		Message:  "the folder changed since it was inspected; nothing was changed",
		NextStep: "Run 'qhost source inspect " + res.Path + "' again, review the result, then run the command again with the new effect."}
}

// expectedRemote is the state and commit of the comparison with a remote that the operator was shown.
type expectedRemote struct{ state, commit string }

// expectedRemoteOf reads --expect-state and --expect-commit. They belong together: the state is one of empty,
// equal and different, and the commit is empty exactly when the state is empty (the remote has no commit).
// Neither given means no check.
func expectedRemoteOf(state, commit string) (*expectedRemote, error) {
	if state == "" && commit == "" {
		return nil, nil
	}
	fix := "Pass the state and the commit of the comparison: --expect-state <empty|equal|different> --expect-commit <commit>."
	switch {
	case state == "":
		return nil, usageFailure("--expect-commit needs --expect-state", fix)
	case state != control.SourceRemoteEmpty && state != control.SourceRemoteEqual && state != control.SourceRemoteDifferent:
		return nil, usageFailure("--expect-state must be empty, equal or different", fix)
	case state == control.SourceRemoteEmpty && commit != "":
		return nil, usageFailure("--expect-commit must be empty when --expect-state is empty: the remote has no commit", fix)
	case state != control.SourceRemoteEmpty && commit == "":
		return nil, usageFailure("--expect-state "+state+" needs --expect-commit, the commit of the comparison", fix)
	}
	return &expectedRemote{state: state, commit: commit}, nil
}

// check refuses, before anything changes, when the fresh comparison differs from the one that was shown.
func (e *expectedRemote) check(cmp control.SourceCompareResult, path, url string) error {
	if e == nil || (cmp.State == e.state && cmp.Commit == e.commit) {
		return nil
	}
	return &cli.Failure{Code: output.Code(codeStateChanged),
		Message:  "the remote or the folder changed since the comparison was shown; nothing was changed",
		NextStep: "Run 'qhost source compare " + path + " " + url + "' again, review the result, then run the command again with the new state and commit."}
}

// remoteFlags adds --expect-state and --expect-commit to a command.
func remoteFlags(cmd *cobra.Command, state, commit *string) {
	cmd.Flags().StringVar(state, "expect-state", "",
		"state of the comparison that was shown: empty, equal or different; with --expect-commit, refuses when it differs now")
	cmd.Flags().StringVar(commit, "expect-commit", "",
		"commit of the remote branch in the comparison that was shown; empty only with --expect-state empty")
}
