package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/localipc"
	"github.com/castrowithcee/qatlas-kit/uiprefs"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

// fakeTerminal makes standard input and the returned stdout count as terminals and records whether the
// interface was started, with which options.
func fakeTerminal(t *testing.T) (stdout *os.File, started *[]tui.Options) {
	t.Helper()
	dir := t.TempDir()
	open := func(name string) *os.File {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		return f
	}
	in, out := open("in"), open("out")
	t.Setenv("HOME", dir)
	oldIn, oldTerm, oldRun := stdinStream, isTerminal, runTUI
	stdinStream = func() io.Reader { return in }
	isTerminal = func(f *os.File) bool { return f == in || f == out }
	var calls []tui.Options
	runTUI = func(opts tui.Options) error { calls = append(calls, opts); return nil }
	t.Cleanup(func() { stdinStream, isTerminal, runTUI = oldIn, oldTerm, oldRun })
	return out, &calls
}

func runOn(stdout io.Writer, args ...string) (code int, stderr string) {
	var errOut bytes.Buffer
	code = Run(args, stdout, &errOut)
	return code, errOut.String()
}

func TestNoArgumentsAtATerminalStartsTheInterface(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	out, started := fakeTerminal(t)
	if code, stderr := runOn(out); code != 0 || len(*started) != 1 {
		t.Fatalf("code %d, started %d, stderr %q", code, len(*started), stderr)
	}
	opts := (*started)[0]
	if opts.Out != out || opts.In == nil || opts.Backend == nil || opts.Prefs.Language != "english" || len(opts.PrefsWarnings) != 0 {
		t.Fatalf("options = %+v", opts)
	}
}

func TestTUICommandReadsThePreferences(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	out, started := fakeTerminal(t)
	dir := filepath.Join(os.Getenv("HOME"), ".qatlas", "qhost")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tui.yaml"), []byte("language: deutsch\nstart: runs\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, stderr := runOn(out, "tui"); code != 0 || len(*started) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if p := (*started)[0].Prefs; p.Language != "deutsch" || p.Start != "runs" {
		t.Fatalf("prefs = %+v", p)
	}
}

func TestTUICommandSavesThePreferencesWhereItReadsThem(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	out, started := fakeTerminal(t)
	if code, stderr := runOn(out, "tui"); code != 0 || len(*started) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	opts := (*started)[0]
	if opts.SavePrefs == nil {
		t.Fatal("the interface cannot save the preferences")
	}
	schema, err := tui.PrefsSchema()
	if err != nil {
		t.Fatal(err)
	}
	want := opts.Prefs
	want.Language, want.Start, want.MouseEnabled = uiprefs.LanguageGerman, "runs", false
	if err := opts.SavePrefs(want); err != nil {
		t.Fatal(err)
	}
	path, err := prefsPath()
	if err != nil {
		t.Fatal(err)
	}
	if got, warnings := uiprefs.Load(path, schema); len(warnings) != 0 || got.Language != want.Language ||
		got.Start != want.Start || got.MouseEnabled || got.Theme != want.Theme || got.Icons != want.Icons {
		t.Fatalf("read back %+v with %v, want %+v", got, warnings, want)
	}
}

func TestTUICommandWithoutHomeCannotSavePreferences(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	out, started := fakeTerminal(t)
	t.Setenv("HOME", "")
	if code, stderr := runOn(out, "tui"); code != 0 || len(*started) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if (*started)[0].SavePrefs != nil {
		t.Fatal("a save callback without a home directory")
	}
}

func TestNoArgumentsWithoutATerminalShowsHelp(t *testing.T) {
	_, started := fakeTerminal(t)
	var out bytes.Buffer
	if code, _ := runOn(&out); code != 0 || !strings.Contains(out.String(), "Usage:") || len(*started) != 0 {
		t.Fatalf("code %d, stdout %q, started %d", code, out.String(), len(*started))
	}
}

func TestTUICommandNeedsATerminal(t *testing.T) {
	_, started := fakeTerminal(t)
	var out bytes.Buffer
	code, stderr := runOn(&out, "tui")
	if code != 2 || !strings.Contains(stderr, "needs a terminal") || len(*started) != 0 {
		t.Fatalf("code %d, stderr %q, started %d", code, stderr, len(*started))
	}
}

// A deploy service that is set up but not running does not keep the interface from starting: it shows the cause.
func TestInterfaceStartsWhenTheSetUpServiceIsNotRunning(t *testing.T) {
	useEndpoint(t, channeltest.Endpoint(t))
	out, started := fakeTerminal(t)
	if code, stderr := runOn(out); code != 0 || len(*started) != 1 {
		t.Fatalf("code %d, stderr %q, started %d", code, stderr, len(*started))
	}
}

func TestInterfaceStartsWithoutAnyDeployService(t *testing.T) {
	useFolderHost(t)
	noSystemService(t)
	out, started := fakeTerminal(t)
	if code, stderr := runOn(out); code != 0 || len(*started) != 1 {
		t.Fatalf("code %d, stderr %q, started %d", code, stderr, len(*started))
	}
}

// A channel that refuses this connection, here by a folder that is not private, is a cause to show, not a failure.
func TestInterfaceStartsWhenTheChannelIsRefused(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	if err := os.Chmod(serviceEndpoint().Dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := newEngineBackend().Status(context.Background()); !errors.Is(err, localipc.ErrRefused) {
		t.Fatalf("the channel did not refuse: %v", err)
	}
	out, started := fakeTerminal(t)
	if code, stderr := runOn(out); code != 0 || len(*started) != 1 {
		t.Fatalf("code %d, stderr %q, started %d", code, stderr, len(*started))
	}
}

func TestInterfaceAsNonRootNamesSudo(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, control.ErrNotRoot() })
	out, started := fakeTerminal(t)
	code, stderr := runOn(out, "tui")
	if code != 1 || !strings.Contains(stderr, "sudo") || len(*started) != 0 {
		t.Fatalf("code %d, stderr %q, started %d", code, stderr, len(*started))
	}
}

// fakeSudo stands in for the programs hostRunner and regroup look up and for the user they run as.
func fakeSudo(t *testing.T, euid int) {
	t.Helper()
	oldLook, oldExe, oldEuid := lookPath, thisProgram, tuiEuid
	lookPath = func(string) (string, error) { return "/invented/bin/sudo", nil }
	thisProgram = func() (string, error) { return "/invented/bin/qhost", nil }
	tuiEuid = func() int { return euid }
	t.Cleanup(func() { lookPath, thisProgram, tuiEuid = oldLook, oldExe, oldEuid })
}

// A secret goes to the child on standard input only, never into its arguments or its environment.
func TestHostRunnerKeepsTheSecretOutOfTheCommandLine(t *testing.T) {
	fakeSudo(t, 1000)
	marker := "demo-initial-4711"
	cmd, err := hostRunner(tui.HostRequest{Args: []string{"host", "apply", "--confirm", "--", "user.create", "dora"}, Secret: []byte(marker)})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/invented/bin/sudo", "-n", "/invented/bin/qhost", "host", "apply", "--confirm", "--", "user.create", "dora"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("args %q, want %q", cmd.Args, want)
	}
	for _, part := range append(slices.Clone(cmd.Args), cmd.Env...) {
		if strings.Contains(part, marker) {
			t.Fatalf("the secret is in the command line or environment: %q", part)
		}
	}
	if !slices.Contains(cmd.Env, "LC_ALL=C") {
		t.Fatal("sudo -n does not run with the C locale")
	}
	if in, _ := io.ReadAll(cmd.Stdin); string(in) != marker {
		t.Fatalf("standard input %q", in)
	}
	check, err := hostRunner(tui.HostRequest{Validate: true})
	if err != nil || !slices.Equal(check.Args, []string{"/invented/bin/sudo", "-v"}) {
		t.Fatalf("password check %v, %v", check, err)
	}
	plain, err := hostRunner(tui.HostRequest{Args: []string{"host", "apply", "--", "user.lock", "cleo"}})
	if err != nil || !slices.Equal(plain.Args, []string{"/invented/bin/sudo", "/invented/bin/qhost", "host", "apply", "--", "user.lock", "cleo"}) || plain.Stdin != nil {
		t.Fatalf("plain %v, %v", plain, err)
	}
}

// An action that reads an empty standard input, as file.write for an empty file, never reads the terminal of sudo.
func TestHostRunnerGivesAnEmptyInputItsOwnStandardInput(t *testing.T) {
	args := []string{"host", "apply", "--confirm", "--", "file.write", "/srv/demo/new.txt"}
	for _, euid := range []int{1000, 0} {
		fakeSudo(t, euid)
		cmd, err := hostRunner(tui.HostRequest{Args: args, EmptyInput: true})
		if err != nil || cmd.Stdin == nil {
			t.Fatalf("euid %d: %v, %v", euid, cmd, err)
		}
		if in, _ := io.ReadAll(cmd.Stdin); len(in) != 0 {
			t.Fatalf("euid %d: standard input %q", euid, in)
		}
		if euid != 0 && !slices.Contains(cmd.Args, "-n") {
			t.Fatalf("the second step of sudo prompts again: %q", cmd.Args)
		}
	}
}

// A request about a proposal runs `qhost proposal`, under sudo or, as root, itself.
func TestHostRunnerRunsProposalCommandsOfTheProposalGroup(t *testing.T) {
	fakeSudo(t, 1000)
	cmd, err := hostRunner(tui.HostRequest{Args: []string{"proposal", "reject", "h-0001"}, Terminal: true})
	if err != nil || !slices.Equal(cmd.Args, []string{"/invented/bin/sudo", "/invented/bin/qhost", "proposal", "reject", "h-0001"}) {
		t.Fatalf("under sudo %v, %v", cmd, err)
	}
	fakeSudo(t, 0)
	cmd, err = hostRunner(tui.HostRequest{Args: []string{"proposal", "reject", "h-0001"}, Terminal: true})
	if err != nil || !slices.Equal(cmd.Args, []string{"/invented/bin/qhost", "proposal", "reject", "h-0001"}) || cmd.Stdin != os.Stdin {
		t.Fatalf("as root %v, %v", cmd, err)
	}
}

// Any public command, not only those of the host, runs under sudo with the C locale; the password check too.
func TestHostRunnerRunsEveryCommandUnderSudoWithTheCLocale(t *testing.T) {
	fakeSudo(t, 1000)
	args := []string{"container", "restart", "--yes", "--output", "json", "--", "0123456789ab"}
	cmd, err := hostRunner(tui.HostRequest{Args: args})
	if err != nil || !slices.Equal(cmd.Args, append([]string{"/invented/bin/sudo", "/invented/bin/qhost"}, args...)) || cmd.Stdin != nil {
		t.Fatalf("container restart %v, %v", cmd, err)
	}
	check, err := hostRunner(tui.HostRequest{Validate: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []*exec.Cmd{cmd, check} {
		// The runner adds the locale and nothing else to the environment it was started with.
		if !slices.Equal(c.Env, append(os.Environ(), "LC_ALL=C")) {
			t.Fatalf("%q runs with the environment %q", c.Args, c.Env)
		}
	}
	for _, bad := range []tui.HostRequest{{}, {Args: []string{"-n", "host", "journal"}}} {
		if cmd, err := hostRunner(bad); err == nil || cmd != nil {
			t.Fatalf("%+v built %v", bad, cmd)
		}
	}
}

func TestHostRunnerAsRootRunsWithoutSudo(t *testing.T) {
	fakeSudo(t, 0)
	if cmd, err := hostRunner(tui.HostRequest{Validate: true}); cmd != nil || err != nil {
		t.Fatalf("as root the password check is %v, %v", cmd, err)
	}
	cmd, err := hostRunner(tui.HostRequest{Args: []string{"host", "apply", "--", "user.set-password", "cleo"}, Secret: []byte("demo-initial-4711")})
	if err != nil || !slices.Equal(cmd.Args, []string{"/invented/bin/qhost", "host", "apply", "--", "user.set-password", "cleo"}) {
		t.Fatalf("as root %v, %v", cmd, err)
	}
	if in, _ := io.ReadAll(cmd.Stdin); string(in) != "demo-initial-4711" {
		t.Fatalf("standard input %q", in)
	}
}

func TestHostRunnerWithoutSudo(t *testing.T) {
	fakeSudo(t, 1000)
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	if _, err := hostRunner(tui.HostRequest{Validate: true}); !errors.Is(err, tui.ErrNoSudo) {
		t.Fatalf("err %v", err)
	}
}

func TestTUICommandHandsTheHostRunnerToTheInterface(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	out, started := fakeTerminal(t)
	if code, stderr := runOn(out, "tui"); code != 0 || len(*started) != 1 || (*started)[0].HostRun == nil {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

// A login session that predates the membership in qhost starts the interface again under sudo -g qhost.
func TestRegroupStartsTheInterfaceAgainWithTheGroup(t *testing.T) {
	fakeSudo(t, 1000)
	oldLacks, oldExec, oldArgs := sessionLacksGroup, execv, regroupArgs
	t.Cleanup(func() { sessionLacksGroup, execv, regroupArgs = oldLacks, oldExec, oldArgs })
	var ran []string
	execv = func(_ string, argv []string, _ []string) error { ran = argv; return nil }
	regroupArgs = func() []string { return []string{"tui"} }
	denied := fmt.Errorf("dial: %w", client.ErrNoAccess)

	sessionLacksGroup = func() bool { return false }
	if handled, _ := regroup(denied, io.Discard); handled || ran != nil {
		t.Fatal("a session with the group started again")
	}
	sessionLacksGroup = func() bool { return true }
	if handled, _ := regroup(errors.New("other"), io.Discard); handled || ran != nil {
		t.Fatal("another failure started again")
	}
	var stderr bytes.Buffer
	if handled, failed := regroup(denied, &stderr); !handled || failed != nil {
		t.Fatalf("handled %v, failed %v", handled, failed)
	}
	if !slices.Equal(ran, []string{"sudo", "-g", "qhost", "/invented/bin/qhost", "tui"}) || !strings.Contains(stderr.String(), "sudo -g qhost") {
		t.Fatalf("ran %q, stderr %q", ran, stderr.String())
	}
}

// The help of tui names the key that sets up the deploy service, not the one of the diagnosis.
func TestTUIHelpNamesTheSetupKey(t *testing.T) {
	long := newTUICommand().Long
	if !strings.Contains(long, "set it up\n(G;") || strings.Contains(long, "(D;") {
		t.Fatalf("help of tui does not name G for setting up the deploy service:\n%s", long)
	}
}

func TestTUICommandReadsEveryNavigationFormAndDefaultsToTabs(t *testing.T) {
	forms := map[string]string{"": uiprefs.NavigationTabs, "navigation: tabs\n": uiprefs.NavigationTabs,
		"navigation: pathline\n": uiprefs.NavigationPathLine, "navigation: header\n": uiprefs.NavigationPathHeader,
		"navigation: choice\n": uiprefs.NavigationHeaderChoice}
	for yaml, want := range forms {
		startService(t, func(net.Conn) (uint32, error) { return 0, nil })
		out, started := fakeTerminal(t)
		dir := filepath.Join(os.Getenv("HOME"), ".qatlas", "qhost")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "tui.yaml"), []byte("start: runs\n"+yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		if code, stderr := runOn(out, "tui"); code != 0 || len(*started) != 1 {
			t.Fatalf("%q: code %d, stderr %q", yaml, code, stderr)
		}
		if p := (*started)[0]; p.Prefs.Navigation != want || len(p.PrefsWarnings) != 0 {
			t.Fatalf("%q: navigation %q, warnings %v", yaml, p.Prefs.Navigation, p.PrefsWarnings)
		}
	}
}
