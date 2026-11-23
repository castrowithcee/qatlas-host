package grants

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func grant(user string, uid uint32, action, stack string) control.Grant {
	return control.Grant{User: user, UID: uid, Action: action, Stack: stack, CreatedAt: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
}

func TestUpsertKeepsIDAndReplacesExpiry(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, replaced, err := s.Upsert(grant("example-admin", 4100, "stack.restart", "demo-stack"))
	if err != nil || replaced || !control.ValidGrantID(first.ID) {
		t.Fatalf("first = %+v, %v, %v", first, replaced, err)
	}
	g := grant("example-admin", 4100, "stack.restart", "demo-stack")
	g.Expires, g.CreatedBy = "2031-05-06", 1000
	second, replaced, err := s.Upsert(g)
	if err != nil || !replaced || second.ID != first.ID || second.Expires != "2031-05-06" || second.CreatedBy != 0 {
		t.Fatalf("second = %+v, %v, %v; want the same ID, the new expiry and the old origin", second, replaced, err)
	}
	if _, _, err := s.Upsert(grant("example-admin", 4100, "update.check", "demo-stack")); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file = %v, %v; want mode 0600", info, err)
	}
}

func TestStoreHoldsAtMostTheLimit(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := range control.GrantMax {
		if _, _, err := s.Upsert(grant("example-admin", uint32(5000+i), "update.check", "demo-stack")); err != nil {
			t.Fatalf("grant %d: %v", i, err)
		}
	}
	if _, _, err := s.Upsert(grant("example-admin", 9000, "update.check", "demo-stack")); !errors.Is(err, ErrFull) {
		t.Fatalf("over the limit: %v", err)
	}
	// Replacing an existing grant needs no room.
	if _, replaced, err := s.Upsert(grant("example-admin", 5000, "update.check", "demo-stack")); err != nil || !replaced {
		t.Fatalf("replace at the limit: %v, %v", replaced, err)
	}
}

func TestRemoveUnknownID(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := s.Upsert(grant("example-admin", 4100, "stack.restart", "demo-stack"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remove("g-00000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Remove(unknown) = %v", err)
	}
	if gone, err := s.Remove(g.ID); err != nil || gone.ID != g.ID {
		t.Fatalf("Remove = %+v, %v", gone, err)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Fatalf("list = %+v", list)
	}
}

func TestDamagedFileIsNeverWritten(t *testing.T) {
	for name, doc := range map[string]string{
		"not json":       `{"schema":1,"grants":[`,
		"unknown member": `{"schema":1,"grants":[],"extra":1}`,
		"unknown action": `{"schema":1,"grants":[{"id":"g-0a0a0a0a","user":"u","uid":1,"action":"host.apply","stack":"s","created_at":"2030-01-01T00:00:00Z","created_by":0}]}`,
		"bad id":         `{"schema":1,"grants":[{"id":"x","user":"u","uid":1,"action":"update.check","stack":"s","created_at":"2030-01-01T00:00:00Z","created_by":0}]}`,
		"bad expiry":     `{"schema":1,"grants":[{"id":"g-0a0a0a0a","user":"u","uid":1,"action":"update.check","stack":"s","expires":"soon","created_at":"2030-01-01T00:00:00Z","created_by":0}]}`,
		"same grant twice": `{"schema":1,"grants":[` + strings.Repeat(`{"id":"g-0a0a0a0a","user":"u","uid":1,"action":"update.check","stack":"s","created_at":"2030-01-01T00:00:00Z","created_by":0},`, 1) +
			`{"id":"g-0b0b0b0b","user":"u","uid":1,"action":"update.check","stack":"s","created_at":"2030-01-01T00:00:00Z","created_by":0}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, fileName)
			formatnumtest.Seed(t, path, doc)
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.List(); err == nil {
				t.Fatal("List accepted the file")
			}
			if _, _, err := s.Upsert(grant("example-admin", 4100, "stack.restart", "demo-stack")); err == nil {
				t.Fatal("Upsert wrote over the file")
			}
			if _, err := s.Remove("g-0a0a0a0a"); err == nil || errors.Is(err, ErrNotFound) {
				t.Fatalf("Remove = %v", err)
			}
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}

func TestUnknownFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, doc := range formatnumtest.Cases("schema", fileSchema, `"grants":[]`) {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, fileName)
			formatnumtest.Seed(t, path, doc)
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.List()
			formatnumtest.Refused(t, err, path)
			_, _, err = s.Upsert(grant("example-admin", 4100, "stack.restart", "demo-stack"))
			formatnumtest.Refused(t, err, path)
			_, err = s.Remove("g-0a0a0a0a")
			formatnumtest.Refused(t, err, path)
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}

func TestInvalidReasons(t *testing.T) {
	known := func(s string) bool { return s == "demo-stack" }
	names := map[uint32]string{4100: "example-admin"}
	nameOf := func(uid uint32) (string, bool) { n, ok := names[uid]; return n, ok }
	base := grant("example-admin", 4100, "stack.restart", "demo-stack")
	at := func(s string) time.Time { ts, _ := time.Parse(time.RFC3339, s); return ts }
	for _, tc := range []struct {
		name string
		edit func(*control.Grant)
		now  time.Time
		want string
	}{
		{"valid without expiry", func(*control.Grant) {}, at("2040-01-01T00:00:00Z"), ""},
		{"valid on the last day", func(g *control.Grant) { g.Expires = "2031-05-06" }, at("2031-05-06T23:59:59Z"), ""},
		{"expired at the end of the day in UTC", func(g *control.Grant) { g.Expires = "2031-05-06" }, at("2031-05-07T00:00:00Z"), control.GrantExpired},
		{"stack gone", func(g *control.Grant) { g.Stack = "old-stack" }, at("2031-01-01T00:00:00Z"), control.GrantStackMissing},
		{"name changed", func(g *control.Grant) { g.User = "example-other" }, at("2031-01-01T00:00:00Z"), control.GrantUserChanged},
		{"user gone", func(g *control.Grant) { g.UID = 4999 }, at("2031-01-01T00:00:00Z"), control.GrantUserChanged},
		{"expiry named first", func(g *control.Grant) { g.Expires, g.Stack = "2031-01-01", "old-stack" }, at("2032-01-01T00:00:00Z"), control.GrantExpired},
	} {
		g := base
		tc.edit(&g)
		if got := Invalid(g, tc.now, known, nameOf); got != tc.want {
			t.Errorf("%s: Invalid = %q, want %q", tc.name, got, tc.want)
		}
	}
}
