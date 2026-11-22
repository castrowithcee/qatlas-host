package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/datadir"
)

// The data directories of a stack that waits for them: the detail shows the finding, one key opens a form for
// owner and mode, a confirmation names what would be created, and only that confirmation creates the
// directories and starts a run of the stack.

const formDataDirs formKind = formSecret + 1

// findingDataDirMissing is the kind of the engine finding for missing data directories; the interface does not
// import the engine, so the name is repeated here.
const findingDataDirMissing = "data-dir-missing"

// findingContainerFailed is the kind of the engine finding for a container failure; stateOf reads the structured
// failure instead, and the finding is only kept out of the error count.
const findingContainerFailed = "container-failed"

// dataDirsResult is what the confirmed creation came to. A creation that failed has createErr and nothing else
// to say; a run that could not be started leaves the directories made.
type dataDirsResult struct {
	stack    string
	res      datadir.Result
	createEr error
	run      control.DeployResult
	deployEr error
}

// dataDirFinding is the data-dir-missing finding of a stack, if the service reports one.
func (m *Model) dataDirFinding(stack string) (control.FindingInfo, bool) {
	engine := m.stackEngineOf(stack)
	for i, st := range m.snap.status.Engine.Stacks {
		if st.Name != stack || !inEngine(st.Engine, engine) || !primary(m.snap.status.Engine.Stacks, i) {
			continue
		}
		for _, f := range st.Findings {
			if f.Kind == findingDataDirMissing && len(f.Dirs) > 0 {
				return f, true
			}
		}
	}
	return control.FindingInfo{}, false
}

// commonOwner is the owner every directory proposes, empty when they propose none or different ones.
func commonOwner(dirs []control.DirInfo) string {
	owner := ""
	for i, d := range dirs {
		if d.Owner == "" || (i > 0 && d.Owner != owner) {
			return ""
		}
		owner = d.Owner
	}
	return owner
}

func dirPaths(dirs []control.DirInfo) []string {
	paths := make([]string, len(dirs))
	for i, d := range dirs {
		paths[i] = d.Path
	}
	return paths
}

// dataDirSection is the finding in the detail: the paths with the proposed owners and the way to create them.
func (m *Model) dataDirSection(stack string, width int) []string {
	f, ok := m.dataDirFinding(stack)
	if !ok {
		return nil
	}
	t := m.text
	return m.framed(t.Lookup(textDetailDataDirs), "", width, 0, func(width int) []string {
		var lines []string
		for _, d := range f.Dirs {
			text := t.Lookup(textDataDirLineNoOwner, d.Path)
			if d.Owner != "" {
				text = t.Lookup(textDataDirLine, d.Path, d.Owner)
			}
			lines = append(lines, m.line(detailIndent, kittui.StyleWarn, text, width))
		}
		return append(lines, m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textDataDirsHint), width)...)
	})
}

// formDataDirLines are the paths a data directory form is about, shown above its fields.
func (m *Model) formDataDirLines(f *settingsForm, width int) []string {
	lines := m.wrapped(detailMargin, kittui.StyleHint, m.text.Lookup(textDataDirsIntro, f.stackDir), width)
	for _, d := range f.dirs {
		lines = append(lines, m.line(detailIndent, kittui.StyleText, d.Path, width))
	}
	return append(lines, "")
}

// openDataDirForm opens the owner and mode form for the missing directories of the detail's stack. Without
// the finding it only says so.
func (m *Model) openDataDirForm() tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || m.detailBusy() {
		return nil
	}
	finding, ok := m.dataDirFinding(stack)
	if !ok {
		m.warn(m.text.Lookup(textDataDirsNone))
		return nil
	}
	dir, ok := m.snap.stacks.StackDir(stack)
	if !ok {
		m.warn(m.text.Lookup(textDataDirsNoFolder))
		return nil
	}
	f := m.newSettingsForm(formDataDirs, textTitleDataDirs, []kittui.FormField{
		textField("owner", textFieldDataOwner, textHintDataOwner, commonOwner(finding.Dirs)),
		textField("mode", textFieldDataMode, textHintDataMode, fmt.Sprintf("%04o", datadir.DefaultMode)),
	})
	if f != nil {
		f.stack, f.dirs, f.stackDir = stack, finding.Dirs, dir
	}
	return m.openForm(f)
}

// submitDataDirs checks owner and mode like the command line does and, when they hold up, asks for the
// confirmation. Nothing is created before its yes.
func (m *Model) submitDataDirs(f *settingsForm, ownerText, modeText string) tea.Cmd {
	t := m.text
	var ps []kittui.FormProblem
	owner, err := datadir.ParseOwner(ownerText)
	if err != nil {
		ps = append(ps, m.bad("owner", textProblemDataOwner))
	}
	mode, err := datadir.ParseMode(modeText)
	if err != nil {
		ps = append(ps, m.bad("mode", textProblemDataMode))
	}
	if len(ps) > 0 {
		f.problems(ps...)
		return nil
	}
	stack, paths, stackDir := f.stack, dirPaths(f.dirs), f.stackDir
	details := []string{t.Lookup(textDataDirsFolder, stackDir), t.Lookup(textDataDirsOwner, owner),
		t.Lookup(textDataDirsMode, fmt.Sprintf("%04o", mode))}
	for _, p := range paths {
		details = append(details, "  "+p)
	}
	req := datadir.Request{StackDir: stackDir, Paths: paths, Owner: owner, Mode: mode}
	if m.euid() != 0 {
		m.askDataDirsSudo(stack, req, details)
		return nil
	}
	m.ask(textTitleDataDirs, t.Lookup(textAskDataDirs, stack), t.Lookup(textWhatDataDirs), details, false, func() tea.Cmd {
		return m.createDataDirs(stack, req)
	})
	return nil
}

// askDataDirsSudo asks to create the directories with `sudo qhost stack datadir`, with the owner and mode that
// the form gave. The command makes the directories and then starts a run of the stack, as the call as root does.
func (m *Model) askDataDirsSudo(stack string, req datadir.Request, details []string) {
	if m.settingsBusy() {
		m.warn(m.text.Lookup(textActionBusy))
		return
	}
	var gen int
	m.askSudo(sudoAsk{title: textTitleDataDirs, question: m.text.Lookup(textAskDataDirs, stack), consequence: m.text.Lookup(textWhatDataDirs),
		lead: dialogLines(details),
		args: m.engineArgs(m.stackEngineOf(stack), []string{"stack", "datadir", "--owner", req.Owner.String(), "--mode", fmt.Sprintf("%04o", req.Mode), "--yes",
			"--output", "json", "--", stack}),
		begin: func() { gen = m.beginSetting() },
		done: func(r sudoRun) tea.Cmd {
			msg := settingMsg{gen: gen, op: opDataDirs, name: stack}
			var v struct {
				Created  string `json:"created"`
				Parents  string `json:"parents"`
				Existing string `json:"existing"`
				Run      string `json:"run"`
				Commit   string `json:"commit"`
			}
			if !r.decode(&v) || v.Run == "" {
				msg.err = sudoProblem(r.problem)
				return m.applySetting(msg)
			}
			msg.res = dataDirsResult{stack: stack,
				res: datadir.Result{Created: splitList(v.Created), Parents: splitList(v.Parents), Existing: splitList(v.Existing)},
				run: control.DeployResult{ID: v.Run, Stack: stack, Commit: v.Commit}}
			return m.applySetting(msg)
		}})
}

// createDataDirs makes the confirmed directories and then starts a run of the stack, off the event loop.
func (m *Model) createDataDirs(stack string, req datadir.Request) tea.Cmd {
	mk, ctx := m.makeDirs, m.engineCtx(m.stackEngineOf(stack))
	return m.writeCall(opDataDirs, stack, func(b Backend) (any, error) {
		out := dataDirsResult{stack: stack}
		if out.res, out.createEr = mk(req); out.createEr != nil {
			return out, nil
		}
		out.run, out.deployEr = b.Deploy(ctx, stack)
		return out, nil
	})
}

// dataDirsDone shows what a confirmed creation came to; a creation that failed stays in the open form.
func (m *Model) dataDirsDone(out dataDirsResult, done func(text string, details ...string)) tea.Cmd {
	t := m.text
	var details []string
	add := func(id kittui.TextID, list []string) {
		if len(list) > 0 {
			details = append(details, t.Lookup(id, strings.Join(list, ", ")))
		}
	}
	add(textDataDirsCreated, out.res.Created)
	add(textDataDirsParents, out.res.Parents)
	add(textDataDirsExisting, out.res.Existing)
	if out.createEr != nil {
		if m.set.form != nil {
			m.set.form.problem(t.Lookup(textErrDataDirs, out.createEr.Error()), t.Lookup(textFormRetry))
		}
		return m.reloadNow()
	}
	switch {
	case out.deployEr != nil:
		details = append(details, t.Lookup(textDataDirsDeployFailed, m.serviceText(out.deployEr)))
	default:
		details = append(details, t.Lookup(textDoneDeploy, out.run.ID, short(out.run.Commit)))
	}
	done(t.Lookup(textDoneDataDirs, out.stack), details...)
	return m.reloadNow()
}
