package service_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func excludedReasons(res control.SourceInspectResult) map[string]string {
	out := map[string]string{}
	for _, e := range res.Excluded {
		out[e.Path] = e.Reason
	}
	return out
}

func TestInspectPlansOnlyWhatTheStacksReferenceInAGrownFolder(t *testing.T) {
	h := realGitService(t)
	dir := newFolder(t)
	const fakeValue = "not-a-real-secret-4711"
	writeTree(t, dir, map[string]string{
		".gitignore":             "*.tmp\n",
		".gitattributes":         "* text=auto\n",
		".qatlas/qhost/web.yaml": "version: 1\nupdate:\n  mode: automatic\n",
		"web/compose.yaml": `include:
  - parts/db.yaml
services:
  app:
    image: registry.example.test/demo/web:${WEB_VERSION}
    env_file: [app.env]
    build: ./src
    volumes:
      - ./data:/var/lib/demo
      - ./conf/site.conf:/etc/site.conf:ro
      - ./acme.json:/acme.json
configs:
  site:
    file: ./site.cfg
secrets:
  token:
    file: ./token.txt
`,
		"web/parts/db.yaml":    "services:\n  db:\n    image: registry.example.test/demo/db:1\n    env_file: ${DB_DIR}/db.env\n",
		"web/.env":             "WEB_VERSION=1.2.3\nDB_DIR=db\nAPI_PASSWORD=" + fakeValue + "\n",
		"web/app.env":          "A=1\n",
		"web/src/Dockerfile":   "FROM scratch\n",
		"web/conf/site.conf":   "x",
		"web/site.cfg":         "x",
		"web/token.txt":        "x",
		"web/acme.json":        "x",
		"web/parts/db/db.env":  "B=2\n",
		"web/data/db.bin":      "x",
		"web/compose.yaml.bak": "old",
		"web/.env.bak.1":       "API_PASSWORD=" + fakeValue + "\n",
		"web/app.log":          "x",
		"web/notes.md":         "x",
		"web/old/x/y":          "x",
		"plain/compose.yaml":   "services:\n  p:\n    image: registry.example.test/demo/p:1\n    env_file: ${MISSING_DIR}/p.env\n",
		"plain/p.env":          "x",
		"misc/readme.txt":      "x",
	})
	res := h.inspect(dir, "")
	if res.Case != control.SourceCaseFiles {
		t.Fatalf("inspect = %+v", res)
	}
	wantPlanned := []string{
		".gitattributes", ".gitignore", ".qatlas/qhost/web.yaml", "plain/compose.yaml",
		"web/.env", "web/app.env", "web/compose.yaml", "web/conf/site.conf", "web/parts/db.yaml", "web/parts/db/db.env",
		"web/site.cfg", "web/src/Dockerfile",
	}
	if !slices.Equal(res.PlannedFiles, wantPlanned) {
		t.Fatalf("planned = %q\nwant    %q", res.PlannedFiles, wantPlanned)
	}
	want := map[string]string{
		"misc":                 control.SourceExcludedNotReferenced,
		"plain/p.env":          control.SourceExcludedNotReferenced,
		"web/.env.bak.1":       control.SourceExcludedNotReferenced,
		"web/acme.json":        control.SourceExcludedWritableFile,
		"web/app.log":          control.SourceExcludedNotReferenced,
		"web/compose.yaml.bak": control.SourceExcludedNotReferenced,
		"web/data":             control.SourceExcludedBindMount,
		"web/notes.md":         control.SourceExcludedNotReferenced,
		"web/old":              control.SourceExcludedNotReferenced,
		"web/token.txt":        control.SourceExcludedSecretsFile,
	}
	got := excludedReasons(res)
	for p, reason := range want {
		if got[p] != reason {
			t.Errorf("%s: %q, want %q (all: %v)", p, got[p], reason, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("excluded = %v", got)
	}
	plain := inspectedStack(t, res, "plain")
	if len(plain.Unresolved) != 1 || !strings.Contains(plain.Unresolved[0], "MISSING_DIR") {
		t.Errorf("unresolved = %q", plain.Unresolved)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), fakeValue) {
		t.Errorf("the answer carries a value of .env: %s", raw)
	}
}

func TestInspectTakesNoFileOfAStackWhoseEnvCannotBeRead(t *testing.T) {
	notRoot(t)
	h := realGitService(t)
	dir := newFolder(t)
	writeTree(t, dir, map[string]string{
		"web/compose.yaml":    bindCompose,
		"web/.env":            "WEB_VERSION=1.2.3\n",
		"web/data/db.bin":     "x",
		"locked/compose.yaml": bindCompose,
		"locked/.env":         "WEB_VERSION=1.2.3\n",
		"locked/data/db.bin":  "x",
		"locked/notes.txt":    "x",
	})
	envPath := filepath.Join(dir, "locked", ".env")
	if err := os.Chmod(envPath, 0); err != nil {
		t.Fatal(err)
	}
	res := h.inspect(dir, "")
	if res.Case != control.SourceCaseFiles {
		t.Fatalf("inspect = %+v", res)
	}
	if want := []string{"web/.env", "web/compose.yaml"}; !slices.Equal(res.PlannedFiles, want) {
		t.Fatalf("planned = %q, want %q", res.PlannedFiles, want)
	}
	got := excludedReasons(res)
	if got["locked"] != control.SourceExcludedUnreadableStack || got["web/data"] != control.SourceExcludedBindMount || len(got) != 2 {
		t.Fatalf("excluded = %v", got)
	}
	f, ok := ruleOf(inspectedStack(t, res, "locked"), "unreadable-stack")
	if !ok || !strings.Contains(f.Fix, "read permission") || !strings.Contains(f.Fix, "group of the deploy service") {
		t.Fatalf("finding = %+v, %v", f, ok)
	}
	if info, _ := os.Lstat(envPath); info.Mode().Perm() != 0 {
		t.Fatalf("the mode changed to %v", info.Mode().Perm())
	}
}

func TestInspectOfARootStackTakesItsReferencesAndTheRootFiles(t *testing.T) {
	h := realGitService(t)
	dir := newFolder(t)
	writeTree(t, dir, map[string]string{
		"compose.yaml": "services:\n  app:\n    image: registry.example.test/demo/web:${WEB_VERSION}\n    env_file: conf/app.env\n",
		".env":         "WEB_VERSION=1.2.3\n",
		"conf/app.env": "A=1\n",
		"conf/old.env": "A=0\n",
		"notes.txt":    "x",
		".gitignore":   "x\n",
	})
	res := h.inspect(dir, "")
	if want := []string{".env", ".gitignore", "compose.yaml", "conf/app.env"}; !slices.Equal(res.PlannedFiles, want) {
		t.Fatalf("planned = %q, want %q", res.PlannedFiles, want)
	}
	got := excludedReasons(res)
	if len(got) != 2 || got["notes.txt"] != control.SourceExcludedNotReferenced || got["conf/old.env"] != control.SourceExcludedNotReferenced {
		t.Fatalf("excluded = %v", got)
	}
}

func TestInspectOfAFolderWithoutStacksTakesOnlyTheRootFiles(t *testing.T) {
	h := realGitService(t)
	dir := newFolder(t)
	writeTree(t, dir, map[string]string{"notes.txt": "x", "docs/a.md": "x", ".gitignore": "x\n"})
	res := h.inspect(dir, "")
	if want := []string{".gitignore"}; !slices.Equal(res.PlannedFiles, want) {
		t.Fatalf("planned = %q, want %q", res.PlannedFiles, want)
	}
	got := excludedReasons(res)
	if len(got) != 2 || got["docs"] != control.SourceExcludedNotReferenced || got["notes.txt"] != control.SourceExcludedNotReferenced {
		t.Fatalf("excluded = %v", got)
	}
}
