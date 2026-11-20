package tui

import (
	"bytes"
	"context"
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
	// also are further commands of the same form that done runs one after another once the first one ended, as
	// one per stack; the question shows them with the first.
	also [][]string
	// input, where set, gives the standard input of args. It runs before the command, off the event loop.
	input func(ctx context.Context, b Backend) ([]byte, error)
	// begin, where set, runs on y before the command, as to mark the action busy.
	begin func()
	// done gets the end of the run.
	done func(sudoRun) tea.Cmd
	// drop, where set, runs on every other answer.
	drop func()
}

// sudoRun is the end of a run through sudo. out is what the command printed on standard output; problem is what
// to say when out holds no usable answer: why the run failed, or that its answer could not be read. code is the
// code of the failure qhost reported, as state-changed; empty when it reported none.
type sudoRun struct {
	out     []byte
	problem string
	code    string
}

// decode reads the JSON answer of the command into v; false when it printed none.
func (r sudoRun) decode(v any) bool {
	out := bytes.TrimSpace(r.out)
	return len(out) > 0 && json.Unmarshal(out, v) == nil
}

// sudoProblem is the failure of a run through sudo in the plain words of problem; it stands where an action got an
// error of the service.
type sudoProblem string

func (e sudoProblem) Error() string { return string(e) }

// sudoInputMsg carries the standard input of a command that was read before it runs.
type sudoInputMsg struct {
	args  []string
	input []byte
	err   error
	done  func(sudoRun) tea.Cmd
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
	commands := []string{sudoCommand(a.args)}
	for _, more := range a.also {
		if len(commands) == maxListedTargets {
			consequence = strings.TrimSpace(consequence + " " + m.text.Lookup(textSudoMoreCommands, len(a.also)+1-len(commands)))
			break
		}
		commands = append(commands, sudoCommand(more))
	}
	m.askCommands(a.title, a.question, consequence, false, a.danger, a.lead, commands, func() tea.Cmd {
		if a.begin != nil {
			a.begin()
		}
		if a.input != nil {
			return m.readSudoInput(a)
		}
		return m.runSudo(a.args, a.done)
	}, a.drop)
}

// runSudo hands the terminal to `sudo qhost args` and gives done the answer.
func (m *Model) runSudo(args []string, done func(sudoRun) tea.Cmd) tea.Cmd {
	return m.runSudoInput(args, nil, done)
}

// runSudoInput is runSudo for a command with standard input. Standard input belongs to the password prompt of sudo,
// so the run goes in two steps, as one with a secret does: the password first, then the command without a prompt.
func (m *Model) runSudoInput(args []string, input []byte, done func(sudoRun) tea.Cmd) tea.Cmd {
	return m.runHost(HostRequest{Args: args, Secret: input}, func(out []byte, err error) tea.Cmd {
		return done(sudoRun{out: out, problem: m.sudoProblem(out, err), code: commandCode(err)})
	})
}

// readSudoInput reads the standard input of the command of a, then runs it.
func (m *Model) readSudoInput(a sudoAsk) tea.Cmd {
	ctx, b := m.ctx, m.backend
	return func() tea.Msg {
		in, err := a.input(ctx, b)
		return sudoInputMsg{args: a.args, input: in, err: err, done: a.done}
	}
}

// applySudoInput runs the command whose standard input was read; a failed read ends the run without it.
func (m *Model) applySudoInput(msg sudoInputMsg) tea.Cmd {
	if msg.err != nil {
		return msg.done(sudoRun{problem: m.serviceText(msg.err)})
	}
	return m.runSudoInput(msg.args, msg.input, msg.done)
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
		// Without a terminal sudo -n holds no password. With one sudo asked itself, so the words mean that the
		// prompt was given up.
		if ranWithoutPrompt(err) {
			return t.Lookup(textHostErrNoCredential)
		}
		return t.Lookup(textSudoErrAborted)
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
	_, msg := commandFailure(err)
	return msg
}

// commandCode is the code of the failure qhost wrote to standard error, as state-changed; empty when there is none.
func commandCode(err error) string {
	code, _ := commandFailure(err)
	return code
}

// commandFailure is the code and the message of the first failure qhost wrote to standard error.
func commandFailure(err error) (code, msg string) {
	var re *hostRunError
	if !errors.As(err, &re) {
		return "", ""
	}
	for _, l := range strings.Split(re.stderr, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(ansi.Strip(l)), "qhost: ")
		if !ok || rest == "" {
			continue
		}
		if c, m, found := strings.Cut(rest, ": "); found && isErrorCode(c) {
			code, rest = c, m
		}
		return code, ansi.Truncate(rest, 200, "…")
	}
	return "", ""
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
