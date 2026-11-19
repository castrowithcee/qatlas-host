package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func TestNotifyChannelProposalIsStoredCanonically(t *testing.T) {
	h := newProposalHarness(t)
	info, err := h.submit(admin, control.ProposalKindNotifyChannel,
		`{"name":"mail","type":"smtp","events":["recovered","failed","failed"],"host":"mail.example.test","from":"qhost@example.test","to":["ops@example.test"],"user":"qhost"}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.show(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	var c control.ProposalNotifyChannel
	if err := json.Unmarshal(got.Content, &c); err != nil {
		t.Fatal(err)
	}
	if strings.Join(c.Events, ",") != "failed,recovered" || c.Security != "starttls" || c.Name != "mail" || c.User != "qhost" {
		t.Fatalf("content = %s", got.Content)
	}
	info, err = h.submit(admin, control.ProposalKindNotifyChannel, `{"name":"hook","type":"webhook"}`)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = h.show(info.ID)
	if err := json.Unmarshal(got.Content, &c); err != nil || len(c.Events) != 8 {
		t.Fatalf("all events are written out: %s", got.Content)
	}
}

func TestNotifyChannelProposalRefusals(t *testing.T) {
	h := newProposalHarness(t)
	const secretValue = "demo-secret-value-not-real"
	for name, content := range map[string]string{
		"secret on webhook":      `{"name":"hook","type":"webhook","secret":"` + secretValue + `"}`,
		"key on webhook":         `{"name":"hook","type":"webhook","key":"` + secretValue + `"}`,
		"empty secret":           `{"name":"hook","type":"webhook","secret":""}`,
		"secret on slack":        `{"name":"chat","type":"slack","secret":"` + secretValue + `"}`,
		"secret on telegram":     `{"name":"bot","type":"telegram","chat_id":"@opschat","secret":"` + secretValue + `"}`,
		"password on smtp":       `{"name":"mail","type":"smtp","host":"mail.example.test","from":"qhost@example.test","to":["ops@example.test"],"user":"qhost","secret":"` + secretValue + `"}`,
		"unknown password field": `{"name":"mail","type":"smtp","password":"` + secretValue + `"}`,
		"unknown url field":      `{"name":"hook","type":"webhook","url":"https://hooks.example.test/` + secretValue + `"}`,
		"bad name":               `{"name":"Hook","type":"webhook"}`,
		"bad type":               `{"name":"hook","type":"pager"}`,
		"bad event":              `{"name":"hook","type":"webhook","events":["exploded"]}`,
		"smtp field on webhook":  `{"name":"hook","type":"webhook","host":"mail.example.test"}`,
		"chat field on smtp":     `{"name":"mail","type":"smtp","host":"mail.example.test","from":"qhost@example.test","to":["ops@example.test"],"chat_id":"1"}`,
		"smtp field on telegram": `{"name":"bot","type":"telegram","chat_id":"1","host":"mail.example.test"}`,
		"bad chat":               `{"name":"bot","type":"telegram","chat_id":"nope"}`,
		"bad base url":           `{"name":"bot","type":"telegram","chat_id":"1","base_url":"ftp://x.example.test"}`,
		"bad port":               `{"name":"mail","type":"smtp","host":"mail.example.test","port":70000,"from":"qhost@example.test","to":["ops@example.test"]}`,
		"bad recipient":          `{"name":"mail","type":"smtp","host":"mail.example.test","from":"qhost@example.test","to":["nope"]}`,
		"trailing data":          `{"name":"hook","type":"webhook"} {}`,
		"not an object":          `"x"`,
		"control in host":        `{"name":"mail","type":"smtp","host":"mail.example.test\u001b[2J","from":"qhost@example.test","to":["ops@example.test"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.submit(admin, control.ProposalKindNotifyChannel, content)
			if codeOf(t, err) != control.CodeInvalid {
				t.Fatalf("err = %v", err)
			}
			if strings.ContainsRune(err.Error(), 0x1b) || strings.Contains(err.Error(), secretValue) {
				t.Fatalf("error holds a control character or a value: %q", err)
			}
		})
	}
	all, _ := h.p.load()
	if len(all) != 0 {
		t.Fatalf("a refused proposal was stored: %d", len(all))
	}
}

func TestNotifyChannelProposalNeverStoresASecret(t *testing.T) {
	h := newProposalHarness(t)
	if _, err := h.submit(admin, control.ProposalKindNotifyChannel, `{"name":"hook","type":"webhook","key":"demo-key-not-real"}`); err == nil {
		t.Fatal("a secret field was accepted")
	}
}

func TestNotifyChannelProposalRefusesATakenName(t *testing.T) {
	h := newProposalHarness(t)
	h.p.channelKnown = func(name string) (bool, error) { return name == "hook", nil }
	_, err := h.submit(admin, control.ProposalKindNotifyChannel, `{"name":"hook","type":"webhook"}`)
	if codeOf(t, err) != control.CodeInvalid || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("err = %v", err)
	}
	if _, err := h.submit(admin, control.ProposalKindNotifyChannel, `{"name":"other","type":"webhook"}`); err != nil {
		t.Fatal(err)
	}
	h.p.channelKnown = func(string) (bool, error) { return false, errors.New("unreadable") }
	_, err = h.submit(admin, control.ProposalKindNotifyChannel, `{"name":"third","type":"webhook"}`)
	if codeOf(t, err) != control.CodeInternal {
		t.Fatalf("err = %v", err)
	}
}
