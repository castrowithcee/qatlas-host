package control

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAHostCommandKeepsItsScope(t *testing.T) {
	reg := NewRegistry()
	h := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	if err := reg.Register("host.example", Command{Class: ClassRead, Scope: ScopeHost, Handle: h}); err != nil {
		t.Fatal(err)
	}
	c, _ := reg.Lookup("host.example")
	if got := c.AccessFor(nil); got != (Access{Class: ClassRead, Scope: ScopeHost}) {
		t.Fatalf("access = %+v", got)
	}
	if ScopeHost.String() != "host" {
		t.Fatalf("name = %s", ScopeHost)
	}
}
