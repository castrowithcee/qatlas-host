package formatnum

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// store is one stored state with a format number: the package and source file that own it, the file it lives
// in below a state folder, and the field (or first line) that carries the number.
type store struct {
	Package string
	Source  string
	File    string
	Field   string
	// New is true for a store that got its number with the format-number work, false for one that had it.
	New bool
}

// stores is the inventory. Every store here has its own test that a higher and a missing number are refused
// with this package's error and leave the file byte-identical.
var stores = []store{
	{"notifier", "notifier/store.go", "notify.json", "schema", false},
	{"notifier", "notifier/store.go", "notify-updates.json", "schema", false},
	{"service", "service/sources.go", "sources.json", "version", false},
	{"service", "service/sources.go", "stacks-repo.json (earlier format, read for the transition)", "version", false},
	{"backup", "backup/store.go", "<stack>/<id>/meta.json", "schema", false},
	{"window", "window/scheduler.go", "window.json", "schema", false},
	{"engine", "engine/engine.go", "deploy/state.json", "schema", false},
	{"engine", "engine/runlog.go", "deploy/runs/<run>.log (first line)", "schema", true},
	{"hookservice", "hookservice/config.go", "/etc/qhost-hook/config.json", "version", false},
	{"changelog", "changelog/cache.go", "<owner>_<repo>.json", "version", false},
	{"stacks", "stacks/rules.go", ".qatlas/qhost/<stack>.yaml", "version", false},
	{"proposals", "proposals/proposals.go", "proposals/<id>.json", "schema", true},
	{"hostchange", "host/hostchange/journal.go", "journal/<id>.json", "schema", true},
	{"service", "service/logstore.go", "service-log/<id>.jsonl (first line)", "schema", true},
}

// exceptions are source files that write below a state folder or to the host but hold no document of this
// program's own format.
var exceptions = []struct{ Source, Reason string }{
	{"backup/backup.go", "the data files of a backup and its command file; meta.json, which is numbered, names them"},
	{"cli/folders.go", "a sudoers drop-in, whose format is sudo's"},
	{"cli/hostproposal.go", "opens the terminal device to ask a question; it stores nothing"},
	{"credentials/credentials.go", "deploy keys in OpenSSH format and raw secret values, which are no document"},
	{"deploysetup/deploysetup.go", "unit files and rules for systemd, sudo and polkit; the hook configuration it writes is numbered"},
	{"gitrepo/commit.go", "files of a stack's repository, in their own formats"},
	{"gitrepo/init.go", "git metadata in the working copy, git's format"},
	{"gitrepo/preview.go", "git metadata in the working copy, git's format"},
	{"gitrepo/remote.go", "git metadata in the working copy, git's format"},
	{"hookservice/service.go", "empty marker files, which hold no content"},
	{"hostkey/hostkey.go", "known_hosts in OpenSSH format"},
	{"service/dockerconfig.go", "the Docker client configuration, Docker's format"},
	{"stacks/secretswrite.go", "secrets.env, an env file read by Docker Compose"},
}

var fileWriters = map[string]map[string]bool{
	"os":         {"WriteFile": true, "Create": true, "CreateTemp": true, "OpenFile": true, "Rename": true},
	"configfile": {"NewStore": true, "WriteNewPrivateFile": true},
}

var writeFlags = []string{"O_WRONLY", "O_RDWR", "O_CREATE", "O_APPEND", "O_TRUNC"}

// writerFiles returns the source files below internal/ whose code writes a file or opens a configfile store.
func writerFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("..", path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch rel {
			case "service/servicetest", "registry/registrytest", "channeltest", "formatnum/formatnumtest":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		found := false
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || !fileWriters[pkg.Name][sel.Sel.Name] {
				return true
			}
			if pkg.Name == "os" && sel.Sel.Name == "OpenFile" && !(len(call.Args) > 1 && mentions(call.Args[1], writeFlags)) {
				return true
			}
			found = true
			return true
		})
		if found {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mentions(e ast.Expr, names []string) bool {
	hit := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			hit = hit || slices.Contains(names, x.Name)
		case *ast.SelectorExpr:
			hit = hit || slices.Contains(names, x.Sel.Name)
		}
		return true
	})
	return hit
}

// TestEveryWriterIsListed fails when a package below internal/ writes files and its source file is in neither
// the inventory of numbered stores nor the exceptions. A new store gets a number and an entry, or an exception
// with its reason.
func TestEveryWriterIsListed(t *testing.T) {
	known := map[string]bool{}
	for _, s := range stores {
		known[s.Source] = true
	}
	for _, e := range exceptions {
		if known[e.Source] {
			t.Errorf("%s is both a store and an exception", e.Source)
		}
		if strings.TrimSpace(e.Reason) == "" {
			t.Errorf("exception %s has no reason", e.Source)
		}
		known[e.Source] = true
	}
	for _, src := range writerFiles(t) {
		if !known[src] {
			t.Errorf("internal/%s writes files but is neither in the inventory of numbered stores nor in the exceptions", src)
		}
	}
}

// TestInventoryIsCurrent keeps the lists honest: every source exists and a store names its number field.
func TestInventoryIsCurrent(t *testing.T) {
	for _, s := range stores {
		data, err := os.ReadFile(filepath.Join("..", s.Source))
		if err != nil {
			t.Errorf("%s: %v", s.Source, err)
			continue
		}
		if !strings.HasPrefix(s.Source, s.Package+"/") && !strings.Contains(s.Source, "/"+s.Package+"/") {
			t.Errorf("%s is not in package %s", s.Source, s.Package)
		}
		if !strings.Contains(string(data), `"`+s.Field) {
			t.Errorf("%s does not mention the field %q of %s", s.Source, s.Field, s.File)
		}
	}
	for _, e := range exceptions {
		if _, err := os.Stat(filepath.Join("..", e.Source)); err != nil {
			t.Errorf("exception %s: %v", e.Source, err)
		}
	}
}
