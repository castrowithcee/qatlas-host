package tui

import (
	"context"
	"strings"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// sourceRead is what a refresh found of the promote source of one stack: the source as its service answered
// it, or that no stack of that name exists (missing) or that it could not be read.
type sourceRead struct {
	src                 control.PromoteSource
	missing, unreadable bool
}

// readSources reads the promote source of every stack that names one, at the service that has it. A failed
// read only marks that source unreadable.
func readSources(ctx context.Context, b Backend, list control.StacksResult) map[stackRef]sourceRead {
	pb, ok := b.(PromoteBackend)
	if !ok {
		return nil
	}
	exists := map[string]bool{}
	for _, si := range list.Stacks {
		if si.Collision == "" {
			exists[si.Name] = true
		}
	}
	out := map[stackRef]sourceRead{}
	read := map[string]sourceRead{}
	for _, si := range list.Stacks {
		if si.PromoteSource == "" || si.Collision != "" {
			continue
		}
		r, done := read[si.PromoteSource]
		if !done {
			if !exists[si.PromoteSource] {
				r.missing = true
			} else if src, err := pb.PromoteSource(ctx, si.PromoteSource); err != nil {
				r.unreadable = true
			} else {
				r.src = src
			}
			read[si.PromoteSource] = r
		}
		out[stackRef{si.Engine, si.Name}] = r
	}
	return out
}

// promoteView is what a row shows of its promote source: the versions the proven source runs by the variable
// of the target, or the one reason none are shown.
type promoteView struct {
	name     string
	versions map[string]string
	note     kittui.TextID
}

func viewOf(si control.StackInfo, r sourceRead) promoteView {
	v := promoteView{name: si.PromoteSource}
	switch why := r.src.Unproven(); {
	case r.missing:
		v.note = textSourceMissing
	case r.unreadable:
		v.note = textSourceUnreadable
	case why != "":
		v.note = textSourceUnproven
	default:
		v.versions = map[string]string{}
		for _, pv := range r.src.VersionsFor(si.Images) {
			v.versions[pv.Variable] = pv.Version
		}
	}
	return v
}

// cell is what a row shows beside its own versions: the versions of the source, or the note why none are shown.
func (v promoteView) cell(t kittui.Translator) string {
	switch {
	case v.name == "":
		return t.Lookup(textNoValue)
	case v.note != "":
		return t.Lookup(v.note)
	}
	var out []string
	for _, ver := range v.versions {
		out = appendDistinct(out, ver)
	}
	if len(out) == 0 {
		return t.Lookup(textNoValue)
	}
	return strings.Join(out, ", ")
}

// sourceLines is the promote source of a stack in its detail: the stack it names and, for each image version
// variable the source runs too, the version it runs, or why none is shown.
func (m *Model) sourceLines(r stackRow, width int) []string {
	t, v := m.text, r.promote
	if v.name == "" {
		return nil
	}
	out := []string{m.line(detailIndent, kittui.StyleText, t.Lookup(textDetailSource, v.name), width)}
	if v.note != "" {
		tone := kittui.StyleWarn
		if v.note == textSourceMissing {
			tone = kittui.StyleFail
		}
		return append(out, m.line(detailIndent+2, tone, t.Lookup(v.note), width))
	}
	items, _ := m.versionItems(r.engine, r.name)
	for _, it := range items {
		if ver, ok := v.versions[it.variable]; ok {
			out = append(out, m.pair(detailIndent+2, it.variable, ver, kittui.StyleOK, width))
		}
	}
	return out
}

// setRules takes the update rules of a listed stack, with its promote source as the refresh read it.
func (r *stackRow) setRules(si control.StackInfo, read sourceRead) {
	r.mode, r.trigger, r.interval, r.promoteSource = si.Update, si.Trigger, si.Interval, si.PromoteSource
	if si.PromoteSource != "" {
		r.promote = viewOf(si, read)
	}
}

// versionAndSourceLines is the version section of the detail: the versions of the stack and, below, those of
// its promote source.
func (m *Model) versionAndSourceLines(r stackRow, width int) []string {
	return append(m.versionLines(r.engine, r.name, width), m.sourceLines(r, width)...)
}

// currentCell is the versions of a row, followed by those of its promote source when it has one.
func (r stackRow) currentCell(t kittui.Translator) string {
	current := t.Lookup(textNoValue)
	if len(r.current) > 0 {
		current = strings.Join(r.current, ", ")
	}
	if r.promote.name != "" {
		current += " " + t.Lookup(textCellSource, r.promote.cell(t))
	}
	return current
}
