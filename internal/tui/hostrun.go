package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// What needs root runs as a public qhost command: the interface hands the terminal to `sudo qhost …` and reads the
// answer that command prints. sudo asks for the password on the terminal; the interface never sees it. Started as
// root, the interface runs the same command without sudo.

// HostRequest is one call of the root hook.
type HostRequest struct {
	// Args are the arguments after "qhost", the command group first, as in "host apply …" or "container restart …".
	Args []string
	// Terminal gives a command that runs without sudo the terminal of the interface on standard input, as rejecting
	// a proposal checks for one.
	Terminal bool
	// Validate asks for the command that makes sudo ask for the password on the terminal and remember it
	// (sudo -v); Args and Secret are empty then. A hook that needs no password returns no command for it.
	Validate bool
	// Secret is the standard input of an action that reads a secret input. Standard input belongs to sudo's
	// password prompt, so a request with a Secret runs in two steps: a Validate request on the terminal, then
	// this request without a prompt (sudo -n) with Secret as the standard input of the command. The hook returns
	// a command whose Stdin carries it; it never goes into the arguments or the environment.
	Secret []byte
}

// HostRunner builds the command that runs a request as root. The interface sets Stdout (it reads the report
// there) and, where unset, Stdin and Stderr.
type HostRunner func(HostRequest) (*exec.Cmd, error)

// ErrNoSudo is what a HostRunner returns when sudo is not installed.
var ErrNoSudo = errors.New("sudo is not installed")

// errNoRunner is the failure of an interface that was given no way to run commands as root.
var errNoRunner = errors.New("no way to run commands as root")

// hostRanMsg delivers the end of a command run through the root hook.
type hostRanMsg struct {
	after func(out []byte, err error) tea.Cmd
	out   []byte
	err   error
}

// hostRoot is how the interface runs commands as root; tests replace exec and bg so nothing real runs.
type hostRoot struct {
	run HostRunner
	// in is the terminal a command that takes the screen over reads from.
	in io.Reader
	// exec hands the terminal to a command; nil takes tea.Exec.
	exec func(tea.ExecCommand, tea.ExecCallback) tea.Cmd
	// bg runs a command without the terminal; nil takes (*exec.Cmd).Run.
	bg func(*exec.Cmd) error
}

// build asks the hook for the command of req.
func (r hostRoot) build(req HostRequest) (*exec.Cmd, error) {
	if r.run == nil {
		return nil, errNoRunner
	}
	return r.run(req)
}

// runHost hands the terminal to the command of req; after gets what it printed on standard output and the error
// of the run. A request with a secret runs through runHostSecret.
func (m *Model) runHost(req HostRequest, after func(out []byte, err error) tea.Cmd) tea.Cmd {
	if len(req.Secret) > 0 {
		return m.runHostSecret(req, secretLines(req.Secret), after)
	}
	cmd, err := m.root.build(req)
	if err != nil {
		return func() tea.Msg { return hostRanMsg{after: after, err: err} }
	}
	if cmd == nil {
		// The hook needs nothing for this request, as for the password check of a hook that runs as root.
		return func() tea.Msg { return hostRanMsg{after: after} }
	}
	if m.euid() == 0 {
		// Run as root, nothing asks for a password: the command runs without taking the screen over.
		return m.runHostBackground(cmd, nil, after)
	}
	// sudo asks for the password on the terminal itself, not on standard error, so capturing standard error
	// leaves the prompt visible and gives the reason of a run that printed no report.
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	if cmd.Stdin == nil && m.root.in != nil {
		cmd.Stdin = m.root.in
	}
	if cmd.Stderr == nil {
		cmd.Stderr = &errOut
	}
	run := m.root.exec
	if run == nil {
		run = tea.Exec
	}
	return run(&handover{cmd: cmd}, func(err error) tea.Msg {
		if err != nil && errOut.Len() > 0 {
			err = &hostRunError{err: err, stderr: errOut.String()}
		}
		return hostRanMsg{after: after, out: out.Bytes(), err: err}
	})
}

// clearScreen moves the cursor to the top left and clears the screen.
const clearScreen = ansi.CursorHomePosition + ansi.EraseEntireScreen

// handover hands the terminal to a command. Leaving the interface shows the main screen with what earlier commands
// left there, such as an old sudo prompt, and the terminal echoes again until sudo turns the echo off: a password
// typed at the old prompt would show in plain text. So the main screen is cleared before the command starts.
type handover struct {
	cmd  *exec.Cmd
	term io.Writer
	// start runs the command; nil takes (*exec.Cmd).Run. Tests replace it so nothing real runs.
	start func(*exec.Cmd) error
}

func (h *handover) SetStdin(r io.Reader) {
	if h.cmd.Stdin == nil {
		h.cmd.Stdin = r
	}
}

// SetStdout takes the terminal; the command writes there only where it has no standard output of its own.
func (h *handover) SetStdout(w io.Writer) {
	h.term = w
	if h.cmd.Stdout == nil {
		h.cmd.Stdout = w
	}
}

func (h *handover) SetStderr(w io.Writer) {
	if h.cmd.Stderr == nil {
		h.cmd.Stderr = w
	}
}

func (h *handover) Run() error {
	if h.term != nil {
		if _, err := io.WriteString(h.term, clearScreen); err != nil {
			return err
		}
	}
	start := h.start
	if start == nil {
		start = (*exec.Cmd).Run
	}
	return start(h.cmd)
}

// hostRunError is the error of a run: the failure, and what the command wrote to standard error with the
// secrets taken out. noPrompt says the command ran without the terminal, as sudo -n does, so that sudo could not
// ask for a password.
type hostRunError struct {
	err      error
	stderr   string
	noPrompt bool
}

func (e *hostRunError) Error() string { return e.err.Error() }
func (e *hostRunError) Unwrap() error { return e.err }

// zeroAll overwrites the buffers that held a secret.
func zeroAll(bufs ...[]byte) {
	for _, b := range bufs {
		clear(b)
	}
}

// scrub takes every secret out of text.
func scrub(text string, secrets ...[]byte) string {
	for _, s := range secrets {
		if len(s) > 0 {
			text = strings.ReplaceAll(text, string(s), "<hidden>")
		}
	}
	return text
}

// secretLines are the single lines of a secret of several lines, as a key or the parts of a channel, that a
// message may quote without the rest; they are masked and overwritten like the secret itself. A line of fewer
// than minMaskedLine bytes or one of a PEM frame holds nothing to protect.
func secretLines(secret []byte) [][]byte {
	if !bytes.Contains(bytes.TrimSpace(secret), []byte("\n")) {
		return nil
	}
	var lines [][]byte
	for _, l := range bytes.Split(secret, []byte("\n")) {
		if l = bytes.TrimSpace(l); len(l) >= minMaskedLine && !bytes.HasPrefix(l, []byte("-----")) {
			lines = append(lines, l)
		}
	}
	return lines
}

// minMaskedLine is the shortest line of a secret that is masked on its own, the shortest value the service stores.
const minMaskedLine = 4

// runHostSecret runs a request that carries a secret in two steps. sudo asks for the password on the real
// terminal in the first (sudo -v), which qhost never sees; the second runs without a prompt, the secret on the
// standard input of the command, and stops where sudo would have asked again. The secret is overwritten when the
// run is over. mask are further texts, such as a password inside the secret, that must not reach the screen.
func (m *Model) runHostSecret(req HostRequest, mask [][]byte, after func(out []byte, err error) tea.Cmd) tea.Cmd {
	secret := req.Secret
	wipe := func() { zeroAll(append([][]byte{secret}, mask...)...) }
	return m.runHost(HostRequest{Validate: true}, func(_ []byte, err error) tea.Cmd {
		if err != nil {
			wipe()
			return func() tea.Msg { return hostRanMsg{after: after, err: err} }
		}
		cmd, err := m.root.build(req)
		if err == nil && cmd == nil {
			err = errNoRunner
		}
		if err != nil {
			wipe()
			return func() tea.Msg { return hostRanMsg{after: after, err: err} }
		}
		return m.runHostBackground(cmd, append([][]byte{secret}, mask...), after)
	})
}

// runHostBackground runs cmd off the event loop without the terminal. secrets are taken out of what it wrote to
// standard error and overwritten when the run is over.
func (m *Model) runHostBackground(cmd *exec.Cmd, secrets [][]byte, after func(out []byte, err error) tea.Cmd) tea.Cmd {
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	run := m.root.bg
	if run == nil {
		run = (*exec.Cmd).Run
	}
	m.sys.busy = true
	return func() tea.Msg {
		err := run(cmd)
		if err != nil {
			err = &hostRunError{err: err, stderr: scrub(errOut.String(), secrets...), noPrompt: true}
		}
		zeroAll(secrets...)
		return hostRanMsg{after: after, out: out.Bytes(), err: err}
	}
}

// hostReport is what `host apply` prints.
type hostReport struct {
	JournalID  string   `json:"journal_id"`
	Action     string   `json:"action"`
	Sentence   string   `json:"sentence"`
	Commands   []string `json:"commands"`
	Result     string   `json:"result"`
	Check      string   `json:"check"`
	CheckError string   `json:"check_error"`
	Restored   bool     `json:"restored"`
	Reversible bool     `json:"reversible"`
	Error      string   `json:"error"`
	Warning    string   `json:"warning"`
}

// parseReport reads the report of a run; false when the output holds none.
func parseReport(out []byte) (hostReport, bool) {
	var r hostReport
	if json.Unmarshal(bytes.TrimSpace(out), &r) != nil || r.Result == "" {
		return hostReport{}, false
	}
	return r, true
}

// The ways a run through sudo ends without a usable answer.
type hostFailure int

const (
	failNoSudo hostFailure = iota
	failDenied
	failAborted
	failExit
	failOutput
)

// classifyRun tells why a run gave no usable answer. exit is the exit code of failExit.
func classifyRun(out []byte, err error) (kind hostFailure, exit int) {
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, ErrNoSudo), errors.Is(err, errNoRunner):
		return failNoSudo, 0
	case err == nil:
		return failOutput, 0
	case !errors.As(err, &exitErr):
		return failNoSudo, 0
	}
	if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return failAborted, 0
	}
	switch code := exitErr.ExitCode(); {
	case code == 130 || code < 0:
		return failAborted, 0
	case code == 1 && len(bytes.TrimSpace(out)) == 0:
		return failDenied, 0
	default:
		return failExit, code
	}
}

// noCredential reports the standard error of sudo -n that has no password to use. The hook runs sudo with the C
// locale, so the words are those of sudo itself.
func noCredential(stderr string) bool {
	return strings.Contains(stderr, "a password is required") || strings.Contains(stderr, "a terminal is required")
}

// ranWithoutPrompt reports a run that had no terminal to ask for a password on.
func ranWithoutPrompt(err error) bool {
	var re *hostRunError
	return errors.As(err, &re) && re.noPrompt
}

// stderrLines are the first lines of what a run wrote to standard error, cut to fit a dialog.
func stderrLines(err error) string {
	var re *hostRunError
	if !errors.As(err, &re) {
		return ""
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(re.stderr), "\n") {
		if l = strings.TrimSpace(ansi.Strip(l)); l != "" && len(lines) < 3 {
			lines = append(lines, ansi.Truncate(l, 200, "…"))
		}
	}
	return strings.Join(lines, " ")
}
