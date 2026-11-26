package service_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const envCompose = "services:\n  app:\n    image: registry.example.test/demo/web:${WEB_VERSION}\n" +
	"    env_file: [app.env, link.env, hard.env, tracked.env]\n    volumes:\n      - ./state.json:/state.json\n" +
	"      - ./data:/var/lib/demo\nsecrets:\n  token:\n    file: ./token.txt\n"

// lockedEnvTree writes a stack whose env files and neighbours the service cannot read, and returns its folder.
func lockedEnvTree(t *testing.T) string {
	t.Helper()
	dir := newFolder(t)
	writeTree(t, dir, map[string]string{
		"web/compose.yaml": envCompose, "web/.env": "WEB_VERSION=1.2.3\n",
		"web/secrets.env": "A=1\n", "web/app.env": "A=1\n", "web/tracked.env": "A=1\n", "web/token.txt": "A=1\n",
		"web/state.json": "{}\n", "web/hard.env": "A=1\n", "web/target.txt": "A=1\n", "web/data/mounted.env": "A=1\n",
	})
	web := filepath.Join(dir, "web")
	if err := os.Remove(filepath.Join(web, "hard.env")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(web, "target.txt"), filepath.Join(web, "hard.env")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(web, "target.txt"), filepath.Join(web, "link.env")); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "web/compose.yaml", "web/.env", "web/tracked.env")
	for _, f := range []string{"secrets.env", "app.env", "tracked.env", "token.txt", "state.json", "target.txt", "data/mounted.env"} {
		if err := os.Chmod(filepath.Join(web, f), 0); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestInspectListsUnversionedEnvFilesTheServiceCannotRead(t *testing.T) {
	notRoot(t)
	h := realGitService(t)
	dir := lockedEnvTree(t)
	res := h.inspect(dir, "")
	if res.Case != control.SourceCaseLocal {
		t.Fatalf("inspect = %+v", res)
	}
	// Left out: the tracked file, the Compose secret, the writable mount, the link, the file with a further hard
	// link and a file below a bind-mount source.
	if want := []string{"web/app.env", "web/secrets.env"}; !slices.Equal(res.Access.UnreadableEnvFiles, want) {
		t.Fatalf("unreadable env files = %q, want %q", res.Access.UnreadableEnvFiles, want)
	}
	for _, f := range res.Access.UnreadableEnvFiles {
		if info, err := os.Lstat(filepath.Join(dir, f)); err != nil || info.Mode().Perm() != 0 {
			t.Fatalf("%s changed: %v, %v", f, info, err)
		}
	}
}

func TestInspectListsNothingForReadableEnvFiles(t *testing.T) {
	h := realGitService(t)
	dir := lockedEnvTree(t)
	for _, f := range []string{"secrets.env", "app.env"} {
		if err := os.Chmod(filepath.Join(dir, "web", f), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if res := h.inspect(dir, ""); len(res.Access.UnreadableEnvFiles) != 0 {
		t.Fatalf("unreadable env files = %q", res.Access.UnreadableEnvFiles)
	}
}

func TestInspectOfAFolderOfFilesListsAnUnreadableSecretsEnv(t *testing.T) {
	notRoot(t)
	h := realGitService(t)
	dir := newFolder(t)
	writeTree(t, dir, map[string]string{
		"web/compose.yaml": envCompose, "web/.env": "WEB_VERSION=1.2.3\n", "web/secrets.env": "A=1\n", "web/app.env": "A=1\n",
	})
	if err := os.Chmod(filepath.Join(dir, "web", "secrets.env"), 0); err != nil {
		t.Fatal(err)
	}
	res := h.inspect(dir, "")
	if res.Case != control.SourceCaseFiles {
		t.Fatalf("inspect = %+v", res)
	}
	// app.env is readable and so part of the first commit, which the folder permissions of source add cover.
	if want := []string{"web/secrets.env"}; !slices.Equal(res.Access.UnreadableEnvFiles, want) {
		t.Fatalf("unreadable env files = %q, want %q", res.Access.UnreadableEnvFiles, want)
	}
}
