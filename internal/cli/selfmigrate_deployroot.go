package cli

import (
	"context"
	"os"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// deployRootTransition lives apart from the register so that a test release can replace the register file and
// keep this transition.
var deployRootTransition = transition{
	name: "deploy-service-as-root",
	done: func(_ context.Context, root string) (bool, error) {
		old, err := deploysetup.Host{Root: root}.HasAccountSetup()
		return !old, err
	},
	apply: func(ctx context.Context, root string) error {
		return deploysetup.Host{Root: root}.MoveToRoot(ctx, os.Stderr)
	},
}
