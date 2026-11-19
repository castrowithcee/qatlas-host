package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/service"
	"github.com/castrowithcee/qatlas-host/internal/uninstall"
)

// uninstallEnv is everything the removal takes from the host. Production code never changes it; tests replace
// it so that no real path, systemctl or user database is involved.
type uninstallEnv struct {
	host uninstall.Host
	euid func() int
}

var uninstaller = uninstallEnv{euid: os.Geteuid}

// manpageNames returns the file names of the manpages that 'manpage' writes for the command tree below root:
// one page for each available command and the page of the program itself.
func manpageNames(root *cobra.Command) []string {
	// The completion command exists only once the command line runs; its pages are written like the others.
	root.InitDefaultCompletionCmd()
	var out []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.IsAvailableCommand() && !sub.IsAdditionalHelpTopicCommand() {
				walk(sub)
			}
		}
		out = append(out, strings.ReplaceAll(c.CommandPath(), " ", "-")+".1")
	}
	walk(root)
	return out
}

func newUninstallCommand(program *cobracli.Program) *cobra.Command {
	var purge bool
	var gate *cobracli.Gate
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove qhost and the deploy service from this host",
		Long: "uninstall stops and disables the host service qhost and, if it is set up, the deploy service qhost-deploy, and\n" +
			"deletes the program, both units, the drop-in of the deploy unit, /etc/tmpfiles.d/qhost-deploy.conf with its\n" +
			"channel directory and the manpages. The state of both services, the journal of the host changes, the system\n" +
			"users qhost and qhost-deploy and the group qhost stay.\n" +
			"With --purge it also deletes /var/lib/qhost, /var/lib/qhost-deploy (credentials and backups included),\n" +
			"/var/lib/qhost-host (the journal) and the users and groups; that cannot be undone.\n" +
			"The stack folders, their data and the containers are never touched, and containers keep running. If a\n" +
			"deletion would reach a bound stack folder, nothing is changed.\n" +
			"Uninstalling needs root. It first shows what it will do and the exact commands and asks; it needs\n" +
			"--confirm and asks for confirmation on a terminal, and --yes answers the question. A removal that\n" +
			"stopped at a step continues from there when run again.\n" +
			"Every deploy service instance for a rootless engine that is set up (see 'qhost help deploy-setup') is stopped and\n" +
			"disabled with systemctl disable --now, and its /etc/tmpfiles.d/qhost-deploy-<user>.conf, channel directory\n" +
			"/run/qhost-deploy-<user> and drop-in directory (only if empty) are deleted, and last the template unit\n" +
			"qhost-deploy@.service; with --purge also /var/lib/qhost-deploy-<user> and /etc/qhost-deploy-<user>. The users,\n" +
			"their homes, their rootless engines and containers always stay.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if uninstaller.euid() != 0 {
				return &cli.Failure{Code: output.CodePermission, Message: "uninstalling qhost needs root; run with sudo",
					NextStep: "Run the same command with sudo."}
			}
			plan, err := uninstaller.host.Plan(purge, manpageNames(c.Root()), service.BoundDirs(stateDir()))
			if err != nil {
				return uninstallFailure(err, false)
			}
			if plan.Empty() {
				return emitUninstall(c, program, purge, 0)
			}
			// Without --confirm the request fails before anything is shown or changed.
			if err := gate.Run(cobracli.Request{Changes: true}, func() error { return nil }); err != nil {
				return err
			}
			_, _ = fmt.Fprint(c.ErrOrStderr(), uninstallText(plan))
			return gate.Run(cobracli.Request{Question: "Remove qhost with these steps?", Changes: true}, func() error {
				if err := plan.Apply(c.Context()); err != nil {
					return uninstallFailure(err, true)
				}
				return emitUninstall(c, program, purge, len(plan.Steps))
			})
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also delete the state of both services, the journal of the host changes, the system users and the group qhost")
	gate = program.AddGate(cmd, cobracli.Gates{Question: true, Change: true})
	return cmd
}

func uninstallFailure(err error, started bool) *cli.Failure {
	next := "Fix the cause named in the message; nothing was changed. Then run the command again."
	if started {
		next = "Fix the cause named in the message, then run the command again; steps already done are not repeated."
	}
	return &cli.Failure{Code: output.CodeRuntime, Message: "uninstall: " + err.Error(), NextStep: next, Cause: err}
}

// uninstallText is the question: the sentence and, below it, the exact commands.
func uninstallText(p uninstall.Plan) string {
	var b strings.Builder
	b.WriteString(p.Sentence())
	b.WriteString("\nThese steps run as root:\n")
	for _, c := range p.Commands() {
		b.WriteString("  " + c + "\n")
	}
	return b.String()
}

func emitUninstall(c *cobra.Command, program *cobracli.Program, purge bool, steps int) error {
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
		{Name: "uninstalled", Value: true},
		{Name: "purged", Value: purge},
		{Name: "changed", Value: steps > 0},
		{Name: "steps", Value: steps},
	}})
}
