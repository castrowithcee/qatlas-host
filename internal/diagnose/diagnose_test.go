package diagnose

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/userfile"
)

var testNow = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// fakeService answers the commands of the package from fixed values; a command in fail answers that error.
type fakeService struct {
	runs  []control.RunInfo
	logs  map[string]string
	fail  map[string]error
	calls []string
	// logPages makes the service log come in pages of this many entries; zero answers one page.
	logPages int
	entries  []control.ServiceLogEntry
}

func (f *fakeService) call(_ context.Context, command string, args, result any) error {
	f.calls = append(f.calls, command)
	if err := f.fail[command]; err != nil {
		return err
	}
	var v any
	switch command {
	case control.StatusCommand:
		v = control.StatusResult{Version: "9.9.9", StartedAt: testNow.Add(-time.Hour),
			Engine: control.EngineStatus{Stacks: []control.StackStatus{{Name: "demo", Source: "s1", Deployed: "abc123", Findings: []control.FindingInfo{}}}}}
	case control.SourceListCommand:
		v = control.SourceListResult{Sources: []control.SourceStatus{{ID: "s1", Path: "/srv/demo", Kind: control.SourceKindRemote,
			URL: "ssh://git.example/demo.git", PollError: "demo fetch failed"}}}
	case control.StacksListCommand:
		v = control.StacksResult{Dir: "/srv/demo", Stacks: []control.StackInfo{{Name: "demo", Update: "manual",
			Images: []control.ImageInfo{{Service: "web", Image: "demo/web", Var: "WEB_VERSION", Version: "1.2.3"}}}}}
	case control.StacksCheckCommand:
		v = control.StacksCheckResult{Stacks: []control.StackReport{{Stack: "demo", Source: "s1",
			Functions: []control.StackFunction{{Function: "deploy", Works: true, Reason: "ok"}}}}}
	case control.RunsCommand:
		v = control.RunsResult{Runs: f.runs}
	case control.LogCommand:
		id := args.(control.LogArgs).ID
		text, ok := f.logs[id]
		if !ok {
			return control.NewError(control.CodeNoSuchRun, "no such run")
		}
		v = control.LogResult{Text: text}
	case control.ServiceLogCommand:
		a := args.(control.ServiceLogArgs)
		all, page, cursor := f.entries, f.entries, 0
		if a.Tail > 0 && len(all) > a.Tail {
			all = all[len(all)-a.Tail:]
			f.entries, page = all, all
		}
		if a.Cursor != "" {
			fmt.Sscan(a.Cursor, &cursor)
			page = all[cursor:]
		}
		res := control.ServiceLogResult{Cursor: "end"}
		if f.logPages > 0 && len(page) > f.logPages {
			page = page[:f.logPages]
			res.More, res.Cursor = true, fmt.Sprint(cursor+f.logPages)
		}
		res.Entries = page
		v = res
	case control.ContainerListCommand:
		v = control.ContainerListResult{Groups: []control.ContainerGroup{{Kind: control.ContainerKindOwn, Name: "demo", State: "ok",
			Containers: []control.ContainerSummary{{Name: "demo-web-1", Image: "demo/web:1.2.3", State: "ok", EngineState: "running"}}}}}
	default:
		return fmt.Errorf("unexpected command %q", command)
	}
	raw, _ := json.Marshal(v)
	return json.Unmarshal(raw, result)
}

func build(f *fakeService) *Package {
	return Build(context.Background(), Options{Call: f.call, Version: "1.0.0", Now: func() time.Time { return testNow },
		Hostname: func() (string, error) { return "demo-host", nil }})
}

// unpack reads the archive into a map from file name to content.
func unpack(t *testing.T, p *Package) (names []string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	if err := p.WriteArchive(&buf); err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	files = map[string]string{}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return names, files
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg || h.Mode != 0o600 {
			t.Errorf("%s: type %c mode %o", h.Name, h.Typeflag, h.Mode)
		}
		b, _ := io.ReadAll(tr)
		names, files[h.Name] = append(names, h.Name), string(b)
	}
}

func runInfo(id, result string) control.RunInfo {
	return control.RunInfo{ID: id, Stack: "demo", Result: result, Started: testNow}
}

func TestArchiveHasATextAndAJSONFilePerPart(t *testing.T) {
	f := &fakeService{runs: []control.RunInfo{runInfo("r1", "succeeded")}, logs: map[string]string{"r1": "pulled\n"},
		entries: []control.ServiceLogEntry{{Time: testNow, Level: "info", Source: "qhost", Message: "service started"}}}
	pkg := build(f)
	names, files := unpack(t, pkg)
	var want []string
	for _, part := range Parts {
		want = append(want, part+".txt", part+".json")
	}
	if !slices.Equal(names, want) {
		t.Fatalf("files %v, want %v", names, want)
	}
	for _, name := range names {
		if strings.HasSuffix(name, ".json") {
			var doc struct {
				OK   bool `json:"ok"`
				Data any  `json:"data"`
			}
			if err := json.Unmarshal([]byte(files[name]), &doc); err != nil || !doc.OK || doc.Data == nil {
				t.Errorf("%s: %v %+v", name, err, doc)
			}
		} else if strings.TrimSpace(files[name]) == "" {
			t.Errorf("%s is empty", name)
		}
	}
	for file, snippet := range map[string]string{
		"version.txt": "qhost 1.0.0", "services.txt": "version 9.9.9", "sources.txt": "demo fetch failed",
		"stacks.txt": "WEB_VERSION=1.2.3", "check.txt": "deploy: works", "runs.txt": "r1", "run-logs.txt": "pulled",
		"service-log.txt": "service started", "containers.txt": "demo-web-1", "manifest.txt": "host name and paths",
	} {
		if !strings.Contains(files[file], snippet) {
			t.Errorf("%s lacks %q:\n%s", file, snippet, files[file])
		}
	}
	if !pkg.Complete() {
		t.Error("a package of answers only is incomplete")
	}
	if got := pkg.FileName(); !strings.HasPrefix(got, "qhost-diagnose-demo-host-") || !strings.HasSuffix(got, ".tar.gz") {
		t.Errorf("name %q", got)
	}
}

func TestUnreachableServiceStillGivesAPackageWithTheReason(t *testing.T) {
	down := errors.New("the qhost service is not running: dial unix /run/demo/control.sock: connection refused")
	fail := map[string]error{}
	for _, c := range []string{control.StatusCommand, control.SourceListCommand, control.StacksListCommand, control.StacksCheckCommand,
		control.RunsCommand, control.ServiceLogCommand, control.ContainerListCommand} {
		fail[c] = down
	}
	pkg := build(&fakeService{fail: fail})
	names, files := unpack(t, pkg)
	if len(names) != 2*len(Parts) {
		t.Fatalf("files %v", names)
	}
	if pkg.Complete() {
		t.Fatal("the package claims to be complete")
	}
	for _, part := range Parts {
		if part == PartVersion || part == PartManifest {
			continue
		}
		var doc struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(files[part+".json"]), &doc); err != nil {
			t.Fatal(err)
		}
		if doc.OK || !strings.Contains(doc.Error, "not running") {
			t.Errorf("%s: %+v", part, doc)
		}
		if !strings.Contains(files[part+".txt"], "PROBLEM:") {
			t.Errorf("%s.txt has no reason:\n%s", part, files[part+".txt"])
		}
	}
	if !strings.Contains(files["version.txt"], "qhost 1.0.0") {
		t.Errorf("version missing: %s", files["version.txt"])
	}
}

func TestOneFailingPartLeavesTheRest(t *testing.T) {
	f := &fakeService{fail: map[string]error{control.ContainerListCommand: control.NewError(control.CodeDockerUnreachable, "no engine")}}
	pkg := build(f)
	_, files := unpack(t, pkg)
	if !strings.Contains(files["containers.txt"], "no engine") || !strings.Contains(files["services.txt"], "running") {
		t.Fatalf("containers %q, services %q", files["containers.txt"], files["services.txt"])
	}
	if slices.Contains(f.calls, control.ContainerLogsCommand) {
		t.Fatal("container logs were asked for")
	}
}

func TestRunsAndLogsAreBounded(t *testing.T) {
	f := &fakeService{logs: map[string]string{}}
	for i := range 30 {
		result := "succeeded"
		if i == 2 || i == 7 || i == 19 || i == 25 { // 25 lies beyond the newest 20
			result = "failed"
		}
		id := fmt.Sprintf("r%02d", i)
		f.runs = append(f.runs, runInfo(id, result))
		f.logs[id] = "log of " + id + "\n"
	}
	big := strings.Repeat("x", MaxRunLogBytes) + "é-end"
	f.logs["r07"] = big
	_, files := unpack(t, build(f))
	var runs struct{ Data []control.RunInfo }
	if err := json.Unmarshal([]byte(files["runs.json"]), &runs); err != nil || len(runs.Data) != MaxRuns {
		t.Fatalf("%d runs, %v", len(runs.Data), err)
	}
	var logs struct{ Data []runLog }
	if err := json.Unmarshal([]byte(files["run-logs.json"]), &logs); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, l := range logs.Data {
		ids = append(ids, l.Run)
		if len(l.Text) > MaxRunLogBytes {
			t.Errorf("%s holds %d bytes", l.Run, len(l.Text))
		}
	}
	if want := []string{"r00", "r01", "r02", "r03", "r04", "r07", "r19"}; !slices.Equal(ids, want) {
		t.Fatalf("logs of %v, want %v", ids, want)
	}
	for _, l := range logs.Data {
		if l.Run == "r07" && (!l.Truncated || !strings.HasSuffix(l.Text, "é-end")) {
			t.Errorf("the long log keeps its end: truncated %v", l.Truncated)
		}
	}
}

func TestAUnreadableRunLogIsNamedAndTheRestStays(t *testing.T) {
	f := &fakeService{runs: []control.RunInfo{runInfo("r1", "failed"), runInfo("r2", "failed")}, logs: map[string]string{"r2": "fine\n"}}
	pkg := build(f)
	_, files := unpack(t, pkg)
	if !strings.Contains(files["run-logs.txt"], "could not be read") || !strings.Contains(files["run-logs.txt"], "fine") || pkg.Complete() {
		t.Fatalf("%s", files["run-logs.txt"])
	}
}

func TestNamesFromAnswersNeverFormArchivePaths(t *testing.T) {
	evil := "../../etc/passwd"
	f := &fakeService{
		runs: []control.RunInfo{{ID: evil, Stack: "/abs/" + evil, Result: "failed", Started: testNow}},
		logs: map[string]string{evil: "log\n"},
	}
	names, files := unpack(t, build(f))
	for _, n := range names {
		if !slices.Contains(Parts, strings.TrimSuffix(strings.TrimSuffix(n, ".txt"), ".json")) || strings.ContainsAny(n, "/\\") {
			t.Errorf("unexpected archive name %q", n)
		}
	}
	if !strings.Contains(files["run-logs.json"], "etc/passwd") {
		t.Error("the id should stay as data")
	}
}

func TestServiceLogPagesAndStaysBounded(t *testing.T) {
	f := &fakeService{logPages: 3}
	for i := range 10 {
		f.entries = append(f.entries, control.ServiceLogEntry{Time: testNow, Level: "info", Source: "qhost", Message: fmt.Sprintf("entry %d", i)})
	}
	_, files := unpack(t, build(f))
	if !strings.Contains(files["service-log.txt"], "entry 0") || !strings.Contains(files["service-log.txt"], "entry 9") {
		t.Fatalf("pages were lost:\n%s", files["service-log.txt"])
	}
	var doc struct {
		Data struct {
			Since   time.Time
			Entries []control.ServiceLogEntry
		}
	}
	if err := json.Unmarshal([]byte(files["service-log.json"]), &doc); err != nil || len(doc.Data.Entries) != 10 ||
		!doc.Data.Since.Equal(testNow.Add(-ServiceLogWindow)) {
		t.Fatalf("%v %+v", err, doc.Data)
	}

	f = &fakeService{logPages: 1000}
	for i := range MaxServiceLogEntries + 500 {
		f.entries = append(f.entries, control.ServiceLogEntry{Time: testNow, Level: "info", Source: "qhost", Message: fmt.Sprint("e", i)})
	}
	_, files = unpack(t, build(f))
	if err := json.Unmarshal([]byte(files["service-log.json"]), &doc); err != nil || len(doc.Data.Entries) != MaxServiceLogEntries ||
		doc.Data.Entries[len(doc.Data.Entries)-1].Message != fmt.Sprint("e", MaxServiceLogEntries+499) {
		t.Fatalf("%v, %d entries", err, len(doc.Data.Entries))
	}
}

func TestSaveWritesAPrivateNewFileAndNeverReplaces(t *testing.T) {
	home := t.TempDir()
	env := userfile.Env{
		Getenv: func(string) string { return "" },
		Euid:   os.Geteuid, Egid: os.Getegid,
		Lookup: func(uid int) (userfile.Account, error) {
			return userfile.Account{UID: uid, GID: os.Getgid(), Home: home}, nil
		},
	}
	pkg := build(&fakeService{})
	path, err := pkg.Save(env, "")
	if err != nil || filepath.Dir(path) != filepath.Join(home, "qhost") {
		t.Fatalf("%q, %v", path, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 || info.Size() == 0 {
		t.Fatalf("%v, %v", info, err)
	}
	out := filepath.Join(home, "chosen.tar.gz")
	if got, err := pkg.Save(env, out); err != nil || got != out {
		t.Fatalf("%q, %v", got, err)
	}
	if _, err := pkg.Save(env, out); err == nil {
		t.Fatal("an existing file was replaced")
	}
}
