package tui

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	channelHookRow = 3
	channelMailRow = 4
)

// channelBackend adds two channel proposals below those of kindsBackend: a webhook and an SMTP channel with a
// user, whose address fields are long.
func channelBackend() *fakeBackend {
	b := kindsBackend()
	info := func(id string, hour int) control.ProposalInfo {
		s := at(30, hour, 0)
		return control.ProposalInfo{ID: id, Kind: control.ProposalKindNotifyChannel, State: control.ProposalOpen, Fingerprint: "fp-" + id,
			SubmitterUID: 1001, SubmitterName: "ana", Submitted: s, Expires: s.Add(control.ProposalLifetime)}
	}
	b.proposals = append(b.proposals, info("p-0013", 15), info("p-0014", 14))
	content := func(c control.ProposalNotifyChannel) string {
		raw, _ := json.Marshal(c)
		return string(raw)
	}
	b.proposalContents["p-0013"] = content(control.ProposalNotifyChannel{Name: "deploy-hook", Type: control.NotifyWebhook,
		Events: []string{control.NotifyEventFailed, control.NotifyEventRolledBack}})
	b.proposalContents["p-0014"] = content(control.ProposalNotifyChannel{Name: "alerts", Type: control.NotifySMTP,
		Host: "mail." + strings.Repeat("long-label.", 12) + "example.test", Port: 2525, Security: "tls", User: "qd",
		From: "qhost@example.test", To: []string{"ops@example.test", "dev@example.test"}})
	return b
}

// approveChannelSteps goes to the question of approving the webhook proposal with both secret parts typed.
func approveChannelSteps(t *testing.T, m *Model) {
	t.Helper()
	hidden := []string{hiddenA, hiddenB}
	sendHidden(t, m, hidden, "A")
	contains(t, m, "Secret parts of the channel", "Webhook URL", "Signing key")
	sendHidden(t, m, hidden, typed(hiddenA)...)
	sendHidden(t, m, hidden, "down")
	sendHidden(t, m, hidden, typed(hiddenB)...)
	sendHidden(t, m, hidden, "f2")
	contains(t, m, "Approve proposal p-0013 and add the channel deploy-hook (webhook)?")
}

func TestChannelViewShowsEveryFieldInFullAndNoSecret(t *testing.T) {
	b := channelBackend()
	m := proposalOpen(t, b, true, channelMailRow)
	var all strings.Builder
	for _, l := range m.proposalLines(116) {
		all.WriteString(strings.TrimSpace(l))
	}
	for _, want := range []string{"notification channel", "name: \"alerts\"", "type: \"smtp\"", "host: \"mail." + strings.Repeat("long-label.", 12) + "example.test\"",
		"port: 2525", "security: \"tls\"", "user: \"qd\"", "from: \"qhost@example.test\"", "to[0]: \"ops@example.test\"", "to[1]: \"dev@example.test\"",
		"no channel of this name exists now"} {
		if !strings.Contains(all.String(), strings.ReplaceAll(want, " ", "")) && !strings.Contains(all.String(), want) {
			t.Fatalf("the view lacks %q:\n%s", want, all.String())
		}
	}
	contains(t, m, "approve", "reject")
	if len(b.proposalActs) != 0 || b.called("notifications") < 1 {
		t.Fatalf("calls %v, notification reads %d", b.proposalActs, b.called("notifications"))
	}
}

func TestChannelViewWithoutRootReadsAndOffersTheKeys(t *testing.T) {
	b := channelBackend()
	m := proposalOpen(t, b, false, channelHookRow)
	contains(t, m, "notification channel", "deploy-hook", "no channel of this name exists now", "approve", "reject")
}

func TestChannelApproveAsksForTheSecretsThenAddsAndKeepsTheResult(t *testing.T) {
	b := channelBackend()
	m := proposalOpen(t, b, true, channelHookRow)
	hidden := []string{hiddenA, hiddenB}
	approveChannelSteps(t, m)
	if len(b.proposalActs) != 0 || len(b.adds) != 0 {
		t.Fatal("something changed before the answer")
	}
	sendHidden(t, m, hidden, "y")
	want := []string{"proposalshow:p-0013", "proposalshow:p-0013", "proposalapprove:p-0013", "notifyadd:deploy-hook",
		"proposalresult:p-0013", "proposalshow:p-0013"}
	if got := b.actions(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(b.adds) != 1 {
		t.Fatalf("adds = %+v", b.adds)
	}
	got := b.adds[0]
	if got.Name != "deploy-hook" || got.Type != control.NotifyWebhook || got.Secret != hiddenA || got.Key != hiddenB ||
		!slices.Equal(got.Events, []string{control.NotifyEventFailed, control.NotifyEventRolledBack}) {
		t.Fatalf("add = %+v", got)
	}
	if len(b.proposalResults) != 1 || b.proposalResults[0] != (control.ProposalResultArgs{ID: "p-0013", Fingerprint: "fp-p-0013", Applied: true}) {
		t.Fatalf("results = %+v", b.proposalResults)
	}
	contains(t, m, "The channel deploy-hook is added")
	noLeak(t, m.render(), hidden...)
	if m.set.form != nil || m.modal != nil {
		t.Fatal("the secret form or the dialog was kept")
	}
}

func TestChannelApproveRefusesAChangedProposalOrAChannelOfTheName(t *testing.T) {
	for name, change := range map[string]func(b *fakeBackend){
		"proposal": func(b *fakeBackend) { b.proposals[3].Fingerprint = "fp-other" },
		"channel": func(b *fakeBackend) {
			b.notify = append(b.notify, control.NotifyChannelInfo{Name: "deploy-hook", Type: control.NotifySlack})
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := channelBackend()
			m := proposalOpen(t, b, true, channelHookRow)
			approveChannelSteps(t, m)
			change(b)
			sendHidden(t, m, []string{hiddenA, hiddenB}, "y")
			if len(b.proposalApproves) != 0 || len(b.adds) != 0 || len(b.proposalResults) != 0 {
				t.Fatalf("something was applied: %v %v", b.proposalActs, b.adds)
			}
			if !strings.Contains(m.failure, "changed since it was shown; nothing was applied") {
				t.Fatalf("failure = %q", m.failure)
			}
			if !m.pv.open || m.set.form != nil {
				t.Fatal("the view was left or the secret form kept")
			}
			noLeak(t, m.render(), hiddenA, hiddenB)
		})
	}
}

func TestChannelOfAnExistingNameCannotBeApproved(t *testing.T) {
	b := channelBackend()
	b.notify = append(b.notify, control.NotifyChannelInfo{Name: "deploy-hook", Type: control.NotifySlack})
	m := proposalOpen(t, b, true, channelHookRow)
	contains(t, m, "A channel named deploy-hook exists now", "reject")
	if strings.Contains(m.render(), "A approve") {
		t.Fatal("approve offered")
	}
	send(m, "A")
	if m.modal != nil || m.set.form != nil || len(b.proposalActs) != 0 {
		t.Fatal("approving went on")
	}
}

func TestChannelFailedAddIsKeptAsFailedResultWithoutTheSecret(t *testing.T) {
	b := channelBackend()
	b.errs["notifyadd:deploy-hook"] = control.NewError(control.CodeInvalid, "cannot reach "+hiddenA+" with "+hiddenB)
	m := proposalOpen(t, b, true, channelHookRow)
	approveChannelSteps(t, m)
	sendHidden(t, m, []string{hiddenA, hiddenB}, "y")
	if len(b.proposalResults) != 1 || b.proposalResults[0].Applied || b.proposalResults[0].Message != "cannot reach *** with ***" {
		t.Fatalf("results = %+v", b.proposalResults)
	}
	if len(b.proposalApproves) != 1 || len(b.adds) != 0 {
		t.Fatalf("approves %v adds %v", b.proposalApproves, b.adds)
	}
	if !strings.Contains(m.failure, "cannot reach ***") || strings.Contains(m.failure, hiddenA) || strings.Contains(m.failure, hiddenB) {
		t.Fatalf("failure = %q", m.failure)
	}
	if m.set.form != nil {
		t.Fatal("the secret form was kept")
	}
	noLeak(t, m.render(), hiddenA, hiddenB)

	// An error that is no service error is scrubbed the same way.
	b = channelBackend()
	b.errs["notifyadd:deploy-hook"] = errors.New("boom " + hiddenA)
	m = proposalOpen(t, b, true, channelHookRow)
	approveChannelSteps(t, m)
	send(m, "y")
	if len(b.proposalResults) != 1 || strings.Contains(b.proposalResults[0].Message, hiddenA) {
		t.Fatalf("results = %+v", b.proposalResults)
	}
}

func TestChannelSecretsAreCheckedLikeTheChannelForm(t *testing.T) {
	b := channelBackend()
	m := proposalOpen(t, b, true, channelHookRow)
	send(m, "A")
	send(m, "a", "down", "b", "f2")
	contains(t, m, "4 to 8192 characters")
	if m.modal != nil || len(b.proposalActs) != 0 {
		t.Fatal("asked with a value that is too short")
	}
}

func TestChannelSMTPAsksForThePasswordOnlyWithAUser(t *testing.T) {
	b := channelBackend()
	m := proposalOpen(t, b, true, channelMailRow)
	send(m, "A")
	contains(t, m, "Password")
	sendHidden(t, m, []string{hiddenA}, typed(hiddenA)...)
	sendHidden(t, m, []string{hiddenA}, "f2", "y")
	if len(b.adds) != 1 || b.adds[0].Secret != hiddenA || b.adds[0].Key != "" || b.adds[0].Host != "mail."+strings.Repeat("long-label.", 12)+"example.test" {
		t.Fatalf("adds = %+v", b.adds)
	}

	b = channelBackend()
	raw, _ := json.Marshal(control.ProposalNotifyChannel{Name: "alerts", Type: control.NotifySMTP, Host: "mail.example.test",
		From: "qhost@example.test", To: []string{"ops@example.test"}})
	b.proposalContents["p-0014"] = string(raw)
	m = proposalOpen(t, b, true, channelMailRow)
	send(m, "A")
	contains(t, m, "Approve proposal p-0014 and add the channel alerts (smtp)?")
	if m.set.form != nil {
		t.Fatal("a form without a field")
	}
	send(m, "y")
	if len(b.adds) != 1 || b.adds[0].Secret != "" {
		t.Fatalf("adds = %+v", b.adds)
	}
}

func TestChannelProposalRejectWorks(t *testing.T) {
	b := channelBackend()
	m := proposalOpen(t, b, true, channelHookRow)
	send(m, "N", "f2", "y")
	if len(b.proposalRejects) != 1 || b.proposalRejects[0].ID != "p-0013" || len(b.proposalApproves) != 0 || len(b.adds) != 0 {
		t.Fatalf("rejects = %+v", b.proposalRejects)
	}
}

func TestChannelProposalShowsNoControlCharacters(t *testing.T) {
	b := channelBackend()
	b.proposalContents["p-0014"] = `{"name":"alerts","type":"smtp","host":"mail\u001b[31m.example.test\u0007","user":"qd"}`
	m := proposalOpen(t, b, true, channelMailRow)
	if screen := m.render(); strings.Contains(screen, "\x1b[31m") || strings.Contains(screen, "\x07") {
		t.Fatalf("control characters reach the screen:\n%q", screen)
	}
	contains(t, m, `mail\u001b[31m.example.test\u0007`)
}

func TestGoldenProposalsViewNotifyChannel(t *testing.T) {
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m := proposalModel(t, lang, channelBackend(), true, dim[0], dim[1])
				send(m, "6")
				for range channelMailRow {
					send(m, "down")
				}
				send(m, "enter")
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				checkGolden(t, "proposals.view.channel."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}
