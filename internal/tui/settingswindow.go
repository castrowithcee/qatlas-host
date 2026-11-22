package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The window page lists every deploy service with the window that applies to it and where that window comes from:
// the host window, which every service without an own one follows, or an own window. Changing the host window
// reaches no service; an own window is set and reset on the instance the person chose with the instance key.
// Every change is a root action like the other settings, so it runs as the command `window set` or `window reset`.

// windowRead is what one deploy service answered to the read of its window.
type windowRead struct {
	engine string
	res    control.WindowResult
	err    error
}

// windowList is the answer of the read of the window page.
type windowList []windowRead

// readWindows asks each deploy service for its window; an empty engine asks the one service there is.
func readWindows(ctx context.Context, b Backend, engines []string) windowList {
	out := make(windowList, len(engines))
	for i, e := range engines {
		c := ctx
		if e != "" {
			c = WithEngine(ctx, e)
		}
		res, err := b.Window(c)
		out[i] = windowRead{engine: e, res: res, err: err}
	}
	return out
}

// windowEngines are the deploy services the page shows: one entry with an empty engine when none is known.
func (m *Model) windowEngines() []string {
	if e := m.engines(); len(e) > 0 {
		return e
	}
	return []string{""}
}

// windowTarget is the deploy service the own window actions concern: the chosen instance, else the only one, else
// the system engine. The commands name it explicitly, so they never fall back to the host window.
func (m *Model) windowTarget() string {
	if e := m.settingsEngine(); e != "" {
		return e
	}
	if e := m.engines(); len(e) == 1 {
		return e[0]
	}
	return control.EngineSystem
}

// windowOf is what the page read for an engine.
func (m *Model) windowOf(engine string) (windowRead, bool) {
	for _, w := range m.set.windows {
		if w.engine == engine || w.engine == "" {
			return w, true
		}
	}
	return windowRead{}, false
}

// hostWindowBase is the window the host window form starts from: what a service that follows the host shows; zero
// while none does, so that every part the person fills counts as a change.
func (m *Model) hostWindowBase() control.WindowResult {
	for _, w := range m.set.windows {
		if w.err == nil && w.res.Origin == control.WindowOriginHost {
			return w.res
		}
	}
	return control.WindowResult{}
}

func (m *Model) originText(origin string) string {
	if origin == control.WindowOriginOwn {
		return m.text.Lookup(textWindowOriginOwn)
	}
	return m.text.Lookup(textWindowOriginHost)
}

func (m *Model) windowLines(width, height int) []string {
	s, t := m.set, m.text
	if !s.window.loaded || s.window.err != nil {
		return m.partLines(s.window, width)
	}
	lines := m.instanceLines(width)
	multi := len(s.windows) > 1
	for _, w := range s.windows {
		title := t.Lookup(textPageWindow)
		if multi {
			title = w.engine
		}
		if w.err != nil {
			lines = append(lines, m.framed(title, "", width, 0, func(width int) []string {
				return m.wrapped(detailIndent, kittui.StyleFail, m.errorText(w.err), width)
			})...)
			continue
		}
		lines = append(lines, m.framed(title, m.originText(w.res.Origin), width, 0, func(width int) []string { return m.windowBody(w.res, width) })...)
	}
	return lines
}

func (m *Model) windowBody(w control.WindowResult, width int) []string {
	t := m.text
	state := t.Lookup(textHeaderWindowShut)
	if w.Open {
		state = t.Lookup(textHeaderWindowOpen)
	}
	lines := []string{
		m.line(detailIndent, kittui.StyleText, t.Lookup(textDetailDays, m.dayList(w.Days)), width),
		m.line(detailIndent, kittui.StyleText, strings.Join([]string{t.Lookup(textDetailStart, w.Start),
			t.Lookup(textDetailDuration, w.Duration), t.Lookup(textDetailZone, w.Zone)}, ", "), width),
		m.line(detailIndent, kittui.StyleText, state, width),
	}
	if !w.NextOpening.IsZero() {
		lines = append(lines, m.line(detailIndent, kittui.StyleText, t.Lookup(textDetailNext, m.stamp(w.NextOpening)), width))
	}
	if !w.LastRun.IsZero() {
		lines = append(lines, m.line(detailIndent, kittui.StyleText, t.Lookup(textDetailLastRun, m.stamp(w.LastRun)), width))
	}
	if w.Warning != "" {
		lines = append(lines, m.wrapped(detailIndent, kittui.StyleWarn, t.Lookup(textWindowWarning, kittui.Escape(w.Warning)), width)...)
	}
	return lines
}

// openWindowForm opens the form of the host window, or for an engine the form of its own window.
func (m *Model) openWindowForm(engine string) tea.Cmd {
	if engine != "" && m.needInstance() {
		return nil
	}
	base, title := m.hostWindowBase(), textTitleWindowHost
	if engine != "" {
		w, ok := m.windowOf(engine)
		if !ok || w.err != nil {
			m.warn(m.text.Lookup(textWindowNotRead))
			return nil
		}
		base, title = w.res, textTitleWindowOwn
	}
	f := m.newSettingsForm(formWindow, title, []kittui.FormField{
		textField("days", textFieldDays, textHintDays, strings.Join(base.Days, ",")),
		textField("start", textFieldStart, textHintStart, base.Start),
		textField("duration", textFieldDuration, textHintDuration, base.Duration),
		textField("zone", textFieldZone, textHintZone, base.Zone),
	})
	if f != nil {
		f.engine = engine
		if engine != "" {
			f.title, f.titleArg = title, engine
		}
	}
	return m.openForm(f)
}

func (m *Model) submitWindow(f *settingsForm, days, start, duration, zone string) tea.Cmd {
	cur := m.hostWindowBase()
	if f.engine != "" {
		if w, ok := m.windowOf(f.engine); ok {
			cur = w.res
		}
	}
	var args control.WindowSetArgs
	var details []string
	if want := splitList(days); len(want) > 0 && strings.Join(want, ",") != strings.Join(cur.Days, ",") {
		args.Days = want
		details = append(details, m.text.Lookup(textDetailDays, m.dayList(want)))
	}
	for _, part := range []struct {
		value, current string
		set            *string
		detail         kittui.TextID
	}{{start, cur.Start, &args.Start, textDetailStart}, {duration, cur.Duration, &args.Duration, textDetailDuration},
		{zone, cur.Zone, &args.Zone, textDetailZone}} {
		if part.value != "" && part.value != part.current {
			*part.set = part.value
			details = append(details, m.text.Lookup(part.detail, part.value))
		}
	}
	if len(details) == 0 {
		// An own window is also a change when it only repeats the window that applies: it then stops following the host.
		if f.engine == "" || cur.Origin == control.WindowOriginOwn {
			f.problem(m.text.Lookup(textProblemNoChange), m.text.Lookup(textFormFix))
			return nil
		}
		args.Days, args.Start, args.Duration, args.Zone = splitList(days), start, duration, zone
		details = append(details, m.text.Lookup(textDetailDays, m.dayList(args.Days)), m.text.Lookup(textDetailStart, start),
			m.text.Lookup(textDetailDuration, duration), m.text.Lookup(textDetailZone, zone))
	}
	argv := []string{"window", "set"}
	if len(args.Days) > 0 {
		argv = append(argv, "--days", strings.Join(args.Days, ","))
	}
	for _, flag := range []struct{ name, value string }{{"--start", args.Start}, {"--duration", args.Duration}, {"--zone", args.Zone}} {
		if flag.value != "" {
			argv = append(argv, flag.name, flag.value)
		}
	}
	argv = append(argv, "--output", "json")
	if f.engine == "" {
		m.askSetting(settingChange{title: textTitleWindowHost, question: m.text.Lookup(textAskWindowHost),
			consequence: m.text.Lookup(textWhatWindowHost), details: details, op: opWindowSet,
			direct: func(b Backend) (any, error) { return b.HostWindowSet(m.ctx, args) },
			args:   argv, decode: decodeWindow})
		return nil
	}
	engine, ctx := f.engine, m.engineCtx(f.engine)
	m.askSetting(settingChange{title: textTitleWindowOwnSet, question: m.text.Lookup(textAskWindowOwn, engine),
		consequence: m.text.Lookup(textWhatWindowOwn), details: details, op: opWindowSet, name: engine,
		direct: func(b Backend) (any, error) { return b.WindowSet(ctx, args) },
		args:   withEngineFlag(argv, engine), decode: decodeWindow})
	return nil
}

// askWindowReset asks to let the chosen instance follow the host window again.
func (m *Model) askWindowReset() tea.Cmd {
	engine := m.windowTarget()
	w, ok := m.windowOf(engine)
	if !ok || w.err != nil {
		m.warn(m.text.Lookup(textWindowNotRead))
		return nil
	}
	if w.res.Origin != control.WindowOriginOwn {
		m.warn(m.text.Lookup(textWindowNoOwn, engine))
		return nil
	}
	ctx := m.engineCtx(engine)
	m.askSetting(settingChange{title: textTitleWindowReset, question: m.text.Lookup(textAskWindowReset, engine),
		consequence: m.text.Lookup(textWhatWindowReset), op: opWindowSet, name: engine,
		direct: func(b Backend) (any, error) { return b.WindowReset(ctx) },
		args:   withEngineFlag([]string{"window", "reset", "--output", "json"}, engine), decode: decodeWindow})
	return nil
}
