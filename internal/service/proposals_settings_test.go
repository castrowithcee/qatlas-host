package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const okSettings = `{"stack":"web","settings":{"version":1,"update":{"mode":"automatic"},"backup":{"paths":["data"],"command":["backup-tool","--quiet"]}}}`

func settingsHarness(t *testing.T) *proposalHarness {
	h := newProposalHarness(t)
	h.hooks.stackKnown = func(name string) bool { return name == "web" }
	return h
}

func TestStackSettingsProposalIsStoredCanonically(t *testing.T) {
	h := settingsHarness(t)
	info, err := h.submit(admin, control.ProposalKindStackSettings, okSettings)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.show(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	var c control.ProposalStackSettings
	if err := json.Unmarshal(got.Content, &c); err != nil {
		t.Fatal(err)
	}
	// Defaults are written out, as 'qhost stack settings' shows them.
	if c.Stack != "web" || c.Settings.Update == nil || c.Settings.Update.Ceiling != "minor" ||
		c.Settings.Backup == nil || strings.Join(c.Settings.Backup.Command, " ") != "backup-tool --quiet" {
		t.Fatalf("content = %s", got.Content)
	}
}

func TestStackSettingsProposalRefusals(t *testing.T) {
	h := settingsHarness(t)
	for name, content := range map[string]string{
		"secret at top level":      `{"stack":"web","secret":"x","settings":{"version":1}}`,
		"token in settings":        `{"stack":"web","settings":{"version":1,"token":"x"}}`,
		"secret in update":         `{"stack":"web","settings":{"version":1,"update":{"secret":"x"}}}`,
		"token in backup":          `{"stack":"web","settings":{"version":1,"backup":{"paths":["data"],"token":"x"}}}`,
		"password in http check":   `{"stack":"web","settings":{"version":1,"check":{"http":{"url":"http://web.example.test/","password":"x"}}}}`,
		"unknown stack":            `{"stack":"other","settings":{"version":1}}`,
		"bad stack name":           `{"stack":"../web","settings":{"version":1}}`,
		"no stack":                 `{"settings":{"version":1}}`,
		"no version":               `{"stack":"web","settings":{}}`,
		"invalid mode":             `{"stack":"web","settings":{"version":1,"update":{"mode":"sometimes"}}}`,
		"absolute backup path":     `{"stack":"web","settings":{"version":1,"backup":{"paths":["/etc"]}}}`,
		"empty backup":             `{"stack":"web","settings":{"version":1,"backup":{}}}`,
		"empty backup program":     `{"stack":"web","settings":{"version":1,"backup":{"command":[""]}}}`,
		"invalid http url":         `{"stack":"web","settings":{"version":1,"check":{"http":{"url":"ftp://web.example.test/"}}}}`,
		"trailing data":            okSettings + ` {}`,
		"settings not an object":   `{"stack":"web","settings":"x"}`,
		"control characters in id": `{"stack":"we\u001bb","settings":{"version":1}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.submit(admin, control.ProposalKindStackSettings, content)
			if codeOf(t, err) != control.CodeInvalid {
				t.Fatalf("err = %v", err)
			}
			if strings.ContainsRune(err.Error(), 0x1b) {
				t.Fatalf("error holds a control character: %q", err)
			}
		})
	}
}

func TestStackSettingsProposalNeedsAStackCheck(t *testing.T) {
	h := newProposalHarness(t)
	if _, err := h.submit(admin, control.ProposalKindStackSettings, okSettings); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("without a stack check = %v", err)
	}
}
