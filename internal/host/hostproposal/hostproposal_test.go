package hostproposal

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// checker builds the production actions on a host whose files do not exist, so any read of the host would fail.
func checker(t *testing.T) Checker {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "missing")
	accounts := hostadmin.NewHost(true)
	accounts.PasswdPath, accounts.GroupPath, accounts.ShadowPath = missing+"/passwd", missing+"/group", missing+"/shadow"
	r := hostchange.NewRegistry()
	if err := hostchange.RegisterUserActions(r, hostchange.UserHost{Accounts: accounts, GShadow: missing + "/gshadow",
		HomeBase: "/home", MailDir: missing + "/mail", Proc: missing + "/proc"}); err != nil {
		t.Fatal(err)
	}
	if err := hostchange.RegisterFolderActions(r, hostchange.FolderHost{Accounts: accounts}); err != nil {
		t.Fatal(err)
	}
	if err := hostchange.RegisterFileActions(r); err != nil {
		t.Fatal(err)
	}
	return New(r)
}

func testKey(t *testing.T, comment string) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + " " + comment
}

func TestProposableActionsPass(t *testing.T) {
	c := checker(t)
	key := testKey(t, "demo@example.test")
	for name, tc := range map[string]struct {
		action     string
		args, keys []string
	}{
		"create with docker, group and key": {hostchange.ActionUserCreate, []string{"demo-new", "docker", "group=demo-team"}, []string{"  " + key + "  "}},
		"create without key":                {hostchange.ActionUserCreate, []string{"demo-new"}, nil},
		"add key":                           {hostchange.ActionUserAddKey, []string{"demo-user"}, []string{key}},
		"enable":                            {hostchange.ActionUserEnable, []string{"demo-user"}, nil},
		"docker":                            {hostchange.ActionUserSetDocker, []string{"demo-user", "on"}, nil},
		"members of another group":          {hostchange.ActionGroupSetMembers, []string{"docker", "+demo-user", "-demo-old"}, nil},
		"folder":                            {hostchange.ActionFolderSetPermissions, []string{"/srv/demo", "demo-user", "demo-team", "read-write", "read", "nothing"}, nil},
	} {
		keys, err := c.Check(tc.action, tc.args, tc.keys)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(tc.keys) > 0 && (len(keys) != 1 || keys[0] != key) {
			t.Errorf("%s: keys %q, want the canonical line", name, keys)
		}
	}
}

func TestRefusedProposals(t *testing.T) {
	c := checker(t)
	key := testKey(t, "demo@example.test")
	for name, tc := range map[string]struct {
		action     string
		args, keys []string
		want       string
	}{
		"set-admin":                  {hostchange.ActionUserSetAdmin, []string{"demo-user", "on"}, nil, "cannot be proposed"},
		"set-password":               {hostchange.ActionUserSetPassword, []string{"demo-user"}, nil, "cannot be proposed"},
		"file write":                 {hostchange.ActionFileWrite, []string{"/srv/demo/app.conf"}, nil, "cannot be proposed"},
		"file create folder":         {hostchange.ActionFileCreateFolder, []string{"/srv/demo/data"}, nil, "cannot be proposed"},
		"file rename":                {hostchange.ActionFileRename, []string{"/srv/demo/a", "/srv/demo/b"}, nil, "cannot be proposed"},
		"file copy":                  {hostchange.ActionFileCopy, []string{"/srv/demo/a", "/srv/demo/b"}, nil, "cannot be proposed"},
		"file remove":                {hostchange.ActionFileRemove, []string{"/srv/demo/a"}, nil, "cannot be proposed"},
		"create with admin switch":   {hostchange.ActionUserCreate, []string{"demo-new", "admin"}, nil, "admin switch"},
		"create with admin later":    {hostchange.ActionUserCreate, []string{"demo-new", "docker", "admin"}, nil, "admin switch"},
		"unknown action":             {"user.make-root", []string{"demo-user"}, nil, "unknown action"},
		"argument outside schema":    {hostchange.ActionUserCreate, []string{"demo-new", "--shell=/bin/sh"}, nil, "do not fit"},
		"invalid name":               {hostchange.ActionUserLock, []string{"Demo;rm"}, nil, "do not fit"},
		"too many arguments":         {hostchange.ActionUserEnable, []string{"demo-user", "demo-other"}, nil, "do not fit"},
		"sudo members added":         {hostchange.ActionGroupSetMembers, []string{hostadmin.GroupAdmin, "+demo-user"}, nil, "members of sudo"},
		"sudo members removed":       {hostchange.ActionGroupSetMembers, []string{hostadmin.GroupAdmin, "-demo-user"}, nil, "members of sudo"},
		"qhost members added":        {hostchange.ActionGroupSetMembers, []string{hostadmin.GroupQhost, "+demo-user"}, nil, "members of qhost"},
		"qhost members removed":      {hostchange.ActionGroupSetMembers, []string{hostadmin.GroupQhost, "-demo-user"}, nil, "members of qhost"},
		"sudo as further group":      {hostchange.ActionUserAddGroups, []string{"demo-user", hostadmin.GroupAdmin}, nil, "do not fit"},
		"key that is no key":         {hostchange.ActionUserAddKey, []string{"demo-user"}, []string{"demo-not-a-key"}, "public key"},
		"private key text":           {hostchange.ActionUserAddKey, []string{"demo-user"}, []string{"-----BEGIN OPENSSH " + "PRIVATE KEY-----"}, "public key"},
		"key with options":           {hostchange.ActionUserAddKey, []string{"demo-user"}, []string{`command="/bin/sh" ` + key}, "options"},
		"add-key without key":        {hostchange.ActionUserAddKey, []string{"demo-user"}, nil, "exactly one"},
		"add-key with two keys":      {hostchange.ActionUserAddKey, []string{"demo-user"}, []string{key, testKey(t, "two")}, "exactly one"},
		"key on an action without":   {hostchange.ActionUserLock, []string{"demo-user"}, []string{key}, "takes no SSH key"},
		"same key twice":             {hostchange.ActionUserCreate, []string{"demo-new"}, []string{key, key}, "twice"},
		"system path unacknowledged": {hostchange.ActionFolderSetPermissions, []string{"/etc/demo", "root", "root", "read-write", "read", "nothing"}, nil, "system path"},
	} {
		_, err := c.Check(tc.action, tc.args, tc.keys)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v, want %q", name, err, tc.want)
		}
	}
}

func TestPlanCarriesThePasswordAndTheProposedKeys(t *testing.T) {
	c := checker(t)
	key := testKey(t, "demo@example.test")
	plan, secret, err := c.Plan(hostchange.ActionUserCreate, []string{"demo-new", "docker"}, []string{key}, "demo-initial-1")
	if err != nil {
		t.Fatal(err)
	}
	var in hostchange.CreateInput
	if err := json.Unmarshal(secret, &in); err != nil || in.Password != "demo-initial-1" || len(in.Keys) != 1 || in.Keys[0] != key {
		t.Fatalf("secret %+v, %v", in, err)
	}
	shown := plan.Sentence
	for _, cmd := range plan.Commands {
		shown += cmd.Display()
	}
	if strings.Contains(shown, "demo-initial-1") || !strings.Contains(shown, "<password>") || !strings.Contains(shown, "SHA256:") {
		t.Fatalf("shown %q", shown)
	}
	view, err := c.View(hostchange.ActionUserAddKey, []string{"demo-user"}, []string{key})
	if err != nil || len(view.Commands) != 1 || !strings.Contains(view.Sentence, "SHA256:") {
		t.Fatalf("view %+v, %v", view, err)
	}
	if !NeedsPassword(hostchange.ActionUserCreate) || NeedsPassword(hostchange.ActionUserAddKey) {
		t.Fatal("NeedsPassword")
	}
}
