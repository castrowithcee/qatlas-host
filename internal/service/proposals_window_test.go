package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func TestWindowProposalIsStoredCanonically(t *testing.T) {
	h := newProposalHarness(t)
	info, err := h.submit(admin, control.ProposalKindWindow, `{"days":["Monday","THU"],"start":"02:30","duration":"90m","zone":"UTC"}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.show(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	var c control.WindowSetArgs
	if err := json.Unmarshal(got.Content, &c); err != nil {
		t.Fatal(err)
	}
	if strings.Join(c.Days, ",") != "mon,thu" || c.Start != "02:30" || c.Duration != "1h30m" || c.Zone != "UTC" {
		t.Fatalf("content = %s", got.Content)
	}
}

func TestWindowProposalRefusals(t *testing.T) {
	h := newProposalHarness(t)
	for name, content := range map[string]string{
		"secret field":    `{"start":"03:00","secret":"x"}`,
		"unknown field":   `{"start":"03:00","token":"x"}`,
		"empty":           `{}`,
		"empty days":      `{"days":[]}`,
		"wrong day":       `{"days":["funday"]}`,
		"wrong time":      `{"start":"25:99"}`,
		"wrong duration":  `{"duration":"13h"}`,
		"wrong zone":      `{"zone":"Mars/Base"}`,
		"trailing data":   `{"start":"03:00"} {}`,
		"control in zone": `{"zone":"UTC\u001b[2J"}`,
		"not an object":   `"x"`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.submit(admin, control.ProposalKindWindow, content)
			if codeOf(t, err) != control.CodeInvalid {
				t.Fatalf("err = %v", err)
			}
			if strings.ContainsRune(err.Error(), 0x1b) {
				t.Fatalf("error holds a control character: %q", err)
			}
		})
	}
}
