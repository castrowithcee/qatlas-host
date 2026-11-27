package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/output"
)

func TestFailureOfNotFoundHintsFitTheObject(t *testing.T) {
	for _, tc := range []struct{ code, hint string }{
		{control.CodeNoSuchKey, "qhost key generate"},
		{control.CodeNoSuchCredential, "qhost credential list"},
		{control.CodeNoSuchSecret, "qhost stack secret list <stack>"},
		{control.CodeNoSuchSource, "qhost source list"},
		{control.CodeNoSuchStack, "qhost stack list"},
		{control.CodeNotFound, ""},
	} {
		t.Run(tc.code, func(t *testing.T) {
			var f *cli.Failure
			if !errors.As(failureOf(control.NewError(tc.code, "gone")), &f) {
				t.Fatal("not a failure")
			}
			if f.Code != output.CodeNotFound {
				t.Fatalf("class %q", f.Code)
			}
			if tc.hint == "" && f.NextStep != "" || tc.hint != "" && !strings.Contains(f.NextStep, tc.hint) {
				t.Fatalf("next step %q, want %q", f.NextStep, tc.hint)
			}
			if tc.code != control.CodeNoSuchKey && strings.Contains(f.NextStep, "key generate") {
				t.Fatalf("next step %q names the key", f.NextStep)
			}
		})
	}
}

func TestFailureOfSourceCommitRefusalsKeepTheirCode(t *testing.T) {
	for _, code := range []string{control.CodeHeadMoved, control.CodeNotChanged, control.CodePushRejected} {
		t.Run(code, func(t *testing.T) {
			var f *cli.Failure
			if !errors.As(failureOf(control.NewError(code, "nothing was committed")), &f) {
				t.Fatal("not a failure")
			}
			if f.Code != output.Code(code) || f.Message != "nothing was committed" {
				t.Fatalf("code %q, message %q", f.Code, f.Message)
			}
			if !strings.Contains(f.NextStep, "qhost source changes <path>") || strings.Contains(f.NextStep, "same version") {
				t.Fatalf("next step %q", f.NextStep)
			}
		})
	}
}

func TestFailureOfNoSuchKeyAddsNoSecondHintWhenTheMessageNamesTheCommand(t *testing.T) {
	for msg, want := range map[string]string{
		"no deploy key exists": "qhost key generate",
		"no deploy key exists for stack folder /srv/a; run 'qhost key generate --dir /srv/a' to create one": "",
	} {
		var f *cli.Failure
		if !errors.As(failureOf(control.NewError(control.CodeNoSuchKey, msg)), &f) {
			t.Fatal("not a failure")
		}
		if want == "" && f.NextStep != "" || want != "" && !strings.Contains(f.NextStep, want) {
			t.Fatalf("%q: next step %q", msg, f.NextStep)
		}
	}
}

func TestStackCheckUnknownStackNamesStackList(t *testing.T) {
	startFakeService(t, fakeHandlers{control.StacksCheckCommand: func(json.RawMessage) any {
		return control.NewError(control.CodeNoSuchStack, "no such stack in the working copy")
	}})
	code, _, stderr := run(t, "stack", "check", "unknown")
	if code == 0 || !strings.Contains(stderr, "not-found") || !strings.Contains(stderr, "qhost stack list") || strings.Contains(stderr, "key generate") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSourceRemoteUnknownFolderNamesSourceList(t *testing.T) {
	useFolderHost(t)
	fake := &remoteFake{compare: differentCompare(control.SourceCaseLocal),
		refusal: control.NewError(control.CodeNoSuchSource, "no source is bound to this folder")}
	startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))
	code, _, stderr := run(t, "source", "remote", t.TempDir(), remoteURL, "--fingerprint", remoteKeyPrint, "--yes")
	if code == 0 || !strings.Contains(stderr, "not-found") || !strings.Contains(stderr, "qhost source list") || strings.Contains(stderr, "key generate") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestFailureOfRefusalHintFitsTheCode(t *testing.T) {
	for _, tc := range []struct{ code, want string }{
		{control.CodeInternal, "journalctl -u qhost-deploy"},
		{control.CodeVersion, "same version"},
		{control.CodeUnknownCommand, "same version"},
		{control.CodeBadRequest, "run the command again"},
		{control.CodeTooLarge, "run the command again"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			var f *cli.Failure
			if !errors.As(failureOf(control.NewError(tc.code, "refused")), &f) {
				t.Fatal("not a failure")
			}
			if !strings.Contains(f.NextStep, tc.want) {
				t.Fatalf("next step %q, want %q", f.NextStep, tc.want)
			}
			sameVersion := strings.Contains(f.NextStep, "same version")
			if sameVersion != (tc.code == control.CodeVersion || tc.code == control.CodeUnknownCommand) {
				t.Fatalf("next step %q", f.NextStep)
			}
		})
	}
}
