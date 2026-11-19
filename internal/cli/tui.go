package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/uiprefs"

	"github.com/castrowithcee/qatlas-host/internal/admins"
	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

// runTUI shows the interface; tests replace it.
var runTUI = tui.Run

// terminalStreams returns standard input and stdout when both are terminals. A buffer or a pipe is no terminal.
func terminalStreams(stdout io.Writer) (in, out *os.File, ok bool) {
	in, inOK := stdinStream().(*os.File)
	out, outOK := stdout.(*os.File)
	if !inOK || !outOK || !isTerminal(in) || !isTerminal(out) {
		return nil, nil, false
	}
	return in, out, true
}

// prefsPath is the device-local preferences file of the interface, ~/.qatlas/qhost/tui.yaml.
func prefsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return uiprefs.Path(filepath.Join(home, ".qatlas", "qhost")), nil
}

// savePrefs writes the preferences file at path; the Settings area saves through it.
func savePrefs(path string, schema uiprefs.Schema) func(uiprefs.Prefs) error {
	return func(p uiprefs.Prefs) error { return uiprefs.Save(path, p, schema) }
}

func newTUICommand(state *notRunning) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the terminal interface",
		Long: "tui opens the terminal interface. It has seven areas, switched with 1 to 7, or with the left and right keys\n" +
			"on the tab line, where the interface starts. Down or enter lead into the content; up in the first row or\n" +
			"at the top of a text, and esc in a list, lead back to the tab line. Tab moves the focus between the two\n" +
			"as well:\n" +
			"Stacks lists every stack with its current and available version, update mode, state, last run and\n" +
			"findings, under a row for each stack folder with its kind and last poll; Stack detail shows the chosen\n" +
			"stack with its image versions, changelog, backups and last runs; Runs lists the runs, and enter opens\n" +
			"the log of a run, which follows a run in progress; Settings holds what applies to the whole host or\n" +
			"this device, on four sub-tabs below the tab line: General (the preferences of this device),\n" +
			"Credentials, Maintenance window and Notifications; Containers lists every container of the\n" +
			"host; Proposals lists the proposals of agents; System lists the users and groups of the host.\n" +
			"? opens the help, r reads the service again, q quits.\n\n" +
			"The sub-tabs of Settings are a second line under the tab line. From the tab line, down leads to it, and\n" +
			"there left and right choose the sub-tab with wrap-around and show its content at once; down, enter or\n" +
			"tab lead into the content, and up in its first row or field, at the top of a text, or esc lead back to\n" +
			"the sub-tab line, and up or esc again to the tab line. Leaving the General form with unsaved changes,\n" +
			"or choosing another sub-tab or area, first asks whether to discard them. Click a sub-tab to choose it.\n\n" +
			"In Stacks the keys act on the stack under the cursor, or on the stacks ticked with space, one stack\n" +
			"after another: u update, d deploy, p pause, P unpause, i pin, I unpin, g resume (only for a stack paused\n" +
			"after a failed rollback) and c asking the registries now about all stacks. Every action first asks:\n" +
			"y runs it, n, esc and enter cancel. The update question lists the version change of each variable and\n" +
			"the changelog, and a newer major version is flagged as not installed automatically. A failed stack does\n" +
			"not stop the others; the result lists every stack, the stacks that failed stay ticked, and l opens the\n" +
			"log of the run an action started, or the Runs list when it started several. One action runs at a time.\n\n" +
			"The stack folders are managed in Stacks too: a adds a folder (with the steps of qhost source add; an\n" +
			"empty host starts there), and on a folder row F fetches it, x releases it after a question and e shows,\n" +
			"generates or stores its deploy key; a folder with no remote has no key to offer. The private key is\n" +
			"never shown. On the row of a local folder O adds a remote: it asks for the SSH address, the host key and\n" +
			"the deploy key (without one, the key view opens), then compares the folder with the repository. A\n" +
			"repository without a commit receives the history, an equal one is taken over, and for a different one\n" +
			"the paths are listed and the state of the host is put on it as one commit after your answer. Secret-like\n" +
			"files in what would be pushed stop it, with no way around. Adding a folder of files offers the same\n" +
			"comparison with an address.\n\n" +
			"System lists the local users and groups of the host, read from its account files, so it works without the\n" +
			"deploy service; Users, Groups and Folders are its sub-tabs. On Users, a creates a user with an initial\n" +
			"password that must be changed at the first login, e or enter offers the changes of the marked user\n" +
			"(administrator, Docker, groups, password, SSH keys, lock and unlock) and x deletes it, keeping its home\n" +
			"unless deleting it is chosen. On Groups, a creates a group, e or enter adds and removes members of the\n" +
			"marked group, several at once, n renames it and x deletes it. On Folders, enter chooses a folder and e\n" +
			"changes its owner, group and what the owner, the group and everyone else may (nothing, read, or read and\n" +
			"write), optionally for everything below it, which names the number of entries first. Every change shows a\n" +
			"sentence and the exact commands; y hands the terminal to sudo qhost host apply, which asks for the password\n" +
			"itself, and started with sudo the interface runs it directly. A password or key is never shown and never\n" +
			"goes into a command line. Changes of root, of system accounts, of system groups and of system paths and\n" +
			"every rename of a group ask in red (see qhost help host-tui).\n\n" +
			"The mouse works too: click a tab, a key in the header or footer, or a row, and turn the wheel to scroll.\n" +
			"A click does what the key does, so every action still asks first. The mouse is on by default; with it\n" +
			"on, hold Shift to select text. Settings, General switches it off.\n\n" +
			"A small icon stands before the product name and before each area on the tab line. Settings, General\n" +
			"chooses them under Icons: unicode (the default), nerdfont (needs a Nerd Font patched terminal font) or\n" +
			"none. A terminal without Unicode shows no icons.\n\n" +
			"It needs a terminal on both standard input and output and the running deploy service. Root and administrators in\n" +
			"the group qhost may open it; without sudo it only reads, and a change says that it needs sudo qhost. A deploy\n" +
			"service that is not set up is no failure: the areas that need it say so and, with y after a question, set it up\n" +
			"(D; as root, otherwise it says that it needs sudo qhost). A set up deploy service that is not running ends with exit code 3 before it draws\n" +
			"anything. Device-local preferences are read from ~/.qatlas/qhost/tui.yaml; a missing file means\n" +
			"the defaults. Change them in Settings under General (theme, start area, language, icons and mouse)\n" +
			"instead of editing the file by hand.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			in, out, ok := terminalStreams(c.OutOrStdout())
			if !ok {
				return usageFailure("the terminal interface needs a terminal on standard input and standard output",
					"Run 'qhost tui' in a terminal, or use the commands such as 'qhost status' in scripts.")
			}
			backend := tui.NewBackend(serviceEndpoint()).WithVersion(version)
			// The interface refuses before it takes the terminal, so a missing service or root fails as every
			// channel command does. A deploy service that is not set up is no failure: the interface offers the setup.
			if _, err := backend.Status(c.Context()); err != nil && !(errors.Is(err, client.ErrNotRunning) && !deployInstalled()) {
				if handled, failed := regroup(err, c.ErrOrStderr()); handled {
					return failed
				}
				if errors.Is(err, client.ErrNotRunning) {
					state.seen = true
				}
				return failureOf(err)
			}
			opts := tui.Options{Backend: backend, Folders: tuiFolders{}, Deploy: tuiDeploySetup{}, In: in, Out: out, HostRun: hostRunner}
			schema, err := tui.PrefsSchema()
			if err != nil {
				return &cli.Failure{Code: output.CodeRuntime, Message: "the terminal interface cannot start: " + err.Error(),
					NextStep: "Report this as a bug of qhost.", Cause: err}
			}
			opts.Prefs = uiprefs.Default(schema)
			if path, err := prefsPath(); err != nil {
				opts.PrefsWarnings = []string{fmt.Sprintf("no home directory, using the defaults: %v", err)}
			} else {
				opts.Prefs, opts.PrefsWarnings = uiprefs.Load(path, schema)
				opts.SavePrefs = savePrefs(path, schema)
			}
			if err := runTUI(opts); err != nil {
				return &cli.Failure{Code: output.CodeRuntime, Message: "the terminal interface failed: " + err.Error(),
					NextStep: "Run 'qhost tui' again; if it keeps failing, use the commands such as 'qhost status'.", Cause: err}
			}
			return nil
		},
	}
}

// The calls hostRunner and regroup make to the host; variables so a test can stand in for them.
var (
	lookPath          = exec.LookPath
	thisProgram       = os.Executable
	execv             = unix.Exec
	tuiEuid           = os.Geteuid
	sessionLacksGroup = lacksServiceGroup
	regroupArgs       = func() []string { return os.Args[1:] }
)

// hostRunner builds `sudo <this program> host ...` for the System area of the interface. sudo prompts on the
// terminal, so the password never reaches this program. Standard input belongs to that prompt, so a request with
// a secret runs in two steps: the interface first runs the Validate request (sudo -v) on the terminal, then this
// one with sudo -n, which never prompts, and the secret on the standard input of the command. The secret goes into
// no argument and no environment variable; the C locale is for the interface to recognise the answer of sudo when
// it holds no password. An interface that runs as root runs the command itself and needs no password check.
func hostRunner(req tui.HostRequest) (*exec.Cmd, error) {
	exe, err := thisProgram()
	if err != nil {
		return nil, fmt.Errorf("cannot find this program: %w", err)
	}
	if tuiEuid() == 0 {
		if req.Validate {
			return nil, nil
		}
		cmd := exec.Command(exe, append([]string{"host"}, req.Args...)...)
		if len(req.Secret) > 0 {
			cmd.Stdin = bytes.NewReader(req.Secret)
		}
		return cmd, nil
	}
	sudo, err := lookPath("sudo")
	if err != nil {
		return nil, tui.ErrNoSudo
	}
	if req.Validate {
		return exec.Command(sudo, "-v"), nil
	}
	if len(req.Secret) == 0 {
		return exec.Command(sudo, append([]string{exe, "host"}, req.Args...)...), nil
	}
	cmd := exec.Command(sudo, append([]string{"-n", exe, "host"}, req.Args...)...)
	cmd.Stdin = bytes.NewReader(req.Secret)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd, nil
}

// regroup starts the interface again under sudo -g qhost when the channel refused this user only because the
// login session predates the membership in the group qhost, as after an administrator gave the access. It returns
// only when that does not apply (not handled) or the new start failed.
func regroup(err error, stderr io.Writer) (handled bool, failed error) {
	if !errors.Is(err, client.ErrNoAccess) || !sessionLacksGroup() {
		return false, nil
	}
	sudo, lookErr := lookPath("sudo")
	if lookErr != nil {
		return false, nil
	}
	exe, exeErr := thisProgram()
	if exeErr != nil {
		return false, nil
	}
	fmt.Fprintf(stderr, "This login session lacks the group %s; starting the interface again under sudo -g %s.\n",
		control.ServiceGroup, control.ServiceGroup)
	argv := append([]string{"sudo", "-g", control.ServiceGroup, exe}, regroupArgs()...)
	if execErr := execv(sudo, argv, os.Environ()); execErr != nil {
		return true, &cli.Failure{Code: output.CodePermission, Message: "cannot start the interface under sudo -g " + control.ServiceGroup + ": " + execErr.Error(),
			NextStep: "Log in again so the session has the group " + control.ServiceGroup + ", then run 'qhost tui'.", Cause: execErr}
	}
	return true, nil
}

// lacksServiceGroup reports whether this user administers the host per the group database, the group qhost
// included, while the process holds no group qhost: the login session started before the membership.
func lacksServiceGroup() bool {
	if ok, err := (admins.Database{}).Admin(uint32(os.Getuid())); err != nil || !ok {
		return false
	}
	g, err := user.LookupGroup(control.ServiceGroup)
	if err != nil {
		return false
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil || os.Getegid() == gid {
		return false
	}
	groups, err := os.Getgroups()
	return err == nil && !slices.Contains(groups, gid)
}
