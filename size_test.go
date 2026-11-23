package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// Size limits for Go code. Existing overruns are frozen in the list file; the list only shrinks.
const (
	maxFileLines     = 500
	maxTestFileLines = 1000
	maxFuncLines     = 60
	maxComplexity    = 15

	sizeListPath = "testdata/size-baseline.txt"
	permanentTag = "permanent:"
	sizeHint     = "the list never grows: split the code, or run `just size-update` to lower or drop entries"
	sizeListHead = "# Frozen size overruns: <kind> <key> <value>, kinds file (lines), len (function lines), cx (complexity).\n" +
		"# Entries marked permanent are data tables that may grow. Rewritten only by `just size-update`.\n"
)

var updateSize = flag.Bool("update", false, "lower the size list to the current values and drop resolved entries")

// sizeEntry is one frozen overrun: a value, or a permanent exemption with its reason.
type sizeEntry struct {
	value  int
	reason string // set for permanent exemptions
}

func TestSizeLimits(t *testing.T) {
	measured, err := measureTree(os.DirFS("."))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(sizeListPath)
	if err != nil {
		t.Fatal(err)
	}
	list, err := parseSizeList(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if *updateSize {
		list = lowerSizeList(measured, list)
		if err := os.WriteFile(sizeListPath, []byte(formatSizeList(list)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range sizeViolations(measured, list) {
		t.Error(v)
	}
}

// skipSizeDir reports whether the go tool would not look into the directory either.
func skipSizeDir(path string) bool {
	if path == "." {
		return false
	}
	name := path[strings.LastIndex(path, "/")+1:]
	return name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// measureTree returns every value over its limit, keyed "<kind> <path>[:<function>]".
func measureTree(fsys fs.FS) (map[string]int, error) {
	out := map[string]int{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipSizeDir(path) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		return measureFile(out, path, src)
	})
	return out, err
}

func measureFile(out map[string]int, path string, src []byte) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return err
	}
	if ast.IsGenerated(f) {
		return nil
	}
	limit := maxFileLines
	if strings.HasSuffix(path, "_test.go") {
		limit = maxTestFileLines
	}
	if n := strings.Count(string(src), "\n"); n > limit {
		out["file "+path] = n
	}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		key := path + ":" + funcName(fd)
		n := fset.Position(fd.End()).Line - fset.Position(fd.Pos()).Line + 1
		recordMax(out, "len "+key, n, maxFuncLines)
		recordMax(out, "cx "+key, complexity(fd), maxComplexity)
	}
	return nil
}

// recordMax keeps the larger value when a key repeats, as with several init functions in one file.
func recordMax(out map[string]int, key string, n, limit int) {
	if n > limit && n > out[key] {
		out[key] = n
	}
}

func funcName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	t := fd.Recv.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		case *ast.Ident:
			return x.Name + "." + fd.Name.Name
		default:
			return "?." + fd.Name.Name
		}
	}
}

// complexity counts like gocyclo: 1 plus branches, non-default cases and short-circuit operators.
func complexity(fd *ast.FuncDecl) int {
	c := 1
	ast.Inspect(fd, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			c++
		case *ast.CaseClause:
			if x.List != nil {
				c++
			}
		case *ast.CommClause:
			if x.Comm != nil {
				c++
			}
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				c++
			}
		}
		return true
	})
	return c
}

func limitOf(key string) int {
	kind, path, _ := strings.Cut(key, " ")
	switch {
	case kind == "len":
		return maxFuncLines
	case kind == "cx":
		return maxComplexity
	case strings.HasSuffix(path, "_test.go"):
		return maxTestFileLines
	}
	return maxFileLines
}

func sizeViolations(measured map[string]int, list map[string]sizeEntry) []string {
	var out []string
	for key, n := range measured {
		e, ok := list[key]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s: %d, limit %d, not in the list; %s", key, n, limitOf(key), sizeHint))
		case e.reason == "" && n > e.value:
			out = append(out, fmt.Sprintf("%s: %d, list value %d (limit %d); %s", key, n, e.value, limitOf(key), sizeHint))
		}
	}
	for key, e := range list {
		if _, ok := measured[key]; !ok {
			out = append(out, fmt.Sprintf("%s: listed at %s but missing or within the limit %d; %s",
				key, entryText(e), limitOf(key), sizeHint))
		}
	}
	sort.Strings(out)
	return out
}

// lowerSizeList lowers values to the measured ones and drops resolved entries. It never adds.
func lowerSizeList(measured map[string]int, list map[string]sizeEntry) map[string]sizeEntry {
	out := map[string]sizeEntry{}
	for key, e := range list {
		n, ok := measured[key]
		if !ok {
			continue
		}
		if e.reason == "" && n < e.value {
			e.value = n
		}
		out[key] = e
	}
	return out
}

func entryText(e sizeEntry) string {
	if e.reason != "" {
		return permanentTag + " " + e.reason
	}
	return strconv.Itoa(e.value)
}

func formatSizeList(list map[string]sizeEntry) string {
	lines := make([]string, 0, len(list))
	for key, e := range list {
		lines = append(lines, key+" "+entryText(e))
	}
	sort.Strings(lines)
	return sizeListHead + strings.Join(lines, "\n") + "\n"
}

func parseSizeList(text string) (map[string]sizeEntry, error) {
	out := map[string]sizeEntry{}
	for i, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("size list line %d: want \"<kind> <key> <value>\"", i+1)
		}
		key := parts[0] + " " + parts[1]
		if reason, ok := strings.CutPrefix(parts[2], permanentTag+" "); ok {
			out[key] = sizeEntry{reason: reason}
			continue
		}
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("size list line %d: bad value %q", i+1, parts[2])
		}
		out[key] = sizeEntry{value: n}
	}
	return out, nil
}

// sizeSrc builds a Go file of the given kind of overrun.
func sizeSrc(lines int) string {
	return "package p\n" + strings.Repeat("var _ = 1\n", lines-1)
}

func funcSrc(name string, body int) string {
	return "package p\n\nfunc " + name + "() {\n" + strings.Repeat("\t_ = 1\n", body) + "}\n"
}

func branchySrc(name string, ifs int) string {
	return "package p\n\nfunc " + name + "(a bool) {\n" + strings.Repeat("\tif a {\n\t}\n", ifs) + "}\n"
}

func checkTree(t *testing.T, files map[string]string, list string) []string {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, src := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(src)}
	}
	measured, err := measureTree(fsys)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := parseSizeList(list)
	if err != nil {
		t.Fatal(err)
	}
	return sizeViolations(measured, entries)
}

func wantViolation(t *testing.T, got []string, substr string) {
	t.Helper()
	if len(got) != 1 || !strings.Contains(got[0], substr) || !strings.Contains(got[0], "just size-update") {
		t.Fatalf("want one violation naming %q and the update hint, got %q", substr, got)
	}
}

func TestSizeRulesRed(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		list  string
		want  string
	}{
		{"new long file", map[string]string{"a.go": sizeSrc(501)}, "", "file a.go: 501"},
		{"new long function", map[string]string{"a.go": funcSrc("F", 60)}, "", "len a.go:F: 62"},
		{"new complex function", map[string]string{"a.go": branchySrc("F", 15)}, "", "cx a.go:F: 16"},
		{"grown entry", map[string]string{"a.go": funcSrc("F", 70)}, "len a.go:F 65\n", "len a.go:F: 72, list value 65"},
		{"vanished entry", map[string]string{"a.go": funcSrc("F", 3)}, "len a.go:F 65\n", "len a.go:F: listed at 65"},
		{"entry for a missing file", map[string]string{}, "file gone.go 600\n", "file gone.go: listed at 600"},
		{"test file over 1000 lines", map[string]string{"a_test.go": sizeSrc(1001)}, "", "file a_test.go: 1001, limit 1000"},
		{"vanished permanent entry", map[string]string{"a.go": funcSrc("F", 3)}, "len a.go:F permanent: data table\n", "listed at permanent: data table"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { wantViolation(t, checkTree(t, c.files, c.list), c.want) })
	}
}

func TestSizeRulesGreen(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		list  string
	}{
		{"test file between 500 and 1000 lines", map[string]string{"a_test.go": sizeSrc(900)}, ""},
		{"generated file", map[string]string{"a.go": "// Code generated by x. DO NOT EDIT.\n\n" + funcSrc("F", 100)}, ""},
		{"frozen entry unchanged", map[string]string{"a.go": funcSrc("F", 70)}, "len a.go:F 72\n"},
		{"frozen entry shrunk", map[string]string{"a.go": funcSrc("F", 70)}, "len a.go:F 90\n"},
		{"permanent entry grows", map[string]string{"a.go": funcSrc("F", 200)}, "len a.go:F permanent: data table\n"},
		{"function limits hold in test files", map[string]string{"a_test.go": funcSrc("F", 58)}, ""},
		{"skipped directories", map[string]string{"vendor/a.go": sizeSrc(900), "testdata/a.go": sizeSrc(900), "_x/a.go": sizeSrc(900), ".x/a.go": sizeSrc(900)}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checkTree(t, c.files, c.list); len(got) != 0 {
				t.Fatalf("want green, got %q", got)
			}
		})
	}
}

func TestSizeUpdateOnlyLowers(t *testing.T) {
	measured := map[string]int{"len a.go:F": 70, "len a.go:G": 80, "len a.go:New": 99, "cx a.go:P": 40}
	list := map[string]sizeEntry{
		"len a.go:F":    {value: 90},
		"len a.go:G":    {value: 80},
		"len a.go:Gone": {value: 70},
		"cx a.go:P":     {reason: "data table"},
	}
	got := lowerSizeList(measured, list)
	want := map[string]sizeEntry{"len a.go:F": {value: 70}, "len a.go:G": {value: 80}, "cx a.go:P": {reason: "data table"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, w := range want {
		if got[k] != w {
			t.Fatalf("%s: got %v, want %v", k, got[k], w)
		}
	}
	text := formatSizeList(got)
	back, err := parseSizeList(text)
	if err != nil || formatSizeList(back) != text {
		t.Fatalf("list does not round-trip: %v\n%s", err, text)
	}
}

func TestSizeCountsLikeGocyclo(t *testing.T) {
	src := "package p\n\nfunc F(a, b bool, c chan int, x int) {\n" +
		"\tif a && b || a {\n\t}\n\tfor range c {\n\t}\n" +
		"\tswitch x {\n\tcase 1:\n\tcase 2, 3:\n\tdefault:\n\t}\n" +
		"\tselect {\n\tcase <-c:\n\tdefault:\n\t}\n" +
		"\tfunc() {\n\t\tif a {\n\t\t}\n\t}()\n}\n"
	f, err := parser.ParseFile(token.NewFileSet(), "a.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 1 + if + && + || + range + 2 cases + comm case + literal's if
	if got := complexity(f.Decls[0].(*ast.FuncDecl)); got != 9 {
		t.Fatalf("complexity %d, want 9", got)
	}
}

func TestSizeKeyNamesReceivers(t *testing.T) {
	src := "package p\n\ntype T[K any] struct{}\n\nfunc (t *T[K]) M() {}\nfunc (T[K]) N() {}\nfunc F() {}\n"
	f, err := parser.ParseFile(token.NewFileSet(), "a.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range f.Decls[1:] {
		got = append(got, funcName(d.(*ast.FuncDecl)))
	}
	if strings.Join(got, " ") != "T.M T.N F" {
		t.Fatalf("got %v", got)
	}
}
