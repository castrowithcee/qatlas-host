package updates

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/registry"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
	"github.com/castrowithcee/qatlas-host/internal/version"
)

// Kinds of a PlanError.
const (
	// PlanRefused: the stack cannot be updated as it is, whatever the version.
	PlanRefused = "refused"
	// PlanVersion: the requested version cannot be taken.
	PlanVersion = "version"
	// PlanRegistry: the registry could not be asked.
	PlanRegistry = "registry"
)

// PlanError is why no update could be planned. Text is fixed, apart from the variable name, and fit to show.
type PlanError struct {
	Kind     string
	Variable string
	Text     string
}

func (e *PlanError) Error() string {
	if e.Variable == "" {
		return e.Text
	}
	return clip(e.Variable) + ": " + e.Text
}

// updatable refuses a stack that has no readable files or no settings, whatever the version.
func updatable(st stacks.Stack) error {
	refuse := func(text string) error { return &PlanError{Kind: PlanRefused, Text: text} }
	switch {
	case st.Err != nil:
		return refuse("the stack cannot be read")
	case st.EnvErr != nil:
		return refuse("the stack is deploy-only: " + st.EnvErr.Error())
	case st.Rules == nil:
		return refuse("the stack has no settings and is never updated; set them in the qhost TUI or with 'qhost stack settings set'")
	}
	return nil
}

// PromoteCheck returns the judge for moving one image variable of the stack to a version taken from another
// stack: it asks the registry as Plan does for a chosen version, in one jump, under the stack's variant filter.
// A version that is not higher gives version.ErrNotHigher. The stack is refused as Plan refuses it. The judge
// keeps the tag lists it fetched and is not for concurrent use.
func (d *Detector) PromoteCheck(st stacks.Stack) (engine.VersionCheck, error) {
	if err := updatable(st); err != nil {
		return nil, err
	}
	run := &run{d: d, clients: map[string]*registry.Client{}, creds: map[string]error{}, tags: map[string]tagResult{}}
	return func(ctx context.Context, img stacks.ImageVar, to string) error {
		repo, err := registry.ParseRepo(img.Image)
		if err != nil {
			return &PlanError{Kind: PlanRefused, Variable: img.Var, Text: textBadImage}
		}
		res := run.list(ctx, repo)
		if err := ctx.Err(); err != nil {
			return err
		}
		if res.text != "" {
			return &PlanError{Kind: PlanRegistry, Variable: img.Var, Text: res.text}
		}
		in := version.Input{Current: img.Value, Candidates: res.tags, Ceiling: st.Rules.Update.Ceiling, Filter: st.Rules.Update.Tags[img.Var]}
		if _, err := version.Path(in, to, false); err != nil {
			if errors.Is(err, version.ErrNotHigher) {
				return err
			}
			return &PlanError{Kind: PlanVersion, Variable: img.Var, Text: err.Error()}
		}
		return nil
	}, nil
}

// Plan returns the steps that update the stack, from the tags the registries list now. Without target, every
// image version variable whose rules allow a newer version moves to the version detection would select; with
// target, one image version variable moves to target, which may lie above the ceiling: the one named by
// variable, or the stack's only one when variable is empty; no other variable is considered. A move across
// major versions under update.major stepwise visits the highest version of every major version on the way.
// The variables keep the order of the stack's images; a stack with nothing to update has no steps. It only
// reads.
//
// Without target, a variable is left out when its path would visit the version failed[variable], a version
// whose update failed and was rolled back; failed is not consulted with target.
func (d *Detector) Plan(ctx context.Context, st stacks.Stack, target, variable string, failed map[string]string) ([]engine.VersionStep, error) {
	refuse := func(variable, text string) error {
		return &PlanError{Kind: PlanRefused, Variable: variable, Text: text}
	}
	if err := updatable(st); err != nil {
		return nil, err
	}
	var vars []stacks.ImageVar
	for _, img := range st.Images {
		i := slices.IndexFunc(vars, func(v stacks.ImageVar) bool { return v.Var == img.Var })
		switch {
		case i < 0:
			vars = append(vars, img)
		case vars[i].Image != img.Image:
			return nil, refuse(img.Var, "the variable is the tag of more than one image")
		}
	}
	if target != "" {
		var err error
		if vars, err = narrow(vars, variable); err != nil {
			return nil, err
		}
	}

	run := &run{d: d, clients: map[string]*registry.Client{}, creds: map[string]error{}, tags: map[string]tagResult{}}
	stepwise := st.Rules.Update.Major == stacks.MajorStepwise
	var steps []engine.VersionStep
	for _, img := range vars {
		repo, err := registry.ParseRepo(img.Image)
		if err != nil {
			return nil, refuse(img.Var, textBadImage)
		}
		res := run.list(ctx, repo)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if res.text != "" {
			return nil, &PlanError{Kind: PlanRegistry, Variable: img.Var, Text: res.text}
		}
		in := version.Input{Current: img.Value, Candidates: res.tags, Ceiling: st.Rules.Update.Ceiling, Filter: st.Rules.Update.Tags[img.Var]}
		to := target
		if to == "" {
			sel, err := version.Select(in)
			if err != nil {
				return nil, refuse(img.Var, textBadCeiling)
			}
			if sel.Status != version.StatusUpdate {
				continue
			}
			to = sel.Selected
		}
		path, err := version.Path(in, to, stepwise)
		if err != nil {
			// Path's errors are fixed texts.
			return nil, &PlanError{Kind: PlanVersion, Variable: img.Var, Text: err.Error()}
		}
		if target == "" && failed[img.Var] != "" && slices.Contains(path, failed[img.Var]) {
			continue
		}
		if img.Source != stacks.SourceEnv {
			return nil, refuse(img.Var, "the version is not set in the stack's .env")
		}
		from := img.Value
		for _, v := range path {
			steps = append(steps, engine.VersionStep{Variable: img.Var, From: from, To: v})
			from = v
		}
	}
	return steps, nil
}

// narrow returns the one variable a target applies to: the one named by variable, or the only one.
func narrow(vars []stacks.ImageVar, variable string) ([]stacks.ImageVar, error) {
	names := make([]string, len(vars))
	for i, v := range vars {
		names[i] = clip(v.Var)
	}
	switch {
	case variable != "":
		i := slices.IndexFunc(vars, func(v stacks.ImageVar) bool { return v.Var == variable })
		if i < 0 {
			return nil, &PlanError{Kind: PlanVersion, Text: "the stack has no image version variable " + clip(variable) +
				"; its variables are " + strings.Join(names, ", ")}
		}
		return []stacks.ImageVar{vars[i]}, nil
	case len(vars) != 1:
		return nil, &PlanError{Kind: PlanVersion, Text: "the stack has several image version variables; name one with --var: " +
			strings.Join(names, ", ")}
	}
	return vars, nil
}
