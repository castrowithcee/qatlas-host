package stacks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const outsideBody = "OUT_KEY=keep-me\n"

// linkCase builds a stack whose folder or secrets.env involves a link to outside and returns the path an error
// must name.
type linkCase struct {
	name string
	// readGate marks a case the shared read of secrets.env refuses before any write.
	readGate bool
	build    func(t *testing.T, base, outside string) (st Stack, named string)
}

var linkCases = []linkCase{
	{"folder of the stack is a link", false, func(t *testing.T, base, outside string) (Stack, string) {
		link := filepath.Join(base, "src", "app")
		mustLink(t, outside, link)
		return Stack{Name: "app", Folder: "app", Dir: link}, link
	}},
	{"source folder is a link", false, func(t *testing.T, base, outside string) (Stack, string) {
		mustWrite(t, filepath.Join(outside, "app", SecretsFile), outsideBody)
		link := filepath.Join(base, "link")
		mustLink(t, outside, link)
		return Stack{Name: "app", Folder: "app", Dir: filepath.Join(link, "app")}, link
	}},
	{"root stack folder is a link", false, func(t *testing.T, base, outside string) (Stack, string) {
		link := filepath.Join(base, "link")
		mustLink(t, outside, link)
		return Stack{Name: "src", Folder: RootFolder, Dir: link}, link
	}},
	{"secrets.env is a link", true, func(t *testing.T, base, outside string) (Stack, string) {
		dir := filepath.Join(base, "src", "app")
		mustWrite(t, filepath.Join(dir, "keep"), "")
		mustLink(t, filepath.Join(outside, "target"), filepath.Join(dir, SecretsFile))
		return Stack{Name: "app", Folder: "app", Dir: dir}, filepath.Join(dir, SecretsFile)
	}},
}

// A link on any level from the stack folder down refuses the write before it has an effect, names the path and
// leaves the outside folder as it was. "write" is the write itself, as when the link appears after the read.
func TestSecretWritesFollowNoLink(t *testing.T) {
	for _, c := range linkCases {
		for _, op := range []string{"set", "delete", "write"} {
			t.Run(c.name+"/"+op, func(t *testing.T) {
				base := t.TempDir()
				outside := filepath.Join(base, "outside")
				mustWrite(t, filepath.Join(outside, "target"), outsideBody)
				mustWrite(t, filepath.Join(base, "src", "keep"), "")
				st, named := c.build(t, base, outside)
				err := runSecretOp(st, op)
				if err == nil || !strings.Contains(err.Error(), wantNamed(c, op, named)) {
					t.Fatalf("error %v does not refuse and name %s", err, named)
				}
				checkOutsideKept(t, outside)
			})
		}
	}
}

func runSecretOp(st Stack, op string) error {
	switch op {
	case "set":
		_, err := SetSecret(st, "OUT_KEY", fakeSecretValue)
		return err
	case "delete":
		return DeleteSecret(st, "OUT_KEY")
	}
	return writeSecretLines(st, []string{"OUT_KEY=changed"})
}

func wantNamed(c linkCase, op, named string) string {
	if c.readGate && op != "write" {
		return SecretsFile // the read of the existing file refuses first
	}
	return named
}

func checkOutsideKept(t *testing.T, outside string) {
	t.Helper()
	for _, p := range []string{"target", filepath.Join("app", SecretsFile)} {
		if b, err := os.ReadFile(filepath.Join(outside, p)); err == nil && string(b) != outsideBody {
			t.Fatalf("%s changed: %q", p, b)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) > 2 {
		t.Fatalf("outside gained files: %v", entries)
	}
	if sub, _ := os.ReadDir(filepath.Join(outside, "app")); len(sub) > 1 {
		t.Fatalf("files created below the link target: %v", sub)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustLink(t *testing.T, to, at string) {
	t.Helper()
	if err := os.Symlink(to, at); err != nil {
		t.Fatal(err)
	}
}
