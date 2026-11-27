package cli

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
)

// migrateCommand is the internal subcommand of 'qhost update' that the update runs in the newly installed program.
const migrateCommand = "migrate"

// transition is one step of a release for hosts that ran an earlier one. done reports whether the host already
// has it, apply makes it; root is prepended to every host path and empty in production. A step changes only what
// belongs to it and follows no links. applyAtStart, if set, makes the step from the start step of a service unit,
// where apply would wait for the start it runs in.
type transition struct {
	name         string
	done         func(ctx context.Context, root string) (bool, error)
	apply        func(ctx context.Context, root string) error
	applyAtStart func(ctx context.Context, root string) error
}

// runTransitions runs every step the host does not have yet, in order, reports each on report and stops at the
// first failure. It returns the names of the steps it applied. A step reports its own lines on reportOf(ctx).
func runTransitions(ctx context.Context, root string, list []transition, report io.Writer) ([]string, error) {
	ctx = context.WithValue(ctx, reportKey{}, report)
	var applied []string
	for _, t := range list {
		done, err := t.done(ctx, root)
		if err != nil {
			return applied, fmt.Errorf("transition %s: %w", t.name, err)
		}
		if done {
			_, _ = fmt.Fprintf(report, "transition %s: already done\n", t.name)
			continue
		}
		if err := t.apply(ctx, root); err != nil {
			return applied, fmt.Errorf("transition %s: %w", t.name, err)
		}
		applied = append(applied, t.name)
		_, _ = fmt.Fprintf(report, "transition %s: applied\n", t.name)
	}
	return applied, nil
}

type reportKey struct{}

// reportOf is the report of the runTransitions that runs the step; nothing outside it.
func reportOf(ctx context.Context) io.Writer {
	if w, ok := ctx.Value(reportKey{}).(io.Writer); ok {
		return w
	}
	return io.Discard
}

// atStart returns the steps as the start step of a service unit runs them.
func atStart(list []transition) []transition {
	out := make([]transition, len(list))
	for i, t := range list {
		if t.applyAtStart != nil {
			t.apply = t.applyAtStart
		}
		out[i] = t
	}
	return out
}

func newMigrateCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:    migrateCommand,
		Short:  "Run the transitions of this release for a host that ran an earlier one",
		Hidden: true,
		Long: "migrate is run by 'qhost update' in the newly installed program after the files are replaced and before\n" +
			"the services restart, because only that program knows the transitions of its release. It runs every\n" +
			"transition the host does not have yet, in order, reports each on standard error and stops at the first\n" +
			"failure; a repeated run changes nothing more. A release without transitions changes nothing. It then renews\n" +
			"the installed manpages that differ from those of the program, all pages, not only the main one that an\n" +
			"older self-update replaces; a failure there is a warning. Needs root.\n" +
			"The deploy service runs its own transitions in the start step of its unit, so a transition that failed there\n" +
			"is continued by a new start of the service or by this command. Securing the .git of stack folders that an\n" +
			"earlier release shared with its service account runs only in that start step, after the service of the earlier\n" +
			"release has stopped (see 'qhost deploy-service prepare'). Updates from v0.7.0 to v0.8.0 call this command.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if selfUpdate.euid() != 0 {
				return &cli.Failure{Code: output.CodePermission, Message: "running the transitions needs root; run with sudo",
					NextStep: "Run the same command with sudo."}
			}
			applied, err := runTransitions(c.Context(), selfUpdate.root, transitions, c.ErrOrStderr())
			if err != nil {
				return &cli.Failure{Code: output.CodeRuntime, Message: "update migrate: " + err.Error(), Cause: err,
					NextStep: "Fix the cause named in the message, then run 'sudo qhost update " + migrateCommand + "' again."}
			}
			reportManpages(program, selfUpdate.root, c.ErrOrStderr())
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "transitions", Value: len(transitions)},
				{Name: "applied", Value: strings.Join(applied, ", ")},
			}})
		},
	}
}

// migrate lets the newly installed program run the transitions of its release. A failure is a warning like that
// of the instances: the program is installed by then, and the command repeats the step.
func (h selfUpdateHost) migrate(ctx context.Context, exe string, stderr io.Writer) {
	run := h.migrateRun
	if run == nil {
		run = func(ctx context.Context, exe string, stderr io.Writer) error {
			cmd := exec.CommandContext(ctx, exe, "update", migrateCommand)
			cmd.Stdout, cmd.Stderr = io.Discard, stderr
			return cmd.Run()
		}
	}
	if err := run(ctx, exe, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: the transitions of the new release did not all run: %v\n"+
			"Run 'sudo qhost update %s' to retry.\n", err, migrateCommand)
	}
}
