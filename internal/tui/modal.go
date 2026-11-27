package tui

import (
	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// modalKind tells which question or result the modal shows.
type modalKind int

const (
	// modalAsk is a confirmation over the screen.
	modalAsk modalKind = iota
	// modalPreview is the update confirmation with what changes and the changelog; it fills the workspace.
	modalPreview
	// modalSummary is the result of an action; it fills the workspace.
	modalSummary
	// modalSheet is a question of the settings with more to read than a dialog holds; it fills the workspace.
	modalSheet
)

// changelogEntry is the changelog of one stack read for the update preview.
type changelogEntry struct {
	res  control.ChangelogResult
	err  error
	done bool
}

// modal is the one question or result that takes every key.
type modal struct {
	kind modalKind
	// screen is the help screen ID of the dialog, see titleScreen and actionKind.screen.
	screen string
	dialog *kittui.Dialog
	plan   plan
	// gen separates the answers of a preview that was left from the current one.
	gen int
	// yes, when set, replaces the plan of the stacks: it is what a confirmed change of the settings does.
	yes func() tea.Cmd
	// alt is what the answer actWithRemote does, for a question that offers it.
	alt func() tea.Cmd
	// choose is what the answer actChoose does, for a question that offers it.
	choose func() tea.Cmd
	// on is what an answer outside the fixed ones does, for a question that offers more of them.
	on map[kittui.ActionID]func() tea.Cmd
	// drop runs on every answer but yes, so a question that holds a secret overwrites it.
	drop func()
	// full draws the dialog over the whole workspace instead of a box, so a long line stays whole and can be copied.
	full bool
	// input is what the answer enter of a dialog with an input does with the text typed.
	input      func(typed string) tea.Cmd
	changelogs map[string]*changelogEntry
	// summary
	title    string
	outcomes []outcome
	// sheet: the question, what follows from it and the lines to read below.
	question, consequence string
	body                  func(width int) []string
	// top is the first shown line of a modal that fills the workspace; total and rows are what the last
	// drawing found.
	top, total, rows int
}

func (md *modal) sheet() bool { return md.kind != modalAsk }
