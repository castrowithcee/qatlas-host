package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/idmap"
)

const (
	rootlessUID = "         0       1000          1\n         1     100000      65536\n"
	rootlessGID = "         0       1000          1\n         1     100000      65536\n"
	otherUID    = "         0       1001          1\n         1     200000      65536\n"
)

const identityRules = "version: 1\nbackup:\n  paths: [data/db]\n"

// asSchema1 rewrites the metadata of a backup the way a version before the identity wrote it.
func asSchema1(t *testing.T, e *env, id string) {
	t.Helper()
	path := filepath.Join(e.root, "demo", id, metaFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["schema"] = 1
	delete(doc, "identity")
	out, _ := json.Marshal(doc)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// destructive lists the calls that change the stack or its data.
func destructive(e *env) []string {
	var out []string
	for _, s := range e.docker.summaries() {
		if strings.HasPrefix(s, "compose stop") || strings.HasPrefix(s, "compose up") || strings.Contains(s, "rm -rf") {
			out = append(out, s)
		}
	}
	return out
}

func TestCreateStoresTheIdentityInSchema2(t *testing.T) {
	for name, tc := range map[string]struct {
		uid, gid, engine string
	}{
		"system":   {"", "", EngineSystem},
		"rootless": {rootlessUID, rootlessGID, EngineRootless},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, identityRules)
			e.docker.uidMap, e.docker.gidMap = tc.uid, tc.gid
			res, err := e.create()
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(filepath.Join(e.root, "demo", res.Meta.ID, metaFile))
			var doc struct {
				Schema   int       `json:"schema"`
				Identity *Identity `json:"identity"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil || doc.Schema != 2 || doc.Identity == nil || doc.Identity.Engine != tc.engine {
				t.Fatalf("meta = %s, %v", raw, err)
			}
			list, err := e.m.List("demo")
			if err != nil || len(list) != 1 || list[0].Incomplete || list[0].Identity == nil || list[0].Identity.Engine != tc.engine {
				t.Fatalf("list = %+v, %v", list, err)
			}
			if tc.engine == EngineRootless && !list[0].Identity.UIDMap.Equal(idmap.Map{{Inside: 0, Outside: 1000, Count: 1}, {Inside: 1, Outside: 100000, Count: 65536}}) {
				t.Fatalf("uid map = %v", list[0].Identity.UIDMap)
			}
		})
	}
}

func TestReadMetaReadsSchema1WithoutIdentityAndSchema2(t *testing.T) {
	e := newEnv(t, identityRules)
	old, err := e.create()
	if err != nil {
		t.Fatal(err)
	}
	asSchema1(t, e, old.Meta.ID)
	e.now = e.now.Add(1e9)
	cur, err := e.create()
	if err != nil {
		t.Fatal(err)
	}
	for id, wantIdentity := range map[string]bool{old.Meta.ID: false, cur.Meta.ID: true} {
		meta, err := readMeta(filepath.Join(e.root, "demo", id), "demo", id)
		if err != nil || (meta.Identity != nil) != wantIdentity {
			t.Fatalf("%s: %+v, %v", id, meta, err)
		}
	}
	if ok, err := e.m.Exists("demo", old.Meta.ID); err != nil || !ok {
		t.Fatalf("exists = %v, %v", ok, err)
	}
}

func TestRestoreRefusesAnOtherNamespaceBeforeAnythingChanges(t *testing.T) {
	tests := map[string]struct{ made, now string }{
		"system to rootless":   {"", rootlessUID},
		"rootless to system":   {rootlessUID, ""},
		"rootless to rootless": {rootlessUID, otherUID},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, identityRules)
			e.docker.uidMap, e.docker.gidMap = tc.made, orIf(tc.made, rootlessGID)
			res, err := e.create()
			if err != nil {
				t.Fatal(err)
			}
			e.docker.calls = nil
			e.docker.uidMap, e.docker.gidMap = tc.now, orIf(tc.now, rootlessGID)
			_, err = e.m.Restore(context.Background(), e.stack(), res.Meta.ID, e.log)
			if !errors.Is(err, ErrOtherNamespace) {
				t.Fatalf("err = %v", err)
			}
			if d := destructive(e); len(d) != 0 {
				t.Fatalf("changed something: %v", d)
			}
		})
	}
}

func orIf(s, v string) string {
	if s == "" {
		return ""
	}
	return v
}

func TestRestoreRefusesAChangedGIDMapAlone(t *testing.T) {
	e := newEnv(t, identityRules)
	e.docker.uidMap, e.docker.gidMap = rootlessUID, rootlessGID
	res, _ := e.create()
	e.docker.gidMap = otherUID
	if _, err := e.m.Restore(context.Background(), e.stack(), res.Meta.ID, e.log); !errors.Is(err, ErrOtherNamespace) {
		t.Fatalf("err = %v", err)
	}
}

func TestRestoreOfAnEqualNamespaceWorksAndSplitRangesAreTheSame(t *testing.T) {
	e := newEnv(t, identityRules)
	e.docker.uidMap, e.docker.gidMap = rootlessUID, rootlessGID
	res, _ := e.create()
	// The same mapping written as two consecutive ranges is the same namespace.
	e.docker.uidMap = "0 1000 1\n1 100000 100\n101 100100 65436\n"
	if _, err := e.m.Restore(context.Background(), e.stack(), res.Meta.ID, e.log); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestBackupWithoutIdentityIsSystemOnly(t *testing.T) {
	e := newEnv(t, identityRules)
	res, _ := e.create()
	asSchema1(t, e, res.Meta.ID)

	if _, err := e.m.Restore(context.Background(), e.stack(), res.Meta.ID, e.log); err != nil {
		t.Fatalf("system engine: %v", err)
	}
	e.docker.calls = nil
	e.docker.uidMap, e.docker.gidMap = rootlessUID, rootlessGID
	if _, err := e.m.Restore(context.Background(), e.stack(), res.Meta.ID, e.log); !errors.Is(err, ErrOtherNamespace) {
		t.Fatalf("rootless engine: %v", err)
	}
	if d := destructive(e); len(d) != 0 {
		t.Fatalf("changed something: %v", d)
	}
}

func TestUnreadableNamespaceIsARefusal(t *testing.T) {
	e := newEnv(t, identityRules)
	res, _ := e.create()
	for name, set := range map[string]func(){
		"helper fails":   func() { e.docker.namespaceErr = errors.New("boom") },
		"garbage map":    func() { e.docker.namespaceErr, e.docker.uidMap = nil, "not a map\n" },
		"overlap":        func() { e.docker.uidMap = "0 0 10\n5 100 10\n" },
		"no gid section": func() { e.docker.uidMap, e.docker.gidMap = "0 0 4294967295\n", "\n" },
	} {
		t.Run(name, func(t *testing.T) {
			e.docker.namespaceErr, e.docker.uidMap, e.docker.gidMap = nil, "", ""
			set()
			e.docker.calls = nil
			if _, err := e.m.Restore(context.Background(), e.stack(), res.Meta.ID, e.log); !errors.Is(err, ErrOtherNamespace) {
				t.Fatalf("err = %v", err)
			}
			if d := destructive(e); len(d) != 0 {
				t.Fatalf("changed something: %v", d)
			}
		})
	}
	// A backup cannot be made in a namespace that cannot be read either.
	e.docker.namespaceErr, e.docker.uidMap = errors.New("boom"), ""
	if _, err := e.create(); err == nil {
		t.Fatal("create succeeded")
	}
}

func TestUnreadableStoredIdentityIsARefusal(t *testing.T) {
	e := newEnv(t, identityRules)
	res, _ := e.create()
	path := filepath.Join(e.root, "demo", res.Meta.ID, metaFile)
	data, _ := os.ReadFile(path)
	bad := strings.Replace(string(data), `"count": 4294967295`, `"count": 0`, 2)
	if bad == string(data) {
		t.Fatal("fixture did not change")
	}
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	e.docker.calls = nil
	if _, err := e.m.Restore(context.Background(), e.stack(), res.Meta.ID, e.log); !errors.Is(err, ErrOtherNamespace) {
		t.Fatalf("err = %v", err)
	}
	if d := destructive(e); len(d) != 0 {
		t.Fatalf("changed something: %v", d)
	}
}

func TestRollbackRestoreRefusesAnOtherNamespace(t *testing.T) {
	e := newEnv(t, identityRules)
	res, _ := e.create()
	e.docker.calls = nil
	e.docker.uidMap, e.docker.gidMap = rootlessUID, rootlessGID
	err := e.m.ForRuns().Restore(context.Background(), e.stack(), res.Meta.ID, e.log)
	if !errors.Is(err, ErrOtherNamespace) {
		t.Fatalf("err = %v", err)
	}
	if d := destructive(e); len(d) != 0 {
		t.Fatalf("changed something: %v", d)
	}
}
