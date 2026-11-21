package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const proposalURL = "ssh://git.example.test/demo/stacks.git"

// proposalBackend is the demo host with proposals of several states: two open ones for a folder to add, one
// of a kind this version does not know, a rejected and a failed one.
func proposalBackend() *fakeBackend {
	b := foldersBackend()
	sub := func(day, hour int) (time.Time, time.Time) {
		t := at(day, hour, 0)
		return t, t.Add(control.ProposalLifetime)
	}
	info := func(id, kind, state, who string, uid uint32, day, hour int, msg string) control.ProposalInfo {
		s, e := sub(day, hour)
		return control.ProposalInfo{ID: id, Kind: kind, State: state, Fingerprint: "fp-" + id, SubmitterUID: uid, SubmitterName: who,
			Submitted: s, Expires: e, Message: msg}
	}
	b.proposals = []control.ProposalInfo{
		info("p-0005", control.ProposalKindSourceAdd, control.ProposalOpen, "ana", 1001, 30, 16, ""),
		info("p-0004", control.ProposalKindSourceAdd, control.ProposalOpen, "ben", 1002, 30, 9, ""),
		info("p-0003", "network-rule", control.ProposalOpen, "ana", 1001, 29, 12, ""),
		info("p-0002", control.ProposalKindSourceAdd, control.ProposalRejected, "ben", 1002, 28, 8, "the folder holds another project"),
		info("p-0001", control.ProposalKindSourceAdd, control.ProposalFailed, "ana", 1001, 27, 15, "cannot clone: host key mismatch"),
	}
	b.proposalContents = map[string]string{
		"p-0005": `{"path":"` + filesDir + `"}`,
		"p-0004": `{"path":"` + cloneDir + `","url":"` + proposalURL + `"}`,
		"p-0003": `{"rule":"anything"}`,
		"p-0002": `{"path":"` + repoDir + `"}`,
		"p-0001": `{"path":"` + adoptDir + `","url":"` + proposalURL + `"}`,
	}
	return b
}

// proposalModel builds a model on b whose user is root or not.
func proposalModel(t *testing.T, lang i18n.Lang, b *fakeBackend, root bool, width, height int) *Model {
	t.Helper()
	old := defaultEuid
	defaultEuid = func() int {
		if root {
			return 0
		}
		return 1000
	}
	t.Cleanup(func() { defaultEuid = old })
	return newTestModel(t, lang, b, width, height)
}

func goldenProposals(t *testing.T, prefix string, root bool, keys ...string) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m := proposalModel(t, lang, proposalBackend(), root, dim[0], dim[1])
				send(m, keys...)
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenProposalsNotice(t *testing.T) { goldenProposals(t, "proposals.notice", true) }

func TestGoldenProposalsList(t *testing.T) { goldenProposals(t, "proposals.list", true, "6") }

func TestGoldenProposalsView(t *testing.T) { goldenProposals(t, "proposals.view", true, "6", "enter") }

func TestGoldenProposalsViewClone(t *testing.T) {
	goldenProposals(t, "proposals.view.clone", true, "6", "down", "enter")
}

func TestGoldenProposalsViewRejected(t *testing.T) {
	goldenProposals(t, "proposals.view.rejected", true, "6", "down", "down", "down", "enter")
}

func TestGoldenProposalsViewWithoutSudo(t *testing.T) {
	goldenProposals(t, "proposals.view.nosudo", false, "6", "enter")
}

func TestProposalsNoticeOnlyWithOpenProposals(t *testing.T) {
	m := proposalModel(t, i18n.English, proposalBackend(), true, 100, 30)
	if !strings.Contains(m.render(), "3 open proposals") {
		t.Fatalf("no notice with open proposals:\n%s", m.render())
	}
	one := proposalBackend()
	one.proposals = one.proposals[2:]
	if r := proposalModel(t, i18n.English, one, true, 100, 30).render(); !strings.Contains(r, "1 open proposal waits") {
		t.Fatalf("no singular notice with one open proposal:\n%s", r)
	}
	b := proposalBackend()
	b.proposals = b.proposals[3:]
	m = proposalModel(t, i18n.English, b, true, 100, 30)
	if strings.Contains(m.render(), "proposals wait") || strings.Contains(m.render(), "proposal waits") {
		t.Fatalf("a notice without open proposals:\n%s", m.render())
	}
}

func TestProposalsNoticeComesOnlyOnce(t *testing.T) {
	m := proposalModel(t, i18n.English, proposalBackend(), true, 100, 30)
	send(m, "down") // any key clears the notice
	load(t, m)
	if strings.Contains(m.render(), "proposals wait") {
		t.Fatal("a refresh raised the notice again")
	}
}

func TestProposalViewWorksOutTheEffectOnlyAsRoot(t *testing.T) {
	b := proposalBackend()
	m := proposalModel(t, i18n.English, b, true, 120, 40)
	send(m, "6", "enter")
	screen := m.render()
	for _, want := range []string{"Effect if applied now", "demo/compose.yaml", "path: " + filesDir} {
		if !strings.Contains(screen, want) {
			t.Errorf("view lacks %q:\n%s", want, screen)
		}
	}
	if b.called("sourceinspect") != 1 {
		t.Errorf("inspections = %d, want 1", b.called("sourceinspect"))
	}

	b = proposalBackend()
	m = proposalModel(t, i18n.English, b, false, 120, 40)
	send(m, "6", "enter")
	screen = m.render()
	if !strings.Contains(screen, "worked out when you approve it") || strings.Contains(screen, "demo/compose.yaml") {
		t.Errorf("without sudo the view shows the effect or no hint:\n%s", screen)
	}
	if b.called("sourceinspect") != 0 {
		t.Error("the folder was inspected without sudo")
	}
}

func TestProposalViewChangesNothing(t *testing.T) {
	b := proposalBackend()
	m := proposalModel(t, i18n.English, b, true, 120, 40)
	send(m, "6", "enter", "down", "esc", "enter", "y", "n", "r")
	if len(b.proposalActs) != 0 || len(b.sourceAdds) != 0 {
		t.Fatalf("a call that changes something: %v %v", b.proposalActs, b.sourceAdds)
	}
}

func TestProposalViewShowsNoControlCharactersAndCutsLongValues(t *testing.T) {
	b := proposalBackend()
	long := strings.Repeat("x", 500)
	b.proposals[0].SubmitterName = "ana\x1b[31m"
	b.proposals[0].Message = ""
	b.proposalContents["p-0005"] = `{"path":"/srv/evil\u001b]0;owned\u0007","url":"ssh://git.example.test/` + long + `"}`
	m := proposalModel(t, i18n.English, b, true, 120, 40)
	send(m, "6")
	list := m.render()
	send(m, "enter")
	for _, screen := range []string{list, m.render()} {
		if strings.Contains(screen, "\x1b]0;") || strings.Contains(screen, "\x07") || strings.Contains(screen, "ana\x1b[31m") {
			t.Fatalf("control character on the screen:\n%q", screen)
		}
	}
	screen := m.render()
	if !strings.Contains(screen, `path: /srv/evil\x1b]0;owned\a`) {
		t.Errorf("the control characters are not made visible:\n%s", screen)
	}
	if strings.Contains(screen, strings.Repeat("x", 300)) || !strings.Contains(screen, "(+") {
		t.Errorf("the long address is not cut visibly:\n%s", m.render())
	}
}

func TestProposalViewShowsReasonAndFailure(t *testing.T) {
	m := proposalModel(t, i18n.English, proposalBackend(), true, 120, 40)
	send(m, "6", "down", "down", "down", "enter")
	if !strings.Contains(m.render(), "reason: the folder holds another project") {
		t.Errorf("no reason:\n%s", m.render())
	}
	send(m, "esc", "down", "enter")
	if !strings.Contains(m.render(), "failed: cannot clone: host key mismatch") {
		t.Errorf("no failure:\n%s", m.render())
	}
}

func TestProposalViewLeavesWithEsc(t *testing.T) {
	m := proposalModel(t, i18n.English, proposalBackend(), true, 120, 40)
	send(m, "6", "enter")
	if !m.pv.open {
		t.Fatal("the proposal did not open")
	}
	send(m, "esc")
	if m.pv.open || m.area != areaProposals {
		t.Fatalf("esc: open = %v, area = %v", m.pv.open, m.area)
	}
}

func TestProposalListFailureShowsInTheArea(t *testing.T) {
	b := proposalBackend()
	b.errs = map[string]error{"proposals:": control.NewError(control.CodeNoAccess, "reserved")}
	m := proposalModel(t, i18n.English, b, true, 120, 40)
	send(m, "6")
	if !strings.Contains(m.render(), "reserved for administrators") {
		t.Errorf("no failure shown:\n%s", m.render())
	}
	if m.failure != "" {
		t.Errorf("the failure of the proposals spoils the other areas: %q", m.failure)
	}
}
