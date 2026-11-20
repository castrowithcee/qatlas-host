package stacks

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func refsOf(t *testing.T, root string) Refs {
	t.Helper()
	s := load(t, root)["demo-stack"]
	if s.Err != nil {
		t.Fatal(s.Err)
	}
	return s.Refs
}

func wantList(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %q\nwant %q", what, got, want)
	}
}

func TestRefsNameTheFilesTheComposeFilesUse(t *testing.T) {
	root := t.TempDir()
	write(t, root, "demo-stack/compose.yaml", `include:
  - parts/base.yaml
  - path: [parts/more.yaml]
    env_file: parts/inc.env
services:
  app:
    image: registry.example.test/demo/app:${DEMO_VERSION}
    env_file:
      - app.env
      - path: opt.env
        required: false
    label_file: labels.yaml
    build:
      context: ./src
      dockerfile: ../Dockerfile.demo
  web:
    extends:
      file: ext/web.yaml
      service: web
    volumes:
      - ./conf/site.conf:/etc/site.conf:ro
      - ./conf/dir:/etc/dir:ro
      - ./state.db:/var/state.db
      - ./data:/var/data
      - ./missing.txt:/x:ro
      - {type: bind, source: ./conf/long.conf, target: /etc/long.conf, read_only: true}
configs:
  site:
    file: ./site.cfg
secrets:
  token:
    file: ./token.txt
`)
	write(t, root, "demo-stack/.env", "DEMO_VERSION=1.0.0\n")
	write(t, root, "demo-stack/compose.override.yaml", "services:\n  app:\n    env_file: over.env\n")
	write(t, root, "demo-stack/parts/base.yaml", "services:\n  base:\n    image: registry.example.test/demo/base:1\n    env_file: base.env\n")
	write(t, root, "demo-stack/parts/more.yaml", "include: [base.yaml]\nconfigs:\n  c:\n    file: more.cfg\n")
	write(t, root, "demo-stack/ext/web.yaml", "services:\n  web:\n    image: registry.example.test/demo/web:1\n    env_file: web.env\n")
	for _, f := range []string{"conf/site.conf", "conf/long.conf", "state.db"} {
		write(t, root, "demo-stack/"+f, "x")
	}
	write(t, root, "demo-stack/conf/dir/a", "x")
	write(t, root, "demo-stack/data/db", "x")

	r := refsOf(t, root)
	wantList(t, "Files", r.Files,
		"demo-stack/.env", "demo-stack/Dockerfile.demo", "demo-stack/app.env", "demo-stack/compose.override.yaml",
		"demo-stack/compose.yaml", "demo-stack/conf/long.conf", "demo-stack/conf/site.conf", "demo-stack/ext/web.env",
		"demo-stack/ext/web.yaml", "demo-stack/labels.yaml", "demo-stack/opt.env",
		"demo-stack/over.env", "demo-stack/parts/base.env", "demo-stack/parts/base.yaml", "demo-stack/parts/inc.env",
		"demo-stack/parts/more.cfg", "demo-stack/parts/more.yaml", "demo-stack/site.cfg")
	wantList(t, "Dirs", r.Dirs, "demo-stack/conf/dir", "demo-stack/src")
	wantList(t, "Writable", r.Writable, "demo-stack/state.db")
	wantList(t, "Secrets", r.Secrets, "demo-stack/token.txt")
	wantList(t, "Unresolved", r.Unresolved)
}

func TestRefsResolveVariablesFromEnvAndReportTheOthers(t *testing.T) {
	root := t.TempDir()
	write(t, root, "demo-stack/compose.yaml", `services:
  app:
    image: registry.example.test/demo/app:${DEMO_VERSION}
    env_file:
      - ${CONF_DIR}/app.env
      - ${OTHER_DIR:-fallback}/b.env
      - ${UNSET_DIR}/c.env
    volumes:
      - ${CONF_DIR}/ro.conf:/etc/ro.conf:ro
      - ${UNSET_VOL}:/data
`)
	write(t, root, "demo-stack/.env", "DEMO_VERSION=1.0.0\nCONF_DIR=./conf\nAPI_TOKEN=do-not-print-me-0001\n")
	write(t, root, "demo-stack/conf/ro.conf", "x")
	r := refsOf(t, root)
	wantList(t, "Files", r.Files, "demo-stack/.env", "demo-stack/compose.yaml", "demo-stack/conf/app.env",
		"demo-stack/conf/ro.conf", "demo-stack/fallback/b.env")
	if len(r.Unresolved) != 2 || !strings.Contains(r.Unresolved[0], "UNSET_DIR") || !strings.Contains(r.Unresolved[1], "UNSET_VOL") {
		t.Fatalf("Unresolved = %q", r.Unresolved)
	}
	for _, u := range r.Unresolved {
		if strings.Contains(u, "do-not-print-me") || strings.Contains(u, "./conf") {
			t.Errorf("Unresolved shows a value: %q", u)
		}
	}
}

func TestRefsNeverLeaveTheRepository(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	write(t, repo, "demo-stack/compose.yaml", `services:
  app:
    image: registry.example.test/demo/app:1
    env_file:
      - /etc/passwd
      - ../../outside.env
      - ../shared/ok.env
      - ~/home.env
      - a\b.env
    build: https://example.test/repo.git
    volumes:
      - /etc/hosts:/etc/hosts:ro
`)
	r := refsOf(t, repo)
	wantList(t, "Files", r.Files, "demo-stack/.env", "demo-stack/compose.yaml", "shared/ok.env")
	wantList(t, "Dirs", r.Dirs)
}

func TestRefsDoNotFollowLinksAndSurviveCycles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "demo-stack/compose.yaml", "include: [a.yaml, linked.yaml]\nservices:\n  app:\n    image: registry.example.test/demo/app:1\n")
	write(t, root, "demo-stack/a.yaml", "include: [b.yaml]\nservices:\n  a:\n    image: x:1\n    env_file: a.env\n")
	write(t, root, "demo-stack/b.yaml", "include: [a.yaml]\nservices:\n  b:\n    image: x:1\n    env_file: b.env\n")
	write(t, root, "elsewhere.yaml", "services:\n  l:\n    image: x:1\n    env_file: leaked.env\n")
	if err := os.Symlink(filepath.Join(root, "elsewhere.yaml"), filepath.Join(root, "demo-stack", "linked.yaml")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "demo-stack/real/x", "x")
	write(t, root, "demo-stack/compose.override.yaml", "services:\n  app:\n    volumes:\n      - ./lnk:/x:ro\n")
	if err := os.Symlink("real", filepath.Join(root, "demo-stack", "lnk")); err != nil {
		t.Fatal(err)
	}
	r := refsOf(t, root)
	if slices.Contains(r.Files, "demo-stack/leaked.env") {
		t.Fatalf("a link was followed: %q", r.Files)
	}
	for _, want := range []string{"demo-stack/a.env", "demo-stack/b.env", "demo-stack/a.yaml", "demo-stack/b.yaml"} {
		if !slices.Contains(r.Files, want) {
			t.Errorf("Files lacks %s: %q", want, r.Files)
		}
	}
	wantList(t, "Dirs", r.Dirs)
}

func TestRefsOfARootStackAreRelativeToTheRoot(t *testing.T) {
	root := t.TempDir()
	write(t, root, "compose.yaml", "services:\n  app:\n    image: registry.example.test/demo/app:1\n    env_file: app.env\n")
	list, err := Load(root)
	if err != nil || len(list) != 1 {
		t.Fatalf("Load = %v, %v", list, err)
	}
	wantList(t, "Files", list[0].Refs.Files, ".env", "app.env", "compose.yaml")
}

func TestInterpolateFollowsCompose(t *testing.T) {
	env := map[string]string{"A": "a", "EMPTY": ""}
	for _, c := range []struct{ in, out, missing string }{
		{"$A/x", "a/x", ""},
		{"${A}/x", "a/x", ""},
		{"$$A", "$A", ""},
		{"${B:-d}", "d", ""},
		{"${EMPTY:-d}", "d", ""},
		{"${EMPTY-d}", "", ""},
		{"${A:+r}", "r", ""},
		{"${B:+r}", "", ""},
		{"${B}", "", "B"},
		{"${B:?need}", "", "B"},
		{"${B:-${A}}", "}", "B"},
		{"$B", "", "B"},
	} {
		out, missing := interpolate(c.in, env)
		if out != c.out || missing != c.missing {
			t.Errorf("interpolate(%q) = %q, %q; want %q, %q", c.in, out, missing, c.out, c.missing)
		}
	}
}

func TestBindMountsResolveVariablesFromEnv(t *testing.T) {
	s := one(t, map[string]string{
		"compose.yaml": "services:\n  app:\n    image: registry.example.test/demo/app:1\n    volumes:\n      - ${DATA_DIR}/db:/db\n      - ${UNSET_DIR}/x:/x\n",
		".env":         "DATA_DIR=./state\n",
	})
	wantList(t, "BindMounts", s.BindMounts, "state/db")
}
