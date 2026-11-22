package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/localipc"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/rootless"
	"github.com/castrowithcee/qatlas-host/internal/service"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

const (
	// stackFoldersDropIn is the systemd drop-in that lists the stack folders the sandboxed service may write.
	stackFoldersDropIn = "/etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf"
	// commitAuthor is the git identity of the commits the service makes; it is not the service account.
	commitAuthor = "qhost"
	// serviceAccount owns the stack folders the client creates; it is also the name of the deploy unit.
	serviceAccount = "qhost-deploy"
	// folderMode is the mode of a created stack folder.
	folderMode = 0o755
	// serviceWait bounds the wait for the control socket after a restart.
	serviceWait = 30 * time.Second
)

// folderHost is everything the stack folder setup takes from the host. Production code never changes it;
// tests replace it so that no real /etc, systemctl or service account is involved.
type folderHost struct {
	euid func() int
	// root is prepended to the drop-in path.
	root string
	// systemctl runs the service manager with the given arguments.
	systemctl func(ctx context.Context, args ...string) error
	// owner returns the ids of the account that runs a deploy service.
	owner func(name string) (uid, gid int, err error)
	// instances lists the deploy service instances that are set up.
	instances func() ([]deploysetup.Instance, error)
	// waitService returns once the control socket of the service answers.
	waitService func(ctx context.Context) error
	// readOnly reports whether the file system of a path lies read-only.
	readOnly func(path string) (bool, error)
	// sameFS reports whether two existing paths lie on the same file system, as a rename between them needs.
	sameFS func(a, b string) (bool, error)
	// lchown and chmod change the group and the mode of the paths source add hands to the service.
	lchown func(path string, uid, gid int) error
	chmod  func(path string, mode fs.FileMode) error
	// detector finds and asks the rootless engines; its zero value is the real host.
	detector rootless.Detector
}

var folders = folderHost{
	euid: os.Geteuid,
	systemctl: func(ctx context.Context, args ...string) error {
		out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	},
	owner: lookupOwner,
	instances: func() ([]deploysetup.Instance, error) {
		return deploysetup.Instances(deploySetup.host.Root)
	},
	waitService: waitForService,
	readOnly:    pathOnReadOnlyFS,
	sameFS:      pathsOnSameFS,
	lchown:      os.Lchown,
	chmod:       os.Chmod,
}

// waitForService polls the control socket until the service answers; any answer counts, also a refusal.
func waitForService(ctx context.Context) error { return waitForEndpoint(ctx, targetOf(ctx).endpoint()) }

// waitForEndpoint is waitForService for the service at ep.
func waitForEndpoint(ctx context.Context, ep localipc.ServiceEndpoint) error {
	ctx, cancel := context.WithTimeout(ctx, serviceWait)
	defer cancel()
	for {
		var res control.StatusResult
		err := client.Do(ctx, ep, 2*time.Second, control.StatusCommand, nil, &res)
		var remote *control.Error
		if err == nil || errors.As(err, &remote) || errors.Is(err, client.ErrNoAccess) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("the service did not answer after the restart")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (h folderHost) dropIn(t engineTarget) string { return t.dropIn(h.root) }

func folderEntry(dir string) string { return "ReadWritePaths=-" + dir }

func hasEntry(content, dir string) bool {
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimSpace(l) == folderEntry(dir) {
			return true
		}
	}
	return false
}

// addEntry appends the folder to the drop-in text and keeps everything already in it, so a folder is never
// dropped from the set.
func addEntry(content, dir string) string {
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if !strings.Contains(content, "[Service]") {
		content += "[Service]\n"
	}
	return content + folderEntry(dir) + "\n"
}

func (h folderHost) requireRoot() error {
	if h.euid() != 0 {
		return &cli.Failure{Code: output.CodePermission, Message: "setting up a stack folder needs root; run with sudo",
			NextStep: "Run the same command with sudo."}
	}
	return nil
}

// missing reports whether dir does not exist; something that is not a directory is an error.
func (h folderHost) missing(dir string) (bool, error) {
	if err := service.ValidateStackDir(dir); err != nil {
		return false, usageFailure("the stack folder "+dir+": "+err.Error(), "Pass an absolute path without spaces.")
	}
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case err != nil:
		return false, folderFailure("cannot inspect the stack folder "+dir, err)
	case !info.IsDir():
		return false, usageFailure("the stack folder "+dir+" is not a directory (a symbolic link is not followed)",
			"Pass the path of a directory.")
	}
	return false, nil
}

func folderFailure(message string, cause error) error {
	return &cli.Failure{Code: output.CodeRuntime, Message: message, Cause: cause,
		NextStep: "Fix the cause and run the command again."}
}

// grant makes the stack folder usable by the service: it creates a missing folder when create is set, adds the
// folder to the drop-in and, when anything changed, reloads systemd, restarts the service and waits for its
// control socket. The caller has confirmed the path before it passes create.
func (h folderHost) grant(ctx context.Context, dir string, create bool, notice io.Writer) error {
	if err := h.requireRoot(); err != nil {
		return err
	}
	absent, err := h.missing(dir)
	if err != nil {
		return err
	}
	t := targetOf(ctx)
	created := false
	if absent {
		if !create {
			return &cli.Failure{Code: output.CodeConfirmationRequired, Usage: true,
				Message:  "the stack folder " + dir + " does not exist and was not created",
				NextStep: "Confirm creating it, or create it yourself and run the command again."}
		}
		if err := h.create(t, dir); err != nil {
			return err
		}
		created = true
		_, _ = fmt.Fprintf(notice, "Created the stack folder %s (owner %s:%s, mode %04o).\n", dir, t.account(), t.account(), folderMode)
	}
	added, err := h.addToDropIn(t, dir)
	if err != nil {
		return err
	}
	if !added && !created {
		return nil
	}
	if added {
		_, _ = fmt.Fprintf(notice, "Granted the service write access to %s in %s.\n", dir, h.dropIn(t))
	}
	return h.restart(ctx, notice)
}

// create makes the folder and its missing parents; only the folder itself belongs to the service account.
func (h folderHost) create(t engineTarget, dir string) error {
	uid, gid, err := h.owner(t.account())
	if err != nil {
		return folderFailure("cannot find the service account "+t.account(), err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), folderMode); err != nil {
		return folderFailure("cannot create the parent of the stack folder "+dir, err)
	}
	if err := os.Mkdir(dir, folderMode); err != nil {
		return folderFailure("cannot create the stack folder "+dir, err)
	}
	// The umask may have narrowed the mode.
	if err := os.Chmod(dir, folderMode); err != nil {
		return folderFailure("cannot set the mode of the stack folder "+dir, err)
	}
	if err := os.Chown(dir, uid, gid); err != nil {
		return folderFailure("cannot give the stack folder "+dir+" to "+t.account(), err)
	}
	return nil
}

// addToDropIn lists dir in the drop-in; added is false when it was listed already.
func (h folderHost) addToDropIn(t engineTarget, dir string) (added bool, err error) {
	path := h.dropIn(t)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, folderFailure("cannot read "+path, err)
	}
	content := string(data)
	if hasEntry(content, dir) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, folderFailure("cannot create "+filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".10-stack-folders-*")
	if err != nil {
		return false, folderFailure("cannot write "+path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(addEntry(content, dir)); err != nil {
		_ = tmp.Close()
		return false, folderFailure("cannot write "+path, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return false, folderFailure("cannot write "+path, err)
	}
	if err := tmp.Close(); err != nil {
		return false, folderFailure("cannot write "+path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return false, folderFailure("cannot write "+path, err)
	}
	return true, nil
}

func (h folderHost) restart(ctx context.Context, notice io.Writer) error {
	unit := targetOf(ctx).unit()
	_, _ = fmt.Fprintf(notice, "Restarting the %s service.\n", unit)
	for _, args := range [][]string{{"daemon-reload"}, {"restart", unit}} {
		if err := h.systemctl(ctx, args...); err != nil {
			return &cli.Failure{Code: output.CodeRuntime, Message: "the service manager refused: " + err.Error(), Cause: err,
				NextStep: "Run 'systemctl daemon-reload' and 'systemctl restart " + unit + "', then run the command again."}
		}
	}
	if err := h.waitService(ctx); err != nil {
		return &cli.Failure{Code: output.CodeUnreachable, Message: err.Error(), Cause: err,
			NextStep: "Check 'systemctl status " + unit + "', then run the command again."}
	}
	return nil
}

// repairBound grants the already bound stack folders when the drop-in lacks them, as after an installation
// that predates the drop-in. Without root, a binding or a missing entry it does nothing.
func (h folderHost) repairBound(ctx context.Context, notice io.Writer) error {
	if h.euid() != 0 {
		return nil
	}
	granted := false
	for _, dir := range service.BoundDirs(stateDir()) {
		data, err := os.ReadFile(h.dropIn(engineTarget{}))
		if err == nil && hasEntry(string(data), dir) {
			continue
		}
		_, _ = fmt.Fprintf(notice, "Notice: the bound stack folder %s has no write grant for the service yet.\n", dir)
		added, err := h.addToDropIn(engineTarget{}, dir)
		if err != nil {
			return err
		}
		if added {
			_, _ = fmt.Fprintf(notice, "Granted the service write access to %s in %s.\n", dir, h.dropIn(engineTarget{}))
			granted = true
		}
	}
	if !granted {
		return nil
	}
	return h.tryRestart(ctx, notice)
}

// tryRestart restarts the service only if it runs, so a service the operator stopped stays stopped; the grant
// then takes effect at its next start.
func (h folderHost) tryRestart(ctx context.Context, notice io.Writer) error {
	for _, args := range [][]string{{"daemon-reload"}, {"try-restart", serviceAccount}} {
		if err := h.systemctl(ctx, args...); err != nil {
			return &cli.Failure{Code: output.CodeRuntime, Message: "the service manager refused: " + err.Error(), Cause: err,
				NextStep: "Run 'systemctl daemon-reload' and 'systemctl try-restart qhost-deploy'."}
		}
	}
	if h.systemctl(ctx, "is-active", "--quiet", serviceAccount) != nil {
		_, _ = fmt.Fprintln(notice, "The qhost-deploy service is not running; the grant takes effect at its next start.")
		return nil
	}
	_, _ = fmt.Fprintln(notice, "Restarted the qhost-deploy service.")
	if err := h.waitService(ctx); err != nil {
		return &cli.Failure{Code: output.CodeUnreachable, Message: err.Error(), Cause: err,
			NextStep: "Check 'systemctl status qhost-deploy'."}
	}
	return nil
}

// repairBoundFolder is the hook every root client command runs first. A failure only warns: the command the
// operator asked for still runs.
func repairBoundFolder(c *cobra.Command) {
	switch c.Name() {
	case "run", "version", "manpage", "help", "completion", "uninstall":
		return
	}
	if err := folders.repairBound(c.Context(), c.ErrOrStderr()); err != nil {
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %v\n", err)
	}
}

// prepareFolder is the step of 'repo set' that precedes the clone. A missing folder is created only after the
// operator confirmed the path: by --create, or by answering at a terminal.
func prepareFolder(c *cobra.Command, dir string, create bool) error {
	if err := folders.requireRoot(); err != nil {
		return err
	}
	absent, err := folders.missing(dir)
	if err != nil {
		return err
	}
	if absent && !create {
		if err := confirmFolderCreate(c, targetOf(c.Context()), dir); err != nil {
			return err
		}
		create = true
	}
	return folders.grant(c.Context(), dir, create, c.ErrOrStderr())
}

// confirmFolderCreate shows the exact path and asks at a terminal; without a terminal it refuses and names the flag.
func confirmFolderCreate(c *cobra.Command, t engineTarget, dir string) error {
	in := stdinStream()
	f, ok := in.(*os.File)
	if !ok || !isTerminal(f) {
		return &cli.Failure{Code: output.CodeConfirmationRequired, Usage: true,
			Message:  fmt.Sprintf("the stack folder %s does not exist and was not created", dir),
			NextStep: "Run the command again with --create to create exactly this folder."}
	}
	w := c.ErrOrStderr()
	_, _ = fmt.Fprintf(w, "The stack folder %s does not exist.\n", dir)
	_, _ = fmt.Fprintf(w, "Create it with owner %s:%s and mode %04o and let the service write to it? [y/N] ", t.account(), t.account(), folderMode)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return &cli.Failure{Code: output.CodeRuntime, Message: "cannot read the answer from the terminal",
			NextStep: "Run the command again.", Cause: err}
	}
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		return &cli.Failure{Code: output.CodeConfirmationDeclined, Message: "the stack folder was not confirmed; nothing was created",
			NextStep: "Run the command again with the path you want, or with --create."}
	}
	return nil
}

// tuiFolders gives the terminal interface the same steps as the command.
type tuiFolders struct{}

func (tuiFolders) Missing(dir string) (bool, error) {
	if err := folders.requireRoot(); err != nil {
		return false, err
	}
	return folders.missing(dir)
}

// engineCtx makes the engine the interface named for the call, with tui.WithEngine, the target of the steps on the
// host: the service account, the drop-in and the unit are those of that engine's deploy service.
func (tuiFolders) engineCtx(ctx context.Context) (context.Context, error) {
	engine := tui.EngineOf(ctx)
	if engine == "" {
		return ctx, nil
	}
	t, err := folders.namedEngine(engine)
	if err != nil {
		return ctx, err
	}
	return withEngine(ctx, t), nil
}

func (h tuiFolders) Grant(ctx context.Context, dir string, create bool) error {
	ctx, err := h.engineCtx(ctx)
	if err != nil {
		return err
	}
	return folders.grant(ctx, dir, create, io.Discard)
}

func (h tuiFolders) PlanPermissions(ctx context.Context, res control.SourceInspectResult) (tui.PermPlan, error) {
	if err := folders.requireRoot(); err != nil {
		return tui.PermPlan{}, err
	}
	ctx, err := h.engineCtx(ctx)
	if err != nil {
		return tui.PermPlan{}, err
	}
	plan, err := planAccess(ctx, res)
	if err != nil || plan == nil {
		return tui.PermPlan{}, err
	}
	return tui.NewPermPlan(plan.changed(), plan.summary(), plan), nil
}

func (h tuiFolders) ApplyPermissions(ctx context.Context, p tui.PermPlan) error {
	plan, ok := p.Handle().(*permPlan)
	if !ok {
		return nil
	}
	if err := folders.requireRoot(); err != nil {
		return err
	}
	ctx, err := h.engineCtx(ctx)
	if err != nil {
		return err
	}
	return folders.applyPermissions(ctx, plan)
}

func (h tuiFolders) Engines(ctx context.Context, res control.SourceInspectResult) control.SourceInspectResult {
	// An engine that cannot be resolved failed the call of the service before; the scan then stays as for the default.
	if named, err := h.engineCtx(ctx); err == nil {
		ctx = named
	}
	return addEngines(ctx, res)
}

func (tuiFolders) ReadOnly(path string) (bool, error) { return folders.readOnly(path) }
