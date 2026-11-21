package hostservice

import (
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/tools"
)

func TestToolTableMatchesTheRegistry(t *testing.T) {
	reg, _ := testRegistry(t)
	inTable := map[string]tools.Entry{}
	for _, e := range tools.All() {
		if e.Exec == tools.ExecHostService {
			inTable[e.Command] = e
		}
	}
	for _, e := range reg.Entries() {
		got, ok := inTable[e.Name]
		if !ok {
			t.Errorf("command %s has no entry in the tool table", e.Name)
			continue
		}
		if string(got.Class) != e.Class.String() || string(got.Scope) != e.Scope.String() {
			t.Errorf("%s: table says %s and %s, registry says %s and %s", e.Name, got.Class, got.Scope, e.Class, e.Scope)
		}
		delete(inTable, e.Name)
	}
	for command := range inTable {
		t.Errorf("the tool table names the command %s, which the host service does not register", command)
	}
}
