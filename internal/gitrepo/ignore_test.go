package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func block(lines ...string) string {
	return ignoreBegin + "\n" + strings.Join(lines, "\n") + "\n" + ignoreEnd + "\n"
}

// entries records mode, mtime and content of everything below dir except .git and the folder itself, whose
// mtime moves with its .git. A file the test cannot read has no content.
func entries(t *testing.T, dir string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" && d.IsDir() {
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil || p == dir {
			return err
		}
		b, _ := os.ReadFile(p)
		out[p] = fileState{info.Mode(), info.ModTime(), string(b)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func wantNoGit(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git: %v", err)
	}
}

// wantStatusEmpty checks the status the engine reads, which honours the ignore rules.
func wantStatusEmpty(t *testing.T, cfg Config, dir string) {
	t.Helper()
	r, err := Open(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := r.Status(context.Background())
	if err != nil || len(changes) != 0 {
		t.Fatalf("status = %+v, %v", changes, err)
	}
}

func TestInitExtendsAnExistingGitignoreInItsOwnBlock(t *testing.T) {
	ctx := context.Background()
	own := "# kept by the operator\r\n*.swp\n!keep.swp"
	dir := initFolder(t, map[string]string{".gitignore": own, "compose.yaml": "a\n", "data/db.bin": "x", "notes.txt": "n\n"})
	if err := os.Chmod(filepath.Join(dir, ".gitignore"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(ctx, Config{Local: true}, dir, []string{".gitignore", "compose.yaml"}, []string{"data", "notes.txt"}); err != nil {
		t.Fatal(err)
	}
	want := own + "\n" + block("/data", "/notes.txt")
	if got := readFile(t, filepath.Join(dir, ".gitignore")); got != want {
		t.Fatalf(".gitignore = %q, want %q", got, want)
	}
	if info, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, %v", info.Mode(), err)
	}
	if got := fixtureGit(t, dir, "show", "HEAD:.gitignore"); got+"\n" != want {
		t.Fatalf("committed .gitignore = %q", got)
	}
	if got := fixtureGit(t, dir, "ls-tree", "-r", "--name-only", "HEAD"); got != ".gitignore\ncompose.yaml" {
		t.Fatalf("tree = %q", got)
	}
	wantClean(t, &Repo{dir: dir})
	wantStatusEmpty(t, Config{Local: true}, dir)

	// Taken over again, the block is extended and never repeated; the lines before and after it stay.
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	writeTree(t, dir, map[string]string{"app.log": "l\n", ".gitignore": want + "# after\n"})
	if _, err := Init(ctx, Config{Local: true}, dir, []string{".gitignore", "compose.yaml"}, []string{"app.log", "data"}); err != nil {
		t.Fatal(err)
	}
	want = own + "\n" + block("/data", "/notes.txt", "/app.log") + "# after\n"
	if got := readFile(t, filepath.Join(dir, ".gitignore")); got != want {
		t.Fatalf("again: .gitignore = %q, want %q", got, want)
	}
	wantStatusEmpty(t, Config{Local: true}, dir)

	// Nothing new to leave out: the file stays as it is.
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	before := entries(t, dir)
	if _, err := Init(ctx, Config{Local: true}, dir, []string{".gitignore", "compose.yaml"}, []string{"data"}); err != nil {
		t.Fatal(err)
	}
	wantSame(t, "unchanged", before, entries(t, dir))
	wantStatusEmpty(t, Config{Local: true}, dir)
}

func TestInitWithoutExclusionsCreatesNoGitignore(t *testing.T) {
	dir := initFolder(t, map[string]string{"compose.yaml": "a\n"})
	if _, err := Init(context.Background(), Config{Local: true}, dir, []string{"compose.yaml"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf(".gitignore: %v", err)
	}
	wantStatusEmpty(t, Config{Local: true}, dir)
}

func TestInitRefusesAGitignoreItCannotExtendBeforeAnyChange(t *testing.T) {
	ctx := context.Background()
	for name, setup := range map[string]func(t *testing.T, dir string){
		"link": func(t *testing.T, dir string) {
			if err := os.Symlink("compose.yaml", filepath.Join(dir, ".gitignore")); err != nil {
				t.Fatal(err)
			}
		},
		"folder": func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, ".gitignore"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"large": func(t *testing.T, dir string) {
			writeTree(t, dir, map[string]string{".gitignore": strings.Repeat("x", 1<<20+1)})
		},
		"damaged": func(t *testing.T, dir string) {
			writeTree(t, dir, map[string]string{".gitignore": "*.swp\n" + ignoreBegin + "\n/data\n"})
		},
		"two blocks": func(t *testing.T, dir string) {
			writeTree(t, dir, map[string]string{".gitignore": block("/a") + block("/b")})
		},
		"unreadable": func(t *testing.T, dir string) {
			if os.Geteuid() == 0 {
				t.Skip("root reads every file")
			}
			writeTree(t, dir, map[string]string{".gitignore": "*.swp\n"})
			if err := os.Chmod(filepath.Join(dir, ".gitignore"), 0); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := initFolder(t, map[string]string{"compose.yaml": "a\n", "notes.txt": "n\n"})
			setup(t, dir)
			before := entries(t, dir)
			_, err := Init(ctx, Config{Local: true}, dir, []string{"compose.yaml"}, []string{"notes.txt"})
			var bad *IgnoreError
			if !errors.As(err, &bad) {
				t.Fatalf("err = %v", err)
			}
			wantSame(t, name, before, entries(t, dir))
			wantNoGit(t, dir)
			if _, err := PlanIgnore(dir, nil); !errors.As(err, &bad) {
				t.Fatalf("plan without exclusions: %v", err)
			}
		})
	}
}

func TestInitLeavesTheGitignoreAsItWasAfterAFailure(t *testing.T) {
	ctx := context.Background()
	dir := initFolder(t, map[string]string{".gitignore": "*.swp\n", "a.txt": "a\n", "notes.txt": "n\n"})
	before := entries(t, dir)
	// A file that is not there is refused before anything is written.
	if _, err := Init(ctx, Config{Local: true}, dir, []string{".gitignore", "a.txt", "gone.txt"}, []string{"notes.txt"}); err == nil {
		t.Fatal("no error")
	}
	wantSame(t, "failed", before, entries(t, dir))
	wantNoGit(t, dir)

	// The file changed after it was planned: it is not overwritten.
	p, err := PlanIgnore(dir, []string{"notes.txt"})
	if err != nil || !p.Write || !p.Exists || strings.Join(p.Added, ",") != "/notes.txt" {
		t.Fatalf("plan = %+v, %v", p, err)
	}
	writeTree(t, dir, map[string]string{".gitignore": "*.tmp\n"})
	var bad *IgnoreError
	if err := p.write(dir); !errors.As(err, &bad) {
		t.Fatalf("write = %v", err)
	}
	if got := readFile(t, filepath.Join(dir, ".gitignore")); got != "*.tmp\n" {
		t.Fatalf(".gitignore = %q", got)
	}

	// A file that appeared after a plan to create one is not overwritten either.
	if err := os.Remove(filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	p, err = PlanIgnore(dir, []string{"notes.txt"})
	if err != nil || !p.Write || p.Exists {
		t.Fatalf("plan = %+v, %v", p, err)
	}
	writeTree(t, dir, map[string]string{".gitignore": "*.tmp\n"})
	if err := p.write(dir); err == nil {
		t.Fatal("overwrote a new .gitignore")
	}
	if got := readFile(t, filepath.Join(dir, ".gitignore")); got != "*.tmp\n" {
		t.Fatalf(".gitignore = %q", got)
	}
}
