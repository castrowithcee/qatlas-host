package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// changesBudget bounds the encoded changes of one answer, well below control.MaxResponseBytes.
const changesBudget = 512 << 10

// folderEngine returns the engine of the source bound at the folder; the folder must have the shape of a bound
// stack folder.
func (s *sourceSet) folderEngine(path string) (*engine.Engine, error) {
	if err := checkDirShape(path); err != nil {
		return nil, control.NewError(control.CodeInvalid, err.Error())
	}
	src := s.byPath(path)
	if src == nil {
		return nil, control.NewError(control.CodeNoSuchSource, "no source is bound to this folder")
	}
	return src.eng, nil
}

// changesAnswer maps an error of the change commands to a fixed answer; the text of git or of a path is never
// passed on.
func changesAnswer(err error) error {
	if ans := gitUnsafeAnswer(err); ans != nil {
		return ans
	}
	switch {
	case errors.Is(err, engine.ErrNotChanged):
		return control.NewError(control.CodeNotChanged, "the path is not a local change of the stack")
	case errors.Is(err, gitrepo.ErrUnmerged):
		return control.NewError(control.CodeInvalid, "the path has an unresolved merge and is not discarded here")
	case errors.Is(err, engine.ErrUnknownStack), errors.Is(err, engine.ErrNoRepo), errors.Is(err, engine.ErrRunActive),
		errors.Is(err, engine.ErrStopped), errors.Is(err, engine.ErrBusy):
		return runsAnswer(err)
	}
	return errors.New("the working copy cannot be read or changed")
}

// shown replaces the characters of a name or a diff that a terminal would act on. Newlines and tabs stay in a
// diff when keepLayout is set.
func shown(s string, keepLayout bool) string {
	return strings.Map(func(r rune) rune {
		if keepLayout && (r == '\n' || r == '\t') || unicode.IsPrint(r) {
			return r
		}
		return '?'
	}, s)
}

// The change commands name the bound stack folder, so they have the scope deploy like the other folder commands.
// The list is a read. The diff shows file contents, and discard and commit change files, so all three are root's alone.
func registerChangeCommands(r *control.Registry, eng *sourceSet) error {
	commands := map[string]control.Command{
		control.SourceChangesCommand: reading(func(ctx context.Context, raw json.RawMessage) (any, error) {
			var a control.SourceChangesArgs
			if err := decodeArgs(raw, &a); err != nil {
				return nil, err
			}
			e, err := eng.folderEngine(a.Path)
			if err != nil {
				return nil, err
			}
			changes, err := e.Changes(ctx)
			if err != nil {
				return nil, changesAnswer(err)
			}
			res := control.SourceChangesResult{Path: a.Path, Changes: make([]control.SourceChange, 0, len(changes))}
			used := 0
			for _, c := range changes {
				used += len(c.Path) + len(c.Stack) + 64
				if used > changesBudget {
					res.Truncated = true
					break
				}
				res.Changes = append(res.Changes, control.SourceChange{Path: shown(c.Path, false), Kind: c.Kind, Stack: printable(c.Stack)})
			}
			return res, nil
		}),
		control.SourceDiffCommand: changing(func(ctx context.Context, raw json.RawMessage) (any, error) {
			var a control.SourceDiffArgs
			if err := decodeArgs(raw, &a); err != nil {
				return nil, err
			}
			e, err := eng.folderEngine(a.Path)
			if err != nil {
				return nil, err
			}
			text, truncated, err := e.Diff(ctx, a.File)
			if err != nil {
				return nil, changesAnswer(err)
			}
			return control.SourceDiffResult{Path: a.Path, File: shown(a.File, false), Diff: shown(text, true), Truncated: truncated}, nil
		}),
		control.SourceDiscardCommand: discardHandler(eng),
		control.SourceLogCommand:     logHandler(eng),
		control.SourceShowCommand:    showHandler(eng),
		control.SourceCommitCommand:  commitHandler(eng),
	}
	for name, h := range commands {
		if err := r.Register(name, h); err != nil {
			return err
		}
	}
	return nil
}

func discardHandler(eng *sourceSet) control.Command {
	return changing(func(ctx context.Context, raw json.RawMessage) (any, error) {
		var a control.SourceDiscardArgs
		if err := decodeArgs(raw, &a); err != nil {
			return nil, err
		}
		if len(a.Files) == 0 || len(a.Files) > control.MaxDiscardPaths {
			return nil, control.NewError(control.CodeInvalid, "name between one and 200 files to discard")
		}
		e, err := eng.folderEngine(a.Path)
		if err != nil {
			return nil, err
		}
		if err := e.Discard(ctx, a.Files); err != nil {
			return nil, changesAnswer(err)
		}
		res := control.SourceDiscardResult{Path: a.Path, Discarded: make([]string, len(a.Files))}
		for i, p := range a.Files {
			res.Discarded[i] = shown(p, false)
		}
		return res, nil
	})
}
