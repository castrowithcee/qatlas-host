package formatnum

import (
	"errors"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	if err := Check("f", "schema", 2, 2); err != nil {
		t.Fatal(err)
	}
	if err := Check("f", "schema", 1, 2, 1); err != nil {
		t.Fatalf("an older number with a transition: %v", err)
	}
	for _, tc := range []struct {
		found int
		want  string
	}{{3, "newer"}, {1, "cannot be read"}, {0, "no format number"}} {
		err := Check("/x/state.json", "schema", tc.found, 2)
		var fe *Error
		if !errors.As(err, &fe) || !errors.Is(err, ErrFormat) || !Is(err) {
			t.Fatalf("found %d: %v", tc.found, err)
		}
		if fe.File != "/x/state.json" || fe.Found != tc.found || fe.Expected != 2 {
			t.Fatalf("found %d: %+v", tc.found, fe)
		}
		if !strings.Contains(err.Error(), "/x/state.json") || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("found %d: %q", tc.found, err)
		}
	}
}

func TestCheckJSON(t *testing.T) {
	for _, tc := range []struct {
		doc     string
		refused bool
	}{
		{`{"schema":1,"a":true}`, false},
		{`{"schema":2,"new_field":[1]}`, true},
		{`{"a":1}`, true},
		{`{"schema":"1"}`, true},
		{`not json`, false},
		{`null`, false},
	} {
		if got := CheckJSON("f", "schema", []byte(tc.doc), 1) != nil; got != tc.refused {
			t.Errorf("%s: refused %v", tc.doc, got)
		}
	}
}
