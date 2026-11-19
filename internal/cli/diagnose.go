package cli

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/diagnose"
	"github.com/castrowithcee/qatlas-host/internal/userfile"
)

// diagnoseEnv is the identity the package file is made for; tests replace it.
var diagnoseEnv = func() userfile.Env { return userfile.Env{} }

// diagnoseNotice is said with the path of every package.
const diagnoseNotice = "The package holds the host name and paths. Check it before you pass it on."

func newDiagnoseCommand(program *cobracli.Program) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "diagnose [--out <path>]",
		Short: "Write a diagnostic package that makes a failure easy to investigate",
		Long: "diagnose collects what a person or an agent needs to investigate a failure into one archive (tar.gz) with a\n" +
			"text file and a JSON file for every part: the versions of qhost and its kit, the operating system, the state\n" +
			"of the service, the sources with their last call and error, the stacks with versions and update mode, the\n" +
			"result of 'qhost stack check', the last 20 runs with the logs of the 5 newest and of every failed one, the\n" +
			"service log of the last 24 hours and the list of the containers. It holds no container logs, no env files\n" +
			"and no compose files, and it asks the service only for what 'qhost status' and its siblings show, so secret\n" +
			"values are masked. A part that cannot be collected, also because the service is not running, is in the\n" +
			"package with its reason, and the others are still written.\n\n" +
			"Without --out the file goes to the folder qhost in your home, named after the host and the time. --out names\n" +
			"the file instead. The file is new, never replaces another and never follows a link, has mode 0600 and belongs\n" +
			"to you, also under sudo. The package holds the host name and paths: check it before you pass it on. It runs\n" +
			"without sudo for an administrator in the group qhost, and nothing is sent anywhere.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if out != "" {
				abs, err := filepath.Abs(out)
				if err != nil {
					return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true, Message: "--out is not a usable path",
						NextStep: "Name a file in an existing folder."}
				}
				out = abs
			}
			call := func(ctx context.Context, command string, args, result any) error {
				return callFor(ctx, client.CallTimeout, serviceEndpoint(), command, args, result)
			}
			pkg := diagnose.Build(c.Context(), diagnose.Options{Call: call, Version: version})
			path, err := pkg.Save(diagnoseEnv(), out)
			if err != nil {
				return &cli.Failure{Code: output.CodeRuntime, Message: "the package could not be written: " + err.Error(),
					NextStep: "Name another file with --out, or free the name and the folder.", Cause: err}
			}
			problems := []string{}
			for _, p := range pkg.Parts {
				if p.Err != "" {
					problems = append(problems, p.Name+": "+p.Err)
				}
			}
			fields := []output.Field{
				{Name: "path", Value: path}, {Name: "complete", Value: len(problems) == 0},
				{Name: "notice", Value: diagnoseNotice},
			}
			if len(problems) > 0 {
				var value any = strings.Join(problems, "\n")
				if structuredFormat(program.Format()) {
					value = problems
				}
				fields = append(fields, output.Field{Name: "problems", Value: value})
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write the package to this new file instead of the folder qhost in your home")
	return cmd
}
