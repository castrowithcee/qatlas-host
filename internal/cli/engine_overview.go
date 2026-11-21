package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The note in the help of the overviews that ask every deploy service.
const overviewEngineNote = "On a host with several deploy services the command asks all of them: the one of the system engine, when it " +
	"is set up, and the instance of each rootless engine; each entry shows its engine, " + control.EngineSystem + " or " +
	rootlessPrefix + "<user>. A deploy service that cannot be asked is named with its engine in a warning and the " +
	"others are still shown."

// engineReply is what the deploy service of one engine answered; err is set when it could not be asked.
type engineReply[T any] struct {
	target engineTarget
	value  T
	err    error
}

// askEach sends one command to the deploy service of every target at the same time and answers in the order
// of targets.
func askEach[T any](ctx context.Context, limit time.Duration, targets []engineTarget, command string, args any) []engineReply[T] {
	out := make([]engineReply[T], len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		out[i].target = t
		wg.Go(func() {
			out[i].err = callFor(ctx, limit, t.endpoint(), command, args, &out[i].value)
		})
	}
	wg.Wait()
	return out
}

// askOverview asks every deploy service of the host for an overview and returns the answers of those that
// could be asked, in the order system engine, then instances by user. On a host with one deploy service a
// failure ends the command as it always did. With several, a service without a bound stack folder adds
// nothing, a service that cannot be asked is a warning with its engine, and the command fails only when none
// has an answer.
func askOverview[T any](c *cobra.Command, state *notRunning, command string) ([]engineReply[T], error) {
	targets, err := folders.engineTargets()
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		// Not set up: the call to the system engine says so.
		targets = []engineTarget{{}}
	}
	limit := client.CallTimeout
	if len(targets) > 1 {
		limit = peerLimit
	}
	ok, failed, empty := splitOverview(len(targets), askEach[T](c.Context(), limit, targets, command, nil))
	if len(ok) == 0 && len(failed) == 0 {
		// No service has a stack folder: the first one says so, as on a host with only that service.
		return nil, targetFailure(withEngine(c.Context(), empty[0].target), empty[0].err)
	}
	if len(ok) == 0 && len(empty) == 0 {
		if errors.Is(failed[0].err, client.ErrNotRunning) {
			state.seen = true
		}
		return nil, targetFailure(withEngine(c.Context(), failed[0].target), failed[0].err)
	}
	for _, f := range failed {
		_, _ = fmt.Fprintln(c.ErrOrStderr(), "warning: "+uncheckedText(engineAnswer{target: f.target, err: f.err}))
	}
	return ok, nil
}

// splitOverview sorts the answers of an overview into those with a result, those whose service could not be
// asked and, on a host with several services, those without a bound stack folder, which add nothing.
func splitOverview[T any](targets int, replies []engineReply[T]) (ok, failed, empty []engineReply[T]) {
	for _, r := range replies {
		var remote *control.Error
		switch {
		case r.err == nil:
			ok = append(ok, r)
		case targets > 1 && errors.As(r.err, &remote) && remote.Code == control.CodeNoRepo:
			empty = append(empty, r)
		default:
			failed = append(failed, r)
		}
	}
	return ok, failed, empty
}

// splitRun sorts the answers about a run into the services that know it, those that could not be asked and
// the first "no such run" error. A service that does not know the run is no problem.
func splitRun(replies []engineReply[control.LogResult]) (known []engineReply[control.LogResult], unchecked []engineAnswer, first error) {
	for _, r := range replies {
		var remote *control.Error
		switch {
		case r.err == nil:
			known = append(known, r)
		case errors.As(r.err, &remote) && remote.Code == control.CodeNoSuchRun:
			if first == nil {
				first = r.err
			}
		default:
			unchecked = append(unchecked, engineAnswer{target: r.target, err: r.err})
		}
	}
	return known, unchecked, first
}

// askRun asks every deploy service for the log of a run and answers with the one that knows it. A service
// that does not know the run is no problem; one that cannot be asked is a warning when another knows the run,
// and otherwise named in the error, because the run may be there.
func askRun(c *cobra.Command, state *notRunning, targets []engineTarget, id string) (engineTarget, control.LogResult, error) {
	known, unchecked, first := splitRun(askEach[control.LogResult](c.Context(), peerLimit, targets, control.LogCommand, control.LogArgs{ID: id}))
	switch {
	case len(known) > 1:
		ids := make([]string, len(known))
		for i, k := range known {
			ids[i] = k.target.id()
		}
		return engineTarget{}, control.LogResult{}, usageFailure(
			fmt.Sprintf("the run %s exists in the engines %s", id, strings.Join(ids, ", ")),
			"Choose one with --engine, for example --engine "+ids[0]+".")
	case len(known) == 1:
		for _, u := range unchecked {
			_, _ = fmt.Fprintln(c.ErrOrStderr(), "warning: "+uncheckedText(u))
		}
		return known[0].target, known[0].value, nil
	case len(unchecked) == len(targets):
		// Nobody answered: the first service tells why, as it would on a host with only that service.
		if errors.Is(unchecked[0].err, client.ErrNotRunning) {
			state.seen = true
		}
		return engineTarget{}, control.LogResult{}, targetFailure(withEngine(c.Context(), unchecked[0].target), unchecked[0].err)
	case len(unchecked) > 0:
		return engineTarget{}, control.LogResult{}, &cli.Failure{Code: output.CodeUnreachable,
			Message: fmt.Sprintf("the run %s is in none of the engines that answered; %s", id, uncheckedList(unchecked)),
			NextStep: "Start that deploy service and run the command again, or name its engine with --engine " +
				unchecked[0].engine() + ".", Cause: unchecked[0].err}
	}
	return engineTarget{}, control.LogResult{}, failureOf(first)
}
