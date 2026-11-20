package tui

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/i18n"
)

// ctReport is what `qhost container <verb> --output json` prints for a container that runs.
func ctReport(name string) string {
	return `{"name":"` + name + `","kind":"single","state":"ok","engine_state":"running","health":"","not_ready":""}`
}

// sudoCtModel is the Containers area of an interface run by the administrator ada (UID 1000) without root, with
// a fake sudo that answers with report.
func sudoCtModel(t *testing.T, b *fakeBackend, report string) (*Model, *fakeRoot) {
	t.Helper()
	m := newTestModel(t, i18n.English, b, ctWide, ctWideRows)
	m.euid = func() int { return 1000 }
	f := &fakeRoot{report: report}
	m.root = f.hook()
	send(m, "5")
	return m, f
}

// exitCode is the error of a real process that ended with code; the shell is no sudo and changes nothing.
func exitCode(t *testing.T, code string) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+code).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("no exit error: %v", err)
	}
	return err
}

// Without root each container action shows the exact command, hands the terminal to sudo with it and reads the
// answer it prints; the service is not called directly.
func TestContainerActionWithoutRootRunsTheCommandThroughSudo(t *testing.T) {
	for _, tt := range []struct{ key, verb, done string }{
		{"S", "start", "started"}, {"X", "stop", "stopped"}, {"Z", "restart", "restarted"},
	} {
		t.Run(tt.verb, func(t *testing.T) {
			b := containersBackend()
			m, f := sudoCtModel(t, b, ctReport("demo-stack-app-1"))
			selectContainer(t, m, "demo-stack-app-1")
			send(m, tt.key)
			command := "sudo qhost container " + tt.verb + " --yes --output json -- 0123456789ab"
			if screen := flat(m.render()); !strings.Contains(screen, "$ "+command) || !strings.Contains(screen, "yes, with sudo") {
				t.Fatalf("the question lacks the command %q:\n%s", command, m.render())
			}
			send(m, "y")
			want := []string{"container", tt.verb, "--yes", "--output", "json", "--", "0123456789ab"}
			if len(f.reqs) != 1 || !slices.Equal(f.reqs[0].Args, want) || len(f.reqs[0].Secret) != 0 || f.reqs[0].Validate {
				t.Fatalf("requests %+v, want %q", f.reqs, want)
			}
			if len(f.execs) != 1 || len(f.bgs) != 0 || !slices.Equal(f.execs[0].Args, append([]string{"sudo-fake", "qhost-fake"}, want...)) {
				t.Fatalf("the terminal was not handed to sudo: execs %d, runs %d", len(f.execs), len(f.bgs))
			}
			if got := b.actions(); len(got) != 0 {
				t.Fatalf("the service was called directly: %v", got)
			}
			screen := m.render()
			for _, w := range []string{"1 ok, 0 failed", tt.done, "State afterwards: ok (running)"} {
				if !strings.Contains(screen, w) {
					t.Errorf("the result lacks %q:\n%s", w, screen)
				}
			}
			if m.run != nil {
				t.Error("the action is still marked as running")
			}
		})
	}
}

// No, esc and enter run nothing, not even sudo.
func TestContainerActionWithoutRootCancelledRunsNothing(t *testing.T) {
	b := containersBackend()
	m, f := sudoCtModel(t, b, ctReport("demo-stack-app-1"))
	selectContainer(t, m, "demo-stack-app-1")
	for _, no := range []string{"n", "esc", "enter"} {
		send(m, "Z")
		if m.modal == nil {
			t.Fatal("no question")
		}
		send(m, no)
		if len(f.reqs) != 0 || len(f.execs) != 0 || len(b.actions()) != 0 || m.run != nil {
			t.Fatalf("answer %q ran something: %v %v", no, f.reqs, b.actions())
		}
	}
}

// The ways a run through sudo ends without an answer are said in plain words, and the action is over.
func TestContainerActionWithoutRootFailuresAreUnderstandable(t *testing.T) {
	for _, tt := range []struct {
		name, report, stderr, want string
		exit                       string
	}{
		{name: "prompt cancelled", exit: "130", want: "The run was interrupted before qhost answered. If the password prompt was cancelled, nothing was changed."},
		{name: "wrong password", exit: "1", stderr: "Sorry, try again.\nsudo: 3 incorrect password attempts",
			want: "sudo did not run it: the password was wrong or this account may not use sudo. Nothing was changed."},
		{name: "no held password", exit: "1", stderr: "sudo: a password is required", want: "sudo no longer holds your password"},
		{name: "command failed", exit: "1", stderr: "qhost: not-found: no such container\nList the containers with 'qhost container list'.",
			want: "qhost reported: no such container"},
		{name: "run active", exit: "1", stderr: "qhost: conflict: a run of stack demo-stack is active",
			want: "qhost reported: a run of stack demo-stack is active"},
		{name: "no answer", exit: "3", want: "The command ended with exit code 3 and no answer."},
		{name: "unreadable answer", report: "not json", want: "The answer of qhost could not be read."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := containersBackend()
			m, f := sudoCtModel(t, b, tt.report)
			f.stderr = tt.stderr
			if tt.exit != "" {
				f.runErr = exitCode(t, tt.exit)
			}
			selectContainer(t, m, "demo-stack-app-1")
			send(m, "S", "y")
			screen := flat(m.render())
			if !strings.Contains(screen, tt.want) || !strings.Contains(screen, "0 ok, 1 failed") {
				t.Errorf("no explanation %q:\n%s", tt.want, m.render())
			}
			if strings.Contains(screen, "List the containers") {
				t.Errorf("the next step of the command line shows:\n%s", screen)
			}
			if len(b.actions()) != 0 || m.run != nil {
				t.Errorf("calls %v, running %v", b.actions(), m.run != nil)
			}
		})
	}
}

// Without sudo nothing is handed the terminal, and the result says so.
func TestContainerActionWithoutSudoSaysSo(t *testing.T) {
	b := containersBackend()
	m, f := sudoCtModel(t, b, "")
	m.root.run = func(HostRequest) (*exec.Cmd, error) { return nil, ErrNoSudo }
	selectContainer(t, m, "demo-stack-app-1")
	send(m, "Z", "y")
	if screen := flat(m.render()); !strings.Contains(screen, "sudo is not installed on this host") || len(f.execs) != 0 {
		t.Fatalf("no explanation without sudo:\n%s", m.render())
	}
}

// A container that is not ready makes the command fail after it printed its answer; the answer still counts.
func TestContainerActionWithoutRootReadsTheAnswerOfAFailedCommand(t *testing.T) {
	b := containersBackend()
	m, f := sudoCtModel(t, b, `{"name":"demo-stack-app-1","kind":"own","state":"unhealthy","engine_state":"running","health":"unhealthy","not_ready":"unhealthy"}`)
	f.runErr, f.stderr = exitCode(t, "1"), "qhost: runtime: container demo-stack-app-1 is not ready: unhealthy"
	selectContainer(t, m, "demo-stack-app-1")
	send(m, "Z", "y")
	screen := flat(m.render())
	for _, want := range []string{"0 ok, 1 failed", "restarted, but the container is not ready: its health check fails"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the result lacks %q:\n%s", want, m.render())
		}
	}
}

// sudo remembers the password only for a while. Every action hands the terminal to sudo again, without -n, so sudo
// asks again once that time is over, as in the System area.
func TestContainerActionWithoutRootLetsSudoAskAgain(t *testing.T) {
	b := containersBackend()
	m, f := sudoCtModel(t, b, ctReport("demo-stack-app-1"))
	held := false
	prompts := 0
	m.root.exec = func(cmd *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
		if !held {
			prompts++
			held = true
		}
		return f.exec(cmd, fn)
	}
	selectContainer(t, m, "demo-stack-app-1")
	send(m, "Z", "y", "esc")
	held = false // the timeout of sudo is over
	send(m, "Z", "y")
	if prompts != 2 || len(f.execs) != 2 || len(f.bgs) != 0 {
		t.Fatalf("prompts %d, execs %d, runs %d", prompts, len(f.execs), len(f.bgs))
	}
	for _, cmd := range f.execs {
		if slices.Contains(cmd.Args, "-n") {
			t.Fatalf("a run cannot ask for the password: %q", cmd.Args)
		}
	}
	if !strings.Contains(m.render(), "1 ok, 0 failed") {
		t.Fatalf("the second run failed:\n%s", m.render())
	}
}

// Started as root, the action calls the service directly and sudo is not involved.
func TestContainerActionAsRootCallsTheServiceDirectly(t *testing.T) {
	b := containersBackend()
	m, f := sudoCtModel(t, b, "")
	m.euid = func() int { return 0 }
	selectContainer(t, m, "demo-stack-app-1")
	send(m, "Z")
	if strings.Contains(m.render(), "sudo qhost") {
		t.Fatalf("the question of root shows a sudo command:\n%s", m.render())
	}
	send(m, "y")
	if got := b.actions(); !slices.Equal(got, []string{"ctrestart:0123456789ab"}) || len(f.reqs) != 0 {
		t.Fatalf("calls %v, requests %v", got, f.reqs)
	}
}

// A remote preview says first which service outside the host is asked, then shows the command.
func TestSudoQuestionLeadsWithTheRemoteSentence(t *testing.T) {
	m, f := sudoCtModel(t, containersBackend(), "{}")
	ran := false
	m.askSudo(sudoAsk{title: kindTexts[kindCheck].word, question: "Ask now?", consequence: "Shows what is new.",
		remote: m.text.Lookup(textSudoRemoteRegistry), args: []string{"updates", "--refresh", "--output", "json"},
		done: func(sudoRun) tea.Cmd { ran = true; return nil }})
	screen := flat(m.render())
	remote := strings.Index(screen, "This asks the registries of the images")
	if remote < 0 || remote > strings.Index(screen, "Shows what is new.") || !strings.Contains(screen, "$ sudo qhost updates --refresh --output json") {
		t.Fatalf("the question:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatal("sudo ran before the answer")
	}
	send(m, "y")
	if !ran || len(f.execs) != 1 {
		t.Fatalf("ran %v, execs %d", ran, len(f.execs))
	}
}

// The command a question shows quotes what a shell would split.
func TestSudoCommandQuotes(t *testing.T) {
	if got := sudoCommand([]string{"stack", "deploy", "--", "demo stack"}); got != "sudo qhost stack deploy -- 'demo stack'" {
		t.Fatalf("command %q", got)
	}
}

func TestGoldenContainerRestartWithoutRoot(t *testing.T) {
	goldenContainerActionAs(t, 1000, "containers.restart.sudo", containersBackend, "example-cache", "Z")
}

func TestGoldenContainerStopWithoutRoot(t *testing.T) {
	goldenContainerActionAs(t, 1000, "containers.stop.sudo", containersBackend, "demo-stack-app-1", "X")
}
