package tui

import (
	"strings"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// shownDigest is how much of a digest or image ID the detail shows: "sha256:" and 12 hex digits.
const shownDigest = len("sha256:") + 12

// fingerprintsOf are the image fingerprints the status reports for a stack, none before its first run.
func (m *Model) fingerprintsOf(engine, stack string) []control.FingerprintInfo {
	for i, st := range m.snap.status.Engine.Stacks {
		if st.Name == stack && inEngine(st.Engine, engine) && primary(m.snap.status.Engine.Stacks, i) {
			return st.Images
		}
	}
	return nil
}

// imageLines shows, per service, the image and the shortened repository digest and image ID; a value the engine
// could not read shows as missing. The values come from Docker and are only shown.
func (m *Model) imageLines(images []control.FingerprintInfo, width int) []string {
	t := m.text
	cut := func(s string) string {
		if s == "" {
			return t.Lookup(textImageMissing)
		}
		return s[:min(len(s), shownDigest)]
	}
	var out []string
	for _, f := range images {
		out = append(out, m.line(detailIndent, kittui.StyleText, strings.TrimSpace(f.Service+"  "+f.Image), width))
		out = append(out, m.line(detailIndent+2, kittui.StyleHint,
			t.Lookup(textImageFacts, cut(f.RepoDigest), cut(f.ImageID)), width))
	}
	return out
}
