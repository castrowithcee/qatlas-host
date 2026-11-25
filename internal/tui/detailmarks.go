package tui

import "github.com/castrowithcee/qatlas-host/internal/control"

// failedVersionsOf is the locked versions the status reports for a stack as text, empty when it reports none.
func (m *Model) failedVersionsOf(engine, stack string) string {
	for i, st := range m.snap.status.Engine.Stacks {
		if st.Name == stack && inEngine(st.Engine, engine) && primary(m.snap.status.Engine.Stacks, i) {
			return control.FailedVersionsText(st.FailedVersions)
		}
	}
	return ""
}

// observeOf is the running observation time the status reports for a stack, nil when it reports none.
func (m *Model) observeOf(engine, stack string) *control.ObserveInfo {
	for i, st := range m.snap.status.Engine.Stacks {
		if st.Name == stack && inEngine(st.Engine, engine) && primary(m.snap.status.Engine.Stacks, i) {
			return st.Observe
		}
	}
	return nil
}
