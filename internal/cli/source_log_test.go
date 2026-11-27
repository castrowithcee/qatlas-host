package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func historyHandlers(seen *[]json.RawMessage) fakeHandlers {
	keep := func(raw json.RawMessage) { *seen = append(*seen, raw) }
	return fakeHandlers{
		control.SourceLogCommand: func(raw json.RawMessage) any {
			keep(raw)
			return control.SourceLogResult{Path: "/srv/stacks", Truncated: true, Commits: []control.SourceLogCommit{
				{ID: strings.Repeat("a", 40), Author: "Ada", Date: "2026-01-02T03:04:05Z", Subject: "add web",
					Paths: []control.SourceLogPath{{Path: "web/compose.yaml", Kind: "new"}}, PathsTruncated: true},
				{ID: strings.Repeat("b", 40), Author: "Bob", Date: "2026-01-01T00:00:00Z", Subject: "init"}}}
		},
		control.SourceShowCommand: func(raw json.RawMessage) any {
			keep(raw)
			return control.SourceShowResult{Path: "/srv/stacks", Commit: strings.Repeat("a", 40), Diff: "-a\n+b\n", Truncated: true}
		},
	}
}

func TestSourceLogListsCommitsAndSaysWhatIsCut(t *testing.T) {
	var seen []json.RawMessage
	startFakeService(t, withStatus(historyHandlers(&seen), []string{"/srv/stacks"}, "web"))

	code, stdout, stderr := run(t, "source", "log", "/srv/stacks", "--limit", "5")
	for _, want := range []string{"aaaaaaaaaaaa", "Ada", "add web", "new web/compose.yaml", "more paths not shown"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("table lacks %q: %q", want, stdout)
		}
	}
	if code != 0 || !strings.Contains(stderr, "more commits") || string(seen[0]) != `{"path":"/srv/stacks","limit":5}` {
		t.Fatalf("log: code %d, stderr %q, arguments %s", code, stderr, seen[0])
	}

	code, stdout, stderr = run(t, "source", "log", "/srv/stacks", "--output", "json")
	var res control.SourceLogResult
	if err := json.Unmarshal([]byte(stdout), &res); code != 0 || err != nil || len(res.Commits) != 2 || !res.Truncated ||
		!res.Commits[0].PathsTruncated || res.Commits[1].Subject != "init" {
		t.Fatalf("json: code %d, stdout %q, stderr %q, err %v", code, stdout, stderr, err)
	}
}

func TestSourceShowPrintsTheDiff(t *testing.T) {
	var seen []json.RawMessage
	startFakeService(t, withStatus(historyHandlers(&seen), []string{"/srv/stacks"}, "web"))

	code, stdout, stderr := run(t, "source", "show", "/srv/stacks", "aaaaaaa", "--output", "json")
	var res control.SourceShowResult
	if err := json.Unmarshal([]byte(stdout), &res); code != 0 || err != nil || res.Diff != "-a\n+b\n" || !res.Truncated {
		t.Fatalf("show: code %d, stdout %q, stderr %q, err %v", code, stdout, stderr, err)
	}
	if string(seen[0]) != `{"path":"/srv/stacks","commit":"aaaaaaa"}` {
		t.Fatalf("show arguments = %s", seen[0])
	}
	if code, stdout, _ := run(t, "source", "show", "/srv/stacks", "aaaaaaa"); code != 0 || !strings.Contains(stdout, "+b") || !strings.Contains(stdout, "TRUNCATED") {
		t.Fatalf("plain show: code %d, stdout %q", code, stdout)
	}
}

func TestSourceLogRefusesBadArgumentsBeforeCallingTheService(t *testing.T) {
	var seen []json.RawMessage
	startFakeService(t, withStatus(historyHandlers(&seen), []string{"/srv/stacks"}, "web"))
	for _, args := range [][]string{{"source", "log"}, {"source", "log", "web"}, {"source", "log", "/srv/stacks", "--limit", "201"},
		{"source", "log", "/srv/stacks", "--limit", "-1"}, {"source", "show", "/srv/stacks"}, {"source", "show", "web", "abcdef1"}} {
		if code, _, stderr := run(t, args...); code == 0 || stderr == "" {
			t.Errorf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
	if len(seen) != 0 {
		t.Fatalf("the service was called: %v", seen)
	}
}
