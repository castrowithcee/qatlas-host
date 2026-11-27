package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// unsafeCopy is a working copy whose .git the service does not trust.
type unsafeCopy struct {
	*fakeCopy
	err error
}

func (u unsafeCopy) CheckGit() error { return u.err }

func TestUnsafeGitStopsThePollUntilItIsFixed(t *testing.T) {
	r := newRig(t, "web")
	r.copy.fetchErr = &gitrepo.UnsafeError{Reason: ".git can be written by a group or by others"}
	if _, err := r.eng.Poll(context.Background()); !errors.Is(err, gitrepo.ErrGitUnsafe) {
		t.Fatalf("err = %v", err)
	}
	snap, _ := r.eng.Snapshot()
	if snap.State.GitUnsafe == "" || len(r.dep.got()) != 0 || len(snap.State.Stacks) != 0 {
		t.Fatalf("state = %+v, runs = %v", snap.State, r.dep.got())
	}
	r.copy.fetchErr = nil
	r.poll()
	if snap, _ := r.eng.Snapshot(); snap.State.GitUnsafe != "" || len(r.dep.got()) != 1 {
		t.Fatalf("finding stays or no run: %+v, runs = %v", snap.State, r.dep.got())
	}
}

func TestUnsafeGitRefusesAManualRun(t *testing.T) {
	r := newRig(t, "web")
	u := unsafeCopy{fakeCopy: r.copy, err: &gitrepo.UnsafeError{Reason: ".git is a link"}}
	r.cfg.Source = func() (Binding, error) { return Binding{Dir: r.wc, Copy: u}, nil }
	r.eng = r.start()
	if _, err := r.eng.Trigger(context.Background(), "web"); !errors.Is(err, gitrepo.ErrGitUnsafe) {
		t.Fatalf("err = %v", err)
	}
	r.eng.Wait()
	if len(r.dep.got()) != 0 {
		t.Fatalf("a run started: %v", r.dep.got())
	}
}
