package service_test

import (
	"encoding/json"
	"errors"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/castrowithcee/qatlas-kit/docker"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/registry/registrytest"
	"github.com/castrowithcee/qatlas-host/internal/service"
)

// The invented users of the access tests: an administrator in the service group, a member of the service group
// who is no administrator, and a user unknown to the group database.
const (
	exampleAdmin    = 4100
	exampleReader   = 4102
	exampleOutsider = 4999
)

// peers lets a test choose the user of every new connection; it starts as root, so the harness can set up.
type peers struct{ uid atomic.Uint32 }

func (p *peers) as(uid uint32) { p.uid.Store(uid) }

// withPeers gives the service an invented group database, takes the user of a connection from p and hands the
// registry to reg.
func withPeers(t *testing.T, p *peers, reg *atomic.Pointer[control.Registry]) func(*service.Config) {
	t.Helper()
	dir := t.TempDir()
	group, passwd := filepath.Join(dir, "group"), filepath.Join(dir, "passwd")
	if err := os.WriteFile(group, []byte("sudo:x:27:example-admin\nqhost:x:990:example-admin,example-reader\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	users := "example-admin:x:4100:4100::/nonexistent:/usr/sbin/nologin\n" +
		"example-reader:x:4102:4102::/nonexistent:/usr/sbin/nologin\n"
	if err := os.WriteFile(passwd, []byte(users), 0o600); err != nil {
		t.Fatal(err)
	}
	return func(c *service.Config) {
		c.PeerCheck = func(net.Conn) (uint32, error) { return p.uid.Load(), nil }
		c.GroupFile, c.PasswdFile = group, passwd
		service.SeeRegistry(c, reg.Store)
	}
}

func commandsOf(reg *control.Registry, access control.Access) []string {
	var names []string
	for _, name := range reg.Names() {
		if c, _ := reg.Lookup(name); c.Access == access {
			names = append(names, name)
		}
	}
	return names
}

func TestAnAdministratorWithoutRootReadsButNeverWrites(t *testing.T) {
	p := &peers{}
	var seen atomic.Pointer[control.Registry]
	fd := &fakeDocker{}
	srv := registrytest.New(t, registrytest.Config{Repos: map[string][]string{"demo/app": {"1.2.3", "1.3.0"}}})
	h := boundWith(t, &fakeDeployer{}, nil, withPeers(t, p, &seen), withDocker(fd), withRegistry(srv.URL))
	reg := seen.Load()
	var before control.RunsResult
	if err := h.call(control.RunsCommand, control.RunsArgs{}, &before); err != nil {
		t.Fatal(err)
	}
	ops, pushed, clones, tags := len(h.git.Ops()), len(h.git.Pushed()), h.git.Clones(), tagListCalls(srv)

	p.as(exampleAdmin)
	for _, name := range reg.Names() {
		c, _ := reg.Lookup(name)
		err := h.call(name, nil, nil)
		var ce *control.Error
		switch {
		case c.Access == control.AccessWrite && (!errors.As(err, &ce) || ce.Code != control.CodeNotRoot):
			t.Errorf("write %s = %v, want %s", name, err, control.CodeNotRoot)
		case c.Access == control.AccessRead && errors.As(err, &ce) && (ce.Code == control.CodeNotRoot || ce.Code == control.CodeNoAccess):
			t.Errorf("read %s = %v", name, err)
		}
	}
	// A listing that would ask the registries or GitHub is a write; arguments it cannot read count as one.
	for _, args := range []any{
		control.UpdatesArgs{Refresh: true}, control.UpdatesArgs{Changelog: true}, control.UpdatesArgs{Stack: "demo-stack", Refresh: true},
		map[string]any{"refresh": true, "unknown": 1}, map[string]any{"REFRESH": true}, "not an object",
	} {
		wantCode(t, h.call(control.UpdatesListCommand, args, nil), control.CodeNotRoot)
	}
	if err := h.call(control.UpdatesListCommand, control.UpdatesArgs{Stack: "demo-stack"}, nil); err != nil {
		t.Fatalf("updates.list without refresh = %v", err)
	}

	for _, uid := range []uint32{exampleReader, exampleOutsider} {
		p.as(uid)
		for _, name := range append(reg.Names(), "nope") {
			wantCode(t, h.call(name, nil, nil), control.CodeNoAccess)
		}
	}

	p.as(0)
	var after control.RunsResult
	if err := h.call(control.RunsCommand, control.RunsArgs{}, &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Runs) != len(before.Runs) || len(h.git.Ops()) != ops || len(h.git.Pushed()) != pushed ||
		h.git.Clones() != clones || tagListCalls(srv) != tags || len(fd.calls()) != 0 {
		t.Fatalf("a refused request had an effect: runs %d->%d, git %v, pushed %v, clones %d, tag lists %d->%d, docker %v",
			len(before.Runs), len(after.Runs), h.git.Ops(), h.git.Pushed(), h.git.Clones(), tags, tagListCalls(srv), fd.calls())
	}
}

func TestReadAnswersHoldNoSecretValue(t *testing.T) {
	const (
		stackSecret = "example-stack-secret-not-real"
		handSecret  = "example-hand-secret-not-real"
		githubToken = "example-github-token-not-real"
		tgToken     = "123456:example-telegram-token-not-real"
	)
	secrets := []string{stackSecret, handSecret, githubToken, tgToken, hookToken, hookKey, mailPassword, "PRIVATE KEY"}

	p := &peers{}
	var seen atomic.Pointer[control.Registry]
	fd := &fakeDocker{}
	rcv := newReceiver(t)
	h := boundWith(t, &printingDeployer{}, map[string]string{
		// A versioned secrets file is a finding of the check, which names the place only.
		"demo-stack/secrets.env": "VERSIONED_TOKEN=" + handSecret + "\n",
	}, withPeers(t, p, &seen), withDocker(fd), withNotify(rcv), withBackups(&fakeBackups{}))

	// Root sets up a value of every kind the service keeps.
	if err := h.call(control.SecretSetCommand, control.SecretSetArgs{Stack: "plain-stack", Key: "PLAIN_TOKEN", Value: stackSecret}, nil); err != nil {
		t.Fatal(err)
	}
	if err := h.call(control.CredentialSetCommand, control.CredentialSetArgs{Name: "github.com", Value: githubToken}, nil); err != nil {
		t.Fatal(err)
	}
	for _, ch := range []control.NotifyAddArgs{
		{Name: "ops", Type: control.NotifyWebhook, Secret: rcv.url("/ok"), Key: hookKey},
		{Name: "chat", Type: control.NotifySlack, Secret: rcv.url("/ok")},
		{Name: "tg", Type: control.NotifyTelegram, Secret: tgToken, ChatID: "-100123", BaseURL: rcv.URL + "/tg"},
		{Name: "mail", Type: control.NotifySMTP, Host: "mail.example.test", User: "example-user", Secret: mailPassword,
			From: "qhost@example.test", To: []string{"ops@example.test"}},
	} {
		if err := h.call(control.NotifyAddCommand, ch, nil); err != nil {
			t.Fatalf("add %s: %v", ch.Name, err)
		}
	}
	var dep control.DeployResult
	if err := h.call(control.DeployCommand, control.DeployArgs{Stack: "plain-stack"}, &dep); err != nil {
		t.Fatal(err)
	}
	waitRun(t, h, dep.ID)
	labels := map[string]string{lblProject: "plain-stack", lblDir: filepath.Join(h.dir, "plain-stack"), "example.auth": stackSecret}
	app := running("c1", "plain-stack-app-1", labels)
	fd.list = []docker.Container{app}
	fd.details = map[string]docker.Details{"c1": {Container: app, EnvNames: []string{"PLAIN_TOKEN"}}, app.Name: {Container: app, EnvNames: []string{"PLAIN_TOKEN"}}}
	fd.logs = map[string][]docker.LogLine{"c1": {
		logAt(10, docker.SourceStdout, "token "+stackSecret+" github "+githubToken),
		logAt(20, docker.SourceStderr, "webhook "+rcv.url("/ok")),
	}}

	var prop control.ProposalInfo
	if err := h.call(control.ProposalSubmitCommand, control.ProposalSubmitArgs{Kind: control.ProposalKindSourceAdd,
		Content: json.RawMessage(`{"path":"/srv/example-stacks"}`)}, &prop); err != nil {
		t.Fatal(err)
	}

	reg := seen.Load()
	reads := map[string][]any{
		control.StatusCommand:           {nil},
		control.RunsCommand:             {control.RunsArgs{}},
		control.LogCommand:              {control.LogArgs{ID: dep.ID}},
		control.StacksListCommand:       {nil},
		control.StacksCheckCommand:      {control.StacksCheckArgs{}},
		control.SourceListCommand:       {nil},
		control.KeyShowCommand:          {control.KeyShowArgs{}},
		control.CredentialListCommand:   {nil},
		control.SecretListCommand:       {control.SecretListArgs{Stack: "plain-stack"}, control.SecretListArgs{Stack: "demo-stack"}},
		control.BackupListCommand:       {control.BackupArgs{Stack: "plain-stack"}},
		control.StackSettingsGetCommand: {control.StackSettingsArgs{Stack: "demo-stack"}},
		control.UpdatesListCommand:      {control.UpdatesArgs{}},
		control.WindowShowCommand:       {nil},
		control.NotifyListCommand:       {nil},
		control.ContainerListCommand:    {control.ContainerListArgs{}},
		control.ContainerGetCommand:     {control.ContainerGetArgs{Name: app.Name}},
		control.ServiceLogCommand:       {control.ServiceLogArgs{}},
		control.ProposalListCommand:     {nil},
		control.ProposalShowCommand:     {control.ProposalIDArgs{ID: prop.ID}},
		control.ContainerLogsCommand:    {control.ContainerLogsArgs{Stack: "plain-stack"}, control.ContainerLogsArgs{Name: app.Name}},
	}
	if got, want := slices.Sorted(maps.Keys(reads)), commandsOf(reg, control.AccessRead); !slices.Equal(got, want) {
		t.Fatalf("the test covers %v, the service reads with %v", got, want)
	}

	p.as(exampleAdmin)
	for name, list := range reads {
		for _, args := range list {
			var raw json.RawMessage
			if err := h.call(name, args, &raw); err != nil {
				t.Errorf("%s %+v = %v", name, args, err)
				continue
			}
			for _, s := range secrets {
				if strings.Contains(string(raw), s) {
					t.Errorf("%s %+v answers %q: %s", name, args, s, raw)
				}
			}
		}
	}
	// The masked places still say that something was there.
	var log control.LogResult
	if err := h.call(control.LogCommand, control.LogArgs{ID: dep.ID}, &log); err != nil || !strings.Contains(log.Text, "PLAIN_TOKEN=[redacted]") {
		t.Fatalf("run log = %q, %v", log.Text, err)
	}
	var keys control.SecretListResult
	if err := h.call(control.SecretListCommand, control.SecretListArgs{Stack: "plain-stack"}, &keys); err != nil || !slices.Contains(keys.Keys, "PLAIN_TOKEN") {
		t.Fatalf("secret keys = %+v, %v", keys, err)
	}
	var creds control.CredentialListResult
	if err := h.call(control.CredentialListCommand, nil, &creds); err != nil ||
		!slices.ContainsFunc(creds.Credentials, func(c control.CredentialInfo) bool { return c.Name == "github.com" }) {
		t.Fatalf("credentials = %+v, %v", creds, err)
	}
}

func TestProposalsOverTheChannelUseThePeerOfTheRequest(t *testing.T) {
	p := &peers{}
	var seen atomic.Pointer[control.Registry]
	h := boundWith(t, &fakeDeployer{}, nil, withPeers(t, p, &seen))
	submit := control.ProposalSubmitArgs{Kind: control.ProposalKindSourceAdd, Content: json.RawMessage(`{"path":"/srv/example-stacks"}`)}

	for _, uid := range []uint32{exampleReader, exampleOutsider} {
		p.as(uid)
		wantCode(t, h.call(control.ProposalSubmitCommand, submit, nil), control.CodeNoAccess)
		wantCode(t, h.call(control.ProposalWithdrawCommand, control.ProposalIDArgs{ID: "p-0000000000000000"}, nil), control.CodeNoAccess)
	}
	p.as(exampleAdmin)
	var info control.ProposalInfo
	if err := h.call(control.ProposalSubmitCommand, submit, &info); err != nil {
		t.Fatal(err)
	}
	if info.SubmitterUID != exampleAdmin || info.SubmitterName != "example-admin" || info.State != control.ProposalOpen {
		t.Fatalf("submitted = %+v", info)
	}
	var shown control.ProposalShowResult
	if err := h.call(control.ProposalShowCommand, control.ProposalIDArgs{ID: info.ID}, &shown); err != nil || shown.Fingerprint != info.Fingerprint {
		t.Fatalf("show = %+v, %v", shown, err)
	}
	if err := h.call(control.ProposalWithdrawCommand, control.ProposalIDArgs{ID: info.ID}, &info); err != nil || info.State != control.ProposalWithdrawn {
		t.Fatalf("withdraw = %+v, %v", info, err)
	}
	// A proposal changes nothing else: an administrator still cannot write.
	wantCode(t, h.call(control.SourceAddCommand, control.SourceAddArgs{Path: "/srv/example-stacks"}, nil), control.CodeNotRoot)
}
