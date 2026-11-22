package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of choosing unclear files into the first commit of a folder of files.
const (
	textAnswerChoose  kittui.TextID = "answer.choose"
	textTitleInclude  kittui.TextID = "title.include"
	textFieldInclude  kittui.TextID = "field.include"
	textHintInclude   kittui.TextID = "hint.include"
	textHeadUnclear   kittui.TextID = "head.unclear"
	textUnclearHint   kittui.TextID = "unclear.hint"
	textUnclearChosen kittui.TextID = "unclear.chosen"
	textUnclearLeft   kittui.TextID = "unclear.left"
	textUnclearForm   kittui.TextID = "unclear.form"
)

func includeEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textAnswerChoose:  "choose files",
		textTitleInclude:  "Choose unclear files",
		textFieldInclude:  "Files to take",
		textHintInclude:   "space ticks a file; ticked files go into the first commit, the others stay out",
		textHeadUnclear:   "Unclear files (%d), data or a secret may be in them",
		textUnclearHint:   "They stay out unless you choose them; choosing replans the folder. A chosen file with secret content is still checked before anything is pushed.",
		textUnclearChosen: "[x] %s: %s",
		textUnclearLeft:   "[ ] %s: %s",
		textUnclearForm:   "%s (%s)",
	}
}

func includeGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textAnswerChoose:  "Dateien wählen",
		textTitleInclude:  "Unklare Dateien wählen",
		textFieldInclude:  "Dateien übernehmen",
		textHintInclude:   "Leertaste wählt eine Datei; gewählte Dateien kommen in den ersten Commit, die anderen bleiben draußen",
		textHeadUnclear:   "Unklare Dateien (%d), Daten oder ein Secret können darin liegen",
		textUnclearHint:   "Sie bleiben draußen, solange du sie nicht wählst; die Wahl plant den Ordner neu. Eine gewählte Datei mit Secret-Inhalt wird vor jedem Push weiter geprüft.",
		textUnclearChosen: "[x] %s: %s",
		textUnclearLeft:   "[ ] %s: %s",
		textUnclearForm:   "%s (%s)",
	}
}
