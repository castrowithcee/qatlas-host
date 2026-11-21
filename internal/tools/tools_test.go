package tools

import (
	"regexp"
	"testing"

	"github.com/castrowithcee/qatlas-kit/jsonschema"
)

var entryName = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9-]*)*$`)

func TestEveryEntryIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	commands := map[string]bool{}
	for _, e := range All() {
		if !entryName.MatchString(e.Name) {
			t.Errorf("name %q is not a command name", e.Name)
		}
		if seen[e.Name] {
			t.Errorf("name %q is twice in the table", e.Name)
		}
		seen[e.Name] = true
		if e.Description == "" {
			t.Errorf("%s has no description", e.Name)
		}
		switch e.Class {
		case ClassRead, ClassPropose, ClassChange, ClassRoot:
		default:
			t.Errorf("%s has class %q", e.Name, e.Class)
		}
		switch e.Scope {
		case ScopeHost, ScopeDeploy, ScopeStack:
		default:
			t.Errorf("%s has scope %q", e.Name, e.Scope)
		}
		switch e.Exec {
		case ExecLocal:
			if e.Command != "" {
				t.Errorf("%s is local and names the command %q", e.Name, e.Command)
			}
			if e.Class == ClassPropose || e.Class == ClassChange {
				t.Errorf("%s is local with class %s: a local function is read or root", e.Name, e.Class)
			}
		case ExecHostService, ExecDeployService:
			if e.Command == "" {
				t.Errorf("%s runs in a service and names no command", e.Name)
			}
			if e.Class == ClassRoot {
				t.Errorf("%s is a service command with class root", e.Name)
			}
			key := string(e.Exec) + "/" + e.Command
			if commands[key] {
				t.Errorf("command %s is twice in the table", key)
			}
			commands[key] = true
		default:
			t.Errorf("%s has execution %q", e.Name, e.Exec)
		}
		if e.Call != nil && (e.Exec != ExecLocal || e.Class != ClassRead) {
			t.Errorf("%s carries a call but is not a local read function", e.Name)
		}
		if len(e.Schema) == 0 {
			t.Errorf("%s has no schema", e.Name)
		}
	}
}

func TestEverySchemaIsValid(t *testing.T) {
	for _, e := range All() {
		if err := jsonschema.CheckSchema(e.Schema); err != nil {
			t.Errorf("%s: %v", e.Name, err)
		}
	}
}

func TestSchemasAcceptAndRefuseArguments(t *testing.T) {
	e, ok := Lookup("stack.start")
	if !ok {
		t.Fatal("stack.start is not in the table")
	}
	if err := jsonschema.ValidateJSON(e.Schema, []byte(`{"stack":"demo"}`)); err != nil {
		t.Errorf("valid arguments refused: %v", err)
	}
	for _, bad := range []string{`{}`, `{"stack":1}`, `{"stack":"demo","extra":true}`} {
		if err := jsonschema.ValidateJSON(e.Schema, []byte(bad)); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}
