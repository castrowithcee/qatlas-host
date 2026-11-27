package service

import (
	"errors"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// gitUnsafeAnswer is the answer to an error that says git was not run because the folder's .git is not safe,
// nil for any other error.
func gitUnsafeAnswer(err error) error {
	reason, ok := gitrepo.UnsafeReason(err)
	if !ok {
		return nil
	}
	return control.NewError(control.CodeGitUnsafe, "git is not run in this folder: "+printable(reason))
}

// stackStateAnswer maps the errors of a stack's hold states to a fixed answer, nil for any other error.
func stackStateAnswer(err error) error {
	switch {
	case errors.Is(err, engine.ErrPaused):
		return control.NewError(control.CodePaused, "the stack is paused after a failed rollback; resume it first")
	case errors.Is(err, engine.ErrNotPaused):
		return control.NewError(control.CodeNotPaused, "the stack is not paused")
	case errors.Is(err, engine.ErrPinned):
		return control.NewError(control.CodePinned, "the stack is pinned; unpin it first")
	case errors.Is(err, engine.ErrStackStopped):
		return control.NewError(control.CodeStackStopped, "the stack was stopped through qhost; start it with start first")
	case errors.Is(err, engine.ErrNoSuchRun):
		return control.NewError(control.CodeNoSuchRun, "no such run")
	case errors.Is(err, engine.ErrNameCollision):
		return control.NewError(control.CodeNameCollision, "another stack folder already uses the name of this stack; it is not deployed")
	case errors.Is(err, engine.ErrBusy):
		return control.NewError(control.CodeBusy, "the working copy is busy with an update or a rollback; try again")
	}
	return nil
}
