// Package formatnumtest holds the checks every store with a format number shares in its tests: a document with a
// higher number and one without a number are refused with the format error, and the file stays as it was.
package formatnumtest

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

// Cases returns the two refused JSON documents of a store whose number is the member field of the current
// number: one with the next higher number and a member this version does not know, and one without the
// number. members is the rest of a valid document, as JSON members without the braces.
func Cases(field string, current int, members string) map[string]string {
	join := func(parts ...string) string {
		var keep []string
		for _, p := range parts {
			if p != "" {
				keep = append(keep, p)
			}
		}
		return "{" + strings.Join(keep, ",") + "}"
	}
	return map[string]string{
		"higher":  join(`"`+field+`":`+strconv.Itoa(current+1), members, `"added_later":true`),
		"missing": join(members),
	}
}

// Seed writes the document at path and creates the folder above it.
func Seed(t testing.TB, path, doc string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Refused fails unless err is the format error and names file.
func Refused(t testing.TB, err error, file string) {
	t.Helper()
	var fe *formatnum.Error
	if !errors.As(err, &fe) || !errors.Is(err, formatnum.ErrFormat) {
		t.Fatalf("got %v, want the format error", err)
	}
	if fe.File != file || !strings.Contains(err.Error(), file) {
		t.Fatalf("the error names %q, want %q: %v", fe.File, file, err)
	}
}

// Unchanged fails unless the file at path still holds exactly doc.
func Unchanged(t testing.TB, path, doc string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the file is gone: %v", err)
	}
	if !bytes.Equal(got, []byte(doc)) {
		t.Fatalf("the file changed:\n%s\nwant:\n%s", got, doc)
	}
}
