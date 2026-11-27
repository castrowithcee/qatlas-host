package cli

import (
	"context"
	"os"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// deployRootTransition and deployServiceTransitions live apart from the register so that a test release can
// replace the register file and keep them.
var deployRootTransition = transition{
	name: "deploy-service-as-root",
	done: func(_ context.Context, root string) (bool, error) {
		old, err := deployHost(root).HasAccountSetup()
		return !old, err
	},
	apply: func(ctx context.Context, root string) error {
		return deployHost(root).MoveToRoot(ctx, os.Stderr)
	},
	applyAtStart: func(ctx context.Context, root string) error {
		return deployHost(root).MoveToRootAtStart(ctx, os.Stderr)
	},
}

// deployServiceTransitions are the transitions of the deploy service, which its start step runs. They change only
// what the deploy service owns, so they are safe before it opens its channel.
var deployServiceTransitions = []transition{deployRootTransition}

// deployHost is the real host below root, or the host tests set up.
func deployHost(root string) deploysetup.Host {
	h := deploySetup.host
	h.Root = root
	return h
}
