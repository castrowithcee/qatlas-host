package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// The deploy service refuses every change of an administrator without root. Such an interface runs the public
// command of the action instead, as `sudo qhost …` through the root hook, after a question with a sentence in plain
// words, the exact command and y or n. sudo asks for the password on the terminal itself, and the interface reads
// the answer the command prints as JSON. Started as root, the interface calls the service directly.

// sudoAsk is one change made through sudo.
type sudoAsk struct {
	// title is the title of the question, the word of the action.
	title kittui.TextID
	// question is the sentence in plain words, consequence what follows.
	question, consequence string
	// remote, when set, is the sentence that the command asks a service outside the host, as GitHub or a registry;
	// it leads the consequence, so it is read before sudo asks for anything.
	remote string
	// lead are the lines between the consequence and the command, as the state before.
	lead []kittui.DialogLine
	// danger makes the confirming answer red.
	danger bool
	// args are the arguments after qhost: a public command that asks no question and prints its answer as JSON.
	args []string
	// begin, where set, runs on y before the command, as to mark the action busy.
	begin func()
	// done gets the end of the run.
	done func(sudoRun) tea.Cmd
	// drop, where set, runs on every other answer.
	drop func()
}

// sudoRun is the end of a run through sudo. out is what the command printed on standard output; problem is what
// to say when out holds no usable answer: why the run failed, or that its answer could not be read.
type sudoRun struct {
	out     []byte
	problem string
}

// decode reads the JSON answer of the command into v; false when it printed none.
func (r sudoRun) decode(v any) bool {
	out := bytes.TrimSpace(r.out)
	return len(out) > 0 && json.Unmarshal(out, v) == nil
}

// sudoCommand is the command a question shows, exactly as it runs apart from the path of this program.
func sudoCommand(args []string) string {
	return hostchange.Command{Argv: append([]string{"sudo", "qhost"}, args...)}.Display()
}

// askSudo shows the question of a change through sudo; y runs the command.
func (m *Model) askSudo(a sudoAsk) {
	consequence := a.consequence
	if a.remote != "" {
		consequence = strings.TrimSpace(a.remote + " " + consequence)
	}
	m.askCommands(a.title, a.question, consequence, false, a.danger, a.lead, []string{sudoCommand(a.args)}, func() tea.Cmd {
		if a.begin != nil {
			a.begin()
		}
		return m.runSudo(a.args, a.done)
	}, a.drop)
}

// runSudo hands the terminal to `sudo qhost args` and gives done the answer.
func (m *Model) runSudo(args []string, done func(sudoRun) tea.Cmd) tea.Cmd {
	return m.runHost(HostRequest{Args: args}, func(out []byte, err error) tea.Cmd {
		return done(sudoRun{out: out, problem: m.sudoProblem(out, err)})
	})
}

// sudoProblem says in plain words why a run gave no usable answer. A failure qhost reported comes first, since
// only qhost writes lines that start with its name; everything else is the answer of sudo.
func (m *Model) sudoProblem(out []byte, err error) string {
	t := m.text
	if err == nil {
		return t.Lookup(textSudoErrOutput)
	}
	if msg := commandMessage(err); msg != "" {
		return t.Lookup(textSudoErrCommand, kittui.Escape(msg))
	}
	if noCredential(stderrLines(err)) {
		return t.Lookup(textHostErrNoCredential)
	}
	switch kind, exit := classifyRun(out, err); kind {
	case failNoSudo:
		if errors.Is(err, ErrNoSudo) || errors.Is(err, errNoRunner) {
			return t.Lookup(textHostErrNoSudo)
		}
		return t.Lookup(textHostErrStart, kittui.Escape(err.Error()))
	case failDenied:
		return t.Lookup(textHostErrDenied)
	case failAborted:
		return t.Lookup(textSudoErrAborted)
	default:
		return t.Lookup(textSudoErrExit, exit)
	}
}

// commandMessage is the message of the failure qhost wrote to standard error ("qhost: <code>: <message>"), without
// the name and the code; empty when there is none, as when sudo did not run it.
func commandMessage(err error) string {
	var re *hostRunError
	if !errors.As(err, &re) {
		return ""
	}
	for _, l := range strings.Split(re.stderr, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(ansi.Strip(l)), "qhost: ")
		if !ok || rest == "" {
			continue
		}
		if code, msg, found := strings.Cut(rest, ": "); found && isErrorCode(code) {
			rest = msg
		}
		return ansi.Truncate(rest, 200, "…")
	}
	return ""
}

// isErrorCode reports whether s has the form of the code of a qhost failure, as not-found.
func isErrorCode(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && r != '-' {
			return false
		}
	}
	return true
}
