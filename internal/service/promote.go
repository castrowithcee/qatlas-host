package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

func (s *sourceSet) ReadPromoteSource(ctx context.Context, name string) (engine.PromoteSource, error) {
	return routed(s, name, func(e *engine.Engine) (engine.PromoteSource, error) { return e.ReadPromoteSource(ctx, name) })
}

func (s *sourceSet) PreviewPromote(ctx context.Context, target string, src engine.PromoteSource, check engine.VersionCheck) (engine.PromotePreview, error) {
	return routed(s, target, func(e *engine.Engine) (engine.PromotePreview, error) {
		return e.PreviewPromote(ctx, target, src, check)
	})
}

func (s *sourceSet) Promote(ctx context.Context, target string, src engine.PromoteSource, commit string, check engine.VersionCheck) (engine.PromoteResult, error) {
	return routed(s, target, func(e *engine.Engine) (engine.PromoteResult, error) {
		return e.Promote(ctx, target, src, commit, check)
	})
}

func registerPromoteCommands(r *control.Registry, s *updatesService) error {
	for name, h := range map[string]control.Command{
		control.PromoteSourceCommand: inStack(reading(s.handlePromoteSource), stackField),
		control.PromoteCheckCommand:  inStack(reading(s.handlePromoteCheck), stackField),
		control.PromoteRunCommand:    inStack(changing(s.handlePromoteRun), stackField),
	} {
		if err := r.Register(name, h); err != nil {
			return err
		}
	}
	return nil
}

// Shapes of the values of a handed-over source. They are the shapes a real source has; the target's own
// rules decide what is taken.
var (
	promoteNameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	promoteVarRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	promoteImageRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,254}$`)
	promoteTagRe    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	promoteResultRe = regexp.MustCompile(`^[a-z-]{0,32}$`)
)

func (s *updatesService) handlePromoteSource(ctx context.Context, raw json.RawMessage) (any, error) {
	var a control.PromoteSourceArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.Stack == "" {
		return nil, control.NewError(control.CodeBadRequest, "a stack name is required")
	}
	src, err := s.eng.ReadPromoteSource(ctx, a.Stack)
	if err != nil {
		return nil, forStack(a.Stack, promoteAnswer(ctx, err))
	}
	images, err := src.ResolvedImages()
	if err != nil {
		return nil, errors.New("the source's versions cannot be read")
	}
	st := src.State
	res := control.PromoteSource{Stack: printable(src.Name), Commit: printable(st.Deployed), FailedCommit: printable(st.FailedCommit),
		Unhealthy: st.Failure != nil || st.Repair != nil && st.Repair.Halted != nil || st.Paused != nil || st.Stopped != nil,
		Versions:  make([]control.PromoteVersion, 0, len(images))}
	if st.LastRun != nil {
		res.LastResult = printable(st.LastRun.Result)
	}
	if st.Observe != nil {
		res.Observe = &control.ObserveInfo{Commit: printable(st.Observe.Commit), Until: st.Observe.Until}
	}
	for _, img := range images {
		res.Versions = append(res.Versions, control.PromoteVersion{Variable: printable(img.Var), Image: printable(img.Image), Version: printable(img.Value)})
	}
	return res, nil
}

// shapeOK says whether the header of a handed-over source has the shape of a real one.
func shapeOK(a control.PromoteSource) bool {
	return promoteNameRe.MatchString(a.Stack) && promoteResultRe.MatchString(a.LastResult) &&
		len(a.Versions) <= control.MaxPromoteVersions && commitsOK(a)
}

func commitsOK(a control.PromoteSource) bool {
	ok := func(c string) bool { return c == "" || commitIDRe.MatchString(c) }
	return ok(a.Commit) && ok(a.FailedCommit) && (a.Observe == nil || commitIDRe.MatchString(a.Observe.Commit))
}

// engineSource checks a handed-over source for shape and size and turns it into what the engine takes. Its
// content decides only which versions are offered and which source refusals apply: the target is refused and
// every version judged by the target's own rules.
func engineSource(a control.PromoteSource) (engine.PromoteSource, error) {
	bad := control.NewError(control.CodeInvalid, "the source has an invalid shape")
	if !shapeOK(a) {
		return engine.PromoteSource{}, bad
	}
	src := engine.PromoteSource{Name: a.Stack, State: engine.StackState{Deployed: a.Commit, FailedCommit: a.FailedCommit}}
	if a.LastResult != "" {
		src.State.LastRun = &engine.RunRecord{Result: a.LastResult}
	}
	if a.Unhealthy {
		src.State.Failure = &engine.Failure{}
	}
	if a.Observe != nil {
		src.State.Observe = &engine.Observation{Commit: a.Observe.Commit, Until: a.Observe.Until}
	}
	for _, v := range a.Versions {
		if !promoteVarRe.MatchString(v.Variable) || !promoteImageRe.MatchString(v.Image) || !promoteTagRe.MatchString(v.Version) {
			return engine.PromoteSource{}, bad
		}
		// Value stands alone: SourceDefault takes it without an .env.
		src.Images = append(src.Images, stacks.ImageVar{Image: v.Image, Var: v.Variable, Value: v.Version, Source: stacks.SourceDefault})
	}
	return src, nil
}

// promoteSetup reads the arguments and returns the source and the judge of the target's versions, refusing the
// target as an update by hand does.
func (s *updatesService) promoteSetup(ctx context.Context, raw json.RawMessage) (control.PromoteArgs, engine.PromoteSource, engine.VersionCheck, error) {
	var a control.PromoteArgs
	if err := decodeArgs(raw, &a); err != nil {
		return a, engine.PromoteSource{}, nil, err
	}
	if a.Stack == "" {
		return a, engine.PromoteSource{}, nil, control.NewError(control.CodeBadRequest, "a stack name is required")
	}
	src, err := engineSource(a.Source)
	if err != nil {
		return a, src, nil, err
	}
	st, err := s.eng.CheckUpdate(ctx, a.Stack)
	if err != nil {
		return a, src, nil, forStack(a.Stack, promoteAnswer(ctx, err))
	}
	check, err := s.detector.PromoteCheck(st)
	if err != nil {
		return a, src, nil, forStack(a.Stack, promoteAnswer(ctx, err))
	}
	return a, src, check, nil
}

func promoteCheckResult(target string, p engine.PromotePreview) control.PromoteCheckResult {
	res := control.PromoteCheckResult{Stack: printable(target), Source: printable(p.Source), SourceCommit: printable(p.Commit),
		Steps: make([]control.PromoteStepInfo, 0, len(p.Steps)), Unchanged: []string{}, Kept: []string{}}
	for _, st := range p.Steps {
		res.Steps = append(res.Steps, control.PromoteStepInfo{Variable: printable(st.Variable), From: printable(st.From), To: printable(st.To)})
	}
	for _, v := range p.Unchanged {
		res.Unchanged = append(res.Unchanged, printable(v))
	}
	for _, v := range p.Kept {
		res.Kept = append(res.Kept, printable(v))
	}
	if p.Observe != nil {
		res.Observe = &control.ObserveInfo{Commit: printable(p.Observe.Commit), Until: p.Observe.Until}
	}
	return res
}

func (s *updatesService) handlePromoteCheck(ctx context.Context, raw json.RawMessage) (any, error) {
	pctx, cancel := context.WithTimeout(ctx, control.UpdatesRefreshTimeout)
	defer cancel()
	a, src, check, err := s.promoteSetup(pctx, raw)
	if err != nil {
		return nil, err
	}
	prev, err := s.eng.PreviewPromote(pctx, a.Stack, src, check)
	if err != nil {
		return nil, forStack(a.Stack, promoteAnswer(pctx, err))
	}
	return promoteCheckResult(a.Stack, prev), nil
}

func (s *updatesService) handlePromoteRun(ctx context.Context, raw json.RawMessage) (any, error) {
	pctx, cancel := context.WithTimeout(ctx, control.UpdatesRefreshTimeout)
	defer cancel()
	a, src, check, err := s.promoteSetup(pctx, raw)
	if err != nil {
		return nil, err
	}
	if !commitIDRe.MatchString(a.Commit) {
		return nil, control.NewError(control.CodeInvalid, "the commit of the preview is required")
	}
	res, err := s.eng.Promote(pctx, a.Stack, src, a.Commit, check)
	if err != nil {
		return nil, forStack(a.Stack, promoteAnswer(pctx, err))
	}
	if res.Result == engine.ResultSucceeded {
		s.recheck(ctx, []string{a.Stack})
	}
	return control.PromoteRunResult{PromoteCheckResult: promoteCheckResult(a.Stack, res.PromotePreview), Run: printable(res.RunID),
		Commit: printable(res.Commit), Result: printable(res.Result), Error: printable(res.Error)}, nil
}

// promoteAnswer maps the refusals of a promotion to fixed answers: its own, then those of an update. The
// texts of the engine's errors are fixed apart from variable and image names, which pass printable.
func promoteAnswer(ctx context.Context, err error) error {
	for _, target := range []error{engine.ErrSourceNotDeployed, engine.ErrSourceRun, engine.ErrSourceFailed, engine.ErrSourceUnhealthy,
		engine.ErrSourceChanged, engine.ErrAmbiguousImage, engine.ErrLowerVersion, engine.ErrSharedVariable, engine.ErrNotInEnv} {
		if errors.Is(err, target) {
			return control.NewError(control.CodeNotUpdatable, printable(err.Error()))
		}
	}
	return updateAnswer(ctx, err)
}
