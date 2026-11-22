package engine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

func changesRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t, "web", "db")
	compose := "services:\n  app:\n    image: registry.example.test/demo/app:${APP_VERSION}\n" +
		"    volumes:\n      - ./uploads:/srv/uploads\n"
	if err := os.WriteFile(filepath.Join(r.wc, "web", "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	r.deployAll()
	return r
}

func TestChangesListAllStacksWithKindAndPathBelowTheFolder(t *testing.T) {
	r := changesRig(t)
	r.copy.set(func(f *fakeCopy) {
		f.changes = []gitrepo.Change{
			{Code: "??", Path: "web/notes.txt"}, {Code: " M", Path: "web/.env"}, {Code: " D", Path: "web/old/compose.yaml"},
			{Code: "A ", Path: "web/added.yaml"}, {Code: "M ", Path: "db/.env"},
			// a folder that is no stack lists too, without a stack
			{Code: " D", Path: "gone/compose.yaml"},
			// the host's own files and the files of the host's mounts do not count
			{Code: "??", Path: "web/secrets.env"}, {Code: "??", Path: "web/data/x.db"}, {Code: "??", Path: "web/uploads/a.jpg"},
			// a file outside every stack folder is no change that classify counts for a stack
			{Code: "??", Path: "README.md"},
		}
	})
	got, err := r.eng.Changes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{"db/.env", ChangeModified, "db"}, {"gone/compose.yaml", ChangeDeleted, ""}, {"web/.env", ChangeModified, "web"},
		{"web/added.yaml", ChangeNew, "web"}, {"web/notes.txt", ChangeNew, "web"}, {"web/old/compose.yaml", ChangeDeleted, "web"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("changes = %+v, want %+v", got, want)
	}
}

func TestDiffAndDiscardRefuseWhatIsNotAReportedChange(t *testing.T) {
	r := changesRig(t)
	r.copy.set(func(f *fakeCopy) {
		f.changes = []gitrepo.Change{{Code: " M", Path: "web/.env"}, {Code: " M", Path: "db/.env"},
			{Code: "??", Path: "web/secrets.env"}, {Code: "??", Path: "web/data/x.db"}, {Code: "??", Path: "web/uploads/a.jpg"}}
	})
	for _, p := range []string{"", ".", "..", "../web/.env", "/web/.env", "./web/.env", "web//.env", "web/../web/.env", "web/secrets.env",
		"web/data/x.db", "web/uploads/a.jpg", "web", "web/gone", "web/.env\x00", ".env"} {
		if _, _, err := r.eng.Diff(t.Context(), p); err != ErrNotChanged {
			t.Errorf("diff %q = %v", p, err)
		}
		if err := r.eng.Discard(t.Context(), []string{"web/.env", p}); err != ErrNotChanged {
			t.Errorf("discard %q = %v", p, err)
		}
	}
	// nothing was discarded, not even the valid path next to an invalid one
	if strings.Contains(strings.Join(r.copy.calls, " "), "discard") {
		t.Fatalf("calls = %v", r.copy.calls)
	}
	if text, truncated, err := r.eng.Diff(t.Context(), "web/.env"); err != nil || truncated || text != "diff of web/.env" {
		t.Fatalf("diff = %q, %v, %v", text, truncated, err)
	}
}

func TestDiscardTakesBackOnlyTheNamedFilesAndTheFindingsGo(t *testing.T) {
	r := changesRig(t)
	r.copy.set(func(f *fakeCopy) {
		f.changes = []gitrepo.Change{{Code: " M", Path: "web/.env"}, {Code: "??", Path: "web/notes.txt"}, {Code: " M", Path: "db/.env"}}
		f.remote = commit(2)
		f.folders[[2]string{commit(1), commit(2)}] = []string{"web"}
	})
	if res := r.poll(); len(res.Started) != 0 || !hasFinding(r.stack("web"), FindingLocalChanges) {
		t.Fatalf("poll = %+v, web = %+v", res, r.stack("web"))
	}
	if err := r.eng.Discard(t.Context(), []string{"web/.env", "db/.env"}); err != nil {
		t.Fatal(err)
	}
	got, _ := r.eng.Changes(t.Context())
	if !slices.Equal(got, []Change{{"web/notes.txt", ChangeNew, "web"}}) {
		t.Fatalf("after the first discard: %+v", got)
	}
	if err := r.eng.Discard(t.Context(), []string{"web/notes.txt"}); err != nil {
		t.Fatal(err)
	}
	res := r.poll()
	if hasFinding(r.stack("web"), FindingLocalChanges) || hasFinding(r.stack("db"), FindingLocalChanges) || !res.FastForwarded {
		t.Fatalf("res = %+v, web = %+v, db = %+v", res, r.stack("web"), r.stack("db"))
	}
}

func TestDiscardDuringARunIsRefused(t *testing.T) {
	r := changesRig(t)
	r.copy.set(func(f *fakeCopy) {
		f.changes = []gitrepo.Change{{Code: " M", Path: "web/.env"}, {Code: " M", Path: "db/.env"}}
	})
	release, err := r.eng.claim("db", "run")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// the run of db refuses the discard of files of both stacks, and nothing is taken back
	if err := r.eng.Discard(t.Context(), []string{"web/.env", "db/.env"}); err != ErrRunActive {
		t.Fatalf("discard = %v", err)
	}
	if strings.Contains(strings.Join(r.copy.calls, " "), "discard") {
		t.Fatalf("calls = %v", r.copy.calls)
	}
}
