package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The constructors of the fields the forms are built from.

func choice(id string, label kittui.TextID) kittui.Choice {
	return kittui.Choice{ID: kittui.ChoiceID(id), Label: label}
}

func textField(id string, label, hint kittui.TextID, value string) kittui.FormField {
	return kittui.FormField{ID: kittui.FieldID(id), Kind: kittui.FieldText, Label: label, Hint: hint, Value: kittui.FormValue{Text: value}}
}

func maskedField(id string, label, hint kittui.TextID) kittui.FormField {
	return kittui.FormField{ID: kittui.FieldID(id), Kind: kittui.FieldMasked, Label: label, Hint: hint}
}

// pathField is a path field of a form; priority are the entries it offers first.
func pathField(id string, label, hint kittui.TextID, kind kittui.PathKind, priority []kittui.Suggestion) kittui.FormField {
	return kittui.FormField{ID: kittui.FieldID(id), Kind: kittui.FieldPath, Label: label, Hint: hint,
		Path: kittui.PathSpec{Kind: kind, Priority: priority}}
}
