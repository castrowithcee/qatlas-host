package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// approveBackend is the proposal backend with the deploy key of the folder to clone into already stored.
func approveBackend() *fakeBackend {
	b := proposalBackend()
	b.dirKeys = map[string]control.KeyResult{cloneDir: {Fingerprint: fingerprint}, adoptDir: {Fingerprint: fingerprint}}
	return b
}

// proposalOpen opens the proposal at row (0 is the newest) in the list of the Proposals area.
func proposalOpen(t *testing.T, b *fakeBackend, root bool, row int) *Model {
	t.Helper()
	m := proposalModel(t, i18n.English, b, root, 120, 40)
	send(m, "6")
	for range row {
		send(m, "down")
	}
	send(m, "enter")
	return m
}

func noProposalChange(t *testing.T, b *fakeBackend) {
	t.Helper()
	if len(b.proposalActs) != 0 || len(b.sourceAdds) != 0 || len(b.repoSet) != 0 || len(b.host.changes()) != 0 {
		t.Fatalf("something changed: proposal calls %v, adds %v, binds %v, host %v", b.proposalActs, b.sourceAdds, b.repoSet, b.host.changes())
	}
}

func TestProposalApproveFilesShowsApprovesAppliesAndKeepsTheResult(t *testing.T) {
	b := approveBackend()
	m := proposalOpen(t, b, true, 0)
	contains(t, m, "approve", "reject")
	send(m, "A")
	contains(t, m, "Approve proposal p-0005 and apply these changes to "+filesDir+"?", "demo/compose.yaml")
	noProposalChange(t, b)
	send(m, "y")
	want := []string{"proposalshow:p-0005", "proposalshow:p-0005", "proposalshow:p-0005", "proposalapprove:p-0005",
		"sourceadd:" + filesDir, "proposalresult:p-0005", "proposalshow:p-0005"}
	if got := b.actions(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(b.proposalApproves) != 1 || b.proposalApproves[0] != (control.ProposalApproveArgs{ID: "p-0005", Fingerprint: "fp-p-0005"}) {
		t.Fatalf("approves = %+v", b.proposalApproves)
	}
	if len(b.proposalResults) != 1 || b.proposalResults[0] != (control.ProposalResultArgs{ID: "p-0005", Fingerprint: "fp-p-0005", Applied: true}) {
		t.Fatalf("results = %+v", b.proposalResults)
	}
	if got := b.host.changes(); !slices.Equal(got, []string{"apply:3", "grant:" + filesDir + ":false"}) {
		t.Fatalf("host changes = %v", got)
	}
	contains(t, m, "The stack folder "+filesDir+" is added")
}

func TestProposalApproveCloneCarriesTheContextThroughTheHandSteps(t *testing.T) {
	b := approveBackend()
	m := proposalOpen(t, b, true, 1)
	send(m, "A")
	contains(t, m, "Clone a repository into "+cloneDir)
	send(m, "y")
	if m.set.form == nil || m.set.form.kind != formFolderURL || m.area != areaProposals {
		t.Fatal("no address step in the Proposals area")
	}
	contains(t, m, proposalURL)
	send(m, "f2")
	contains(t, m, "Trust this host key for", proposalURL, fingerprint)
	if len(b.proposalApproves) != 0 {
		t.Fatal("the host key was taken for the approval")
	}
	send(m, "y")
	contains(t, m, "Approve proposal p-0004 and apply these changes to "+cloneDir+"?", fingerprint)
	noProposalChange(t, b)
	send(m, "y")
	want := control.RepoSetArgs{URL: proposalURL, Dir: cloneDir, Fingerprint: fingerprint}
	if len(b.repoSet) != 1 || b.repoSet[0] != want {
		t.Fatalf("binds = %+v, want %+v", b.repoSet, want)
	}
	acts := b.actions()
	pos := func(s string) int { return slices.Index(acts, s) }
	if !(pos("probe:"+proposalURL) < pos("proposalapprove:p-0004") && pos("proposalapprove:p-0004") < pos("reposet:"+proposalURL) &&
		pos("reposet:"+proposalURL) < pos("proposalresult:p-0004")) {
		t.Fatalf("order of calls: %v", acts)
	}
	if len(b.proposalResults) != 1 || !b.proposalResults[0].Applied {
		t.Fatalf("results = %+v", b.proposalResults)
	}
}

func TestProposalApproveTheAddressIsChangeableAndThePathIsNot(t *testing.T) {
	b := approveBackend()
	m := proposalOpen(t, b, true, 1)
	send(m, "A", "y")
	other := "ssh://git.example.test/other/stacks.git"
	send(m, "end")
	// The field is a text field: clear it with backspaces and type the new address.
	for range len(proposalURL) {
		send(m, "backspace")
	}
	send(m, typed(other)...)
	send(m, "f2", "y", "y")
	if len(b.repoSet) != 1 || b.repoSet[0].URL != other || b.repoSet[0].Dir != cloneDir {
		t.Fatalf("binds = %+v", b.repoSet)
	}
}

func TestProposalApproveRefusesAChangedProposalOrEffect(t *testing.T) {
	for _, c := range []struct {
		name   string
		row    int
		change func(b *fakeBackend)
		steps  []string
		// show is what the view shows after the refusal.
		show string
	}{
		{"fingerprint before the first question", 0, func(b *fakeBackend) { b.proposals[0].Fingerprint = "fp-new" }, nil, ""},
		{"fingerprint at the last question", 0, func(b *fakeBackend) { b.proposals[0].Fingerprint = "fp-new" }, keys("A"), ""},
		{"files at the last question", 0, func(b *fakeBackend) {
			r := b.inspects[filesDir]
			r.PlannedFiles = append(slices.Clone(r.PlannedFiles), "demo/new-file.yaml")
			b.inspects[filesDir] = r
		}, keys("A"), "demo/new-file.yaml"},
		{"left out at the last question", 0, func(b *fakeBackend) {
			r := b.inspects[filesDir]
			r.Excluded = nil
			b.inspects[filesDir] = r
		}, keys("A"), "Left out (0)"},
		{"conflict at the last question", 0, func(b *fakeBackend) {
			r := b.inspects[filesDir]
			r.Stacks = []control.SourceStack{{Name: "demo", Folder: "demo"}}
			b.inspects[filesDir] = r
		}, keys("A"), ""},
		{"permission plan of a clone", 1, func(b *fakeBackend) {
			b.host.plans[cloneDir] = NewPermPlan(2, []string{"2 paths change"}, "plan")
		}, keys("A", "y", "f2", "y"), "2 paths change"},
		{"case at the last question", 1, func(b *fakeBackend) {
			r := b.inspects[cloneDir]
			r.Case = control.SourceCaseReject
			r.Reason = "now a repository of another project"
			b.inspects[cloneDir] = r
		}, keys("A", "y", "f2", "y"), "now a repository of another project"},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := approveBackend()
			m := proposalOpen(t, b, true, c.row)
			send(m, c.steps...)
			c.change(b)
			if len(c.steps) == 0 {
				send(m, "A")
			} else {
				send(m, "y")
			}
			noProposalChange(t, b)
			if m.modal != nil {
				t.Fatalf("a question is still open:\n%s", m.render())
			}
			contains(t, m, "changed since it was shown; nothing was applied")
			if c.show != "" {
				contains(t, m, c.show)
			}
			if !m.pv.open || m.area != areaProposals {
				t.Fatal("the view was left")
			}
		})
	}
}

func TestProposalApproveRefusedByTheServiceAppliesNothing(t *testing.T) {
	b := approveBackend()
	b.errs = map[string]error{"proposalapprove:p-0005": control.NewError(control.CodeInvalid, "the proposal changed")}
	m := proposalOpen(t, b, true, 0)
	send(m, "A", "y")
	if len(b.sourceAdds) != 0 || len(b.proposalResults) != 0 || len(b.host.changes()) != 0 {
		t.Fatalf("applied after a refusal: adds %v, results %v, host %v", b.sourceAdds, b.proposalResults, b.host.changes())
	}
	if m.failure == "" {
		t.Fatal("no message")
	}
}

func TestProposalApproveFailedApplyKeepsTheFailureOnTheProposal(t *testing.T) {
	b := approveBackend()
	b.errs = map[string]error{"sourceadd:" + filesDir: control.NewError(control.CodeInvalid, "the working copy is not clean")}
	m := proposalOpen(t, b, true, 0)
	send(m, "A", "y")
	if len(b.proposalResults) != 1 {
		t.Fatalf("results = %+v", b.proposalResults)
	}
	r := b.proposalResults[0]
	if r.Applied || r.Message != "the working copy is not clean" || r.ID != "p-0005" || r.Fingerprint != "fp-p-0005" {
		t.Fatalf("result = %+v", r)
	}
	if m.failure == "" {
		t.Fatal("the failure is not shown")
	}
}

func TestProposalApproveResultNotKeptIsSaid(t *testing.T) {
	b := approveBackend()
	b.errs = map[string]error{"proposalresult:p-0005": errors.New("channel lost")}
	m := proposalOpen(t, b, true, 0)
	send(m, "A", "y")
	if len(b.sourceAdds) != 1 {
		t.Fatalf("adds = %v", b.sourceAdds)
	}
	if !strings.Contains(m.failure, "could not be kept on the proposal") {
		t.Fatalf("failure = %q", m.failure)
	}
}

func TestProposalApproveCancelChangesNothing(t *testing.T) {
	b := approveBackend()
	m := proposalOpen(t, b, true, 0)
	send(m, "A", "n")
	noProposalChange(t, b)
	m = proposalOpen(t, b, true, 1)
	send(m, "A", "y", "esc", "y")
	noProposalChange(t, b)
}

func TestProposalRejectWithReason(t *testing.T) {
	b := approveBackend()
	m := proposalOpen(t, b, true, 0)
	send(m, "N")
	if m.set.form == nil || m.set.form.kind != formProposalReject {
		t.Fatal("no form")
	}
	send(m, typed("not this host")...)
	send(m, "f2")
	contains(t, m, "Reject proposal p-0005?")
	noProposalChange(t, b)
	send(m, "y")
	if len(b.proposalRejects) != 1 || b.proposalRejects[0] != (control.ProposalRejectArgs{ID: "p-0005", Reason: "not this host"}) {
		t.Fatalf("rejects = %+v", b.proposalRejects)
	}
	if len(b.sourceAdds) != 0 || len(b.proposalApproves) != 0 || len(b.proposalResults) != 0 {
		t.Fatal("more than the rejection reached the service")
	}
	contains(t, m, "Proposal p-0005 is rejected")
}

func TestProposalRejectWithoutReasonAndWithTooLongOne(t *testing.T) {
	b := approveBackend()
	m := proposalOpen(t, b, true, 0)
	send(m, "N", "f2", "y")
	if len(b.proposalRejects) != 1 || b.proposalRejects[0].Reason != "" {
		t.Fatalf("rejects = %+v", b.proposalRejects)
	}
	b = approveBackend()
	m = proposalOpen(t, b, true, 0)
	send(m, "N")
	send(m, typed(strings.Repeat("x", 301))...)
	send(m, "f2")
	contains(t, m, "at most 300 characters")
	if m.modal != nil || len(b.proposalRejects) != 0 {
		t.Fatal("a too long reason went on")
	}
}

func TestProposalRejectDiscardLeavesNothing(t *testing.T) {
	b := approveBackend()
	m := proposalOpen(t, b, true, 0)
	send(m, "N", "esc")
	if m.set.form != nil || !m.pv.open {
		t.Fatal("the form stayed or the view was left")
	}
	noProposalChange(t, b)
}

func TestProposalDecidedProposalOffersNoKeys(t *testing.T) {
	b := approveBackend()
	m := proposalOpen(t, b, true, 3)
	if screen := m.render(); strings.Contains(screen, "approve") {
		t.Fatalf("approve offered on a rejected proposal:\n%s", screen)
	}
	send(m, "A", "N")
	if m.modal != nil || m.set.form != nil {
		t.Fatal("a decided proposal opened a step")
	}
}

func TestGoldenProposalApproveFilesQuestion(t *testing.T) {
	goldenProposals(t, "proposals.approve.files", true, "6", "enter", "A")
}

func TestGoldenProposalApproveCloneFinalQuestion(t *testing.T) {
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				b := approveBackend()
				m := proposalModel(t, lang, b, true, dim[0], dim[1])
				send(m, "6", "down", "enter", "A", "y", "f2", "y")
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				checkGolden(t, "proposals.approve.clone."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}
