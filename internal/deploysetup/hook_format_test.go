package deploysetup

import (
	"context"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
	"github.com/castrowithcee/qatlas-host/internal/hookservice"
)

func TestHookSetupKeepsAConfigurationWithAnUnknownFormatNumber(t *testing.T) {
	for name, doc := range formatnumtest.Cases("version", hookservice.ConfigVersion, `"listen":"127.0.0.1:8480","hooks":[]`) {
		t.Run(name, func(t *testing.T) {
			h := newHookHost(t)
			h.put(HookConfigPath, doc)
			_, err := h.host.PlanHook(context.Background(), "")
			formatnumtest.Refused(t, err, h.host.path(HookConfigPath))
			_, err = HookConfig(h.root)
			formatnumtest.Refused(t, err, hostPath(h.root, HookConfigPath))
			if h.read(HookConfigPath) != doc {
				t.Fatal("the configuration was changed")
			}
		})
	}
}
