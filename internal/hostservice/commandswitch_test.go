package hostservice

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func commandSubmit(t *testing.T, content any) control.ProposalSubmitArgs {
	t.Helper()
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	return control.ProposalSubmitArgs{Kind: control.ProposalKindCommand, Content: raw}
}

func validCommands() control.ProposalCommand {
	return control.ProposalCommand{Commands: []string{"touch /srv/example/ready"}, Reason: "the example folder needs a marker"}
}

func setSwitch(t *testing.T, e env, peer *atomic.Uint32, uid uint32, enabled bool) error {
	t.Helper()
	peer.Store(uid)
	return e.do(control.HostCommandsSetCommand, control.HostCommandsSetArgs{Enabled: enabled}, nil)
}

func TestCommandProposalIsRefusedWhileTheSwitchIsOff(t *testing.T) {
	var peer atomic.Uint32
	peer.Store(4100)
	e := startAs(t, &peer)
	var status control.HostCommandsResult
	if err := e.do(control.HostCommandsStatusCommand, nil, &status); err != nil || status.Enabled {
		t.Fatalf("status = %+v, %v", status, err)
	}
	err := e.do(control.ProposalSubmitCommand, commandSubmit(t, validCommands()), nil)
	wantCode(t, err, control.CodeCommandsDisabled)
	if !strings.Contains(err.Error(), "sudo qhost proposal commands enable") {
		t.Fatalf("the message does not name the switch: %v", err)
	}
	// The switch is checked before the content, so an invalid one gets the same answer.
	wantCode(t, e.do(control.ProposalSubmitCommand, commandSubmit(t, map[string]any{"nonsense": 1}), nil), control.CodeCommandsDisabled)
}

func TestOnlyRootSwitchesCommandProposals(t *testing.T) {
	var peer atomic.Uint32
	e := startAs(t, &peer)
	wantCode(t, setSwitch(t, e, &peer, 4100, true), control.CodeNotRoot)
	wantCode(t, setSwitch(t, e, &peer, 4102, true), control.CodeNotAdmin)
	if err := setSwitch(t, e, &peer, 0, true); err != nil {
		t.Fatal(err)
	}
	peer.Store(4100)
	var status control.HostCommandsResult
	if err := e.do(control.HostCommandsStatusCommand, nil, &status); err != nil || !status.Enabled {
		t.Fatalf("status = %+v, %v", status, err)
	}
	wantCode(t, setSwitch(t, e, &peer, 4100, false), control.CodeNotRoot)
	peer.Store(0)
	wantCode(t, e.do(control.HostCommandsSetCommand, map[string]any{"enabled": true, "x": 1}, nil), control.CodeBadRequest)
}

func TestCommandProposalChecksItsContent(t *testing.T) {
	var peer atomic.Uint32
	e := startAs(t, &peer)
	if err := setSwitch(t, e, &peer, 0, true); err != nil {
		t.Fatal(err)
	}
	peer.Store(4100)
	many := make([]string, 11)
	for i := range many {
		many[i] = "true"
	}
	cases := map[string]any{
		"no commands":        control.ProposalCommand{Commands: []string{}, Reason: "r"},
		"null commands":      control.ProposalCommand{Reason: "r"},
		"eleven commands":    control.ProposalCommand{Commands: many, Reason: "r"},
		"line break":         control.ProposalCommand{Commands: []string{"true\nfalse"}, Reason: "r"},
		"control character":  control.ProposalCommand{Commands: []string{"true\x1b[2J"}, Reason: "r"},
		"tab":                control.ProposalCommand{Commands: []string{"true\tfalse"}, Reason: "r"},
		"bidi control":       control.ProposalCommand{Commands: []string{"true \u202e"}, Reason: "r"},
		"too long command":   control.ProposalCommand{Commands: []string{strings.Repeat("a", 1001)}, Reason: "r"},
		"empty command":      control.ProposalCommand{Commands: []string{"  "}, Reason: "r"},
		"no reason":          control.ProposalCommand{Commands: []string{"true"}},
		"blank reason":       control.ProposalCommand{Commands: []string{"true"}, Reason: " "},
		"too long reason":    control.ProposalCommand{Commands: []string{"true"}, Reason: strings.Repeat("a", 1001)},
		"unknown field":      map[string]any{"commands": []string{"true"}, "reason": "r", "env": "X=1"},
		"commands not array": map[string]any{"commands": "true", "reason": "r"},
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			wantCode(t, e.do(control.ProposalSubmitCommand, commandSubmit(t, content), nil), control.CodeInvalid)
		})
	}
	edge := control.ProposalCommand{Commands: []string{strings.Repeat("a", 1000), "true"}, Reason: strings.Repeat("ä", 1000)}
	var info control.ProposalInfo
	if err := e.do(control.ProposalSubmitCommand, commandSubmit(t, edge), &info); err != nil || info.Kind != control.ProposalKindCommand ||
		!strings.HasPrefix(info.ID, control.HostProposalIDPrefix) {
		t.Fatalf("edge content: %+v, %v", info, err)
	}
}

func TestSwitchingOffKeepsOpenCommandProposals(t *testing.T) {
	var peer atomic.Uint32
	e := startAs(t, &peer)
	if err := setSwitch(t, e, &peer, 0, true); err != nil {
		t.Fatal(err)
	}
	peer.Store(4100)
	var info control.ProposalInfo
	if err := e.do(control.ProposalSubmitCommand, commandSubmit(t, validCommands()), &info); err != nil {
		t.Fatal(err)
	}
	if err := setSwitch(t, e, &peer, 0, false); err != nil {
		t.Fatal(err)
	}
	peer.Store(4100)
	var shown control.ProposalShowResult
	if err := e.do(control.ProposalShowCommand, control.ProposalIDArgs{ID: info.ID}, &shown); err != nil || shown.State != control.ProposalOpen {
		t.Fatalf("shown = %+v, %v", shown, err)
	}
	var content control.ProposalCommand
	if err := json.Unmarshal(shown.Content, &content); err != nil || content.Reason != validCommands().Reason {
		t.Fatalf("content = %s, %v", shown.Content, err)
	}
	wantCode(t, e.do(control.ProposalSubmitCommand, commandSubmit(t, validCommands()), nil), control.CodeCommandsDisabled)
}
