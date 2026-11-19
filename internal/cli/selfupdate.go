package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/selfupdate"
	"github.com/castrowithcee/qatlas-kit/sysservice"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/release"
)

const (
	// releaseAPI is the GitHub API base of the public repository the releases come from.
	releaseAPI = "https://api.github.com/repos/castrowithcee/qatlas-host"
	// minimumVersion is the oldest release that can be installed.
	minimumVersion = "v0.1.0"
	// programName is the executable, the service and the release archive prefix.
	programName = "qhost"
	// manEntry is the main manpage inside the release archive; manPath is where the installer puts it.
	manEntry = "share/man/man1/qhost.1"
	manPath  = "/usr/local/share/man/man1/qhost.1"
)

// deployService is the unit and the system user of the deploy service, which exists only after the setup.
const deployService = deploysetup.Service

// selfUpdateHost is everything the self-update takes from the host. Production code never changes it;
// tests replace it so that no real service manager, release server or installed file is involved.
type selfUpdateHost struct {
	apiURL     string
	keys       func() []ssh.PublicKey
	euid       func() int
	executable func() (string, error)
	// root is prepended to every host path of the units and the manpage; empty in production.
	root string
	// systemctl runs the service manager; nil runs the real one.
	systemctl func(ctx context.Context, args ...string) error
}

var selfUpdate = selfUpdateHost{
	apiURL:     releaseAPI,
	keys:       release.TrustedKeys,
	euid:       os.Geteuid,
	executable: os.Executable,
}

func (h selfUpdateHost) options() []sysservice.Option {
	var opts []sysservice.Option
	if h.root != "" {
		opts = append(opts, sysservice.WithRoot(h.root))
	}
	if h.systemctl != nil {
		opts = append(opts, sysservice.WithRunner(h.systemctl))
	}
	return opts
}

// deployInstalled reports whether the setup of the deploy service installed its unit. An update never installs
// that unit by itself.
func (h selfUpdateHost) deployInstalled() bool { return deploysetup.Installed(h.root) }

func (h selfUpdateHost) companions() []selfupdate.Companion {
	opts := h.options()
	list := []selfupdate.Companion{sysservice.Companion(programName, opts...)}
	if h.deployInstalled() {
		list = append(list, sysservice.Companion(deployService, opts...))
	}
	man := manPath
	if h.root != "" {
		man = filepath.Join(h.root, filepath.FromSlash(manPath))
	}
	return append(list, selfupdate.Companion{Entry: manEntry, Path: man})
}

// services are the units the update restarts: the host service always, the deploy service when it is set up.
func (h selfUpdateHost) services() []string {
	if h.deployInstalled() {
		return []string{programName, deployService}
	}
	return []string{programName}
}

func (h selfUpdateHost) restart(ctx context.Context, services []string, result selfupdate.Result) error {
	for _, service := range services {
		if err := sysservice.Restart(ctx, service, result, h.options()...); err != nil {
			return err
		}
	}
	return nil
}

// shadowed returns the services whose unit an administrator's own unit in /etc/systemd/system overrides.
func (h selfUpdateHost) shadowed(services []string) []string {
	var out []string
	for _, service := range services {
		if ok, err := sysservice.Shadowed(service, h.options()...); err == nil && ok {
			out = append(out, service)
		}
	}
	return out
}

func (h selfUpdateHost) client() *selfupdate.Client {
	return &selfupdate.Client{
		RepositoryAPI:  strings.TrimRight(h.apiURL, "/"),
		CurrentVersion: version,
		MinimumVersion: minimumVersion,
		ArchiveName: func(v string) string {
			if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
				return ""
			}
			return fmt.Sprintf("%s_%s_linux_%s.tar.gz", programName, v, runtime.GOARCH)
		},
		UserAgent:      programName + "/" + version,
		Namespace:      release.Namespace,
		SignatureAsset: release.SignatureAsset,
		TrustedKeys:    h.keys(),
		ProgramName:    programName,
		Companions:     h.companions(),
	}
}

// newSelfUpdateCommand is the self-update of the program. Stack updates are 'qhost stack update'.
func newSelfUpdateCommand(program *cobracli.Program) *cobra.Command {
	var gate *cobracli.Gate
	cmd := &cobra.Command{
		Use:   "update [version]",
		Short: "Update qhost to the latest stable release or to a given newer one",
		Long: "update replaces the installed program, its systemd unit and its main manpage with those of a signed\n" +
			"release, and the unit of the deploy service if it is set up, and restarts the services; drop-ins of the unit stay as they are. Without an argument it\n" +
			"installs the newest stable release if that is newer than the running version; with a version such as\n" +
			"v1.2.3 it installs exactly that release, which must be newer than the running one. Updating never\n" +
			"installs an older release.\n" +
			"The signature over the release's checksums.txt is verified against the release key built into\n" +
			"qhost, then the archive's SHA-256 checksum; any failure leaves the installation unchanged.\n" +
			"Installing needs root. The service restarts afterwards, so installing needs --confirm and asks for\n" +
			"confirmation on a terminal; --yes answers the question. To update the image versions of stacks use\n" +
			"'qhost stack update'.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageFailure("update takes at most one argument, the release version",
					"Run 'qhost update [version]'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			tag := ""
			if len(args) == 1 {
				tag = args[0]
			}
			return runSelfUpdate(c, program, gate, tag)
		},
	}
	gate = program.AddGate(cmd, cobracli.Gates{Question: true, Change: true})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the installed version, the latest stable release and whether an update is available",
		Long: "status asks the release server for the newest stable release and compares it with the running\n" +
			"version. It changes nothing and does not need root.",
		RunE: func(c *cobra.Command, _ []string) error {
			res, err := selfUpdate.client().Check(c.Context())
			if err != nil {
				return selfUpdateFailure(err)
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "installed", Value: res.Current},
				{Name: "latest", Value: res.Latest},
				{Name: "update_available", Value: res.UpdateAvailable},
			}})
		},
	})
	return cmd
}

func runSelfUpdate(c *cobra.Command, program *cobracli.Program, gate *cobracli.Gate, tag string) error {
	if selfUpdate.euid() != 0 {
		return &cli.Failure{Code: output.CodePermission, Message: "updating qhost needs root; run with sudo",
			NextStep: "Run the same command with sudo."}
	}
	// A request without --confirm fails before the release server is asked.
	if err := gate.Run(cobracli.Request{Changes: true}, func() error { return nil }); err != nil {
		return err
	}
	exe, err := selfUpdate.executable()
	if err != nil {
		return selfUpdateFailure(fmt.Errorf("cannot find the running program: %w", err))
	}
	// The set of units is fixed before the archive is read, so what is replaced is what is restarted.
	services := selfUpdate.services()
	updater := selfUpdate.client()
	updater.Executable = exe
	if updater.ArchiveName("v0.0.0") == "" {
		return selfUpdateFailure(fmt.Errorf("no release is published for architecture %s", runtime.GOARCH))
	}
	ctx := c.Context()
	if tag == "" {
		check, err := updater.Check(ctx)
		if err != nil {
			return selfUpdateFailure(err)
		}
		if !check.UpdateAvailable {
			return emitSelfUpdate(c, program, version, version, false, false)
		}
		tag = check.Latest
	}
	target, err := updater.Target(ctx, tag)
	if err != nil {
		var te *selfupdate.TargetError
		if errors.As(err, &te) {
			return usageFailure(err.Error(), "Run 'qhost update status' to see the latest stable release.")
		}
		return selfUpdateFailure(err)
	}
	if target.Current {
		return emitSelfUpdate(c, program, version, version, false, false)
	}
	if target.Older {
		return usageFailure("release "+target.Version+" is older than the running version "+version+"; update never installs an older release",
			"Run 'qhost update status' to see the latest stable release.")
	}
	question := fmt.Sprintf("Update qhost from %s to %s. The service restarts.", version, target.Version)
	return gate.Run(cobracli.Request{Question: question, Changes: true}, func() error {
		prepared, err := updater.Prepare(ctx, target)
		if err != nil {
			return selfUpdateFailure(err)
		}
		result, err := updater.InstallPrepared(ctx, prepared)
		if err != nil {
			return selfUpdateFailure(err)
		}
		for _, service := range selfUpdate.shadowed(services) {
			fmt.Fprintf(c.ErrOrStderr(), "warning: a unit in /etc/systemd/system overrides the installed unit of %s\n", service)
		}
		if err := selfUpdate.restart(ctx, services, result); err != nil {
			return selfUpdateFailure(err)
		}
		return emitSelfUpdate(c, program, result.Current, result.Latest, true, true)
	})
}

// selfUpdateFailure is a failed update or release lookup; the message says whether the installation changed.
func selfUpdateFailure(err error) error {
	message := "update: " + err.Error()
	if errors.Is(err, selfupdate.ErrDevelopmentBuild) {
		message = "update: self-update is unavailable in a development build; install a tagged release first"
	}
	return &cli.Failure{Code: output.CodeRuntime, Message: message, Cause: err,
		NextStep: "Read the message: the installation is unchanged unless it says otherwise. Fix the cause, then run the command again."}
}

func emitSelfUpdate(c *cobra.Command, program *cobracli.Program, previous, current string, updated, restarted bool) error {
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
		{Name: "previous", Value: previous},
		{Name: "version", Value: current},
		{Name: "updated", Value: updated},
		{Name: "restarted", Value: restarted},
	}})
}
