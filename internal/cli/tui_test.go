package cli

import (
	"bytes"
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

func TestInterfaceWithoutServiceEndsWithCode3(t *testing.T) {
	useEndpoint(t, channeltest.Endpoint(t))
	out, started := fakeTerminal(t)
	code, stderr := runOn(out)
	if code != ExitServiceNotRunning || !strings.Contains(stderr, "not running") || len(*started) != 0 {
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
	cmd, err := hostRunner(tui.HostRequest{Args: []string{"apply", "--confirm", "--", "user.create", "dora"}, Secret: []byte(marker)})
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
	plain, err := hostRunner(tui.HostRequest{Args: []string{"apply", "--", "user.lock", "cleo"}})
	if err != nil || !slices.Equal(plain.Args, []string{"/invented/bin/sudo", "/invented/bin/qhost", "host", "apply", "--", "user.lock", "cleo"}) || plain.Stdin != nil {
		t.Fatalf("plain %v, %v", plain, err)
	}
}

func TestHostRunnerAsRootRunsWithoutSudo(t *testing.T) {
	fakeSudo(t, 0)
	if cmd, err := hostRunner(tui.HostRequest{Validate: true}); cmd != nil || err != nil {
		t.Fatalf("as root the password check is %v, %v", cmd, err)
	}
	cmd, err := hostRunner(tui.HostRequest{Args: []string{"apply", "--", "user.set-password", "cleo"}, Secret: []byte("demo-initial-4711")})
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
