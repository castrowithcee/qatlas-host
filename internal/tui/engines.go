package tui

import "slices"

// The commands of the actions run through sudo find the deploy service on their own: a stack or a stack folder
// goes to the engine that has it. A host with several engines gets --engine as well, when the interface knows
// the engine from what it read, so that the command reaches the very engine that was shown and a name that
// several engines have is no mistake. A host with one engine runs the commands unchanged.

// engines lists the engines the last read showed, in order of appearance.
func (m *Model) engines() []string {
	var out []string
	add := func(e string) {
		if e != "" && !slices.Contains(out, e) {
			out = append(out, e)
		}
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
	return out
}

// stackEngine is the engine of a stack, or empty when the host shows one engine or the stack is not known.
func (m *Model) stackEngine(stack string) string {
	if len(m.engines()) < 2 {
		return ""
	}
	for _, s := range m.snap.stacks.Stacks {
		if s.Name == stack {
			return s.Engine
		}
	}
	return ""
}

// folderEngine is the engine that has the stack folder bound, or empty as for stackEngine.
func (m *Model) folderEngine(path string) string {
	if len(m.engines()) < 2 {
		return ""
	}
	for _, src := range m.snap.status.Engine.Sources {
		if src.Path == path {
			return src.Engine
		}
	}
	return ""
}

// stackArgs adds --engine to the arguments of a command about a stack when the engine is known.
func (m *Model) stackArgs(stack string, args []string) []string {
	return withEngineFlag(args, m.stackEngine(stack))
}

// folderArgs adds --engine to the arguments of a command about a stack folder when the engine is known.
func (m *Model) folderArgs(path string, args []string) []string {
	return withEngineFlag(args, m.folderEngine(path))
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
