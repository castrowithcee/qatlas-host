package tui

import (
	"slices"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// newDialog is kittui.NewDialog with the answer of a click outside the frame set to the one Esc gives: no, or
// back where the dialog has no no. The kit ignores it for a dialog with a danger answer.
func newDialog(spec kittui.DialogSpec, keys *kittui.Keymap, text kittui.Translator) (*kittui.Dialog, error) {
	has := func(id kittui.ActionID) bool {
		return slices.ContainsFunc(spec.Answers, func(a kittui.Answer) bool { return a.Action == id })
	}
	switch {
	case has(actNo):
		spec.CloseOutside = actNo
	case has(actBack):
		spec.CloseOutside = actBack
	}
	return kittui.NewDialog(spec, keys, text)
}
