package tui

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/castrowithcee/qatlas-kit/helppages"
	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/tuidocs"
)

const (
	helpMissingFile = "testdata/help-missing.txt"
	// maxHelpLines is the length of a page in text lines, without its front matter and blank lines.
	maxHelpLines    = 40
	helpMissingHead = "# Screens that have no help page yet, one ID per line. The list only shrinks: `just help-update`\n" +
		"# drops the entries that got a page and never adds one.\n"
)

// helpStateTable lists the states of helpState.
var helpStateTable = []string{
	"stacks/setup", "detail/setup", "runs/setup", "containers/setup", "proposals/setup",
	"detail/clog", "containers/clog", "runs/clog",
	"stacks/key", "stacks/changes", "stacks/preview", "detail/pick", "containers/detail", "proposals/preview",
	"runs/log", "system/users/pick", "system/folders/rights", "system/folders/preview",
}

// helpOtherTitles are the titles of dialogs and forms whose text ID does not start with "title.".
var helpOtherTitles = []string{string(textSetupTitle), string(textTitleMove), string(textSettingDoneTitle), string(textTitleDiscardChange)}

// helpScreenTable is every screen ID the interface can name: areas, tabs, states, the dialogs and forms of the
// text catalog and the stack actions.
func helpScreenTable() []string {
	var ids []string
	for _, a := range areas() {
		ids = append(ids, string(a.ID))
		for _, t := range a.Tabs {
			ids = append(ids, string(a.ID)+"/"+string(t.ID))
		}
	}
	ids = append(ids, helpStateTable...)
	for id := range catalog()[i18n.English] {
		if strings.HasPrefix(string(id), "title.") {
			ids = append(ids, "title:"+string(id))
		}
	}
	for _, id := range helpOtherTitles {
		ids = append(ids, "title:"+id)
	}
	for id := range actionKinds {
		ids = append(ids, "action:"+string(id))
	}
	sort.Strings(ids)
	return ids
}

// helpPlaceholders are the key placeholders a page may use: the actions of the key table.
func helpPlaceholders() func(string) bool {
	known := map[string]bool{}
	for _, a := range keyTable() {
		known[string(a.ID)] = true
	}
	return func(name string) bool { return known[name] }
}

// helpCoverage returns what is wrong with the pages against the screens and the list of screens without a page.
func helpCoverage(pages fs.FS, screens, missing []string) []string {
	set, err := helppages.Load(pages, helppages.Options{Placeholder: helpPlaceholders()})
	if err != nil {
		return []string{"pages: " + err.Error()}
	}
	var out []string
	have := map[string]bool{}
	for _, topic := range set.Topics {
		for _, p := range append([]*helppages.Page{topic.Index}, topic.Pages...) {
			path := topic.ID + "/" + p.Name
			for _, id := range p.Screens {
				have[id] = true
				if !slices.Contains(screens, id) {
					out = append(out, fmt.Sprintf("%s: unknown screen %q", path, id))
				}
			}
			out = append(out, pageProblems(pages, path)...)
		}
	}
	for _, id := range screens {
		if !have[id] && !slices.Contains(missing, id) {
			out = append(out, fmt.Sprintf("screen %q has no page and is not in %s", id, helpMissingFile))
		}
	}
	for _, id := range missing {
		switch {
		case have[id]:
			out = append(out, fmt.Sprintf("%s lists %q, which has a page; run `just help-update`", helpMissingFile, id))
		case !slices.Contains(screens, id):
			out = append(out, fmt.Sprintf("%s lists %q, which is no screen; run `just help-update`", helpMissingFile, id))
		}
	}
	sort.Strings(out)
	return out
}

// pageProblems checks that the page has a German file and that no file is longer than maxHelpLines.
func pageProblems(pages fs.FS, path string) []string {
	var out []string
	for _, name := range []string{path + ".md", path + ".de.md"} {
		body, err := fs.ReadFile(pages, name)
		if err != nil {
			out = append(out, fmt.Sprintf("%s: missing", name))
			continue
		}
		if n := textLines(string(body)); n > maxHelpLines {
			out = append(out, fmt.Sprintf("%s: %d text lines, limit %d", name, n, maxHelpLines))
		}
	}
	return out
}

// textLines counts the non-blank lines after the front matter.
func textLines(page string) int {
	lines := strings.Split(page, "\n")
	if len(lines) > 0 && lines[0] == "---" {
		if end := slices.Index(lines[1:], "---"); end >= 0 {
			lines = lines[end+2:]
		}
	}
	n := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

func parseHelpMissing(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

// lowerHelpMissing drops the entries that got a page or are no screen. It never adds one.
func lowerHelpMissing(pages fs.FS, screens, missing []string) []string {
	var out []string
	for _, id := range missing {
		if !slices.Contains(screens, id) {
			continue
		}
		if set, err := helppages.Load(pages, helppages.Options{}); err == nil {
			if _, _, ok := set.Screen(id); ok {
				continue
			}
		}
		out = append(out, id)
	}
	return out
}

func TestHelpCoverage(t *testing.T) {
	raw, err := os.ReadFile(helpMissingFile)
	if err != nil {
		t.Fatal(err)
	}
	screens, missing := helpScreenTable(), parseHelpMissing(string(raw))
	if *update {
		missing = lowerHelpMissing(tuidocs.FS, screens, missing)
		sort.Strings(missing)
		text := helpMissingHead + strings.Join(missing, "\n") + "\n"
		if err := os.WriteFile(helpMissingFile, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if problems := helpCoverage(tuidocs.FS, screens, missing); len(problems) > 0 {
		t.Fatalf("the help pages do not cover the screens:\n%s", strings.Join(problems, "\n"))
	}
	if !slices.IsSorted(missing) {
		t.Errorf("%s is not sorted", helpMissingFile)
	}
}

func helpPage(title, extra, body string) string {
	return "---\ntitle: " + title + "\n" + extra + "---\n" + body + "\n"
}

// helpFS is a page set of one topic with the English and German page of the screens.
func helpFS(screens string) fstest.MapFS {
	return fstest.MapFS{
		"start/index.md":    {Data: []byte(helpPage("Start", "screens: ["+screens+"]\n", "Text."))},
		"start/index.de.md": {Data: []byte(helpPage("Start", "", "Text."))},
	}
}

func TestHelpCoverageFindsEveryKindOfError(t *testing.T) {
	body := strings.Repeat("line\n\n", maxHelpLines+1)
	long, longDe := helpPage("Start", "screens: [a]\n", body), helpPage("Start", "", body)
	noGerman := helpFS("a")
	delete(noGerman, "start/index.de.md")
	tests := []struct {
		name     string
		pages    fs.FS
		missing  []string
		contains string
	}{
		{"covered", helpFS("a, b"), nil, ""},
		{"listed and without page", helpFS("a"), []string{"b"}, ""},
		{"no page and not listed", helpFS("a"), nil, `screen "b" has no page`},
		{"listed but has a page", helpFS("a, b"), []string{"b"}, `lists "b", which has a page`},
		{"listed but unknown", helpFS("a, b"), []string{"gone"}, `lists "gone", which is no screen`},
		{"page of an unknown screen", helpFS("a, b, c"), nil, `unknown screen "c"`},
		{"no German page", noGerman, nil, "start/index.de.md: missing"},
		{"page too long", fstest.MapFS{"start/index.md": {Data: []byte(long)},
			"start/index.de.md": {Data: []byte(longDe)}}, []string{"b"}, "text lines, limit 40"},
		{"unknown key placeholder", fstest.MapFS{"start/index.md": {Data: []byte(helpPage("Start", "screens: [a, b]\n", "Press {nokey}."))},
			"start/index.de.md": {Data: []byte(helpPage("Start", "", "Text."))}}, nil, "nokey"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(helpCoverage(tt.pages, []string{"a", "b"}, tt.missing), "\n")
			switch {
			case tt.contains == "" && got != "":
				t.Errorf("unexpected problems:\n%s", got)
			case !strings.Contains(got, tt.contains):
				t.Errorf("want %q in:\n%s", tt.contains, got)
			}
		})
	}
}

func TestHelpUpdateOnlyRemovesEntries(t *testing.T) {
	pages := helpFS("a")
	got := lowerHelpMissing(pages, []string{"a", "b", "c"}, []string{"a", "b", "gone"})
	if !slices.Equal(got, []string{"b"}) {
		t.Errorf("lowered list %v, want [b] (a has a page, gone is no screen, c is never added)", got)
	}
}

func TestTheStartTopicComesFirst(t *testing.T) {
	set, err := helppages.Load(tuidocs.FS, helppages.Options{Placeholder: helpPlaceholders()})
	if err != nil {
		t.Fatal(err)
	}
	if set.Topics[0].ID != "start" {
		t.Errorf("the first topic is %q: the start page is the one a screen without page falls back to", set.Topics[0].ID)
	}
}
