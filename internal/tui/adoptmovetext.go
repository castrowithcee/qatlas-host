package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of moving a foreign project into a source that exists. Paths, names and the reasons the command gives
// come in as arguments and are escaped where they are drawn.
const (
	textTitleMove       kittui.TextID = "move.title"
	textFieldMoveTarget kittui.TextID = "move.field"
	textHintMoveTarget  kittui.TextID = "move.hint"
	textMoveOwn         kittui.TextID = "move.own"
	textMoveInto        kittui.TextID = "move.into"
	textDetailMoveInto  kittui.TextID = "move.detail.into"
	textDetailMoveTo    kittui.TextID = "move.detail.target"
	textDetailMoveName  kittui.TextID = "move.detail.name"
	textHeadBlocks      kittui.TextID = "move.head.blocks"
	textMoveIssue       kittui.TextID = "move.issue"
	textMoveItems       kittui.TextID = "move.items"
	textAskMoveBlocked  kittui.TextID = "move.ask.blocked"
	textAskMove         kittui.TextID = "move.ask"
	textWhatMove        kittui.TextID = "move.what"
	textDoneMoved       kittui.TextID = "move.done"
	textMoveRolledBack  kittui.TextID = "move.rolledback"
	textMoveIncomplete  kittui.TextID = "move.incomplete"
	textMoveLocation    kittui.TextID = "move.location"
	textMoveBackStep    kittui.TextID = "move.backstep"
	textMoveRemaining   kittui.TextID = "move.remaining"
)

func moveEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textTitleMove:       "Move a project into a source",
		textFieldMoveTarget: "Take over as",
		textHintMoveTarget:  "a source of its own, or moved into a source that exists, where the preview shows what the move would do",
		textMoveOwn:         "a source of its own",
		textMoveInto:        "moved into the source %s",
		textDetailMoveInto:  "source: %s",
		textDetailMoveTo:    "target folder: %s",
		textDetailMoveName:  "project name after the move: %s",
		textHeadBlocks:      "Blocks (%d), the move cannot be done as it is",
		textMoveIssue:       "%s: %s",
		textMoveItems:       "concerns: %s",
		textAskMoveBlocked:  "The project %s cannot be moved into %s.",
		textAskMove:         "Move the project %s into the source %s as the folder %s?",
		textWhatMove: "The project goes down, its folder is renamed into the source on the same file system, committed " +
			"(and pushed when the source has a remote) and deployed there. Data is never copied or deleted. " +
			"A failed step takes everything back.",
		textDoneMoved:      "The project %s is moved to %s and deployed there.",
		textMoveRolledBack: "The move failed at the step %s and was taken back: %s",
		textMoveIncomplete: "The move failed at the step %s, and the way back did not finish: %s",
		textMoveLocation:   "the folder is now in: %s",
		textMoveBackStep:   "way back, %s: %s",
		textMoveRemaining:  "left to do by hand: %s",
	}
}

func moveGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textTitleMove:       "Projekt in eine Quelle verschieben",
		textFieldMoveTarget: "Übernehmen als",
		textHintMoveTarget:  "eigene Quelle oder in eine bestehende Quelle verschoben, wobei die Vorschau zeigt, was der Umzug täte",
		textMoveOwn:         "eigene Quelle",
		textMoveInto:        "in die Quelle %s verschoben",
		textDetailMoveInto:  "Quelle: %s",
		textDetailMoveTo:    "Zielordner: %s",
		textDetailMoveName:  "Projektname nach dem Umzug: %s",
		textHeadBlocks:      "Sperren (%d), der Umzug geht so nicht",
		textMoveIssue:       "%s: %s",
		textMoveItems:       "betrifft: %s",
		textAskMoveBlocked:  "Das Projekt %s kann nicht in %s verschoben werden.",
		textAskMove:         "Das Projekt %s in die Quelle %s verschieben, als Ordner %s?",
		textWhatMove: "Das Projekt wird heruntergefahren, sein Ordner im selben Dateisystem in die Quelle umbenannt, " +
			"committet (und gepusht, wenn die Quelle ein Remote hat) und dort deployt. Daten werden nie kopiert oder gelöscht. " +
			"Scheitert ein Schritt, wird alles zurückgenommen.",
		textDoneMoved:      "Das Projekt %s ist nach %s verschoben und dort deployt.",
		textMoveRolledBack: "Der Umzug scheiterte im Schritt %s und wurde zurückgenommen: %s",
		textMoveIncomplete: "Der Umzug scheiterte im Schritt %s, und der Rückweg wurde nicht fertig: %s",
		textMoveLocation:   "der Ordner liegt jetzt in: %s",
		textMoveBackStep:   "Rückweg, %s: %s",
		textMoveRemaining:  "noch von Hand zu tun: %s",
	}
}
