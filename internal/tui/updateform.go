package tui

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The update rules of a stack: a form in the detail with the mode, the ceiling, the major jump, the
// observation time and, per version variable, a tag filter and a changelog repository. It sets the update
// section only; the rest of the settings stays as the service holds it.

const formUpdate formKind = formFolderURL + 1

// The limits of the rules, as the service enforces them; the form checks them early so a refusal comes at the
// field. The interface cannot import the rules, so they are repeated here.
const (
	maxObserveMinutes   = 1440
	minIntervalSecs     = 30
	maxIntervalSecs     = 86400
	defaultIntervalSecs = 60
	defaultTrigger      = "window"
)

var (
	varNameRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
	repoOwnerRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
	repoNameRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	// fieldPathRe finds the field path the service names in a refusal, such as "update.tags.APP_VERSION:".
	fieldPathRe = regexp.MustCompile(`update\.(mode|ceiling|major|observe|trigger|interval|tags|changelog|promote_source)(?:\.([A-Za-z0-9_.-]+?))?:`)
)

// Defaults of a stack's rules when it has none, as the service applies them.
const (
	defaultMode    = "manual"
	defaultCeiling = "minor"
	defaultMajor   = "stepwise"
	defaultObserve = 60
)

// ruleTexts name the values of the three choices.
var ruleTexts = map[string]kittui.TextID{"automatic": textModeAutomatic, "manual": textModeManual,
	"patch": textCeilingPatch, "minor": textCeilingMinor, "major": textCeilingMajor,
	"stepwise": textMajorStepwise, "direct": textMajorDirect,
	"window": textTriggerWindow, "interval": textTriggerInterval, "webhook": textTriggerWebhook}

// validRepo reports whether s is a GitHub repository as owner/name.
func validRepo(s string) bool {
	owner, name, ok := strings.Cut(s, "/")
	return ok && repoOwnerRe.MatchString(owner) && repoNameRe.MatchString(name) && name != "." && name != ".."
}

func checkTag(m *Model, e listEntry) []kittui.FormProblem {
	var ps []kittui.FormProblem
	if !varNameRe.MatchString(e.key) {
		ps = append(ps, m.bad("key", textProblemVarName))
	}
	if _, err := regexp.Compile(e.value); err != nil {
		ps = append(ps, m.bad("value", textProblemRegex, err.Error()))
	}
	return ps
}

func checkChangelog(m *Model, e listEntry) []kittui.FormProblem {
	var ps []kittui.FormProblem
	if !varNameRe.MatchString(e.key) {
		ps = append(ps, m.bad("key", textProblemVarName))
	}
	if !validRepo(e.value) {
		ps = append(ps, m.bad("value", textProblemRepo))
	}
	return ps
}

// stackInfo is the listed stack of that name.
func (m *Model) stackInfo(stack string) (control.StackInfo, bool) {
	engine := m.stackEngineOf(stack)
	for _, s := range m.snap.stacks.Stacks {
		if s.Name == stack && inEngine(s.Engine, engine) {
			return s, true
		}
	}
	return control.StackInfo{}, false
}

// stackVars are the version variables of the images of a stack, each once, in the order of the images.
func (m *Model) stackVars(stack string) []string {
	info, _ := m.stackInfo(stack)
	var vars []string
	for _, img := range info.Images {
		if img.Var != "" && !slices.Contains(vars, img.Var) {
			vars = append(vars, img.Var)
		}
	}
	return vars
}

// sourceKind is the kind of the stack folder a stack comes from, empty when it is not known.
func (m *Model) sourceKind(stack string) string {
	info, ok := m.stackInfo(stack)
	if !ok {
		return ""
	}
	sources := m.snap.status.Engine.Sources
	for _, s := range sources {
		if s.ID == info.Source && inEngine(s.Engine, info.Engine) {
			return s.Kind
		}
	}
	if len(sources) == 1 {
		return sources[0].Kind
	}
	return ""
}

func entriesOf(m map[string]string) []listEntry {
	out := make([]listEntry, 0, len(m))
	for k, v := range m {
		out = append(out, listEntry{key: k, value: v})
	}
	return out
}

func mapOf(entries []listEntry) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		out[e.key] = e.value
	}
	return out
}

// choicesWith are the variables of the stack and the keys the rules name that the stack no longer has, so
// that such an entry can still be changed or seen.
func choicesWith(vars []string, entries []listEntry) []string {
	out := slices.Clone(vars)
	for _, e := range entries {
		if !slices.Contains(out, e.key) {
			out = append(out, e.key)
		}
	}
	slices.Sort(out)
	return out
}

// openUpdateForm reads the settings of the detail's stack; the form opens when they arrive.
func (m *Model) openUpdateForm() tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || m.detailBusy() {
		return nil
	}
	ctx := m.engineCtx(m.stackEngineOf(stack))
	return m.writeCall(opStackSettings, stack, func(b Backend) (any, error) { return b.StackSettings(ctx, stack) })
}

// updateFormFor builds the form from the stored rules, or from the defaults for a stack without any.
func (m *Model) updateFormFor(res control.StackSettingsResult) tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || stack != res.Stack || m.set.form != nil || m.modal != nil {
		return nil
	}
	u := control.StackUpdateSettings{Mode: defaultMode, Ceiling: defaultCeiling, Major: defaultMajor}
	observe := defaultObserve
	trigger, interval := defaultTrigger, defaultIntervalSecs
	if res.Settings != nil && res.Settings.Update != nil {
		cur := res.Settings.Update
		u.Mode, u.Ceiling, u.Major = cmpOr(cur.Mode, defaultMode), cmpOr(cur.Ceiling, defaultCeiling), cmpOr(cur.Major, defaultMajor)
		if cur.Observe != nil {
			observe = *cur.Observe
		}
		trigger = cmpOr(cur.Trigger, defaultTrigger)
		if cur.Interval != nil {
			interval = *cur.Interval
		}
		u.Tags, u.Changelog, u.PromoteSource = cur.Tags, cur.Changelog, cur.PromoteSource
	}
	tags, logs := entriesOf(u.Tags), entriesOf(u.Changelog)
	vars := m.stackVars(stack)
	tagEd := m.newListEditor(listEditorSpec{field: "tags", title: textTitleTags, pair: true, keyLabel: textColVariable, keyHint: textHintVariable,
		valueLabel: textColPattern, valueHint: textHintPattern, intro: textTagsIntro, empty: textTagsEmpty,
		choices: choicesWith(vars, tags), check: checkTag}, tags)
	logEd := m.newListEditor(listEditorSpec{field: "changelog", title: textTitleChangelog, pair: true, keyLabel: textColVariable, keyHint: textHintVariable,
		valueLabel: textColRepository, valueHint: textHintRepository, intro: textChangelogIntro, empty: textChangelogEmpty,
		choices: choicesWith(vars, logs), check: checkChangelog}, logs)
	if tagEd == nil || logEd == nil {
		return nil
	}
	pick := func(id string, label, hint kittui.TextID, cur string, cs ...kittui.Choice) kittui.FormField {
		return kittui.FormField{ID: kittui.FieldID(id), Kind: kittui.FieldChoice, Label: label, Hint: hint, Choices: cs,
			Value: kittui.FormValue{Choice: kittui.ChoiceID(cur)}}
	}
	list := func(ed *listEditor, label, hint kittui.TextID) kittui.FormField {
		return kittui.FormField{ID: ed.spec.field, Kind: kittui.FieldList, Label: label, Hint: hint, Value: kittui.FormValue{Text: ed.summary()}}
	}
	f := m.newSettingsForm(formUpdate, textTitleUpdate, []kittui.FormField{
		pick("mode", textFieldUpdateMode, textHintUpdateMode, u.Mode, choice("automatic", textModeAutomatic), choice("manual", textModeManual)),
		pick("ceiling", textFieldCeiling, textHintCeiling, u.Ceiling, choice("patch", textCeilingPatch), choice("minor", textCeilingMinor),
			choice("major", textCeilingMajor)),
		pick("major", textFieldMajor, textHintMajor, u.Major, choice("stepwise", textMajorStepwise), choice("direct", textMajorDirect)),
		textField("observe", textFieldObserve, textHintObserve, strconv.Itoa(observe)),
		pick("trigger", textFieldTrigger, textHintTrigger, trigger, choice("window", textTriggerWindow), choice("interval", textTriggerInterval),
			choice("webhook", textTriggerWebhook)),
		textField("interval", textFieldInterval, textHintInterval, strconv.Itoa(interval)),
		list(tagEd, textFieldTags, textHintTags),
		list(logEd, textFieldChangelog, textHintChangelog),
		textField("promote_source", textFieldPromoteSource, textHintPromoteSource, u.PromoteSource),
	})
	if f == nil {
		return nil
	}
	f.stack = stack
	f.lists = map[kittui.FieldID]*listEditor{"tags": tagEd, "changelog": logEd}
	return m.openForm(f)
}

func cmpOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// updateRules are the values a saved form came to.
type updateRules struct {
	mode, ceiling, major string
	promoteSource        string
	observe              int
	trigger              string
	interval             int
	tags, changelog      []listEntry
}

// submitUpdate checks the form like the service checks the rules and, when they hold up, asks for the
// confirmation. Nothing is sent before its yes.
func (m *Model) submitUpdate(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	r := updateRules{mode: string(v["mode"].Choice), ceiling: string(v["ceiling"].Choice), major: string(v["major"].Choice),
		trigger: string(v["trigger"].Choice), promoteSource: strings.TrimSpace(v["promote_source"].Text), tags: f.lists["tags"].entries, changelog: f.lists["changelog"].entries}
	var ps []kittui.FormProblem
	n, err := strconv.Atoi(strings.TrimSpace(v["observe"].Text))
	if err != nil || n < 0 || n > maxObserveMinutes {
		ps = append(ps, m.bad("observe", textProblemObserve, maxObserveMinutes))
	}
	r.observe = n
	ps = append(ps, m.checkPromoteSource(r.promoteSource, f.stack)...)
	if r.trigger == "interval" {
		iv, err := strconv.Atoi(strings.TrimSpace(v["interval"].Text))
		if err != nil || iv < minIntervalSecs || iv > maxIntervalSecs {
			ps = append(ps, m.bad("interval", textProblemInterval, minIntervalSecs, maxIntervalSecs))
		}
		r.interval = iv
	}
	for _, group := range []struct {
		field   kittui.FieldID
		entries []listEntry
		check   func(*Model, listEntry) []kittui.FormProblem
	}{{"tags", r.tags, checkTag}, {"changelog", r.changelog, checkChangelog}} {
		for _, e := range group.entries {
			for _, p := range group.check(m, e) {
				ps = append(ps, kittui.FormProblem{Field: group.field, Cause: e.key + ": " + p.Cause, NextStep: p.NextStep})
			}
		}
	}
	if len(ps) > 0 {
		f.problems(ps...)
		return nil
	}
	t := m.text
	stack := f.stack
	details := []string{t.Lookup(textDetailRuleModes, t.Lookup(ruleTexts[r.mode]), t.Lookup(ruleTexts[r.ceiling]), t.Lookup(ruleTexts[r.major])), t.Lookup(textDetailRuleObserve, observeText(t, r.observe)), t.Lookup(textDetailRuleTrigger, triggerText(t, r))}
	details = append(details, promoteDetails(t, r)...)
	for _, e := range r.tags {
		details = append(details, t.Lookup(textDetailRuleTag, e.key, e.value))
	}
	for _, e := range r.changelog {
		details = append(details, t.Lookup(textDetailRuleChangelog, e.key, e.value))
	}
	what := textWhatUpdateUnknown
	switch m.sourceKind(stack) {
	case control.SourceKindRemote:
		what = textWhatUpdateRemote
	case control.SourceKindLocal:
		what = textWhatUpdateLocal
	}
	m.askSettingsSet(textTitleUpdate, t.Lookup(textAskUpdateRules, stack), t.Lookup(what), details, stack, updateEdit(r))
	return nil
}

// triggerText is the trigger of the rules with the interval when it polls.
func triggerText(t kittui.Translator, r updateRules) string {
	if r.trigger == "interval" {
		return t.Lookup(textTriggerPolling, r.interval)
	}
	return t.Lookup(ruleTexts[r.trigger])
}

func observeText(t kittui.Translator, minutes int) string {
	if minutes == 0 {
		return t.Lookup(textObserveOff)
	}
	return t.Lookup(textObserveMinutes, minutes)
}

// updateEdit sets the update rules on the settings the service holds now, so that what the form does not
// show stays as it is.
func updateEdit(r updateRules) settingsEdit {
	return func(s *control.StackSettings) {
		u := &control.StackUpdateSettings{Mode: r.mode, Ceiling: r.ceiling, Major: r.major, Observe: &r.observe,
			Tags: mapOf(r.tags), Changelog: mapOf(r.changelog), PromoteSource: r.promoteSource}
		if r.trigger == "interval" {
			u.Trigger, u.Interval = r.trigger, &r.interval
		} else if r.trigger != defaultTrigger {
			u.Trigger = r.trigger
		}
		s.Update = u
	}
}

// updateProblems places a refusal of the service at the field its path names; ok is false for any other
// refusal, which stands above the form.
func (m *Model) updateProblems(err error) (kittui.FormProblem, bool) {
	var remote *control.Error
	if !errors.As(err, &remote) || remote.Code != control.CodeInvalid {
		return kittui.FormProblem{}, false
	}
	sub := fieldPathRe.FindStringSubmatch(remote.Message)
	if sub == nil {
		return kittui.FormProblem{}, false
	}
	return kittui.FormProblem{Field: kittui.FieldID(sub[1]), Cause: m.serviceText(err), NextStep: m.text.Lookup(textFormFix)}, true
}

// formUpdateLines is the line above the form.
func (m *Model) formUpdateLines(f *settingsForm, width int) []string {
	lines := m.wrapped(detailMargin, kittui.StyleHint, m.text.Lookup(textUpdateIntro, f.stack), width)
	return append(lines, "")
}

// checkPromoteSource refuses a source that is not the name of a stack or is the stack itself, as the service does.
func (m *Model) checkPromoteSource(source, stack string) []kittui.FormProblem {
	if source != "" && (!stackNameRe.MatchString(source) || source == stack) {
		return []kittui.FormProblem{m.bad("promote_source", textProblemPromoteSource)}
	}
	return nil
}

func promoteDetails(t kittui.Translator, r updateRules) []string {
	if r.promoteSource == "" {
		return nil
	}
	return []string{t.Lookup(textDetailRulePromote, r.promoteSource)}
}
