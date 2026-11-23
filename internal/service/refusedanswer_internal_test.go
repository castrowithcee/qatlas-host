package service

import (
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

func TestAnswersNameWhatTheHookOfARefusingRemotePrinted(t *testing.T) {
	refused := &gitrepo.RemoteRejectedError{Reason: "(pre-receive hook declined)", Output: "only maintainers push\x1b[2J here"}
	for name, got := range map[string]error{"commit": commitAnswer(refused), "revert": revertAnswer(refused)} {
		msg := got.Error()
		if !strings.Contains(msg, "(pre-receive hook declined)") || !strings.Contains(msg, "remote output: only maintainers push [2J here") {
			t.Errorf("%s answer = %q", name, msg)
		}
		if strings.ContainsRune(msg, 0x1b) {
			t.Errorf("%s answer keeps a control character: %q", name, msg)
		}
	}
	plain := commitAnswer(&gitrepo.RemoteRejectedError{Reason: "(hook declined)"}).Error()
	if strings.Contains(plain, "remote output") {
		t.Errorf("answer without output = %q", plain)
	}
}
