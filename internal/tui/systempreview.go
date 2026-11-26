package tui

import (
	"path"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/highlight"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// filePreview is the file a place shows instead of the browser. note says why no text is shown: a
// binary or too large file, a link, no regular file, or a read that failed.
type filePreview struct {
	path    string
	reading bool
	note    string
}

// previewText is the text view of the preview while it shows a text; nil while it reads or shows a note.
func (v *fileView) previewText() *kittui.TextView {
	if v.preview == nil || v.preview.reading || v.preview.note != "" {
		return nil
	}
	return v.text
}

// previewCurrent reports whether gen is the preview shown now.
func (v *fileView) previewCurrent(gen int) bool {
	return v.preview != nil && gen == v.fileGen
}

// openFilePreview reads the file at path for the preview of the place.
func (m *Model) openFilePreview(p *fileView, path string) tea.Cmd {
	p.fileGen++
	p.preview = &filePreview{path: path, reading: true}
	return m.readHostPath(p, path, nil, p.fileGen)
}

// showPreview shows the file read for the preview gen; a text is coloured by the spans of the place, else by the
// format its name names.
func (m *Model) showPreview(p *fileView, gen int, r hostRead) {
	if !p.previewCurrent(gen) {
		return
	}
	t := m.text
	pv := p.preview
	pv.reading = false
	switch {
	case r.Folder:
		pv.note = t.Lookup(textFileIsFolder, pv.path)
	case r.Binary:
		pv.note = t.Lookup(textFileBinary, pv.path)
	case r.TooLarge:
		pv.note = t.Lookup(textFileTooLarge, pv.path, hostchange.MaxFileContent>>20)
	case r.Kind == hostchange.KindLink:
		pv.note = t.Lookup(textFileLink, pv.path, r.Target)
	case r.Kind != hostchange.KindFile:
		pv.note = t.Lookup(textFileOther, pv.path)
	}
	if pv.note != "" {
		return
	}
	if p.text == nil {
		v, err := kittui.NewTextView(kittui.TextViewSpec{Keys: textKeys(), MaxLines: kittui.TextViewMaxLines, Numbers: true},
			m.keys, m.text)
		if err != nil {
			pv.note = t.Lookup(kittui.TextError, err.Error())
			return
		}
		p.text = v
	}
	spans := p.rules.spans
	if spans == nil {
		spans = func(name, text string) [][]kittui.TextSpan { return highlight.Highlight(path.Base(name), text) }
	}
	p.text.SetText(r.Content, spans(pv.path, r.Content))
}

// previewView draws the preview of the place: the path, then the text or the note in its place.
func (m *Model) previewView(p *fileView, width, height int) string {
	pv := p.preview
	lines := []string{m.line(detailMargin, kittui.StyleActive, pv.path, width)}
	switch v := p.previewText(); {
	case pv.reading:
		lines = append(lines, m.line(detailMargin, kittui.StyleHint, m.text.Lookup(textFolderReading, pv.path), width))
	case v == nil:
		lines = append(append(lines, ""), m.wrapped(detailMargin, kittui.StyleWarn, pv.note, width)...)
	default:
		view, targets := v.View(m.styles, width, max(height-1, 1))
		m.parts = append(m.parts, mousePart{rect: kittui.Rect{Y: 1, Width: width, Height: max(height-1, 1)}, targets: targets,
			scroll: v.Wheel, click: func(res kittui.MouseResult) tea.Cmd { v.Click(res); return nil }})
		lines = append(lines, view)
	}
	out := strings.Split(strings.Join(lines, "\n"), "\n")
	return strings.Join(out[:min(len(out), height)], "\n")
}
