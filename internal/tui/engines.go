package tui

import (
	"context"
	"slices"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The commands of the actions run through sudo find the deploy service on their own: a stack or a stack folder
// goes to the engine that has it. A host with several engines gets --engine as well, when the interface knows
// the engine from what it read, so that the command reaches the very engine that was shown and a name that
// several engines have is no mistake. A host with one engine runs the commands unchanged.

// stackRef names a stack, or a stack folder, within its engine. Names and folder IDs are only unique inside one
// engine, so the interface never matches an entry by name alone.
type stackRef struct{ engine, name string }

// primary is control.Primary within the engine of stacks[i].
func primary(stacks []control.StackStatus, i int) bool {
	var same []control.StackStatus
	at := 0
	for j, s := range stacks {
		if s.Engine != stacks[i].Engine {
			continue
		}
		if j == i {
			at = len(same)
		}
		same = append(same, s)
	}
	return control.Primary(same, at)
}

// inEngine reports whether an entry of engine is the one of want; an empty want, as a backend over one service
// gives, matches every entry.
func inEngine(engine, want string) bool { return want == "" || engine == want }

// engines lists the engines the interface shows, in order: the deploy services the last read named, then those
// the stacks, folders and runs came from, then those that could not be asked.
func (m *Model) engines() []string {
	var out []string
	add := func(e string) {
		if e != "" && !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	for _, e := range m.snap.engines {
		add(e)
	}
	for _, s := range m.snap.stacks.Stacks {
		add(s.Engine)
	}
	for _, src := range m.snap.status.Engine.Sources {
		add(src.Engine)
	}
	for _, s := range m.snap.status.Engine.Stacks {
		add(s.Engine)
	}
	for _, n := range m.notChecked() {
		add(n.Engine)
	}
	return out
}

// multiEngine reports whether the interface shows more than one engine.
func (m *Model) multiEngine() bool { return len(m.engines()) > 1 }

// notChecked lists the engines whose deploy service could not be asked, once each, with the first reason.
func (m *Model) notChecked() []control.EngineNotChecked {
	var out []control.EngineNotChecked
	for _, list := range [][]control.EngineNotChecked{m.snap.status.NotChecked, m.snap.stacks.NotChecked,
		m.snap.runs.NotChecked, m.snap.check.NotChecked, m.snap.updates.NotChecked} {
		for _, n := range list {
			if !slices.ContainsFunc(out, func(o control.EngineNotChecked) bool { return o.Engine == n.Engine }) {
				out = append(out, n)
			}
		}
	}
	return out
}

// engineCtx is the context of a call about an entry of engine: a routing backend sends it to that engine's
// service and nowhere else.
func (m *Model) engineCtx(engine string) context.Context {
	if engine == "" {
		return m.ctx
	}
	return WithEngine(m.ctx, engine)
}

// engineArgs adds --engine to the arguments of a command handed to sudo when the interface shows several engines,
// so that the command reaches the very engine of the entry the person chose.
func (m *Model) engineArgs(engine string, args []string) []string {
	if !m.multiEngine() {
		return args
	}
	return withEngineFlag(args, engine)
}

// withEngineFlag puts --engine <engine> before the end of the flags, which "--" marks, or at the end. Without an
// engine the arguments stay as they are.
func withEngineFlag(args []string, engine string) []string {
	if engine == "" {
		return args
	}
	at := slices.Index(args, "--")
	if at < 0 {
		at = len(args)
	}
	out := slices.Clone(args[:at])
	out = append(out, "--engine", engine)
	return append(out, args[at:]...)
}

// chosenEngine is the engine of the stack the detail shows, empty when none is shown.
func (m *Model) chosenEngine() string { return m.rows[m.chosen].engine }

// stackEngineOf is the engine of the stack name for a change started from the detail or a form of it: the
// engine of the stack shown, else the engine of the one stack of that name. When several engines have the name
// it is empty, so that the backend refuses instead of guessing.
func (m *Model) stackEngineOf(name string) string {
	if r, ok := m.rows[m.chosen]; ok && r.name == name {
		return r.engine
	}
	engine, n := "", 0
	for _, r := range m.rows {
		if r.name == name {
			engine, n = r.engine, n+1
		}
	}
	if n != 1 {
		return ""
	}
	return engine
}

// folderEngineOf is the engine that has the stack folder at path bound. A folder is bound in one engine only;
// without exactly one match the engine stays empty.
func (m *Model) folderEngineOf(path string) string {
	engine, n := "", 0
	for _, src := range m.snap.status.Engine.Sources {
		if src.Path == path {
			engine, n = src.Engine, n+1
		}
	}
	if n != 1 {
		return ""
	}
	return engine
}
