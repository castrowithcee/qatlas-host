package tui

import (
	"strings"
	"time"
	"unicode/utf8"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/userfile"
)

// saveLogFile writes a new file for the person who operates the interface; tests replace it.
var saveLogFile = func(name string, data []byte) (string, error) {
	p, err := userfile.Operator(userfile.Env{})
	if err != nil {
		return "", err
	}
	return p.Save(name, data)
}

// heldLogView is a log view that also keeps the lines it holds as text, because the view itself cannot hand
// them out. It follows the view's own rules: lines outside the period are dropped on arrival, and the oldest
// go when the buffer is full.
type heldLogView struct {
	*kittui.LogView
	now  func() time.Time
	size int
	held []string
}

// Append hands the lines to the view and keeps those it keeps.
func (v *heldLogView) Append(lines ...kittui.LogLine) {
	v.LogView.Append(lines...)
	begin, end := v.Filter().Bounds(v.now())
	for _, in := range lines {
		if !in.Time.IsZero() && (!begin.IsZero() && in.Time.Before(begin) || !end.IsZero() && in.Time.After(end)) {
			continue
		}
		text := strings.TrimSuffix(strings.ReplaceAll(in.Text, "\r\n", "\n"), "\n")
		for _, part := range strings.Split(text, "\n") {
			v.held = append(v.held, heldLine(in, part))
		}
	}
	if len(v.held) > v.size {
		v.held = append([]string(nil), v.held[len(v.held)-v.size:]...)
	}
}

// Clear drops the lines of the view and the kept copy.
func (v *heldLogView) Clear() {
	v.LogView.Clear()
	v.held = nil
}

// text is the held lines, one per row, as the view shows them but without colour or wrapping.
func (v *heldLogView) text() string {
	if len(v.held) == 0 {
		return ""
	}
	return strings.Join(v.held, "\n") + "\n"
}

// heldLine is one line: its time, its source and its text, with control characters written as escapes.
func heldLine(in kittui.LogLine, part string) string {
	if utf8.RuneCountInString(part) > kittui.LogViewMaxLineRunes {
		part = string([]rune(part)[:kittui.LogViewMaxLineRunes])
	}
	var fields []string
	if !in.Time.IsZero() {
		fields = append(fields, in.Time.Format("2006-01-02 15:04:05"))
	}
	if in.Source != "" {
		fields = append(fields, kittui.Escape(in.Source))
	}
	return strings.Join(append(fields, kittui.Escape(part)), " ")
}

// saveLog writes the lines of the log view to a file and tells where it went. kind and subject make the name.
func (m *Model) saveLog(v *heldLogView, kind, subject string) {
	m.saveText(userfile.Name(kind, subject, m.now().In(m.loc), ".log"), v.text())
}

// saveText writes text to a new file called name and tells where it went.
func (m *Model) saveText(name, text string) {
	path, err := saveLogFile(name, []byte(text))
	if err != nil {
		m.notice = m.text.Lookup(kittui.TextWarning, m.text.Lookup(textLogSaveFailed, err.Error()))
		return
	}
	m.result = m.text.Lookup(textLogSaved, path)
}

// saveOpenLog saves the log the active area shows; ok is false when none is shown.
func (m *Model) saveOpenLog() (ok bool) {
	switch {
	case m.clogShown() && m.clog.service:
		m.saveLog(m.clog.view, "service", productName)
	case m.clogShown() && m.clog.stack != "":
		m.saveLog(m.clog.view, "container", m.clog.stack)
	case m.clogShown():
		m.saveLog(m.clog.view, "container", m.clog.name)
	case m.area == areaRuns && m.log.open && m.log.view != nil:
		m.saveLog(m.log.view, "run", m.log.row.id)
	default:
		return m.saveShownLog()
	}
	return true
}
