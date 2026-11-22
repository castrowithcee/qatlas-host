package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

func TestMetaCarriesTheTagOnlyWhenOneIsGiven(t *testing.T) {
	const tag = "qhost/demo01/demo/20260504T030000Z-0a1b2c3d"
	cases := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"none", context.Background(), ""},
		{"given", stacks.WithDeployedTag(context.Background(), testCommit, tag), tag},
		{"other commit", stacks.WithDeployedTag(context.Background(), strings.Repeat("a", 40), tag), ""},
		{"stray text", stacks.WithDeployedTag(context.Background(), testCommit, "bad tag\nname"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, rulesYAML)
			res, err := e.m.Create(c.ctx, e.stack(), testCommit, e.log)
			if err != nil {
				t.Fatal(err)
			}
			if res.Meta.Tag != c.want {
				t.Fatalf("tag = %q, want %q", res.Meta.Tag, c.want)
			}
			raw, err := os.ReadFile(filepath.Join(e.root, "demo", res.Meta.ID, metaFile))
			if err != nil {
				t.Fatal(err)
			}
			if has := strings.Contains(string(raw), `"tag"`); has != (c.want != "") {
				t.Fatalf("meta.json has tag = %v:\n%s", has, raw)
			}
			list, err := e.m.List("demo")
			if err != nil || len(list) != 1 || list[0].Tag != c.want || list[0].Incomplete {
				t.Fatalf("list = %+v, %v", list, err)
			}
		})
	}
}
