package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// logBudget bounds the encoded commits of one answer, well below control.MaxResponseBytes.
const logBudget = 512 << 10

// historyAnswer maps an error of the history commands to a fixed answer.
func historyAnswer(err error) error {
	switch {
	case errors.Is(err, gitrepo.ErrCommitNotFound):
		return control.NewError(control.CodeNotFound, "the commit is not in the history of the stack folder")
	case errors.Is(err, gitrepo.ErrBadCommitID):
		return control.NewError(control.CodeInvalid, "the commit must be a hexadecimal commit id of at least 7 digits")
	}
	return changesAnswer(err)
}

// logCommit masks the foreign text of a commit and adds its encoded size to used.
func logCommit(c gitrepo.Commit, used *int) control.SourceLogCommit {
	out := control.SourceLogCommit{
		ID: c.ID, Author: printable(c.Author), Date: c.Date.Format(time.RFC3339), Subject: printable(c.Subject),
		Paths: make([]control.SourceLogPath, len(c.Paths)), PathsTruncated: c.PathsTruncated,
	}
	*used += len(out.ID) + len(out.Author) + len(out.Subject) + 128
	for i, p := range c.Paths {
		out.Paths[i] = control.SourceLogPath{Path: shown(p.Path, false), Kind: string(p.Kind)}
		*used += len(p.Path) + 48
	}
	return out
}

func logHandler(eng *sourceSet) control.Command {
	return reading(func(ctx context.Context, raw json.RawMessage) (any, error) {
		var a control.SourceLogArgs
		if err := decodeArgs(raw, &a); err != nil {
			return nil, err
		}
		if a.Limit == 0 {
			a.Limit = control.DefaultLogLimit
		}
		if a.Limit < 1 || a.Limit > control.MaxLogLimit {
			return nil, control.NewError(control.CodeInvalid, "the limit must be between 1 and 200")
		}
		e, err := eng.folderEngine(a.Path)
		if err != nil {
			return nil, err
		}
		commits, truncated, err := e.History(ctx, a.Limit)
		if err != nil {
			return nil, historyAnswer(err)
		}
		res := control.SourceLogResult{Path: a.Path, Commits: make([]control.SourceLogCommit, 0, len(commits)), Truncated: truncated}
		used := 0
		for _, c := range commits {
			entry := logCommit(c, &used)
			if used > logBudget {
				res.Truncated = true
				break
			}
			res.Commits = append(res.Commits, entry)
		}
		return res, nil
	})
}

// showHandler is root's alone because the diff shows file contents.
func showHandler(eng *sourceSet) control.Command {
	return changing(func(ctx context.Context, raw json.RawMessage) (any, error) {
		var a control.SourceShowArgs
		if err := decodeArgs(raw, &a); err != nil {
			return nil, err
		}
		e, err := eng.folderEngine(a.Path)
		if err != nil {
			return nil, err
		}
		text, truncated, err := e.ShowCommit(ctx, a.Commit)
		if err != nil {
			return nil, historyAnswer(err)
		}
		return control.SourceShowResult{Path: a.Path, Commit: shown(a.Commit, false), Diff: shown(text, true), Truncated: truncated}, nil
	})
}
