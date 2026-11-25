package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
	"github.com/castrowithcee/qatlas-host/internal/version"
)

// TriggerPromote is the takeover of the image versions of another stack, see Engine.Promote.
const TriggerPromote = "promote"

// Refusals of PreviewPromote and Promote, besides those of an update of the target (ErrPaused, ErrStackStopped,
// ErrPinned, ErrNotDeployed and the others of CheckUpdate).
var (
	ErrSourceNotDeployed = errors.New("the source stack was never deployed successfully")
	ErrSourceRun         = errors.New("the last run of the source stack did not succeed")
	ErrSourceFailed      = errors.New("the source stack has a commit its last run failed at")
	ErrSourceUnhealthy   = errors.New("the source stack is not healthy: it has a failure, a halted repair, or is paused or stopped")
	ErrSourceChanged     = errors.New("the source stack deployed another commit since the preview; preview again")
	ErrAmbiguousImage    = errors.New("the source stack runs one image in different versions")
	ErrLowerVersion      = errors.New("the source version is lower than the target's; to go back to an earlier version use 'qhost stack rollback'")
	ErrSharedVariable    = errors.New("the variable is the tag of more than one image")
	ErrNotInEnv          = errors.New("the version is not set in the stack's .env")
)

// PromoteSource is what the target's engine needs to know of the source stack. The source may belong to
// another engine (another deploy instance on the same host): its engine reads this with ReadPromoteSource and
// the caller hands it over, since an engine sees only the state and working copy of its own stacks.
type PromoteSource struct {
	Name  string
	State StackState
	// Images are the source's image variables; only Var, Image, Source and Value are used.
	Images []stacks.ImageVar
	// Env is the source's .env as its deployed commit (State.Deployed) has it. It is foreign input.
	Env []byte
}

// VersionCheck judges moving the target's image variable to version to as an update by hand would: the caller
// asks the registry and applies version.Path. It returns version.ErrNotHigher for a lower version. The engine
// calls it without holding any lock.
type VersionCheck func(ctx context.Context, img stacks.ImageVar, to string) error

// PromotePreview is what a promotion does: the variables it sets, the target variables that already have the
// source's version or have no image in common with the source, and the source's observation time while it runs.
type PromotePreview struct {
	Source string
	// Commit is the source's deployed commit the versions were read at; Promote is bound to it.
	Commit    string
	Steps     []VersionStep
	Unchanged []string
	Kept      []string
	Observe   *Observation
}

// PromoteResult is the outcome of Promote. With no steps nothing was to do and no run was made. Commit is the
// pushed commit, or the deployed one when the run failed before its push.
type PromoteResult struct {
	PromotePreview
	RunID, Commit, Result, Error string
}

// ReadPromoteSource reads the stack name of this engine's working copy as a promotion source: its state, its
// image variables and the .env of its deployed commit.
func (e *Engine) ReadPromoteSource(ctx context.Context, name string) (PromoteSource, error) {
	b, err := e.cfg.Source()
	if err != nil {
		return PromoteSource{}, err
	}
	if err := e.lockCommand(ctx); err != nil {
		return PromoteSource{}, err
	}
	defer e.lock.Unlock()
	list, err := stacks.Load(b.Dir)
	if err != nil {
		return PromoteSource{}, fmt.Errorf("read stacks: %w", err)
	}
	i := slices.IndexFunc(list, func(s stacks.Stack) bool { return s.Name == name })
	if i < 0 {
		return PromoteSource{}, ErrUnknownStack
	}
	cur, _, err := e.Stack(name)
	if err != nil {
		return PromoteSource{}, err
	}
	src := PromoteSource{Name: name, State: cur, Images: list[i].Images}
	if cur.Deployed != "" {
		if src.Env, err = b.Copy.FileAt(ctx, cur.Deployed, list[i].RepoPath(envName)); err != nil {
			return PromoteSource{}, fmt.Errorf("read %s of the source: %w", envName, err)
		}
	}
	return src, nil
}

// checkSource refuses a source whose last run did not succeed, that has a failed commit or that is not healthy.
// A running observation time prevents nothing.
func checkSource(s StackState) error {
	switch {
	case s.Deployed == "" || s.LastRun == nil:
		return ErrSourceNotDeployed
	case s.LastRun.Result != ResultSucceeded:
		return ErrSourceRun
	case s.FailedCommit != "":
		return ErrSourceFailed
	case s.Failure != nil, s.Repair != nil && s.Repair.Halted != nil, s.Paused != nil, s.Stopped != nil:
		return ErrSourceUnhealthy
	}
	return nil
}

// ResolvedImages returns the source's image variables that have a version at the deployed commit, each with
// that version in Value and Source set to stacks.SourceDefault, so that the result stands alone: a
// PromoteSource built from it needs no Env.
func (s PromoteSource) ResolvedImages() ([]stacks.ImageVar, error) {
	env, err := stacks.ParseEnv(s.Env)
	if err != nil {
		return nil, fmt.Errorf("read %s of the source: %w", envName, err)
	}
	var out []stacks.ImageVar
	for _, img := range s.Images {
		v, ok := env[img.Var]
		if !ok && img.Source == stacks.SourceDefault {
			v, ok = img.Value, true
		}
		if img.Var == "" || !ok || v == "" {
			continue
		}
		img.Value, img.Source = v, stacks.SourceDefault
		out = append(out, img)
	}
	return out, nil
}

// sourceVersions maps each image repository of the source to its version at the deployed commit, by the
// repository and not by the variable name. A repository with different versions is ambiguous and listed.
func sourceVersions(src PromoteSource) (map[string]string, error) {
	images, err := src.ResolvedImages()
	if err != nil {
		return nil, err
	}
	versions := map[string]string{}
	var ambiguous []string
	for _, img := range images {
		if old, seen := versions[img.Image]; seen && old != img.Value && !slices.Contains(ambiguous, img.Image) {
			ambiguous = append(ambiguous, img.Image)
		}
		versions[img.Image] = img.Value
	}
	if len(ambiguous) > 0 {
		slices.Sort(ambiguous)
		return nil, fmt.Errorf("%w: %s", ErrAmbiguousImage, strings.Join(ambiguous, ", "))
	}
	return versions, nil
}

// PreviewPromote works out what Promote would do, refusing as Promote does, and changes nothing. The target is
// refused as an update by hand to a chosen version refuses it. Every variable of the target whose image the
// source runs in another version moves to the source's version; check judges each such move. A higher
// version is taken, the same one is left, and a lower one is refused with ErrLowerVersion.
func (e *Engine) PreviewPromote(ctx context.Context, target string, src PromoteSource, check VersionCheck) (PromotePreview, error) {
	if err := checkSource(src.State); err != nil {
		return PromotePreview{}, err
	}
	st, err := e.CheckUpdate(ctx, target)
	if err != nil {
		return PromotePreview{}, err
	}
	versions, err := sourceVersions(src)
	if err != nil {
		return PromotePreview{}, err
	}
	prev := PromotePreview{Source: src.Name, Commit: src.State.Deployed}
	if o := src.State.Observe; o != nil && o.Until.After(e.cfg.Clock.Now()) {
		prev.Observe = o
	}
	seen := map[string]string{}
	for _, img := range st.Images {
		if img.Var == "" {
			continue
		}
		if repo, dup := seen[img.Var]; dup {
			if repo != img.Image {
				return PromotePreview{}, fmt.Errorf("%w: %s", ErrSharedVariable, img.Var)
			}
			continue
		}
		seen[img.Var] = img.Image
		to, ok := versions[img.Image]
		switch {
		case !ok:
			prev.Kept = append(prev.Kept, img.Var)
			continue
		case to == img.Value:
			prev.Unchanged = append(prev.Unchanged, img.Var)
			continue
		}
		if err := check(ctx, img, to); err != nil {
			if errors.Is(err, version.ErrNotHigher) {
				err = fmt.Errorf("%w (%s)", ErrLowerVersion, err)
			}
			return PromotePreview{}, fmt.Errorf("%s: %w", img.Var, err)
		}
		if img.Source != stacks.SourceEnv {
			return PromotePreview{}, fmt.Errorf("%s: %w", img.Var, ErrNotInEnv)
		}
		prev.Steps = append(prev.Steps, VersionStep{Variable: img.Var, From: img.Value, To: to})
	}
	return prev, nil
}

// Promote sets every variable of the target that PreviewPromote names to the source's version, as one run of
// the target with trigger promote: stop, backup, one commit that sets all the variables, pushed like an
// update's, then pull, up and check; a failing run is rolled back like any run. The run is bound to the
// preview: commit is the PreviewPromote's Commit, and when src is not at that commit (the source deployed
// another one) nothing is done and the answer is ErrSourceChanged. The target is refused as in PreviewPromote
// and as Update refuses it; with no variable to set no run is made. The run goes on in the caller's goroutine
// under the run time limit and independent of ctx.
func (e *Engine) Promote(ctx context.Context, target string, src PromoteSource, commit string, check VersionCheck) (PromoteResult, error) {
	if commit == "" || src.State.Deployed != commit {
		return PromoteResult{}, ErrSourceChanged
	}
	prev, err := e.PreviewPromote(ctx, target, src, check)
	if err != nil || len(prev.Steps) == 0 {
		return PromoteResult{PromotePreview: prev}, err
	}
	b, err := e.cfg.Source()
	if err != nil {
		return PromoteResult{}, err
	}
	st, deployed, err := e.takeTarget(ctx, b, target, prev.Steps)
	if err != nil {
		return PromoteResult{}, err
	}
	held, slotFree := true, false
	defer func() {
		if !slotFree {
			e.unslot(target)
		}
		e.finish(held)
	}()

	res := PromoteResult{PromotePreview: prev}
	r, err := e.stepRun(b, st, deployed, TriggerPromote)
	if err != nil {
		res.Error = "the run log cannot be created"
		e.cfg.Logger.Error("promote not started", "stack", target, "error", err)
		return res, nil
	}
	parts := make([]string, len(prev.Steps))
	for i, s := range prev.Steps {
		parts[i] = fmt.Sprintf("%s from %s to %s", s.Variable, s.From, s.To)
	}
	message := fmt.Sprintf("Promote %s from %s: %s", target, src.Name, strings.Join(parts, "; "))
	r.held, r.change, r.back = held, &prev.Steps[0], &returnJob{changes: prev.Steps, message: message}
	out := r.perform()
	held = r.held
	rec := r.record(out, true)
	slotFree = true
	e.notify(Event{Kind: rec.Result, Stack: rec.Stack, Run: rec.ID, Commit: rec.Commit, Error: rec.Error})
	res.RunID, res.Commit, res.Result, res.Error = rec.ID, rec.Commit, rec.Result, rec.Error
	return res, nil
}

// takeTarget takes the target's slot for the promotion and returns the stack and its deployed commit, refusing
// it as an update is refused and when it no longer has the versions the steps start from.
func (e *Engine) takeTarget(ctx context.Context, b Binding, target string, steps []VersionStep) (stacks.Stack, string, error) {
	if err := e.lockCommand(ctx); err != nil {
		return stacks.Stack{}, "", err
	}
	defer e.lock.Unlock()
	st, head, err := e.updatable(ctx, b, target)
	if err != nil {
		return stacks.Stack{}, "", err
	}
	if err := e.take(target, "promote"); err != nil {
		return stacks.Stack{}, "", err
	}
	deployed, err := e.updateBase(ctx, b, target, st.Folder, head, false)
	if err == nil {
		err = checkPlan(st, steps)
	}
	if err != nil {
		e.free(target, true)
		return stacks.Stack{}, "", err
	}
	return st, deployed, nil
}

// changesVersions reports whether a run of the trigger sets image versions by a commit of its own, so that it
// is under observation and can be returned.
func changesVersions(trigger string) bool {
	return trigger == TriggerUpdate || trigger == TriggerPromote
}
