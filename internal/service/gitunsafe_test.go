package service_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/service"
)

func TestUnsafeGitIsAFindingOfTheSourceAndAnErrorCode(t *testing.T) {
	h := startWith(t, &fakeDeployer{})
	h.withKey()
	unsafe := fmt.Errorf("fetch: %w", &gitrepo.UnsafeError{Reason: ".git can be written by a group or by others"})
	h.git.SetFetchErrFor(h.dir, unsafe)
	if _, err := h.set(h.srv.Key().Fingerprint); err != nil {
		t.Fatal(err)
	}
	st := h.waitStatus("the finding is shown", func(st control.StatusResult) bool {
		return len(st.Engine.Sources) == 1 && st.Engine.Sources[0].GitUnsafe != ""
	})
	if got := st.Engine.Sources[0].GitUnsafe; !strings.Contains(got, ".git can be written") || len(st.Engine.Stacks) != 0 {
		t.Fatalf("finding = %q, stacks = %+v", got, st.Engine.Stacks)
	}
}

func TestDirectCommandsAnswerGitUnsafe(t *testing.T) {
	err := fmt.Errorf("status failed: %w", &gitrepo.UnsafeError{Reason: ".git is a link"})
	for name, answer := range map[string]func(error) error{"runs": service.GitUnsafeAnswer, "changes": service.ChangesAnswer} {
		got := answer(err)
		var ce *control.Error
		if !errors.As(got, &ce) || ce.Code != control.CodeGitUnsafe || !strings.Contains(ce.Message, ".git is a link") {
			t.Fatalf("%s: answer = %v", name, got)
		}
	}
	if service.GitUnsafeAnswer(errors.New("other")) != nil {
		t.Fatal("another error was answered as git-unsafe")
	}
}
