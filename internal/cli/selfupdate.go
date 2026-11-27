package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	// refresh runs the internal command of the newly installed program at exe that brings the deploy service
	// instances to its template; nil starts that program.
	refresh func(ctx context.Context, exe string, stderr io.Writer) error
	// refreshHook runs the internal command of the newly installed program that renews the unit of the webhook
	// receiver and restarts it; nil starts that program.
	refreshHook func(ctx context.Context, exe string, stderr io.Writer) error
	// migrateRun runs the transitions of the newly installed program; nil starts that program.
	migrateRun func(ctx context.Context, exe string, stderr io.Writer) error
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

// hookInstalled reports whether the setup of the webhook receiver installed its unit. An update never installs
// that unit by itself, and the archive does not carry it: the new program replaces it.
func (h selfUpdateHost) hookInstalled() bool { return deploysetup.HookInstalled(h.root) }

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

// planText says what an update replaces and restarts, from what is known without the release server: the target
// version is checked only when the update runs.
func (h selfUpdateHost) planText(exe, tag string) string {
	target := "the newest stable release"
	if tag != "" {
		target = "release " + tag
	}
	if exe == "" {
		exe = "the running program"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Update qhost from %s to %s. The release server is asked, and the version checked, only with --confirm.\n", version, target)
	b.WriteString("These files are replaced:\n  " + exe + "\n")
	for _, c := range h.companions() {
		if c.Path != "" {
			b.WriteString("  " + c.Path + "\n")
		}
	}
	b.WriteString("The newly installed program then runs the transitions of its release, and these services restart: " +
		strings.Join(h.services(), ", ") + "\n")
	if h.hookInstalled() {
		b.WriteString("The unit qhost-hook.service is renewed where it differs, and the webhook receiver qhost-hook restarts.\n")
	}
	if h.instances() {
		b.WriteString("The template unit qhost-deploy@.service is renewed where it differs, and every deploy service instance restarts.\n")
	}
	return b.String()
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

// instances reports whether a deploy service instance for a rootless engine is set up.
func (h selfUpdateHost) instances() bool {
	list, err := deploysetup.Instances(h.root)
	return err == nil && len(list) > 0
}

// refreshInstances lets the newly installed program renew the template unit and restart the instances: the
// running program knows only its own template, and sysservice takes no instance names. A failure is a warning,
// because the program and its services are installed and restarted by then.
func (h selfUpdateHost) refreshInstances(ctx context.Context, exe string, stderr io.Writer) {
	if !h.instances() {
		return
	}
	run := h.refresh
	if run == nil {
		run = func(ctx context.Context, exe string, stderr io.Writer) error {
			cmd := exec.CommandContext(ctx, exe, "deploy-service", refreshInstancesCommand)
			cmd.Stdout, cmd.Stderr = io.Discard, stderr
			return cmd.Run()
		}
	}
	if err := run(ctx, exe, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: the deploy service instances were not all renewed and restarted: %v\n"+
			"Run 'sudo qhost deploy-service %s' to retry.\n", err, refreshInstancesCommand)
	}
}

// refreshHookService lets the newly installed program renew the unit of the webhook receiver and restart it,
// where it is set up. A failure is a warning, like that of the instances.
func (h selfUpdateHost) refreshHookService(ctx context.Context, exe string, stderr io.Writer) {
	if !h.hookInstalled() {
		return
	}
	run := h.refreshHook
	if run == nil {
		run = func(ctx context.Context, exe string, stderr io.Writer) error {
			cmd := exec.CommandContext(ctx, exe, "hook-service", refreshHookCommand)
			cmd.Stdout, cmd.Stderr = io.Discard, stderr
			return cmd.Run()
		}
	}
	if err := run(ctx, exe, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: the webhook receiver was not renewed and restarted: %v\n"+
			"Run 'sudo qhost hook-service %s' to retry.\n", err, refreshHookCommand)
	}
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
			"release, and the unit of the deploy service if it is set up, and restarts the services; drop-ins of the unit stay as they are.\n" +
			"Before it changes anything it reads from the verified release which installed versions update to it directly: an\n" +
			"older installed version is refused, and the message names the release to update to first; a release that needs a\n" +
			"fresh setup instead of an update is refused with that step. After the files are replaced and before the services\n" +
			"restart, the newly installed program runs the transitions of its release for the setup and the stored state, each\n" +
			"once and reported on standard error; a failure there is a warning and does not undo the update, and\n" +
			"'sudo qhost update migrate' repeats that step. The deploy service also runs its own transitions in the start\n" +
			"step of its unit, before it opens its channel, so the restart finishes them even where an older program replaced\n" +
			"the files and did not run this step.\n" +
			"If deploy service instances for rootless engines are set up, the newly installed program then replaces the\n" +
			"template unit qhost-deploy@.service where it differs, reloads systemd and restarts every instance; a failure there is a\n" +
			"warning and does not undo the update, and 'sudo qhost deploy-service refresh-instances' repeats that step. Where the\n" +
			"webhook receiver is set up, the newly installed program likewise replaces qhost-hook.service where it differs and\n" +
			"restarts it, which is not in the release archive; a failure is a warning, and 'sudo qhost hook-service refresh' repeats it. A unit of\n" +
			"the same name in /etc/systemd/system overrides the installed one, and the update warns about it. Without an argument it\n" +
			"installs the newest stable release if that is newer than the running version; with a version such as\n" +
			"v1.2.3 it installs exactly that release, which must be newer than the running one. Updating never\n" +
			"installs an older release.\n" +
			"The signature over the release's checksums.txt is verified against the release key built into\n" +
			"qhost, then the archive's SHA-256 checksum; any failure leaves the installation unchanged.\n" +
			"Installing needs root. The service restarts afterwards. It first shows on standard error what it would\n" +
			"replace and restart, without asking the release server; without --confirm it stops there with exit\n" +
			"code 2 and changes nothing. With --confirm it asks for confirmation on a terminal, and --yes answers\n" +
			"the question. To update the image versions of stacks use 'qhost stack update'.",
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
	cmd.AddCommand(newMigrateCommand(program))
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the installed version, the latest stable release and whether an update is available",
		Long: "status asks the release server for the newest stable release and compares it with the running\n" +
			"version. It changes nothing and does not need root.",
		RunE: func(c *cobra.Command, _ []string) error {
			result, err := updateStatusResult(c.Context())
			if err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), result)
		},
	})
	return cmd
}

func updateStatusResult(ctx context.Context) (output.Object, error) {
	res, err := selfUpdate.client().Check(ctx)
	if err != nil {
		return output.Object{}, selfUpdateFailure(err)
	}
	return output.Object{Fields: []output.Field{
		{Name: "installed", Value: res.Current},
		{Name: "latest", Value: res.Latest},
		{Name: "update_available", Value: res.UpdateAvailable},
	}}, nil
}

func runSelfUpdate(c *cobra.Command, program *cobracli.Program, gate *cobracli.Gate, tag string) error {
	if selfUpdate.euid() != 0 {
		return &cli.Failure{Code: output.CodePermission, Message: "updating qhost needs root; run with sudo",
			NextStep: "Run the same command with sudo."}
	}
	// A request without --confirm shows the plan and fails before the release server is asked.
	exe, exeErr := selfUpdate.executable()
	if err := gate.Run(cobracli.Request{Changes: true}, func() error { return nil }); err != nil {
		_, _ = fmt.Fprint(c.ErrOrStderr(), selfUpdate.planText(exe, tag))
		return err
	}
	if exeErr != nil {
		return selfUpdateFailure(fmt.Errorf("cannot find the running program: %w", exeErr))
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
		return installSelfUpdate(c, program, updater, target, services)
	})
}

// installSelfUpdate downloads and verifies the release, checks its requirements of the installed version, replaces
// the files, lets the new program run its transitions and restarts the services. Nothing changes before the check.
func installSelfUpdate(c *cobra.Command, program *cobracli.Program, updater *selfupdate.Client, target selfupdate.Target,
	services []string) error {
	ctx, stderr, exe := c.Context(), c.ErrOrStderr(), updater.Executable
	prepared, err := updater.Prepare(ctx, target)
	if err != nil {
		return selfUpdateFailure(err)
	}
	if err := checkRequirements(prepared); err != nil {
		return err
	}
	result, err := updater.InstallPrepared(ctx, prepared)
	if err != nil {
		return selfUpdateFailure(err)
	}
	for _, service := range selfUpdate.shadowed(services) {
		fmt.Fprintf(stderr, "warning: a unit in /etc/systemd/system overrides the installed unit of %s\n", service)
	}
	selfUpdate.migrate(ctx, exe, stderr)
	if err := selfUpdate.restart(ctx, services, result); err != nil {
		return selfUpdateFailure(err)
	}
	selfUpdate.refreshInstances(ctx, exe, stderr)
	selfUpdate.refreshHookService(ctx, exe, stderr)
	return emitSelfUpdate(c, program, result.Current, result.Latest, true, true)
}

// checkRequirements refuses a release that the installed version may not update to directly.
func checkRequirements(prepared *selfupdate.Prepared) error {
	req, found, err := release.ReadRequirements(prepared.Archive())
	if err != nil {
		return selfUpdateFailure(fmt.Errorf("release %s: %w; the installation is unchanged", prepared.Version(), err))
	}
	if !found {
		return nil
	}
	if err := req.Check(version, prepared.Version()); err != nil {
		var refusal *release.Refusal
		if errors.As(err, &refusal) {
			return usageFailure(refusal.Message, refusal.NextStep)
		}
		return selfUpdateFailure(fmt.Errorf("%w; the installation is unchanged", err))
	}
	return nil
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
