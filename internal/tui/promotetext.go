package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the promotion of the versions of another stack. The command words stay English in every language.
const (
	textKeyPromote       kittui.TextID = "key.promote"
	textTitlePromotePick kittui.TextID = "title.promotepick"
	textTitlePromote     kittui.TextID = "title.promote"
	textAskPromote       kittui.TextID = "ask.promote"
	textWhatPromote      kittui.TextID = "what.promote"
	textPromoteNoSource  kittui.TextID = "promote.nosource"
	textPromoteNothing   kittui.TextID = "promote.nothing"
	textPromoteSource    kittui.TextID = "promote.source"
	textPromoteObserve   kittui.TextID = "promote.observe"
	textPromoteCompare   kittui.TextID = "promote.compare"
	textPromoteNoCompare kittui.TextID = "promote.nocompare"
	textPromoteFailed    kittui.TextID = "promote.failed"
	textDonePromote      kittui.TextID = "done.promote"
)

func promoteEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyPromote:       "take versions of other stack",
		textTitlePromotePick: "Take over the versions of",
		textTitlePromote:     "Take over versions",
		textAskPromote:       "Set the image versions of %s to those %s runs?",
		textWhatPromote:      "The stack is stopped, backed up if it has backup rules, its .env gets the new versions in one commit, which is pushed, and the stack starts and is checked. A failing run is rolled back. Only the images both stacks run are set.",
		textPromoteNoSource:  "There is no other stack to take versions from.",
		textPromoteNothing:   "Nothing to take over: %s already has the versions of %s for the images they share.",
		textPromoteSource:    "Source %s at commit %s: last run succeeded, healthy.",
		textPromoteObserve:   "The source is under observation until %s: its update is not settled yet.",
		textPromoteCompare:   "The image fingerprints are compared with the source's, before the commit and after the pull.",
		textPromoteNoCompare: "The image fingerprints are not compared (setting promote.compare_images is off).",
		textPromoteFailed:    "The promotion to %s did not succeed (%s). Run %s has the log.",
		textDonePromote:      "Stack %s took over the versions, run %s.",
	}
}

func promoteGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyPromote:       "Versionen von anderem Stack",
		textTitlePromotePick: "Versionen übernehmen von",
		textTitlePromote:     "Versionen übernehmen",
		textAskPromote:       "Die Image-Versionen von %s auf die setzen, die %s betreibt?",
		textWhatPromote:      "Der Stack wird gestoppt, gesichert, wenn er Backup-Regeln hat, seine .env bekommt die neuen Versionen in einem Commit, der gepusht wird, und der Stack startet und wird geprüft. Ein fehlschlagender Lauf wird zurückgenommen. Gesetzt werden nur die Images, die beide Stacks betreiben.",
		textPromoteNoSource:  "Es gibt keinen anderen Stack, dessen Versionen sich übernehmen lassen.",
		textPromoteNothing:   "Nichts zu übernehmen: %s hat für die gemeinsamen Images schon die Versionen von %s.",
		textPromoteSource:    "Quelle %s bei Commit %s: letzter Lauf erfolgreich, gesund.",
		textPromoteObserve:   "Die Quelle steht unter Beobachtung bis %s: ihr Update ist noch nicht bestätigt.",
		textPromoteCompare:   "Die Fingerabdrücke der Images werden mit denen der Quelle verglichen, vor dem Commit und nach dem Pull.",
		textPromoteNoCompare: "Die Fingerabdrücke der Images werden nicht verglichen (Einstellung promote.compare_images ist aus).",
		textPromoteFailed:    "Die Übernahme für %s ist nicht gelungen (%s). Der Lauf %s hat das Log.",
		textDonePromote:      "Stack %s hat die Versionen übernommen, Lauf %s.",
	}
}
