package proposals

import (
	"context"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestUnknownFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, build := range map[string]func(id string) string{
		"higher": func(id string) string {
			return `{"schema":2,"id":"` + id + `","kind":"example","added_later":true}`
		},
		"missing": func(id string) string {
			return `{"id":"` + id + `","kind":"example","content":{"name":"example-one"},"fingerprint":"` +
				`","submitter_uid":4100,"submitter_name":"example-admin","submitted":"2030-01-01T00:00:00Z","state":"open"}`
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			id, err := h.p.newID()
			if err != nil {
				t.Fatal(err)
			}
			doc := build(id)
			path := h.p.path(id)
			formatnumtest.Seed(t, path, doc)

			_, err = readRecord(path)
			formatnumtest.Refused(t, err, path)

			// The others stay usable, the file is neither listed, replaced, pruned nor withdrawn.
			if _, err := h.submit(admin, kindExample, okContent); err != nil {
				t.Fatal(err)
			}
			if err := h.withdraw(admin, id); err == nil {
				t.Fatal("an unreadable proposal was withdrawn")
			}
			h.now = h.now.Add(10 * keep)
			if _, err := h.submit(other, kindExample, okContent); err != nil {
				t.Fatal(err)
			}
			v, err := h.p.List(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			list := v.(control.ProposalListResult)
			for _, p := range list.Proposals {
				if p.ID == id {
					t.Fatal("an unreadable proposal was listed")
				}
			}
			// The readable ones stay listed, and the unreadable one is named instead of missing silently.
			if len(list.Proposals) != 1 {
				t.Fatalf("proposals = %d, want the one readable", len(list.Proposals))
			}
			if len(list.Unreadable) != 1 || list.Unreadable[0].File != path || !strings.Contains(list.Unreadable[0].Error, path) {
				t.Fatalf("unreadable = %+v", list.Unreadable)
			}
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}
