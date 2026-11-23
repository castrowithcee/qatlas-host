package engine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

func commitRig(t *testing.T) *rig {
	r := changesRig(t)
	r.copy.set(func(f *fakeCopy) {
		f.changes = []gitrepo.Change{{Code: " M", Path: "web/.env"}, {Code: " D", Path: "web/old.txt"}, {Code: " M", Path: "db/.env"}}
	})
	return r
}

func TestCommitChangesCommitsTheSortedPathsAndNamesThePerson(t *testing.T) {
	r := commitRig(t)
	c, err := r.eng.CommitChanges(t.Context(), commit(1), []string{"web/old.txt", "web/.env"}, "alice (1000)")
	if err != nil || c != commit(3001) {
		t.Fatalf("commit = %q, %v", c, err)
	}
	want := pathsCall{commit(1), "Commit web/.env, web/old.txt by alice (1000)", []string{"web/.env", "web/old.txt"}}
	if got := r.copy.pathsCalls; len(got) != 1 || got[0].Head != want.Head || got[0].Message != want.Message || !slices.Equal(got[0].Paths, want.Paths) {
		t.Fatalf("calls = %+v, want %+v", got, want)
	}
	if got, _ := r.eng.Changes(t.Context()); !slices.Equal(got, []Change{{"db/.env", ChangeModified, "db"}}) {
		t.Fatalf("changes after the commit: %+v", got)
	}
}

func TestCommitChangesRefusesAListThatIsNotExactlyChangesAndAStackThatDoesNotLoad(t *testing.T) {
	r := commitRig(t)
	for _, p := range []string{"web/secrets.env", "web/missing.txt", "../db/.env", "", "web"} {
		if _, err := r.eng.CommitChanges(t.Context(), commit(1), []string{"web/.env", p}, "x"); err != ErrNotChanged {
			t.Errorf("commit with %q = %v", p, err)
		}
	}
	if err := os.WriteFile(filepath.Join(r.wc, "web", "compose.yaml"), []byte("services: [broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.copy.set(func(f *fakeCopy) { f.changes = append(f.changes, gitrepo.Change{Code: " M", Path: "web/compose.yaml"}) })
	if _, err := r.eng.CommitChanges(t.Context(), commit(1), []string{"db/.env", "web/compose.yaml"}, "x"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("a stack that does not load: %v", err)
	}
	if len(r.copy.pathsCalls) != 0 {
		t.Fatalf("committed anyway: %+v", r.copy.pathsCalls)
	}
}

func TestCommitChangesDuringARunIsRefused(t *testing.T) {
	r := commitRig(t)
	release, err := r.eng.claim("db", "run")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := r.eng.CommitChanges(t.Context(), commit(1), []string{"web/.env", "db/.env"}, "x"); err != ErrRunActive {
		t.Fatalf("commit = %v", err)
	}
	if len(r.copy.pathsCalls) != 0 {
		t.Fatalf("committed anyway: %+v", r.copy.pathsCalls)
	}
}

func TestCommitChangesRepeatsOnceAfterARejectedPushOnTheNewRemoteHead(t *testing.T) {
	r := commitRig(t)
	r.copy.set(func(f *fakeCopy) { f.remote, f.pathsErrs = commit(2), []error{gitrepo.ErrPushRejected} })
	c, err := r.eng.CommitChanges(t.Context(), commit(1), []string{"web/.env"}, "x")
	if err != nil || c != commit(3002) {
		t.Fatalf("commit = %q, %v", c, err)
	}
	calls := r.copy.pathsCalls
	if len(calls) != 2 || calls[0].Head != commit(1) || calls[1].Head != commit(2) || !slices.Equal(r.copy.ffTo, []string{commit(2)}) {
		t.Fatalf("calls = %+v, fast-forwards = %v", calls, r.copy.ffTo)
	}
}

func TestCommitChangesGivesUpWhenTheRepeatCannotKeepTheSameChanges(t *testing.T) {
	for name, set := range map[string]func(*fakeCopy){
		"rejected twice": func(f *fakeCopy) {
			f.remote, f.pathsErrs = commit(2), []error{gitrepo.ErrPushRejected, gitrepo.ErrPushRejected}
		},
		"the remote is not ahead": func(f *fakeCopy) {
			f.pathsErrs = []error{gitrepo.ErrPushRejected}
		},
		"the change is gone after the catch-up": func(f *fakeCopy) {
			f.remote, f.pathsErrs = commit(2), []error{gitrepo.ErrPushRejected}
			f.onFetch = func(f *fakeCopy) {
				f.changes = slices.DeleteFunc(f.changes, func(c gitrepo.Change) bool { return c.Path == "web/.env" })
			}
		},
		"the catch-up is refused": func(f *fakeCopy) {
			f.remote, f.pathsErrs, f.ffErr = commit(2), []error{gitrepo.ErrPushRejected}, errors.New("would overwrite")
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := commitRig(t)
			r.copy.set(set)
			_, err := r.eng.CommitChanges(t.Context(), commit(1), []string{"web/.env"}, "x")
			// the vanished change is not a push problem, it is a list that no longer matches
			if !errors.Is(err, gitrepo.ErrPushRejected) && !(strings.HasPrefix(name, "the change") && errors.Is(err, ErrNotChanged)) {
				t.Fatalf("commit = %v", err)
			}
		})
	}
}

func TestCommitChangesPassesAMovedHeadOn(t *testing.T) {
	r := commitRig(t)
	r.copy.set(func(f *fakeCopy) { f.pathsErrs = []error{gitrepo.ErrHeadMoved} })
	if _, err := r.eng.CommitChanges(t.Context(), commit(9), []string{"web/.env"}, "x"); !errors.Is(err, gitrepo.ErrHeadMoved) {
		t.Fatalf("commit = %v", err)
	}
	if len(r.copy.pathsCalls) != 1 {
		t.Fatalf("calls = %+v", r.copy.pathsCalls)
	}
}

func TestCommitChangesLeavesTheNextPollFreeToFastForwardACommitFromOutside(t *testing.T) {
	r := commitRig(t)
	c, err := r.eng.CommitChanges(t.Context(), commit(1), []string{"db/.env", "web/.env", "web/old.txt"}, "x")
	if err != nil {
		t.Fatal(err)
	}
	r.copy.set(func(f *fakeCopy) { f.remote = commit(4) })
	res := r.poll()
	if !res.FastForwarded || res.Head != commit(4) || !slices.Equal(r.copy.ffTo, []string{commit(4)}) {
		t.Fatalf("poll after the commit %s = %+v, fast-forwards = %v", c, res, r.copy.ffTo)
	}
	if snap, _ := r.eng.Snapshot(); snap.State.Note != "" || snap.State.PollError != "" {
		t.Fatalf("note = %q, poll error = %q", snap.State.Note, snap.State.PollError)
	}
}
