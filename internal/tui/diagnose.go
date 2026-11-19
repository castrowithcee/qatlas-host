package tui

import (
	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/diagnose"
	"github.com/castrowithcee/qatlas-host/internal/userfile"
)

// saveDiagnose writes the package as a new file for the person who operates the interface; tests replace it.
var saveDiagnose = func(pkg *diagnose.Package) (string, error) {
	return pkg.Save(userfile.Env{}, "")
}

// diagnoseMsg carries the outcome of collecting and writing one package.
type diagnoseMsg struct {
	path     string
	problems int
	err      error
}

// startDiagnose collects the package off the event loop. A second press while it runs only says so.
func (m *Model) startDiagnose() tea.Cmd {
	if m.diagnosing {
		m.result = m.text.Lookup(textDiagBusy)
		return nil
	}
	m.diagnosing = true
	ctx, backend := m.ctx, m.backend
	return func() tea.Msg {
		pkg := backend.Diagnose(ctx)
		path, err := saveDiagnose(pkg)
		problems := 0
		for _, p := range pkg.Parts {
			if p.Err != "" {
				problems++
			}
		}
		return diagnoseMsg{path: path, problems: problems, err: err}
	}
}

func (m *Model) applyDiagnose(msg diagnoseMsg) {
	m.diagnosing = false
	switch {
	case msg.err != nil:
		m.notice = m.text.Lookup(kittui.TextWarning, m.text.Lookup(textDiagFailed, msg.err.Error()))
	case msg.problems > 0:
		m.result = m.text.Lookup(textDiagPartial, msg.path, msg.problems)
	default:
		m.result = m.text.Lookup(textDiagSaved, msg.path)
	}
}
