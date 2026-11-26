package tui

import (
	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// The Webhooks section of the stack detail lists the hooks of the stack, adds one and removes one. Both writes need
// root, so they run like the other changes of the detail: directly as root, else as `sudo qhost hook …`. The secret
// address of a new hook travels only in the answer of the add and is shown by one dialog; nothing keeps it.

// hookRemoved is the answer of a removed hook.
type hookRemoved struct{ id string }

type hooksMsg struct {
	stack string
	gen   int
	res   HookListResult
	err   error
}

// readHooks reads the hooks of the stack shown; a read under way is not asked twice.
func (m *Model) readHooks() tea.Cmd {
	d := &m.detail
	if d.stack == "" || d.hk.loading {
		return nil
	}
	d.hk.loading = true
	ctx, b, stack, gen := m.engineCtx(d.engine), m.backend, d.stack, d.gen
	return func() tea.Msg {
		res, err := b.HookList(ctx, stack)
		return hooksMsg{stack: stack, gen: gen, res: res, err: err}
	}
}

func (m *Model) applyHooks(msg hooksMsg) {
	d := &m.detail
	if msg.gen != d.gen || msg.stack != d.stack {
		return
	}
	d.hk.loading = false
	if d.hk.err = msg.err; msg.err != nil {
		d.hk.loaded, d.hooks = false, HookListResult{}
		return
	}
	d.hk.loaded, d.hooks = true, msg.res
}

func (m *Model) hookSectionLines(r stackRow, width int) []string {
	t, d := m.text, &m.detail
	if note, ready := m.partNote(d.hk, width); !ready {
		return note
	}
	hint := func(id kittui.TextID, style kittui.TextStyle) []string {
		return m.wrapped(detailIndent, style, t.Lookup(id), width)
	}
	if !d.hooks.SetUp {
		return hint(textHooksNoReceiver, kittui.StyleHint)
	}
	var out []string
	if len(d.hooks.Hooks) == 0 {
		out = hint(textHooksNone, kittui.StyleHint)
	}
	for _, h := range d.hooks.Hooks {
		created := t.Lookup(textNoValue)
		if !h.Created.IsZero() {
			created = h.Created.In(m.loc).Format(polledLayout)
		}
		out = append(out, m.line(detailIndent, kittui.StyleText, h.ID+"  "+created, width))
	}
	if r.trigger == "window" || r.trigger == "" {
		out = append(out, hint(textHooksWindow, kittui.StyleWarn)...)
	}
	return out
}

// hooksReady reports whether hooks can be added or removed; else it says why not.
func (m *Model) hooksReady() bool {
	d := &m.detail
	if !d.hk.loaded {
		m.warn(m.text.Lookup(textDetailLoading))
		return false
	}
	if !d.hooks.SetUp {
		m.warn(m.text.Lookup(textHooksNoReceiver))
		return false
	}
	return true
}

func (m *Model) askHookAdd() tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || m.detailBusy() || !m.hooksReady() {
		return nil
	}
	engine := m.stackEngineOf(stack)
	ctx := m.engineCtx(engine)
	m.askSetting(settingChange{title: textTitleHookAdd, question: m.text.Lookup(textAskHookAdd, stack),
		consequence: m.text.Lookup(textWhatHookAdd), op: opHookAdd, name: stack,
		direct: func(b Backend) (any, error) { return b.HookAdd(ctx, stack) },
		args:   m.engineArgs(engine, []string{"hook", "add", "--output", "json", "--", stack}),
		decode: decodeHookAdd})
	return nil
}

// decodeHookAdd reads what `hook add` prints. The run's output holds the secret, so it is overwritten once read.
func decodeHookAdd(r sudoRun) (any, bool) {
	defer zeroAll(r.out)
	var v struct {
		ID     string `json:"id"`
		Path   string `json:"path"`
		Listen string `json:"listen"`
	}
	if !r.decode(&v) || v.ID == "" || v.Path == "" {
		return nil, false
	}
	return HookAddResult{ID: v.ID, Listen: v.Listen, Path: v.Path}, true
}

func (m *Model) openHookPick() tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || m.detailBusy() {
		return nil
	}
	d := &m.detail
	if !d.hk.loaded {
		m.warn(m.text.Lookup(textDetailLoading))
		return nil
	}
	if len(d.hooks.Hooks) == 0 {
		m.warn(m.text.Lookup(textHooksNoneToRemove))
		return nil
	}
	items := make([]kittui.ListItem, len(d.hooks.Hooks))
	for i, h := range d.hooks.Hooks {
		items[i] = kittui.ListItem{ID: kittui.ItemID(h.ID), Cells: []string{h.ID}}
	}
	m.pick = m.newPicker(pickHook, stack, textTitleHookRemove, []kittui.Column{{Title: textColHookID}},
		textHooksNone, items)
	return nil
}

// askHookRemove asks to remove the hook the person chose.
func (m *Model) askHookRemove(stack, id string) {
	ctx := m.engineCtx(m.stackEngineOf(stack))
	m.askSetting(settingChange{title: textTitleHookRemove, question: m.text.Lookup(textAskHookRemove, id, stack),
		consequence: m.text.Lookup(textWhatHookRemove), danger: true, op: opHookRemove, name: id,
		direct: func(b Backend) (any, error) { return hookRemoved{id}, b.HookRemove(ctx, id) },
		args:   []string{"hook", "remove", "--yes", "--output", "json", "--", id},
		decode: func(r sudoRun) (any, bool) {
			_, ok := decodeFlag("removed")(r)
			return hookRemoved{id}, ok
		}})
}

// hookAdded shows the address of the new hook once and reads the list again. The dialog is the only holder of the
// secret: closing it drops the dialog, and no other field, message or log takes a copy. It fills the workspace, as the
// box of the kit is too narrow for the address, which would break inside the secret.
func (m *Model) hookAdded(res HookAddResult, stack string) tea.Cmd {
	t := m.text
	address := t.Lookup(textHookSecretAddress, kittui.Escape(res.Listen), kittui.Escape(res.Path))
	dialog, err := kittui.NewDialog(kittui.DialogSpec{Title: textTitleHookSecret, Question: t.Lookup(textHookSecretShown, res.ID, stack),
		Notice: true, Details: dialogLines([]string{address, t.Lookup(textHookSecretOnce)}),
		Answers: []kittui.Answer{{Action: actNo, Label: textAnswerClose}}, Default: actNo,
		Keys: kittui.DialogKeys{Enter: actEnter}}, m.keys, m.text)
	if err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
	} else {
		m.modal = &modal{kind: modalAsk, dialog: dialog, full: true}
	}
	return m.readHooks()
}
