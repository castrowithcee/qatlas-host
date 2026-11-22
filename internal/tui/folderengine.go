package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// formFolderEngine chooses the Docker engine a folder is taken over into.
const formFolderEngine formKind = formFolderInclude + 1

// flowEngine is the engine a flow works in: the engine as given where the interface shows several, else none, so
// that a host with one engine runs everything unchanged.
func (m *Model) flowEngine(engine string) string {
	if m.multiEngine() {
		return engine
	}
	return ""
}

// engineOfFlow is the engine of the calls of a flow. A folder that gets a remote is bound already, and the engine
// that has it bound is the one; any other flow has the engine of its choice or its proposal.
func (m *Model) engineOfFlow(f *addFlow) string {
	if f.remote {
		return m.folderEngineOf(f.res.Path)
	}
	return f.engine
}

// offeredEngines are the engines a folder can be taken over into: those with a deploy service set up.
func (m *Model) offeredEngines() []string {
	return slices.Clone(m.snap.engines)
}

// engineContainers counts the containers of the folder of res by the engine they run in. A container the service
// reports without an engine runs in the engine of the service, which inspected the folder: the system engine.
func engineContainers(res control.SourceInspectResult) map[string]int {
	counts := map[string]int{}
	for _, st := range res.Stacks {
		for _, c := range st.Conflicts {
			counts[engineName(c.Engine)]++
		}
	}
	return counts
}

// suggestEngine is the engine to preselect: the one the most containers of the folder run in, the system engine
// on a tie and where none run, as long as it is offered; else the first engine offered.
func suggestEngine(res control.SourceInspectResult, offered []string) string {
	counts := engineContainers(res)
	best, most := "", 0
	for _, e := range offered {
		if n := counts[e]; n > most || (n == most && n > 0 && e == control.EngineSystem) {
			best, most = e, n
		}
	}
	switch {
	case best != "":
		return best
	case slices.Contains(offered, control.EngineSystem):
		return control.EngineSystem
	case len(offered) > 0:
		return offered[0]
	}
	return control.EngineSystem
}

// otherEngines are the engines other than the one of the flow in which containers of the folder run. A conflict
// without an engine is one of the engine that inspected the folder, which is the engine of the flow.
func otherEngines(flow *addFlow) []string {
	var out []string
	for _, st := range flow.res.Stacks {
		for _, c := range st.Conflicts {
			if c.Engine != "" && c.Engine != flow.engine && !slices.Contains(out, c.Engine) {
				out = append(out, c.Engine)
			}
		}
	}
	slices.Sort(out)
	return out
}

// engineLines are the engine the folder is taken over into, and the warning when containers of the folder run
// in another engine. The warning does not block anything.
func (m *Model) engineLines(flow *addFlow, width int) []string {
	if flow.engine == "" {
		return nil
	}
	t := m.text
	lines := m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textDetailEngine, flow.engine), width)
	if other := otherEngines(flow); len(other) > 0 {
		lines = append(lines, m.wrapped(detailIndent, kittui.StyleWarn, t.Lookup(textEngineDiffers, strings.Join(other, ", "), flow.engine, flow.engine), width)...)
	}
	return lines
}

// askEngine asks for the engine of a folder that was just inspected and reports whether it did. Nothing is asked
// for a rejection, for a proposal, which belongs to the engine it was submitted to, for an engine already chosen
// and on a host with one engine.
func (m *Model) askEngine(flow *addFlow) bool {
	offered := m.offeredEngines()
	if flow.foreign != nil {
		if _, bad := adoptRejection(*flow.foreign, &flow.res); bad {
			return false
		}
	}
	if flow.engineSet || flow.proposal != nil || flow.remote || flow.res.Case == control.SourceCaseReject || len(offered) < 2 {
		return false
	}
	flow.engine = suggestEngine(flow.res, offered)
	m.set.form = m.folderEngineForm(flow, offered)
	return m.set.form != nil
}

// folderEngineForm asks for the engine, with the one the containers of the folder run in preselected.
func (m *Model) folderEngineForm(flow *addFlow, offered []string) *settingsForm {
	t, counts := m.text, engineContainers(flow.res)
	field := kittui.FormField{ID: "engine", Kind: kittui.FieldChoice, Label: textFieldEngine,
		Hint: textHintEngine, Value: kittui.FormValue{Choice: kittui.ChoiceID(flow.engine)}}
	for _, e := range offered {
		label := kittui.Escape(e)
		if n := counts[e]; n > 0 {
			label = kittui.Escape(t.Lookup(textEngineRuns, e, n))
		}
		field.Choices = append(field.Choices, kittui.Choice{ID: kittui.ChoiceID(e), Label: kittui.TextID(label)})
	}
	if len(counts) > 0 {
		field.Hint = textHintEngineFound
	}
	return m.newSettingsForm(formFolderEngine, flow.title(), []kittui.FormField{field})
}

// submitFolderEngine takes the engine. The inspection was made for the system engine, so the folder is
// inspected again for any other: its deploy service, its service account and its containers decide what is shown.
func (m *Model) submitFolderEngine(v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	flow := m.set.add
	engine := string(v["engine"].Choice)
	if flow == nil || engine == "" {
		m.dropForm()
		return nil
	}
	flow.engine, flow.engineSet = engine, true
	if engine == control.EngineSystem {
		m.dropForm()
		m.showInspection(flow)
		return nil
	}
	return m.replan(flow, flow.include)
}
