package tui

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// Without root a proposal is approved and rejected through `sudo qhost proposal …`: the question shows the exact
// command, nothing runs before y, and the service is only read.

// sudoProposalOpen opens the proposal at row as an administrator without root and a fake sudo that prints
// replies[<group> <command>].
func sudoProposalOpen(t *testing.T, lang i18n.Lang, b *fakeBackend, row int, replies map[string]string) (*Model, *fakeRoot) {
	t.Helper()
	return sudoProposalAt(t, lang, b, row, replies, wideWidth, 50)
}

// sudoProposalAt is sudoProposalOpen on a screen of the given size.
func sudoProposalAt(t *testing.T, lang i18n.Lang, b *fakeBackend, row int, replies map[string]string, width, rows int) (*Model, *fakeRoot) {
	t.Helper()
	m := proposalModel(t, lang, b, false, width, rows)
	m.euid = func() int { return 1000 }
	f := &fakeRoot{}
	f.reports = func(argv []string) string {
		if len(argv) < 4 {
			return ""
		}
		return replies[argv[2]+" "+argv[3]]
	}
	m.root = f.hook()
	send(m, "6")
	for range row {
		send(m, "down")
	}
	send(m, "enter")
	return m, f
}

func approveArgsOf(id string, extra ...string) []string {
	args := []string{"proposal", "approve", id, "--fingerprint", "fp-" + id, "--confirm"}
	return append(append(args, extra...), "--output", "json")
}

func TestKindSettingsApproveWithoutRootRunsTheCommand(t *testing.T) {
	b := kindsBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, kindSettingsRow, map[string]string{
		"proposal approve": `{"proposal":"p-0011","stack":"demo","changed":true,"commit":"0123456789abcdef"}`})
	send(m, "A")
	args := approveArgsOf("p-0011")
	says(t, m, "Approve proposal p-0011 and apply these settings to the stack demo?", "These settings would change:", "!! backup.command[0]")
	showsCommand(t, m, args)
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	wantCommands(t, m, f, args)
	if got := b.actions(); slices.ContainsFunc(got, func(a string) bool {
		return strings.HasPrefix(a, "proposalapprove") || strings.HasPrefix(a, "settingsset") || strings.HasPrefix(a, "proposalresult")
	}) {
		t.Fatalf("the service was called directly: %v", got)
	}
	contains(t, m, "The settings of the stack demo are saved")
}

func TestKindWindowApproveWithoutRootRunsTheCommand(t *testing.T) {
	b := kindsBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, kindWindowRow, map[string]string{
		"proposal approve": `{"proposal":"p-0012","days":"mon,thu","start":"02:30","duration":"2h","zone":"UTC","open":false,"next_opening":"2026-11-23T02:30:00Z"}`})
	send(m, "A")
	args := approveArgsOf("p-0012")
	says(t, m, "Approve proposal p-0012 and set this maintenance window?")
	showsCommand(t, m, args)
	send(m, "y")
	wantCommands(t, m, f, args)
	contains(t, m, "maintenance window")
	if m.set.busy || m.modal != nil {
		t.Fatalf("busy %v, modal %v", m.set.busy, m.modal)
	}
}

func TestKindApproveWithoutRootCancelRunsNothing(t *testing.T) {
	for _, row := range []int{kindSettingsRow, kindWindowRow} {
		b := kindsBackend()
		m, f := sudoProposalOpen(t, i18n.English, b, row, nil)
		send(m, "A", "n")
		if len(f.reqs) != 0 || m.set.busy || m.modal != nil {
			t.Fatalf("row %d: requests %+v, busy %v", row, f.reqs, m.set.busy)
		}
		for _, a := range b.actions() {
			if strings.HasPrefix(a, "proposalapprove") || strings.HasPrefix(a, "settingsset") {
				t.Fatalf("the service changed something: %v", b.actions())
			}
		}
	}
}

func TestKindApproveWithoutRootShowsAFailureAndReadsTheProposalAgain(t *testing.T) {
	b := kindsBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, kindSettingsRow, nil)
	f.stderr = "qhost: invalid-request: the proposal is not the one that was shown; nothing was applied\n"
	f.runErr = exitCode(t, "1")
	before := b.called("proposal")
	send(m, "A", "y")
	contains(t, m, "qhost reported: the proposal is not the one that was shown")
	if b.called("proposal") <= before {
		t.Fatal("the proposal was not read again")
	}
}

func TestChannelApproveWithoutRootHandsTheSecretsOnStandardInput(t *testing.T) {
	b := channelBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, channelHookRow, nil)
	f.reports = nil
	f.report = `{"proposal":"p-0013","name":"deploy-hook","type":"webhook","events":"failed,rolled-back"}`
	hidden := []string{hiddenA, hiddenB}
	approveChannelSteps(t, m)
	args := approveArgsOf("p-0013")
	showsCommand(t, m, args)
	noLeak(t, m.render(), hidden...)
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	sendHidden(t, m, hidden, "y")
	in := secretRun(t, f, args)
	var got map[string]string
	if err := json.Unmarshal(in, &got); err != nil || got["secret"] != hiddenA || got["key"] != hiddenB || len(got) != 2 {
		t.Fatalf("standard input %q (%v)", in, err)
	}
	noValueIn(t, f, hiddenA, hiddenB)
	for _, a := range b.actions() {
		if strings.HasPrefix(a, "notifyadd") || strings.HasPrefix(a, "proposalapprove") {
			t.Fatalf("the service was called directly: %v", b.actions())
		}
	}
	contains(t, m, "The channel deploy-hook is added")
	noLeak(t, m.render(), hidden...)
}

func TestChannelApproveWithoutRootFailureHoldsNoSecret(t *testing.T) {
	b := channelBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, channelHookRow, nil)
	f.stderr = "qhost: invalid: the url " + hiddenA + " is refused\n"
	f.runErr = exitCode(t, "1")
	hidden := []string{hiddenA, hiddenB}
	approveChannelSteps(t, m)
	sendHidden(t, m, hidden, "y")
	contains(t, m, "qhost reported: the url <hidden> is refused")
	noLeak(t, m.render(), hidden...)
}

func TestChannelApproveWithoutRootCancelRunsNothing(t *testing.T) {
	b := channelBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, channelHookRow, nil)
	hidden := []string{hiddenA, hiddenB}
	approveChannelSteps(t, m)
	sendHidden(t, m, hidden, "n")
	if len(f.reqs) != 0 || m.set.busy {
		t.Fatalf("requests %+v, busy %v", f.reqs, m.set.busy)
	}
}

func TestProposalRejectWithoutRoot(t *testing.T) {
	for name, tt := range map[string]struct {
		reason string
		args   []string
	}{
		"with reason": {"not this host", []string{"proposal", "reject", "p-0005", "--reason", "not this host", "--output", "json"}},
		"no reason":   {"", []string{"proposal", "reject", "p-0005", "--output", "json"}},
	} {
		t.Run(name, func(t *testing.T) {
			b := approveBackend()
			m, f := sudoProposalOpen(t, i18n.English, b, 0, map[string]string{
				"proposal reject": `{"id":"p-0005","kind":"source-add","state":"rejected","submitter":"ana","submitter_uid":1001}`})
			send(m, "N")
			send(m, typed(tt.reason)...)
			send(m, "f2")
			says(t, m, "Reject proposal p-0005?")
			showsCommand(t, m, tt.args)
			if len(f.reqs) != 0 {
				t.Fatalf("sudo ran before the answer: %+v", f.reqs)
			}
			send(m, "y")
			wantCommands(t, m, f, tt.args)
			if len(b.proposalRejects) != 0 {
				t.Fatalf("the service was called directly: %+v", b.proposalRejects)
			}
			contains(t, m, "Proposal p-0005 is rejected")
		})
	}
}

func TestProposalRejectWithoutRootCancelRunsNothing(t *testing.T) {
	b := approveBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, 0, nil)
	send(m, "N", "f2", "n")
	if len(f.reqs) != 0 || m.set.busy || len(b.proposalRejects) != 0 {
		t.Fatalf("requests %+v, busy %v", f.reqs, m.set.busy)
	}
}

func TestSourceAddApproveWithoutRootFiles(t *testing.T) {
	b := approveBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, 0, map[string]string{
		"source inspect":   inspectJSON(t, b.inspects[filesDir], false, nil),
		"proposal approve": filesJSON})
	send(m, "A")
	says(t, m, "Approve proposal p-0005 and apply these changes to "+filesDir+"?", "demo/compose.yaml")
	args := approveArgsOf("p-0005", "--expect-effect", effectExample)
	showsCommand(t, m, args)
	wantCommands(t, m, f, inspectArgs(filesDir))
	untouched(t, b)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(filesDir), args)
	untouched(t, b)
	contains(t, m, "The stack folder "+filesDir+" is added")
}

func TestSourceAddApproveWithoutRootCloneBindsAddressHostKeyAndEffect(t *testing.T) {
	b := approveBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, 1, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[cloneDir], true, nil), "repo probe": probeJSON,
		"proposal approve": cloneJSON})
	send(m, "A", "y")
	contains(t, m, proposalURL)
	send(m, "f2", "y")
	contains(t, m, "Trust this host key for", fingerprint)
	send(m, "y")
	args := approveArgsOf("p-0004", "--url", proposalURL, "--host-key-fingerprint", fingerprint, "--expect-effect", effectExample)
	says(t, m, "Approve proposal p-0004 and apply these changes to "+cloneDir+"?")
	showsCommand(t, m, args)
	wantCommands(t, m, f, inspectArgs(cloneDir), probeArgs(proposalURL))
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(cloneDir), probeArgs(proposalURL), args)
	untouched(t, b)
	contains(t, m, "The stack folder "+cloneDir+" is bound.")
}

func TestSourceAddApproveWithoutRootCancelRunsNothingMore(t *testing.T) {
	b := approveBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, 0, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[filesDir], false, nil)})
	send(m, "A", "n")
	wantCommands(t, m, f, inspectArgs(filesDir))
	if m.set.busy || m.modal != nil {
		t.Fatal("the flow was not ended")
	}
	untouched(t, b)
}

func TestSourceAddApproveWithoutRootExplainsAChangedState(t *testing.T) {
	b := approveBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, 0, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[filesDir], false, nil)})
	send(m, "A")
	f.stderr = "qhost: state-changed: the effect of the proposal changed since it was shown; nothing was applied\n"
	f.runErr = exitCode(t, "1")
	before := b.called("proposal")
	send(m, "y")
	says(t, m, "The folder "+filesDir+" or its remote changed since you reviewed the proposal. Nothing was applied.",
		"Approve the proposal again to review it as it is now.")
	if m.set.add != nil || m.set.busy {
		t.Fatalf("flow %v, busy %v", m.set.add, m.set.busy)
	}
	if b.called("proposal") <= before {
		t.Fatal("the proposal was not read again")
	}
}

func TestSourceAddApproveWithoutRootShowsOtherFailures(t *testing.T) {
	b := approveBackend()
	m, f := sudoProposalOpen(t, i18n.English, b, 0, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[filesDir], false, nil)})
	send(m, "A")
	f.stderr = "qhost: config-missing: the folder has no deploy key; nothing was applied\n"
	f.runErr = exitCode(t, "1")
	send(m, "y")
	contains(t, m, "qhost reported: the folder has no deploy key")
}

func TestProposalKeysWithoutRootGermanText(t *testing.T) {
	b := kindsBackend()
	m, _ := sudoProposalOpen(t, i18n.German, b, kindWindowRow, nil)
	send(m, "A")
	contains(t, m, "sudo qhost proposal approve p-0012")
}

// goldenSudoProposal renders the screen after steps on the proposal at row, as an administrator without root.
func goldenSudoProposal(t *testing.T, prefix string, backend func() *fakeBackend, row int, replies func(t *testing.T, b *fakeBackend) map[string]string, steps func(t *testing.T, m *Model, f *fakeRoot)) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, width := range goldenSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				b := backend()
				var r map[string]string
				if replies != nil {
					r = replies(t, b)
				}
				m, f := sudoProposalAt(t, lang, b, row, r, width, 40)
				steps(t, m, f)
				screen := m.render()
				checkScreen(t, screen, width, 40)
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenSudoProposalSettings(t *testing.T) {
	goldenSudoProposal(t, "proposals.sudo.settings", kindsBackend, kindSettingsRow, nil,
		func(_ *testing.T, m *Model, _ *fakeRoot) { send(m, "A") })
}

func TestGoldenSudoProposalChannel(t *testing.T) {
	goldenSudoProposal(t, "proposals.sudo.channel", channelBackend, channelHookRow, nil,
		func(t *testing.T, m *Model, _ *fakeRoot) {
			hidden := []string{hiddenA, hiddenB}
			sendHidden(t, m, hidden, "A")
			sendHidden(t, m, hidden, typed(hiddenA)...)
			sendHidden(t, m, hidden, "down")
			sendHidden(t, m, hidden, typed(hiddenB)...)
			sendHidden(t, m, hidden, "f2")
			noLeak(t, m.render(), hidden...)
		})
}

func TestGoldenSudoProposalReject(t *testing.T) {
	goldenSudoProposal(t, "proposals.sudo.reject", approveBackend, 0, nil,
		func(_ *testing.T, m *Model, _ *fakeRoot) {
			send(m, seq(keys("N"), typed("not this host"), keys("f2"))...)
		})
}

func filesReplies(t *testing.T, b *fakeBackend) map[string]string {
	return map[string]string{"source inspect": inspectJSON(t, b.inspects[filesDir], false, nil), "proposal approve": filesJSON}
}

func TestGoldenSudoProposalFiles(t *testing.T) {
	goldenSudoProposal(t, "proposals.sudo.files", approveBackend, 0, filesReplies,
		func(_ *testing.T, m *Model, _ *fakeRoot) { send(m, "A") })
}

func TestGoldenSudoProposalChanged(t *testing.T) {
	goldenSudoProposal(t, "proposals.sudo.changed", approveBackend, 0,
		func(t *testing.T, b *fakeBackend) map[string]string {
			return map[string]string{"source inspect": inspectJSON(t, b.inspects[filesDir], false, nil)}
		},
		func(t *testing.T, m *Model, f *fakeRoot) {
			send(m, "A")
			f.stderr = "qhost: state-changed: the effect of the proposal changed since it was shown; nothing was applied\n"
			f.runErr = exitCode(t, "1")
			send(m, "y")
		})
}

// A folder of files whose proposal names an address is offered only with that remote: the command takes the
// address along, so the answer without a remote would be refused every time.
func TestSourceAddApproveWithoutRootFilesWithAddressOffersOnlyTheRemote(t *testing.T) {
	b := approveBackend()
	b.proposalContents["p-0005"] = `{"path":"` + filesDir + `","url":"` + remoteURL + `"}`
	b.dirKeys[filesDir] = control.KeyResult{Fingerprint: fingerprint}
	cmp := cmpDiff
	cmp.Case = control.SourceCaseFiles
	m, f := sudoProposalOpen(t, i18n.English, b, 0, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[filesDir], false, nil), "repo probe": probeJSON,
		"source compare": compareJSON(t, cmp), "proposal approve": filesJSON})
	send(m, "A")
	screen := m.render()
	if !strings.Contains(screen, "y with a remote") {
		t.Fatalf("the remote is not offered:\n%s", screen)
	}
	send(m, "w")
	if m.set.form != nil || m.modal == nil {
		t.Fatal("the key for the answer without a remote had an effect")
	}
	send(m, "y")
	if m.set.form == nil || m.set.form.kind != formFolderURL {
		t.Fatal("no address step")
	}
	contains(t, m, remoteURL)
	send(m, "f2", "y", "y", "y")
	args := approveArgsOf("p-0005", "--url", remoteURL, "--host-key-fingerprint", fingerprint, "--expect-effect", effectExample,
		"--expect-state", control.SourceRemoteDifferent, "--expect-commit", commitA)
	showsCommand(t, m, args)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(filesDir), probeArgs(remoteURL),
		[]string{"source", "compare", "--fingerprint", fingerprint, "--output", "json", "--", filesDir, remoteURL}, args)
	untouched(t, b)
}

// Without an address in the proposal the answer without a remote stays.
func TestSourceAddApproveWithoutRootFilesWithoutAddressOffersBoth(t *testing.T) {
	b := approveBackend()
	m, _ := sudoProposalOpen(t, i18n.English, b, 0, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[filesDir], false, nil)})
	send(m, "A")
	contains(t, m, "y yes", "w with a remote")
}
