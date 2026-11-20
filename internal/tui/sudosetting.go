package tui

import (
	"context"
	"encoding/json"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// settingChange is a change of the detail or the Settings area that the service takes only from root. Started as
// root it is the call of the service, otherwise the public command run through sudo; the answer of either goes
// through applySetting, so the result and the failure look the same.
type settingChange struct {
	title                 kittui.TextID
	question, consequence string
	details               []string
	// danger marks a change that cannot be undone.
	danger bool
	op     settingOp
	name   string
	// direct is the call made as root.
	direct func(b Backend) (any, error)
	// args are the arguments after qhost of the command made without root; it asks no question and prints JSON.
	args []string
	// input, where set, gives what the command reads on standard input. It runs before the command, so it reads
	// what the service holds now.
	input func(ctx context.Context, b Backend) ([]byte, error)
	// decode reads the answer of the command into what applySetting takes; false when the answer holds none.
	decode func(sudoRun) (any, bool)
}

// askSetting asks for the confirmation of a change; without root the question shows the command.
func (m *Model) askSetting(c settingChange) {
	if m.euid() == 0 {
		m.ask(c.title, c.question, c.consequence, c.details, c.danger, func() tea.Cmd {
			return m.writeCall(c.op, c.name, c.direct)
		})
		return
	}
	if m.settingsBusy() {
		m.warn(m.text.Lookup(textActionBusy))
		return
	}
	var gen int
	m.askSudo(sudoAsk{title: c.title, question: c.question, consequence: c.consequence, lead: dialogLines(c.details),
		danger: c.danger, args: c.args, input: c.input,
		begin: func() { gen = m.beginSetting() },
		done: func(r sudoRun) tea.Cmd {
			msg := settingMsg{gen: gen, op: c.op, name: c.name}
			if res, ok := c.decode(r); ok {
				msg.res = res
			} else {
				msg.err = sudoProblem(r.problem)
			}
			return m.applySetting(msg)
		}})
}

// beginSetting marks a write as under way, as writeCall does, and returns its generation.
func (m *Model) beginSetting() int {
	m.set.busy = true
	m.set.gen++
	return m.set.gen
}

// settingsEdit changes the settings the service holds now.
type settingsEdit func(s *control.StackSettings)

// askSettingsSet asks for the replacement of the settings of stack by what edit makes of the ones held now.
func (m *Model) askSettingsSet(title kittui.TextID, question, consequence string, details []string, stack string, edit settingsEdit) {
	m.askSetting(settingChange{title: title, question: question, consequence: consequence, details: details,
		op: opStackSettingsSet, name: stack,
		direct: func(b Backend) (any, error) {
			s, err := currentSettings(m.ctx, b, stack)
			if err != nil {
				return control.StackSettingsSetResult{}, err
			}
			edit(&s)
			return b.StackSettingsSet(m.ctx, stack, s)
		},
		args: []string{"stack", "settings", "set", "--file", "-", "--output", "json", "--", stack},
		input: func(ctx context.Context, b Backend) ([]byte, error) {
			s, err := currentSettings(ctx, b, stack)
			if err != nil {
				return nil, err
			}
			edit(&s)
			return json.Marshal(s)
		},
		decode: decodeSudo[control.StackSettingsSetResult](func(r control.StackSettingsSetResult) bool { return r.Stack != "" })})
}

// decodeSudo reads the JSON answer of a command as T; ok says whether it holds an answer.
func decodeSudo[T any](ok func(T) bool) func(sudoRun) (any, bool) {
	return func(r sudoRun) (any, bool) {
		var v T
		if !r.decode(&v) || !ok(v) {
			return nil, false
		}
		return v, true
	}
}

// decodeRestore reads what `stack backup restore` prints.
func decodeRestore(r sudoRun) (any, bool) {
	var v struct {
		Backup string `json:"backup"`
		Stack  string `json:"stack"`
	}
	if !r.decode(&v) || v.Backup == "" || v.Stack == "" {
		return nil, false
	}
	return control.BackupRestoreResult{Backup: control.BackupInfo{ID: v.Backup, Stack: v.Stack}}, true
}
