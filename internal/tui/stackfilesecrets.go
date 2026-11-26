package tui

import (
	"path"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/highlight"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// secretsFile is the file of the secrets of a stack in its folder; the interface does not import the stacks
// package, so the name is repeated here.
const secretsFile = "secrets.env"

// stackFileSpans colour the text of p by its format; in secrets.env every value is masked as well, which the
// preview uncovers only on its reveal key.
func stackFileSpans(p, text string) [][]kittui.TextSpan {
	spans := highlight.Highlight(path.Base(p), text)
	if path.Base(p) != secretsFile {
		return spans
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([][]kittui.TextSpan, len(lines))
	for i, line := range lines {
		var own []kittui.TextSpan
		if i < len(spans) {
			own = spans[i]
		}
		out[i] = maskedValue(line, own)
	}
	return out
}

// maskedValue adds to the spans of one line of secrets.env the masked part: what follows the = of a line that
// names a key, and all of any other line that is no comment, as the rest of a value over several lines may be.
func maskedValue(line string, spans []kittui.TextSpan) []kittui.TextSpan {
	lead := len(line) - len(strings.TrimLeft(line, " \t"))
	if lead == len(line) {
		return spans
	}
	from := 0
	for _, s := range spans {
		if s.Start == lead && s.Role == kittui.TextRoleComment {
			return spans
		}
		if s.Role == kittui.TextRoleKey {
			if eq := strings.IndexByte(line[s.End:], '='); eq >= 0 {
				from = s.End + eq + 1
			}
			break
		}
	}
	return append(spans, kittui.TextSpan{Start: from, End: len(line), Masked: true})
}

// secretsStack is the stack whose folder holds the secrets.env at p.
func (m *Model) secretsStack(p string) (name, engine string, ok bool) {
	dir := path.Dir(p)
	for _, s := range m.stacksOfFolder(m.sf.folder) {
		if path.Join(s.SourceDir, s.Folder) == dir {
			return s.Name, s.Engine, true
		}
	}
	return "", "", false
}

// offerSecretsDeploy says, once the result of the write is closed, that the new secrets take effect at the next
// deploy of their stack and offers to deploy it now.
func (m *Model) offerSecretsDeploy(p string) {
	t := m.text
	ask := func() {
		stack, engine, ok := m.secretsStack(p)
		if !ok {
			m.result = t.Lookup(textSecretsNextDeploy, p)
			return
		}
		m.askQuestion(kittui.DialogSpec{Title: textTitleSecretsSaved, Question: t.Lookup(textAskDeploySecrets, p, stack),
			Consequence: t.Lookup(textWhatDeploySecrets), Answers: []kittui.Answer{{Action: actYes, Label: textAnswerDeployNow},
				{Action: actNo, Label: textAnswerLater}}}, func() tea.Cmd { return m.deployNow(stack, engine) }, func() {})
	}
	md := m.modal
	if md == nil {
		ask()
		return
	}
	drop := md.drop
	md.drop = func() {
		if drop != nil {
			drop()
		}
		ask()
	}
}

// deployNow deploys the stack as the Deploy key of the list does: as root directly, else through a grant that
// covers it or through sudo, after the question that names the command.
func (m *Model) deployNow(stack, engine string) tea.Cmd {
	if m.busy() {
		m.warn(m.text.Lookup(textActionBusy))
		return nil
	}
	p := plan{kind: kindDeploy, stacks: []string{stack}, engines: []string{engine}}
	if m.euid() == 0 {
		return m.start(p)
	}
	if cmd, ok := m.coverPlan(p); ok {
		return cmd
	}
	m.askSudoPlan(p)
	return nil
}
