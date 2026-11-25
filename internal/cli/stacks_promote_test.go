package cli

import (
	"encoding/json"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func TestStackListShowsTheVersionsOfThePromoteSource(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	images := []control.ImageInfo{{Var: "APP_TAG", Image: "registry.example.test/demo/app", Version: "1.2.0"}}
	info := func(name, source string) control.StackInfo {
		return control.StackInfo{Name: name, Folder: name, Source: "x", PromoteSource: source, Images: images}
	}
	system := startFakeService(t, fakeHandlers{control.StacksListCommand: func(json.RawMessage) any {
		return control.StacksResult{Dir: "/srv/x", Stacks: []control.StackInfo{info("prod", "stage"), info("lost", "nowhere"),
			info("careful", "shaky"), info("plain", "")}}
	}})
	alice := serveFake(t, eps["alice"], fakeHandlers{
		control.StacksListCommand: func(json.RawMessage) any {
			return control.StacksResult{Dir: "/srv/y", Stacks: []control.StackInfo{info("stage", ""), info("shaky", "")}}
		},
		control.PromoteSourceCommand: func(raw json.RawMessage) any {
			var a control.PromoteSourceArgs
			_ = json.Unmarshal(raw, &a)
			res := control.PromoteSource{Stack: a.Stack, Commit: promoteCommit, LastResult: "succeeded",
				Versions: []control.PromoteVersion{{Variable: "STAGE_TAG", Image: "registry.example.test/demo/app", Version: "1.3.0"}}}
			if a.Stack == "shaky" {
				res.LastResult = "failed"
			}
			return res
		},
	})

	code, out, stderr := run(t, "stack", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	got := map[string][2]any{}
	for _, r := range rowsOf(t, out) {
		got[r["stack"].(string)] = [2]any{r["promote_source"], r["promote_versions"]}
	}
	want := map[string][2]any{
		"prod":    {"stage", "APP_TAG=1.3.0"},
		"lost":    {"nowhere", "(source missing)"},
		"careful": {"shaky", "(source not proven: last run did not succeed)"},
		"plain":   {nil, nil},
		"stage":   {nil, nil},
		"shaky":   {nil, nil},
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s: %v, want %v", name, got[name], w)
		}
	}
	if alice.count(control.PromoteSourceCommand) != 2 || system.count(control.PromoteSourceCommand) != 0 {
		t.Fatal("the sources were not read at the engine that has them, once each")
	}
}
