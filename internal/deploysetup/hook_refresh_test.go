package deploysetup

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-host/packaging"
)

func TestRefreshHookWithoutSetupChangesNothing(t *testing.T) {
	h, ran := refreshRoot(t, "")
	res, err := h.RefreshHook(context.Background())
	if err != nil || len(*ran) != 0 || res.UnitWritten || res.Restarted {
		t.Fatalf("res %+v, err %v, ran %v", res, err, *ran)
	}
	if _, err := os.Stat(h.path(HookUnitPath())); err == nil {
		t.Error("the unit was written without the setup")
	}
}

func TestRefreshHookReplacesADifferingUnitReloadsAndRestarts(t *testing.T) {
	h, ran := refreshRoot(t, "")
	if err := h.write(HookUnitPath(), []byte("old unit")); err != nil {
		t.Fatal(err)
	}
	res, err := h.RefreshHook(context.Background())
	if err != nil || !res.UnitWritten || !res.Restarted {
		t.Fatalf("res %+v, err %v", res, err)
	}
	if got, _ := os.ReadFile(h.path(HookUnitPath())); string(got) != string(packaging.HookUnit) {
		t.Errorf("unit = %q", got)
	}
	want := []string{"systemctl daemon-reload", "systemctl restart qhost-hook.service"}
	if !slices.Equal(*ran, want) {
		t.Errorf("ran %q, want %q", *ran, want)
	}
}

func TestRefreshHookKeepsTheSameUnitAndOnlyRestarts(t *testing.T) {
	h, ran := refreshRoot(t, "")
	if err := h.write(HookUnitPath(), packaging.HookUnit); err != nil {
		t.Fatal(err)
	}
	res, err := h.RefreshHook(context.Background())
	if err != nil || res.UnitWritten || !res.Restarted || !slices.Equal(*ran, []string{"systemctl restart qhost-hook.service"}) {
		t.Fatalf("res %+v, err %v, ran %q", res, err, *ran)
	}
}

func TestRefreshHookReportsAFailedRestartAndAShadowingUnit(t *testing.T) {
	h, _ := refreshRoot(t, "qhost-hook.service")
	if err := h.write(HookUnitPath(), packaging.HookUnit); err != nil {
		t.Fatal(err)
	}
	if err := h.write(shadowDir+"/qhost-hook.service", []byte("[Service]\n")); err != nil {
		t.Fatal(err)
	}
	res, err := h.RefreshHook(context.Background())
	if err == nil || res.Restarted || !res.Shadowed {
		t.Fatalf("res %+v, err %v", res, err)
	}
}
