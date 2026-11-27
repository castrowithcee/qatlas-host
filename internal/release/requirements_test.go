package release

import (
	"errors"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/release/releasetest"
)

func TestParseRequirementsIgnoresUnknownFieldsAndChecksTheValues(t *testing.T) {
	r, err := ParseRequirements([]byte(`{"updatesFrom": "v0.4.0", "reinstall": "", "later": true}`))
	if err != nil || r.UpdatesFrom != "v0.4.0" || r.Reinstall != "" {
		t.Fatalf("%+v, %v", r, err)
	}
	for _, bad := range []string{
		`{"updatesFrom": ""}`,
		`{"updatesFrom": "0.4.0"}`,
		`{"updatesFrom": "v0.4.0-rc.1"}`,
		`{"updatesFrom": "v0.4.0", "reinstall": "line\nbreak"}`,
		`{"updatesFrom": "v0.4.0", "reinstall": "` + strings.Repeat("x", 501) + `"}`,
		`not json`,
	} {
		if _, err := ParseRequirements([]byte(bad)); err == nil {
			t.Errorf("%s passed", bad)
		}
	}
}

func TestRequirementsCheck(t *testing.T) {
	r := Requirements{UpdatesFrom: "v0.4.0"}
	for installed, refused := range map[string]bool{
		"v0.3.9": true, "v0.4.0-rc.2": true, "v0.4.0": false, "v0.4.1": false, "v0.10.0": false, "v1.0.0-rc.1": false,
	} {
		err := r.Check(installed, "v0.5.0")
		var refusal *Refusal
		if refused != errors.As(err, &refusal) || (!refused && err != nil) {
			t.Errorf("%s: %v", installed, err)
			continue
		}
		if refused && (!strings.Contains(refusal.Message, "v0.4.0") || !strings.Contains(refusal.NextStep, "sudo qhost update v0.4.0 --confirm")) {
			t.Errorf("%s: %+v", installed, refusal)
		}
	}
	if err := r.Check("dev", "v0.5.0"); err == nil || errors.As(err, new(*Refusal)) {
		t.Errorf("an unordered installed version: %v", err)
	}
	r.Reinstall = "Install v0.5.0 afresh."
	var refusal *Refusal
	if err := r.Check("v0.4.1", "v0.5.0"); !errors.As(err, &refusal) || refusal.NextStep != r.Reinstall {
		t.Fatalf("reinstall: %v", err)
	}
}

func TestReadRequirementsFromTheArchive(t *testing.T) {
	name := "qhost_v0.5.0_linux_amd64.tar.gz"
	program := releasetest.File{Name: "bin/qhost", Body: []byte("program")}
	archive := releasetest.Archive(t, name, program,
		releasetest.File{Name: RequirementsEntry, Body: []byte(`{"updatesFrom": "v0.4.0", "reinstall": ""}`)})
	r, found, err := ReadRequirements(archive)
	if err != nil || !found || r.UpdatesFrom != "v0.4.0" {
		t.Fatalf("%+v, %v, %v", r, found, err)
	}
	if _, found, err := ReadRequirements(releasetest.Archive(t, name, program)); err != nil || found {
		t.Fatalf("without the entry: %v, %v", found, err)
	}
	bad := releasetest.Archive(t, name, program, releasetest.File{Name: RequirementsEntry, Body: []byte(`{}`)})
	if _, _, err := ReadRequirements(bad); err == nil {
		t.Fatal("an entry without updatesFrom passed")
	}
	if _, _, err := ReadRequirements([]byte("no archive")); err == nil {
		t.Fatal("bytes that are no archive passed")
	}
}
