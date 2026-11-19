package deploysetup

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/packaging"
)

// refreshRoot is a host below a temporary root with the given instances set up and a runner that records.
func refreshRoot(t *testing.T, fail string, users ...string) (Host, *[]string) {
	t.Helper()
	root := t.TempDir()
	var ran []string
	h := Host{Root: root, Run: func(_ context.Context, name string, args ...string) error {
		call := name + " " + strings.Join(args, " ")
		ran = append(ran, call)
		if fail != "" && strings.HasSuffix(call, fail) {
			return errors.New("exit status 1")
		}
		return nil
	}}
	for _, u := range users {
		inst, err := NewInstance(u)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.write(inst.TmpfilesConf(), []byte("d "+inst.ChannelDir()+"\n")); err != nil {
			t.Fatal(err)
		}
	}
	return h, &ran
}

func unitOf(t *testing.T, user string) string {
	t.Helper()
	inst, err := NewInstance(user)
	if err != nil {
		t.Fatal(err)
	}
	return inst.Unit()
}

func TestRefreshWithoutInstanceChangesNothing(t *testing.T) {
	h, ran := refreshRoot(t, "")
	res, err := h.RefreshInstances(context.Background())
	if err != nil || len(*ran) != 0 || res.TemplateWritten || len(res.Restarted) != 0 {
		t.Fatalf("res %+v, err %v, ran %v", res, err, *ran)
	}
	if _, err := os.Stat(h.path(TemplatePath())); err == nil {
		t.Error("the template was written without an instance")
	}
}

func TestRefreshWritesDifferingTemplateAndRestartsEveryInstance(t *testing.T) {
	h, ran := refreshRoot(t, "", "bob", "alice")
	if err := h.write(TemplatePath(), []byte("old template")); err != nil {
		t.Fatal(err)
	}
	res, err := h.RefreshInstances(context.Background())
	if err != nil || !res.TemplateWritten {
		t.Fatalf("res %+v, err %v", res, err)
	}
	if got, _ := os.ReadFile(h.path(TemplatePath())); string(got) != string(packaging.DeployInstanceUnit) {
		t.Errorf("template = %q", got)
	}
	want := []string{"systemctl daemon-reload", "systemctl restart " + unitOf(t, "alice"), "systemctl restart " + unitOf(t, "bob")}
	if !slices.Equal(*ran, want) {
		t.Errorf("ran %q, want %q", *ran, want)
	}
}

func TestRefreshKeepsTheSameTemplate(t *testing.T) {
	h, ran := refreshRoot(t, "", "alice")
	if err := h.write(TemplatePath(), packaging.DeployInstanceUnit); err != nil {
		t.Fatal(err)
	}
	old := h.path(TemplatePath())
	before, _ := os.Stat(old)
	res, err := h.RefreshInstances(context.Background())
	after, _ := os.Stat(old)
	if err != nil || res.TemplateWritten || !os.SameFile(before, after) || len(*ran) != 2 {
		t.Fatalf("res %+v, err %v, ran %v", res, err, *ran)
	}
}

func TestRefreshReportsOverriddenUnits(t *testing.T) {
	h, _ := refreshRoot(t, "", "alice", "bob")
	if err := h.write(shadowDir+"/"+unitOf(t, "bob"), []byte("[Service]\n")); err != nil {
		t.Fatal(err)
	}
	res, err := h.RefreshInstances(context.Background())
	if err != nil || !slices.Equal(res.Shadowed, []string{unitOf(t, "bob")}) {
		t.Fatalf("res %+v, err %v", res, err)
	}
	// An overriding template covers every instance.
	if err := h.write(shadowDir+"/"+Service+"@.service", []byte("[Service]\n")); err != nil {
		t.Fatal(err)
	}
	res, err = h.RefreshInstances(context.Background())
	if err != nil || len(res.Shadowed) != 2 {
		t.Fatalf("res %+v, err %v", res, err)
	}
}

func TestRefreshRestartsTheOthersAfterAFailure(t *testing.T) {
	h, _ := refreshRoot(t, "", "alice", "bob")
	failing := unitOf(t, "alice")
	h.Run = func(_ context.Context, name string, args ...string) error {
		if len(args) == 2 && args[1] == failing {
			return errors.New("exit status 1")
		}
		return nil
	}
	res, err := h.RefreshInstances(context.Background())
	if err == nil || !strings.Contains(err.Error(), failing) || !slices.Equal(res.Restarted, []string{unitOf(t, "bob")}) {
		t.Fatalf("res %+v, err %v", res, err)
	}
}

func TestRefreshStopsWhenTheReloadFails(t *testing.T) {
	h, ran := refreshRoot(t, "daemon-reload", "alice")
	if _, err := h.RefreshInstances(context.Background()); err == nil || len(*ran) != 1 {
		t.Fatalf("err %v, ran %v", err, *ran)
	}
}
