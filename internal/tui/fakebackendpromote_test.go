package tui

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// promoteFake is the state of the promotion calls: promotes records the calls of Promote and promoteReads those of
// PromoteSource; promoteCheck and promoteRes, when set, are the answers.
type promoteFake struct {
	promotes     []control.PromoteArgs
	promoteReads []string
	promoteCheck *control.PromoteCheckResult
	promoteRes   *control.PromoteRunResult
}

const promoteCommit = "89abcdef0123456789abcdef0123456789abcdef"

func (f *fakeBackend) PromoteSource(ctx context.Context, stack string) (control.PromoteSource, error) {
	if err := f.actCtx(ctx, "promotesource", stack); err != nil {
		return control.PromoteSource{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.promoteReads = append(f.promoteReads, stack)
	return control.PromoteSource{Stack: stack, Commit: promoteCommit, LastResult: "succeeded",
		Versions: []control.PromoteVersion{{Variable: "APP_VERSION", Image: "example/app", Version: "1.5.0"}}}, nil
}

func (f *fakeBackend) PromoteCheck(_ context.Context, args control.PromoteArgs) (control.PromoteCheckResult, error) {
	if err := f.fail("promotecheck", args.Stack); err != nil {
		return control.PromoteCheckResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.promoteCheck != nil {
		return *f.promoteCheck, nil
	}
	return control.PromoteCheckResult{Stack: args.Stack, Source: args.Source.Stack, SourceCommit: args.Source.Commit,
		Steps: []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.4.2", To: "1.5.0"}}}, nil
}

func (f *fakeBackend) Promote(ctx context.Context, args control.PromoteArgs) (control.PromoteRunResult, error) {
	if err := f.actCtx(ctx, "promote", args.Stack); err != nil {
		return control.PromoteRunResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.promotes = append(f.promotes, args)
	if f.promoteRes != nil {
		return *f.promoteRes, nil
	}
	return control.PromoteRunResult{PromoteCheckResult: control.PromoteCheckResult{Stack: args.Stack, Source: args.Source.Stack,
		SourceCommit: args.Commit, Steps: []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.4.2", To: "1.5.0"}}},
		Run: "20261101T040000Z-00000003", Commit: "0123456789abcdef", Result: "succeeded"}, nil
}
