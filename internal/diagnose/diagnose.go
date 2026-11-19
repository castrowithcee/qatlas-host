// Package diagnose assembles the diagnostic package of a host: the facts a person needs to investigate a
// failure, taken from read-only commands of the service channel and written as one archive. It holds nothing
// the service does not already answer with its secrets masked, and it never reads a file of the service on its
// own. Names that come from answers of the service never become names in the archive.
package diagnose

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/userfile"
)

// The bounds of the package.
const (
	// MaxRuns is how many of the newest runs are listed.
	MaxRuns = 20
	// RecentRunLogs is how many of the newest runs get their log even when they succeeded; the log of every
	// failed run among MaxRuns is added.
	RecentRunLogs = 5
	// MaxRunLogBytes bounds one run log. A longer log keeps its end, where a failure shows.
	MaxRunLogBytes = 256 << 10
	// ServiceLogWindow is how far back the service log reaches.
	ServiceLogWindow = 24 * time.Hour
	// MaxServiceLogEntries bounds the service log part; the newest entries stay.
	MaxServiceLogEntries = control.ServiceLogMaxTail
	// maxLogPages stops the paging through the service log, whatever the service answers.
	maxLogPages = 40
)

// The parts of the package, in archive order. Each part is a text file and a JSON file of this name.
const (
	PartManifest   = "manifest"
	PartVersion    = "version"
	PartServices   = "services"
	PartSources    = "sources"
	PartStacks     = "stacks"
	PartCheck      = "check"
	PartRuns       = "runs"
	PartRunLogs    = "run-logs"
	PartServiceLog = "service-log"
	PartContainers = "containers"
)

// Parts lists the part names in archive order.
var Parts = []string{PartManifest, PartVersion, PartServices, PartSources, PartStacks, PartCheck, PartRuns,
	PartRunLogs, PartServiceLog, PartContainers}

// Call sends one read-only command to the service and decodes its answer into result.
type Call func(ctx context.Context, command string, args, result any) error

// Options say how a package is built. Call is required.
type Options struct {
	Call Call
	// Version is the version of this qhost binary.
	Version string
	// Now returns the current time; the zero value uses the clock.
	Now func() time.Time
	// Hostname returns the name of the host; the zero value asks the system.
	Hostname func() (string, error)
}

// Part is one part of the package. Err says why the part is missing or incomplete; whatever could be
// collected stays in Data and Text.
type Part struct {
	Name string
	Err  string
	Data any
	Text string
}

// Package is a collected diagnostic package.
type Package struct {
	Created  time.Time
	Hostname string
	Parts    []Part
}

// Build collects every part. A part that cannot be collected carries its reason, and the others are still
// built; Build itself does not fail.
func Build(ctx context.Context, o Options) *Package {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Hostname == nil {
		o.Hostname = os.Hostname
	}
	c := &collector{o: o, ctx: ctx}
	pkg := &Package{Created: o.Now().UTC()}
	host, err := o.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	pkg.Hostname = host
	c.pkg = pkg

	status, statusErr := c.status()
	pkg.Parts = append(pkg.Parts,
		c.version(host),
		c.services(status, statusErr),
		c.sources(),
		c.stacks(status, statusErr),
		c.check(),
	)
	runs, runsPart := c.runs()
	pkg.Parts = append(pkg.Parts, runsPart, c.runLogs(runs, runsPart.Err), c.serviceLog(), c.containers())
	pkg.Parts = append([]Part{c.manifest(pkg.Parts)}, pkg.Parts...)
	return pkg
}

// FileName is the name the archive gets in the folder of the person.
func (p *Package) FileName() string {
	return userfile.Name("diagnose", p.Hostname, p.Created.Local(), ".tar.gz")
}

// Complete reports whether every part was collected without a problem.
func (p *Package) Complete() bool {
	return !slices.ContainsFunc(p.Parts, func(x Part) bool { return x.Err != "" })
}

// Save writes the archive as a new file. Without out it goes to the folder qhost in the home of the person who
// operates the process, under FileName; otherwise out is the absolute path of the file. The file is new, is
// never a replacement or a followed link, has mode 0600 and belongs to that person. It returns the full path.
func (p *Package) Save(env userfile.Env, out string) (string, error) {
	who, err := userfile.Operator(env)
	if err != nil {
		return "", err
	}
	if out == "" {
		return who.SaveWith(p.FileName(), p.WriteArchive)
	}
	return who.SavePathWith(out, p.WriteArchive)
}

// WriteArchive writes the package as a gzip-compressed tar archive with a text file and a JSON file for every
// part. The file names are fixed; nothing of the collected data forms a name.
func (p *Package) WriteArchive(w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	add := func(name string, data []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), ModTime: p.Created,
			Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
	for _, part := range p.Parts {
		if !slices.Contains(Parts, part.Name) {
			return fmt.Errorf("unknown part %q", part.Name)
		}
		doc := struct {
			Part  string `json:"part"`
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
			Data  any    `json:"data"`
		}{part.Name, part.Err == "", part.Err, part.Data}
		raw, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("encode part %s: %w", part.Name, err)
		}
		text := "== " + part.Name + " ==\n"
		if part.Err != "" {
			text += "PROBLEM: " + part.Err + "\n"
		}
		text += part.Text
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		if err := add(part.Name+".txt", []byte(text)); err != nil {
			return err
		}
		if err := add(part.Name+".json", append(raw, '\n')); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

type collector struct {
	o   Options
	ctx context.Context
	pkg *Package
}

func (c *collector) call(command string, args, result any) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	return c.o.Call(c.ctx, command, args, result)
}

func reason(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}

func (c *collector) status() (*control.StatusResult, error) {
	var st control.StatusResult
	if err := c.call(control.StatusCommand, nil, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func kitVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, d := range info.Deps {
		if d.Path == "github.com/castrowithcee/qatlas-kit" {
			if d.Replace != nil && d.Replace.Version != "" {
				return d.Replace.Version
			}
			return d.Version
		}
	}
	return "unknown"
}

func (c *collector) version(host string) Part {
	d := map[string]any{
		"qhost": c.o.Version, "kit": kitVersion(), "go": runtime.Version(),
		"os": runtime.GOOS, "arch": runtime.GOARCH, "hostname": host,
	}
	text := fmt.Sprintf("qhost %s\nkit %s\ngo %s\nos %s\narch %s\nhostname %s\n",
		c.o.Version, kitVersion(), runtime.Version(), runtime.GOOS, runtime.GOARCH, host)
	return Part{Name: PartVersion, Data: d, Text: text}
}

// serviceState is one service in the services part; today qhost is the only one.
type serviceState struct {
	Name      string                `json:"name"`
	Reachable bool                  `json:"reachable"`
	Error     string                `json:"error,omitempty"`
	Version   string                `json:"version,omitempty"`
	StartedAt time.Time             `json:"started_at,omitzero"`
	Window    *control.WindowResult `json:"window,omitempty"`
}

func (c *collector) services(st *control.StatusResult, err error) Part {
	s := serviceState{Name: control.ServiceLogSourceHost}
	p := Part{Name: PartServices}
	if err != nil {
		s.Error = reason(err)
		p.Err = "the service qhost is not reachable: " + s.Error
		p.Text = "qhost: not reachable (" + s.Error + ")\n"
	} else {
		s.Reachable, s.Version, s.StartedAt, s.Window = true, st.Version, st.StartedAt, st.Window
		p.Text = fmt.Sprintf("qhost: running, version %s, started %s\n", st.Version, st.StartedAt.UTC().Format(time.RFC3339))
		if w := st.Window; w != nil {
			p.Text += "maintenance window: " + jsonLine(w) + "\n"
		}
	}
	p.Data = []serviceState{s}
	return p
}

func jsonLine(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "?"
	}
	return string(raw)
}

func (c *collector) sources() Part {
	var res control.SourceListResult
	p := Part{Name: PartSources}
	if err := c.call(control.SourceListCommand, nil, &res); err != nil {
		p.Err = reason(err)
		p.Data = []control.SourceStatus{}
		return p
	}
	p.Data = nonNil(res.Sources)
	var b strings.Builder
	for _, s := range res.Sources {
		fmt.Fprintf(&b, "%s  %s  %s", s.ID, s.Kind, s.Path)
		if s.URL != "" {
			b.WriteString("  " + s.URL)
		}
		fmt.Fprintf(&b, "\n  last poll: %s  head: %s  remote head: %s\n", timeText(s.LastPoll), orDash(s.Head), orDash(s.RemoteHead))
		if s.PollError != "" {
			b.WriteString("  poll error: " + s.PollError + "\n")
		}
		if s.Note != "" {
			b.WriteString("  note: " + s.Note + "\n")
		}
	}
	if len(res.Sources) == 0 {
		b.WriteString("no stack folder is bound\n")
	}
	p.Text = b.String()
	return p
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func timeText(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.UTC().Format(time.RFC3339)
}

func (c *collector) stacks(st *control.StatusResult, statusErr error) Part {
	var list control.StacksResult
	listErr := c.call(control.StacksListCommand, nil, &list)
	p := Part{Name: PartStacks}
	data := struct {
		List  []control.StackInfo   `json:"list"`
		State []control.StackStatus `json:"state"`
	}{List: nonNil(list.Stacks), State: []control.StackStatus{}}
	var problems []string
	if listErr != nil {
		problems = append(problems, "stack list: "+reason(listErr))
	}
	if statusErr != nil {
		problems = append(problems, "stack state: "+reason(statusErr))
	} else {
		data.State = nonNil(st.Engine.Stacks)
	}
	p.Err = strings.Join(problems, "; ")
	p.Data = data
	var b strings.Builder
	for _, s := range data.List {
		fmt.Fprintf(&b, "%s (source %s)  update mode: %s\n", s.Name, orDash(s.Source), orDash(s.Update))
		for _, im := range s.Images {
			fmt.Fprintf(&b, "  %s  %s  %s=%s\n", im.Service, im.Image, im.Var, orDash(im.Version))
		}
		if s.Error != "" {
			b.WriteString("  error: " + s.Error + "\n")
		}
		if s.Collision != "" {
			b.WriteString("  not deployed: " + s.Collision + "\n")
		}
	}
	for _, s := range data.State {
		fmt.Fprintf(&b, "state %s (source %s): deployed %s, pending %s, last run %s", s.Name, orDash(s.Source),
			orDash(s.Deployed), orDash(s.Pending), orDash(s.LastRun))
		if s.ActiveRun != "" {
			b.WriteString(", active run " + s.ActiveRun)
		}
		b.WriteString("\n")
		for _, f := range s.Findings {
			fmt.Fprintf(&b, "  finding %s %s\n", f.Kind, f.Detail)
		}
		if s.Paused != nil {
			b.WriteString("  paused after a failed rollback\n")
		}
		if s.Stopped != nil {
			b.WriteString("  stopped\n")
		}
		if s.Failure != nil {
			fmt.Fprintf(&b, "  container failure: %s %s\n", s.Failure.Kind, s.Failure.Container)
		}
	}
	p.Text = b.String()
	return p
}

func (c *collector) check() Part {
	var res control.StacksCheckResult
	p := Part{Name: PartCheck}
	if err := c.call(control.StacksCheckCommand, nil, &res); err != nil {
		p.Err = reason(err)
		p.Data = control.StacksCheckResult{Stacks: []control.StackReport{}}
		return p
	}
	res.Stacks = nonNil(res.Stacks)
	p.Data = res
	var b strings.Builder
	fmt.Fprintf(&b, "%d errors, %d warnings\n", res.Errors, res.Warnings)
	for _, s := range res.Stacks {
		fmt.Fprintf(&b, "\n%s (source %s)\n", s.Stack, orDash(s.Source))
		for _, f := range s.Functions {
			state := "works"
			if !f.Works {
				state = "does not work"
			}
			fmt.Fprintf(&b, "  %s: %s  %s\n", f.Function, state, f.Reason)
			if f.Action != "" {
				b.WriteString("    to do: " + f.Action + "\n")
			}
		}
		for _, f := range s.Findings {
			fmt.Fprintf(&b, "  [%s] %s: %s\n    fix: %s\n", f.Severity, f.Rule, f.Cause, f.Fix)
		}
	}
	p.Text = b.String()
	return p
}

func (c *collector) runs() ([]control.RunInfo, Part) {
	var res control.RunsResult
	p := Part{Name: PartRuns}
	if err := c.call(control.RunsCommand, control.RunsArgs{}, &res); err != nil {
		p.Err = reason(err)
		p.Data = []control.RunInfo{}
		return nil, p
	}
	runs := res.Runs
	if len(runs) > MaxRuns {
		runs = runs[:MaxRuns]
	}
	p.Data = nonNil(runs)
	var b strings.Builder
	for _, r := range runs {
		fmt.Fprintf(&b, "%s  %s  %s  %s  %s  %dms  %s\n", r.ID, r.Started.UTC().Format(time.RFC3339), r.Stack,
			orDash(r.Commit), r.Trigger, r.DurationMS, orDash(r.Result))
		if r.Error != "" {
			b.WriteString("  error: " + r.Error + "\n")
		}
	}
	if len(runs) == 0 {
		b.WriteString("no run recorded\n")
	}
	p.Text = b.String()
	return runs, p
}

// failedRun says whether a recorded result is a failure; a run still in progress has none yet.
func failedRun(r control.RunInfo) bool {
	switch r.Result {
	case "failed", "rolled-back", "rollback-failed":
		return true
	}
	return false
}

// runLog is one run log in the run-logs part.
type runLog struct {
	Run       string `json:"run"`
	Stack     string `json:"stack"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Text      string `json:"text"`
}

func (c *collector) runLogs(runs []control.RunInfo, runsErr string) Part {
	p := Part{Name: PartRunLogs}
	logs := []runLog{}
	var b strings.Builder
	var failed int
	for i, r := range runs {
		if i >= RecentRunLogs && !failedRun(r) {
			continue
		}
		l := runLog{Run: r.ID, Stack: r.Stack, Result: r.Result}
		var res control.LogResult
		if err := c.call(control.LogCommand, control.LogArgs{ID: r.ID}, &res); err != nil {
			l.Error = reason(err)
			failed++
		} else {
			l.Text, l.Truncated = tailBytes(res.Text, MaxRunLogBytes)
		}
		logs = append(logs, l)
		fmt.Fprintf(&b, "=== run %s, stack %s, result %s ===\n", l.Run, l.Stack, orDash(l.Result))
		switch {
		case l.Error != "":
			b.WriteString("the log could not be read: " + l.Error + "\n")
		case l.Truncated:
			fmt.Fprintf(&b, "(only the last %d bytes)\n", MaxRunLogBytes)
			fallthrough
		default:
			b.WriteString(l.Text)
			if !strings.HasSuffix(l.Text, "\n") {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	if len(logs) == 0 {
		b.WriteString("no run log\n")
	}
	if failed > 0 {
		p.Err = fmt.Sprintf("%d run logs could not be read", failed)
	}
	if runsErr != "" {
		p.Err = "the runs are not known: " + runsErr
	}
	p.Data, p.Text = logs, b.String()
	return p
}

// tailBytes keeps the last max bytes of s, starting at a character boundary.
func tailBytes(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	s = s[len(s)-max:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s, true
}

func (c *collector) serviceLog() Part {
	p := Part{Name: PartServiceLog}
	since := c.pkg.Created.Add(-ServiceLogWindow)
	args := control.ServiceLogArgs{Source: control.ServiceLogSourceHost, Since: since, Tail: MaxServiceLogEntries}
	entries := []control.ServiceLogEntry{}
	truncated := false
	var err error
	for page := 0; ; page++ {
		var res control.ServiceLogResult
		if err = c.call(control.ServiceLogCommand, args, &res); err != nil {
			break
		}
		entries = append(entries, res.Entries...)
		if !res.More || res.Cursor == "" {
			break
		}
		if page+1 >= maxLogPages || len(entries) >= MaxServiceLogEntries {
			truncated = true
			break
		}
		args = control.ServiceLogArgs{Source: args.Source, Cursor: res.Cursor}
	}
	if len(entries) > MaxServiceLogEntries {
		entries, truncated = entries[len(entries)-MaxServiceLogEntries:], true
	}
	if err != nil {
		p.Err = reason(err)
	}
	p.Data = struct {
		Since     time.Time                 `json:"since"`
		Truncated bool                      `json:"truncated,omitempty"`
		Entries   []control.ServiceLogEntry `json:"entries"`
	}{since, truncated, entries}
	var b strings.Builder
	if truncated {
		fmt.Fprintf(&b, "(more entries exist; at most %d are kept)\n", MaxServiceLogEntries)
	}
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %-5s %s", e.Time.UTC().Format("2006-01-02T15:04:05.000Z"), strings.ToUpper(e.Level), e.Message)
		for _, a := range e.Attrs {
			fmt.Fprintf(&b, " %s=%q", a.Key, a.Value)
		}
		b.WriteString("\n")
	}
	if len(entries) == 0 && err == nil {
		b.WriteString("no entry in the last 24 hours\n")
	}
	p.Text = b.String()
	return p
}

func (c *collector) containers() Part {
	var res control.ContainerListResult
	p := Part{Name: PartContainers}
	if err := c.call(control.ContainerListCommand, control.ContainerListArgs{}, &res); err != nil {
		p.Err = reason(err)
		p.Data = control.ContainerListResult{Groups: []control.ContainerGroup{}}
		return p
	}
	res.Groups = nonNil(res.Groups)
	p.Data = res
	var b strings.Builder
	for _, g := range res.Groups {
		fmt.Fprintf(&b, "%s %s  %s  restarts %d\n", g.Kind, g.Name, g.State, g.Restarts)
		for _, ct := range g.Containers {
			fmt.Fprintf(&b, "  %s  %s  %s (%s)  health %s  up %ds  restarts %d\n", ct.Name, ct.Image, ct.State,
				ct.EngineState, orDash(ct.Health), ct.UptimeSeconds, ct.Restarts)
		}
	}
	if len(res.Groups) == 0 {
		b.WriteString("no container\n")
	}
	p.Text = b.String()
	return p
}

func (c *collector) manifest(parts []Part) Part {
	type entry struct {
		Part  string `json:"part"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	list := make([]entry, 0, len(parts))
	var b strings.Builder
	fmt.Fprintf(&b, "qhost diagnostic package, created %s\n", c.pkg.Created.Format(time.RFC3339))
	b.WriteString("It holds the host name and paths. Secret values are masked by the service. Check it before you pass it on.\n\n")
	for _, p := range parts {
		list = append(list, entry{p.Name, p.Err == "", p.Err})
		if p.Err == "" {
			fmt.Fprintf(&b, "%s: ok\n", p.Name)
		} else {
			fmt.Fprintf(&b, "%s: PROBLEM: %s\n", p.Name, p.Err)
		}
	}
	return Part{Name: PartManifest, Text: b.String(), Data: struct {
		Created  time.Time `json:"created"`
		Hostname string    `json:"hostname"`
		Parts    []entry   `json:"parts"`
	}{c.pkg.Created, c.pkg.Hostname, list}}
}
