package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
	"github.com/castrowithcee/qatlas-kit/uiprefs"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// fakeHostProposals stands in for the channel of the host service with invented proposals of the host.
type fakeHostProposals struct {
	list    []control.ProposalInfo
	content map[string]control.ProposalHostChange
	// free is what the proposals of free commands hold; freeOn is the switch.
	free   map[string]control.ProposalCommand
	freeOn bool
	// err fails both calls; lists and shows count them.
	err          error
	lists, shows int
}

func (f *fakeHostProposals) CommandsStatus(context.Context) (control.HostCommandsResult, error) {
	return control.HostCommandsResult{Enabled: f.freeOn}, f.err
}

func (f *fakeHostProposals) ProposalList(context.Context) (control.ProposalListResult, error) {
	f.lists++
	return control.ProposalListResult{Proposals: slices.Clone(f.list)}, f.err
}

func (f *fakeHostProposals) ProposalShow(_ context.Context, id string) (control.ProposalShowResult, error) {
	f.shows++
	if f.err != nil {
		return control.ProposalShowResult{}, f.err
	}
	for _, p := range f.list {
		if p.ID == id {
			raw, _ := json.Marshal(f.content[id])
			if p.Kind == control.ProposalKindCommand {
				raw, _ = json.Marshal(f.free[id])
			}
			return control.ProposalShowResult{ProposalInfo: p, Content: raw}, nil
		}
	}
	return control.ProposalShowResult{}, control.NewError(control.CodeNotFound, "no such proposal")
}

const (
	propDocker = "h-0001"
	propCreate = "h-0002"
	propFolder = "h-0003"
	propLock   = "h-0004"
)

// demoHostProposals are four invented proposals: set-docker for cleo, a new user dora with a key, a folder change
// that is applied already, and a rejected lock.
func demoHostProposals(t *testing.T) *fakeHostProposals {
	t.Helper()
	key, err := hostchange.ParseKey(demoKey(t, 7))
	if err != nil {
		t.Fatal(err)
	}
	info := func(id, state, who string, uid uint32, day, hour int, msg string) control.ProposalInfo {
		s := at(day, hour, 0)
		return control.ProposalInfo{ID: id, Kind: control.ProposalKindHostChange, State: state, Fingerprint: "fp-" + id,
			SubmitterUID: uid, SubmitterName: who, Submitted: s, Expires: s.Add(control.ProposalLifetime), Message: msg}
	}
	return &fakeHostProposals{
		list: []control.ProposalInfo{
			info(propDocker, control.ProposalOpen, "ben", 1001, 30, 16, ""),
			info(propCreate, control.ProposalOpen, "ada", 1000, 30, 9, ""),
			info(propFolder, control.ProposalApplied, "ada", 1000, 29, 12, ""),
			info(propLock, control.ProposalRejected, "ben", 1001, 28, 8, "not now"),
		},
		content: map[string]control.ProposalHostChange{
			propDocker: {Action: hostchange.ActionUserSetDocker, Args: []string{"cleo", "on"}},
			propCreate: {Action: hostchange.ActionUserCreate, Args: []string{"dora", "group=media"}, Keys: []string{key.Line}},
			propFolder: {Action: hostchange.ActionFolderSetPermissions, Args: []string{"/srv/media", "cleo", "media", "read-write", "read", "nothing"}},
			propLock:   {Action: hostchange.ActionUserLock, Args: []string{"cleo"}},
		},
	}
}

// toProposalsPage opens the proposals page of the System area with the invented proposals of the host service.
func toProposalsPage(t *testing.T, m *Model, h *fakeHostProposals) {
	t.Helper()
	m.hostProps = h
	m.setZone(kittui.ZoneSubtabs)
	send(m, "right", "right", "right")
	if m.sys.page != pageProposals {
		t.Fatalf("page %s, want the proposals page", m.sys.page)
	}
	send(m, "down")
}

func TestGoldenSystemProposals(t *testing.T) {
	goldenSystem(t, "system.proposals", func(t *testing.T, m *Model) { toProposalsPage(t, m, demoHostProposals(t)) })
}

// The view of set-docker is marked red and names the consequence of the Docker right.
func TestGoldenSystemProposalDocker(t *testing.T) {
	goldenSystem(t, "system.proposal.docker", func(t *testing.T, m *Model) {
		toProposalsPage(t, m, demoHostProposals(t))
		send(m, "enter")
	})
}

func TestProposalsPageNeedsNeitherSudoNorTheDeployService(t *testing.T) {
	m, f := notSetUpModel(t, i18n.English, newFakeDeploy(), 1000, wideWidth, 40)
	h := demoHostProposals(t)
	m.hostProps = h
	send(m, "7")
	toProposalsPage(t, m, h)
	screen := m.render()
	for _, want := range []string{"h-0001", "h-0002", "ben", "ada", "open", "applied", "rejected"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the page lacks %q:\n%s", want, screen)
		}
	}
	if h.lists != 1 || len(f.acts) != 0 {
		t.Errorf("lists %d, deploy calls %v", h.lists, f.acts)
	}
}

func TestProposalsPageSaysWhyItCannotRead(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{client.ErrNotRunning, "The host service is not running."},
		{errors.New("boom"), "The proposals cannot be read: boom"},
	} {
		m, _ := systemModel(t, i18n.English, wideWidth, 40)
		h := demoHostProposals(t)
		h.err = c.err
		toProposalsPage(t, m, h)
		if screen := flat(m.render()); !strings.Contains(screen, c.want) {
			t.Errorf("no %q:\n%s", c.want, screen)
		}
	}
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	m.setZone(kittui.ZoneSubtabs)
	send(m, "right", "right", "right")
	if screen := flat(m.render()); !strings.Contains(screen, "does not reach the host service") {
		t.Errorf("no note about the missing channel:\n%s", screen)
	}
}

func TestProposalViewShowsSentenceCommandsKeyAndNoControlCharacters(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	h := demoHostProposals(t)
	h.list[1].SubmitterName = "ada\x1b[31m"
	toProposalsPage(t, m, h)
	send(m, "down", "enter")
	screen := flat(m.render())
	key, _ := hostchange.ParseKey(demoKey(t, 7))
	for _, want := range []string{"Proposal h-0002 from ada", "(1000)", "state open", "Create the user dora", "$ useradd", "chpasswd < <password>",
		"ed25519, comment: demo@example.test, fingerprint: " + key.Fingerprint, "you enter it when you approve"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the view lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(m.render(), "\x1b[31m") {
		t.Errorf("a control character reached the screen")
	}
	if len(f.reqs) != 0 || h.shows != 1 {
		t.Errorf("requests %v, shows %d", f.reqs, h.shows)
	}
}

func TestProposalRiskNamesWhatMakesAChangeRed(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	tx := m.text
	for _, c := range []struct {
		action string
		args   []string
		want   []kittui.TextID
	}{
		{hostchange.ActionUserSetDocker, []string{"cleo", "on"}, []kittui.TextID{textRiskSetDocker}},
		{hostchange.ActionUserCreate, []string{"dora", "docker"}, []kittui.TextID{textRiskSetDocker}},
		{hostchange.ActionUserCreate, []string{"dora"}, nil},
		{hostchange.ActionUserEnable, []string{"ben"}, []kittui.TextID{textRiskEnable}},
		{hostchange.ActionUserLock, []string{"root"}, []kittui.TextID{textRiskRoot}},
		{hostchange.ActionUserLock, []string{"daemon"}, []kittui.TextID{textRiskAccount}},
		{hostchange.ActionUserAddGroups, []string{"cleo", "sudo"}, []kittui.TextID{textRiskGroup}},
		{hostchange.ActionGroupDelete, []string{"docker"}, []kittui.TextID{textRiskGroupDocker}},
		{hostchange.ActionFolderSetPermissions, []string{"/etc/ssh", "root", "root", "read", "read", "nothing", "system-path"}, []kittui.TextID{textRiskFolderSystem}},
		{hostchange.ActionUserLock, []string{"cleo"}, nil},
		{hostchange.ActionFolderSetPermissions, []string{"/srv/media", "cleo", "media", "read", "read", "nothing"}, nil},
	} {
		got := m.hostProposalRisk(c.action, c.args)
		if len(c.want) == 0 && got != "" {
			t.Errorf("%s %v: risk %q, want none", c.action, c.args, got)
		}
		for _, id := range c.want {
			if !strings.Contains(got, tx.Lookup(id)) {
				t.Errorf("%s %v: risk %q lacks %q", c.action, c.args, got, tx.Lookup(id))
			}
		}
	}
}

// markProposal marks the proposal id in the list of the page.
func markProposal(t *testing.T, m *Model, id string) {
	t.Helper()
	if !m.sys.prop.list.Select(kittui.ItemID(id)) {
		t.Fatalf("no row for %s", id)
	}
}

func TestApprovingAsksLikeAChangeByHandAndRunsApplyOverSudo(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	h := demoHostProposals(t)
	toProposalsPage(t, m, h)
	f.report = appliedReport(hostchange.ActionUserSetDocker)
	send(m, "A")
	screen := flat(m.render())
	for _, want := range []string{"Apply proposal h-0001 to the host:", "full control over this host", "$ ", "usermod"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the question lacks %q:\n%s", want, screen)
		}
	}
	if len(f.reqs) != 0 {
		t.Fatalf("something ran before the answer: %v", f.reqs)
	}
	send(m, "y")
	if len(f.execs) != 1 || len(f.bgs) != 0 {
		t.Fatalf("execs %d, background runs %d", len(f.execs), len(f.bgs))
	}
	want := []string{"sudo-fake", "qhost-fake", "host", "apply", "--proposal", propDocker, "--fingerprint", "fp-" + propDocker,
		"--confirm", "--output", "json"}
	if !slices.Equal(f.execs[0].Args, want) {
		t.Fatalf("arguments %q, want %q", f.execs[0].Args, want)
	}
	if screen := flat(m.render()); !strings.Contains(screen, "Done: Apply proposal h-0001") || !strings.Contains(screen, "Journal entry 20261102T090000-0001") {
		t.Fatalf("no result:\n%s", screen)
	}
	if h.lists != 2 {
		t.Fatalf("the list was read %d times, want again after the change", h.lists)
	}
	if !m.sys.jrnl.stale {
		t.Fatal("the journal is not marked older than the host")
	}
}

func TestApprovingAnythingElseThanAnOpenProposalIsRefused(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	h := demoHostProposals(t)
	toProposalsPage(t, m, h)
	markProposal(t, m, propFolder)
	send(m, "A")
	if m.modal != nil || len(f.reqs) != 0 || !strings.Contains(m.render(), "no longer open") {
		t.Fatalf("a proposal that is not open was offered:\n%s", m.render())
	}
	send(m, "N")
	if m.modal != nil || len(f.reqs) != 0 {
		t.Fatalf("rejecting a proposal that is not open was offered")
	}
}

func TestApprovingANewUserAsksForThePasswordAndKeepsItOnStandardInput(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	h := demoHostProposals(t)
	toProposalsPage(t, m, h)
	markProposal(t, m, propCreate)
	f.report = appliedReport(hostchange.ActionUserCreate)
	hidden := []string{hiddenA}
	send(m, "A")
	contains(t, m, "Initial password")
	send(m, seq(typed(hiddenA), keys("down"), typed(hiddenA))...)
	noLeak(t, m.render(), hidden...)
	send(m, "f2")
	screen := flat(m.render())
	for _, want := range []string{"Apply proposal h-0002 to the host:", "chpasswd < <password>"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the question lacks %q:\n%s", want, screen)
		}
	}
	noLeak(t, m.render(), hidden...)
	send(m, "y")
	if len(f.execs) != 1 || !slices.Contains(f.execs[0].Args, "-v") || len(f.bgs) != 1 {
		t.Fatalf("want sudo -v, then one run without a prompt: execs %d, runs %d", len(f.execs), len(f.bgs))
	}
	child := f.bgs[0]
	want := []string{"-n", "qhost-fake", "host", "apply", "--proposal", propCreate, "--fingerprint", "fp-" + propCreate,
		"--confirm", "--output", "json"}
	if !slices.Equal(child.Args[1:], want) {
		t.Fatalf("arguments %q, want %q", child.Args[1:], want)
	}
	for _, part := range append(slices.Clone(child.Args), child.Env...) {
		if strings.Contains(part, hiddenA) {
			t.Fatalf("the password is in the command line or environment: %q", part)
		}
	}
	if string(f.stdins[0]) != `{"password":"`+hiddenA+`"}` {
		t.Fatalf("standard input %q is not the password alone", f.stdins[0])
	}
	for _, req := range f.reqs {
		if len(req.Secret) > 0 && slices.ContainsFunc(req.Secret, func(b byte) bool { return b != 0 }) {
			t.Fatal("the secret was not overwritten after the run")
		}
	}
	screen = m.render()
	noLeak(t, screen, hidden...)
	if !strings.Contains(flat(screen), "Done: Apply proposal h-0002") {
		t.Fatalf("no result:\n%s", screen)
	}
}

func TestCancelledApprovalRunsNothingAndDropsItsSecret(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toProposalsPage(t, m, demoHostProposals(t))
	markProposal(t, m, propCreate)
	send(m, "A")
	send(m, seq(typed(hiddenA), keys("down"), typed(hiddenA), keys("f2"))...)
	if m.modal == nil || m.modal.drop == nil {
		t.Fatalf("no question that drops its secret:\n%s", m.render())
	}
	send(m, "n")
	if len(f.reqs) != 0 {
		t.Fatalf("something ran: %v", f.reqs)
	}
}

func TestViewApprovesWithItsYes(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toProposalsPage(t, m, demoHostProposals(t))
	send(m, "enter", "y")
	if !strings.Contains(flat(m.render()), "Apply proposal h-0001 to the host:") {
		t.Fatalf("no question after approving in the view:\n%s", m.render())
	}
	send(m, "n")
	if len(f.reqs) != 0 {
		t.Fatalf("something ran: %v", f.reqs)
	}
}

func TestRejectingAsksAndRunsProposalRejectOverSudo(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	h := demoHostProposals(t)
	toProposalsPage(t, m, h)
	send(m, "N")
	if screen := flat(m.render()); !strings.Contains(screen, "Reject proposal h-0001?") || !strings.Contains(screen, "$ qhost proposal reject h-0001") {
		t.Fatalf("no question:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("something ran before the answer: %v", f.reqs)
	}
	send(m, "y")
	if len(f.execs) != 1 || !slices.Equal(f.execs[0].Args, []string{"sudo-fake", "qhost-fake", "proposal", "reject", propDocker}) {
		t.Fatalf("commands %v", f.execs)
	}
	if !strings.Contains(m.render(), "Proposal h-0001 is rejected.") || h.lists != 2 {
		t.Fatalf("lists %d:\n%s", h.lists, m.render())
	}
}

func TestRejectingThatFailsIsExplained(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toProposalsPage(t, m, demoHostProposals(t))
	f.runErr = exitError(t, 3)
	send(m, "N", "y")
	if screen := flat(m.render()); !strings.Contains(screen, "was not changed as asked") {
		t.Fatalf("a failed rejection says nothing:\n%s", screen)
	}
	if strings.Contains(m.render(), "is rejected.") {
		t.Fatal("a failed rejection is reported as done")
	}
}

// noticeModel is a model whose start reads the proposals of both services; order says which answer comes first.
func noticeModel(t *testing.T, deploy *fakeBackend, host HostProposals, hostFirst bool, langs ...i18n.Lang) *Model {
	t.Helper()
	lang := i18n.English
	if len(langs) > 0 {
		lang = langs[0]
	}
	m, err := newModel(config{ctx: context.Background(), backend: deploy, deploy: newFakeDeploy(), lang: lang, theme: kittui.ThemeTerminal,
		profile: kittui.ProfileNone, charset: kittui.CharsetUnicode, loc: time.UTC, hostProps: host,
		prefs: uiprefs.Prefs{Theme: kittui.ThemeTerminal, Start: string(areaStacks), Icons: uiprefs.IconsUnicode,
			Language: uiprefs.LanguageEnglish, MouseEnabled: true}, accounts: testAccounts(false)})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	timersOf[m] = &timers{}
	m.after = timersOf[m].start
	if hostFirst {
		drain(m, m.readHostProposals())
		if m.notice != "" {
			t.Fatalf("the notice came before the deploy service answered: %q", m.notice)
		}
	}
	load(t, m)
	if !hostFirst {
		drain(m, m.readHostProposals())
	}
	return m
}

func TestStartNoticeCountsTheOpenProposalsOfBothServices(t *testing.T) {
	for _, hostFirst := range []bool{false, true} {
		m := noticeModel(t, proposalBackend(), demoHostProposals(t), hostFirst)
		if !strings.Contains(m.render(), "5 open proposals wait for you") {
			t.Fatalf("hostFirst %v, no notice of both:\n%s", hostFirst, m.render())
		}
	}
}

func TestStartNoticeCountsTheOtherServiceWhenOneIsUnreachable(t *testing.T) {
	down := demoHostProposals(t)
	down.err = client.ErrNotRunning
	if m := noticeModel(t, proposalBackend(), down, false); !strings.Contains(m.render(), "3 open proposals wait for you") {
		t.Fatalf("no notice of the deploy proposals with the host service down:\n%s", m.render())
	}
	b := proposalBackend()
	b.statusErr = fmt.Errorf("dial: %w", client.ErrNotRunning)
	if m := noticeModel(t, b, demoHostProposals(t), true); !strings.Contains(m.render(), "2 open proposals wait for you") {
		t.Fatalf("no notice of the host proposals with the deploy service down:\n%s", m.render())
	}
	b.statusErr = errors.New("the deploy service is down")
	if m := noticeModel(t, b, demoHostProposals(t), false); !strings.Contains(m.notice, "2 open proposals wait for you") {
		t.Fatalf("no notice of the host proposals with the deploy service failing: %q", m.notice)
	}
	both := proposalBackend()
	both.statusErr = errors.New("down")
	if m := noticeModel(t, both, down, false); m.notice != "" {
		t.Fatalf("a notice with both services down: %q", m.notice)
	}
}

// hostOnlyProposals keeps the open proposals of the host service, first only the given count of them.
func hostOnlyProposals(t *testing.T, open int) *fakeHostProposals {
	h := demoHostProposals(t)
	h.list = h.list[:open]
	return h
}

func TestStartNoticeNamesTheKeyOfEachKind(t *testing.T) {
	noDeploy := func() *fakeBackend { b := proposalBackend(); b.proposals = nil; return b }
	oneDeploy := func() *fakeBackend { b := proposalBackend(); b.proposals = b.proposals[2:3]; return b }
	twoDeploy := func() *fakeBackend { b := proposalBackend(); b.proposals = b.proposals[:2]; return b }
	cases := []struct {
		name   string
		deploy *fakeBackend
		host   HostProposals
		en, de string
	}{
		{"deploy plural", proposalBackend(), nil, "3 open proposals wait for you. Press 6 to review them.", "3 offene Vorschläge warten auf dich. Mit 6 prüfst du sie."},
		{"deploy singular", oneDeploy(), nil, "1 open proposal waits for you. Press 6 to review it.", "1 offener Vorschlag wartet auf dich. Mit 6 prüfst du ihn."},
		{"host plural", noDeploy(), hostOnlyProposals(t, 2), "2 open proposals wait for you. Press 7 to review them.", "2 offene Vorschläge warten auf dich. Mit 7 prüfst du sie."},
		{"host singular", noDeploy(), hostOnlyProposals(t, 1), "1 open proposal waits for you. Press 7 to review it.", "1 offener Vorschlag wartet auf dich. Mit 7 prüfst du ihn."},
		{"both", twoDeploy(), hostOnlyProposals(t, 1), "3 open proposals wait for you: 2 under 6 (Proposals), 1 under 7 (System).",
			"3 offene Vorschläge warten auf dich: 2 unter 6 (Vorschläge), 1 unter 7 (System)."},
	}
	for _, c := range cases {
		for lang, want := range map[i18n.Lang]string{i18n.English: c.en, i18n.German: c.de} {
			m := noticeModel(t, c.deploy, c.host, false, lang)
			if !strings.Contains(m.notice, want) {
				t.Errorf("%s (%v): notice %q lacks %q", c.name, lang, m.notice, want)
			}
		}
	}
}

func TestNoticeKeysOpenTheirAreas(t *testing.T) {
	if k := areaKey(areaProposals); k != "6" {
		t.Fatalf("key of the proposals area %q", k)
	}
	for id, key := range map[kittui.AreaID]string{areaProposals: areaKey(areaProposals), areaSystem: areaKey(areaSystem)} {
		m := newTestModel(t, i18n.English, demoBackend(), wideWidth, goldenRows)
		send(m, key)
		if m.area != id {
			t.Errorf("key %s opens %s, want %s", key, m.area, id)
		}
	}
}

func TestGoldenStartNoticeOfBothKinds(t *testing.T) {
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m := noticeModel(t, proposalBackend(), demoHostProposals(t), false, lang)
				m.Update(tea.WindowSizeMsg{Width: dim[0], Height: dim[1]})
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				checkGolden(t, "proposals.notice.both."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}
