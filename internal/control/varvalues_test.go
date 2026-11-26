package control

import (
	"slices"
	"testing"
)

func TestDistinctVars(t *testing.T) {
	got := DistinctVars([]VarValue{
		{"APP", "web", "1.0"}, {"DB", "db", "5"}, {"APP", "worker", "1.0"},
		{"V", "web", "1.0.0"}, {"V", "worker", "1.1.0"}, {"E", "a", ""},
	})
	want := []string{"APP=1.0", "DB=5", "V=1.0.0 (web) / 1.1.0 (worker)", "E=(unset)"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
