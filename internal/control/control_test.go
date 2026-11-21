package control

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadFrame(t *testing.T) {
	br := bufio.NewReaderSize(strings.NewReader("one\r\ntwo\n"+strings.Repeat("x", 10000)+"\nlast"), 16)
	for _, want := range []string{"one", "two", strings.Repeat("x", 10000)} {
		got, err := ReadFrame(br, 20000)
		if err != nil || string(got) != want {
			t.Fatalf("ReadFrame = %.20q, %v; want %.20q", got, err, want)
		}
	}
	if _, err := ReadFrame(br, 20000); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("incomplete frame = %v", err)
	}
	if _, err := ReadFrame(br, 20000); !errors.Is(err, io.EOF) {
		t.Fatalf("end = %v", err)
	}
}

func TestReadFrameLimit(t *testing.T) {
	if _, err := ReadFrame(bufio.NewReader(strings.NewReader(strings.Repeat("x", 11)+"\n")), 10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("over the limit = %v", err)
	}
	if got, err := ReadFrame(bufio.NewReader(strings.NewReader(strings.Repeat("x", 10)+"\n")), 10); err != nil || len(got) != 10 {
		t.Fatalf("at the limit = %q, %v", got, err)
	}
}

func TestWriteFrameLimit(t *testing.T) {
	var b bytes.Buffer
	if err := WriteFrame(&b, Request{V: Version, Command: strings.Repeat("a", 100)}, 50); !errors.Is(err, ErrTooLarge) || b.Len() != 0 {
		t.Fatalf("WriteFrame = %v, wrote %d", err, b.Len())
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	h := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	read, write := Command{Class: ClassRead, Scope: ScopeDeploy, Handle: h}, Command{Class: ClassChange, Scope: ScopeDeploy, Handle: h}
	if err := r.Register("status", read); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("stacks.list", write); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "Status", "a b", "a..b", ".a", "a."} {
		if r.Register(bad, read) == nil {
			t.Errorf("name %q accepted", bad)
		}
	}
	if r.Register("status", read) == nil || r.Register("other", Command{Class: ClassRead, Scope: ScopeDeploy}) == nil {
		t.Error("duplicate or nil handler accepted")
	}
	for _, unmarked := range []Command{
		{Handle: h}, {Class: 4, Scope: ScopeDeploy, Handle: h}, {Class: ClassRead, Handle: h}, {Class: ClassRead, Scope: 3, Handle: h},
		{Class: ClassRead, Scope: ScopeStack, Handle: h}, {Class: ClassRead, Scope: ScopeDeploy, Stack: one, Handle: h},
	} {
		if r.Register("unmarked", unmarked) == nil {
			t.Errorf("command %+v accepted", unmarked)
		}
	}
	if r.Register("raised", Command{Class: ClassChange, Scope: ScopeDeploy, Handle: h, Writes: func(json.RawMessage) bool { return true }}) == nil {
		t.Error("a write command with Writes accepted")
	}
	if c, ok := r.Lookup("status"); !ok || c.Class != ClassRead {
		t.Error("registered command not found")
	}
	if _, ok := r.Lookup("missing"); ok {
		t.Error("unknown command found")
	}
	if got := strings.Join(r.Names(), ","); got != "stacks.list,status" {
		t.Errorf("names = %s", got)
	}
	if got := r.Entries(); len(got) != 2 || got[0] != (Entry{"stacks.list", ClassChange, ScopeDeploy}) || got[1] != (Entry{"status", ClassRead, ScopeDeploy}) {
		t.Errorf("entries = %+v", got)
	}
}

func one(args json.RawMessage) []string {
	var a struct{ Stack string }
	if json.Unmarshal(args, &a) != nil {
		return nil
	}
	return []string{a.Stack}
}

func TestAccessForRaisesOnlyAReadWithASideEffect(t *testing.T) {
	h := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	refresh := func(args json.RawMessage) bool { return string(args) != `{}` }
	deploy := func(c Class) Access { return Access{Class: c, Scope: ScopeDeploy} }
	for _, tc := range []struct {
		c    Command
		args string
		want Access
	}{
		{Command{Class: ClassRead, Scope: ScopeDeploy, Handle: h}, `{"refresh":true}`, deploy(ClassRead)},
		{Command{Class: ClassRead, Scope: ScopeDeploy, Handle: h, Writes: refresh}, `{}`, deploy(ClassRead)},
		{Command{Class: ClassRead, Scope: ScopeDeploy, Handle: h, Writes: refresh}, `{"refresh":true}`, deploy(ClassChange)},
		{Command{Class: ClassChange, Scope: ScopeDeploy, Handle: h}, `{}`, deploy(ClassChange)},
		{Command{Class: ClassPropose, Scope: ScopeDeploy, Handle: h}, `{}`, deploy(ClassPropose)},
		{Command{Handle: h}, `{}`, deploy(ClassChange)},
		{Command{Class: ClassChange, Scope: ScopeStack, Stack: one, Handle: h}, `{"stack":"demo"}`, Access{ClassChange, ScopeStack, "demo"}},
		{Command{Class: ClassChange, Scope: ScopeStack, Stack: one, Handle: h}, `{}`, deploy(ClassChange)},
		{Command{Class: ClassRead, Scope: ScopeStack, Stack: one, Handle: h, Writes: refresh}, `{"stack":"demo","x":1}`, Access{ClassChange, ScopeStack, "demo"}},
	} {
		if got := tc.c.AccessFor(json.RawMessage(tc.args)); got != tc.want {
			t.Errorf("access of %+v with %s = %+v, want %+v", tc.c.Class, tc.args, got, tc.want)
		}
	}
}

func TestStackDirUsesTheFolderOfTheListedName(t *testing.T) {
	r := StacksResult{Dir: "/srv/stacks", Stacks: []StackInfo{
		{Name: "demo-project", Folder: "demo-folder"},
		{Name: "escape", Folder: "../etc"},
		{Name: "nested", Folder: "a/b"},
		{Name: "empty"},
	}}
	if got, ok := r.StackDir("demo-project"); !ok || got != "/srv/stacks/demo-folder" {
		t.Fatalf("StackDir(demo-project) = %q, %v", got, ok)
	}
	for _, name := range []string{"demo-folder", "escape", "nested", "empty", "missing"} {
		if got, ok := r.StackDir(name); ok {
			t.Errorf("StackDir(%s) = %q, want none", name, got)
		}
	}
	if _, ok := (StacksResult{Stacks: r.Stacks}).StackDir("demo-project"); ok {
		t.Error("StackDir without Dir answered")
	}
}

func TestPrimaryEntryOfAName(t *testing.T) {
	collision := []FindingInfo{{Kind: FindingNameCollision}}
	list := []StackStatus{
		{Name: "demo", Source: "a", Findings: collision},
		{Name: "demo", Source: "b", Removed: true},
		{Name: "demo", Source: "c"},
		{Name: "web", Source: "a", Findings: collision},
		{Name: "web", Source: "b", Findings: collision},
	}
	want := []bool{false, false, true, true, false}
	for i, w := range want {
		if got := Primary(list, i); got != w {
			t.Errorf("Primary(%d) = %v, want %v", i, got, w)
		}
	}
	if !Primary(list[1:2], 0) {
		t.Error("a single entry does not stand for its name")
	}
}

func TestStackDirUsesTheSourceOfTheHoldingStack(t *testing.T) {
	r := StacksResult{Dir: "/srv/first", Stacks: []StackInfo{
		{Name: "demo", Folder: "demo", SourceDir: "/srv/second", Collision: "taken"},
		{Name: "demo", Folder: "demo", SourceDir: "/srv/first"},
		{Name: "web", Folder: "web", SourceDir: "/srv/second"},
	}}
	if got, ok := r.StackDir("demo"); !ok || got != "/srv/first/demo" {
		t.Fatalf("demo = %q, %v", got, ok)
	}
	if got, ok := r.StackDir("web"); !ok || got != "/srv/second/web" {
		t.Fatalf("web = %q, %v", got, ok)
	}
}

func TestStackDirOfARootStackIsTheStackFolder(t *testing.T) {
	r := StacksResult{Stacks: []StackInfo{{Name: "demo-root", Folder: ".", SourceDir: "/srv/demo-root"}}}
	if got, ok := r.StackDir("demo-root"); !ok || got != "/srv/demo-root" {
		t.Fatalf("StackDir(demo-root) = %q, %v", got, ok)
	}
}
