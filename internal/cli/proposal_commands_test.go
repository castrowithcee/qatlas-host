package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func TestProposalCommandsSwitchNeedsRoot(t *testing.T) {
	startFakeService(t, fakeHandlers{})
	svc := startFakeHostService(t, fakeHandlers{control.HostCommandsSetCommand: func(json.RawMessage) any { return control.ErrNotRoot() }})
	for _, verb := range []string{"enable", "disable"} {
		code, _, stderr := run(t, "proposal", "commands", verb)
		if code != 1 || !strings.Contains(stderr, "sudo") {
			t.Fatalf("%s: code %d, stderr %q", verb, code, stderr)
		}
	}
	if svc.count(control.HostCommandsSetCommand) != 2 {
		t.Fatalf("the service was asked %d times", svc.count(control.HostCommandsSetCommand))
	}
}

func TestProposalCommandsSwitchSendsTheChoice(t *testing.T) {
	var got []bool
	startFakeService(t, fakeHandlers{})
	startFakeHostService(t, fakeHandlers{control.HostCommandsSetCommand: func(raw json.RawMessage) any {
		var a control.HostCommandsSetArgs
		_ = json.Unmarshal(raw, &a)
		got = append(got, a.Enabled)
		return control.HostCommandsResult{Enabled: a.Enabled}
	}})
	for _, verb := range []string{"enable", "disable"} {
		if code, _, stderr := run(t, "proposal", "commands", verb, "--output", "json"); code != 0 {
			t.Fatalf("%s: code %d, stderr %q", verb, code, stderr)
		}
	}
	if len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("sent %v", got)
	}
}

func TestProposalCommandsStatusJSON(t *testing.T) {
	startFakeService(t, fakeHandlers{})
	startFakeHostService(t, fakeHandlers{control.HostCommandsStatusCommand: func(json.RawMessage) any {
		return control.HostCommandsResult{Enabled: true}
	}})
	code, stdout, stderr := run(t, "proposal", "commands", "status", "--output", "json")
	if got := objectOf(t, stdout); code != 0 || got["enabled"] != true {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestProposalSubmitCommandKindNamesTheSwitchWhenOff(t *testing.T) {
	startFakeService(t, fakeHandlers{})
	svc := startFakeHostService(t, fakeHandlers{control.ProposalSubmitCommand: func(json.RawMessage) any {
		return control.NewError(control.CodeCommandsDisabled, "proposals of commands are switched off")
	}})
	withStdin(t, strings.NewReader(`{"commands":["true"],"reason":"example"}`))
	code, _, stderr := run(t, "proposal", "submit", "--kind", "command")
	if code != 1 || !strings.Contains(stderr, "sudo qhost proposal commands enable") || svc.count(control.ProposalSubmitCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

const hostileCommand = "echo evil\x1b]0;title\x07\x1b[2J\u202e\r\nrm"

func showCommandProposal(json.RawMessage) any {
	info := proposalInfo("h-0123456789abcdef", control.ProposalOpen)
	info.Kind = control.ProposalKindCommand
	content, _ := json.Marshal(control.ProposalCommand{Commands: []string{"touch /srv/example/ready", hostileCommand}, Reason: "why\x1b[31m"})
	return control.ProposalShowResult{ProposalInfo: info, Content: content}
}

func TestProposalShowCommandNamesNoticeAndMarksForeignText(t *testing.T) {
	startFakeService(t, fakeHandlers{})
	startFakeHostService(t, fakeHandlers{control.ProposalShowCommand: showCommandProposal})
	for _, format := range []string{"table", "compact"} {
		code, stdout, stderr := run(t, "proposal", "show", "h-0123456789abcdef", "--output", format)
		if code != 0 {
			t.Fatalf("%s: code %d, stderr %q", format, code, stderr)
		}
		for _, r := range stdout {
			if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f || r == 0x202e {
				t.Fatalf("%s: output holds %U:\n%q", format, r, stdout)
			}
		}
		for _, want := range []string{"unchecked, no computed effect, no undo", "untrusted_command", "touch /srv/example/ready", "untrusted_reason", "\uFFFD"} {
			if !strings.Contains(strings.ToLower(stdout), strings.ToLower(want)) {
				t.Fatalf("%s: %q is missing:\n%s", format, want, stdout)
			}
		}
	}
}

func TestProposalShowCommandJSONCarriesTheContent(t *testing.T) {
	startFakeService(t, fakeHandlers{})
	startFakeHostService(t, fakeHandlers{control.ProposalShowCommand: showCommandProposal})
	code, stdout, stderr := run(t, "proposal", "show", "h-0123456789abcdef", "--output", "json")
	var got struct {
		Notice  string `json:"notice"`
		Content struct {
			Commands []string `json:"commands"`
			Reason   string   `json:"reason"`
		} `json:"content"`
		Untrusted bool `json:"content_untrusted"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil {
		t.Fatalf("code %d, stdout %q, stderr %q: %v", code, stdout, stderr, err)
	}
	if got.Notice != "unchecked, no computed effect, no undo" || !got.Untrusted || len(got.Content.Commands) != 2 ||
		got.Content.Commands[1] != hostileCommand || got.Content.Reason != "why\x1b[31m" {
		t.Fatalf("got %+v", got)
	}
}
