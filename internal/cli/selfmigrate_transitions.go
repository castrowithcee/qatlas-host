package cli

import (
	"context"
	"os"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// transitions are the steps of this release that bring the setup and the stored state of an earlier one to it, in
// the order they run. 'qhost update migrate' runs them after every update.
var transitions = []transition{{
	name: "deploy-service-as-root",
	done: func(_ context.Context, root string) (bool, error) {
		old, err := deploysetup.Host{Root: root}.HasAccountSetup()
		return !old, err
	},
	apply: func(ctx context.Context, root string) error {
		return deploysetup.Host{Root: root}.MoveToRoot(ctx, os.Stderr)
	},
}}
