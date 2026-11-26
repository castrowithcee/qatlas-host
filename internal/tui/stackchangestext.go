package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the view of the uncommitted changes of a stack folder. No text quotes a diff or the content of a file;
// a refusal adds only the message of the service, which names places, never a value.
const (
	textKeyChanges           kittui.TextID = "key.changes"
	textKeyCommit            kittui.TextID = "key.commit"
	textKeyDiff              kittui.TextID = "key.diff"
	textKeyDiscard           kittui.TextID = "key.discard"
	textAnswerRuns           kittui.TextID = "answer.runs"
	textChangesOf            kittui.TextID = "stackchanges.of"
	textChangesNone          kittui.TextID = "stackchanges.none"
	textChangesFolderClean   kittui.TextID = "stackchanges.clean"
	textChangesNote          kittui.TextID = "stackchanges.note"
	textChangesCut           kittui.TextID = "stackchanges.cut"
	textChangeModified       kittui.TextID = "stackchanges.modified"
	textChangeNew            kittui.TextID = "stackchanges.new"
	textChangeDeleted        kittui.TextID = "stackchanges.deleted"
	textDiffCut              kittui.TextID = "stackchanges.diffcut"
	textTitleDiff            kittui.TextID = "title.diff"
	textAskDiff              kittui.TextID = "ask.diff"
	textWhatDiff             kittui.TextID = "what.diff"
	textTitleCommit          kittui.TextID = "title.commit"
	textAskCommit            kittui.TextID = "ask.commit"
	textWhatCommitRemote     kittui.TextID = "what.commitremote"
	textWhatCommitLocal      kittui.TextID = "what.commitlocal"
	textDoneCommitted        kittui.TextID = "done.committed"
	textCommitRuns           kittui.TextID = "stackchanges.commitruns"
	textTitleCommitRefused   kittui.TextID = "title.commitrefused"
	textRefusedSecrets       kittui.TextID = "stackchanges.refusedsecrets"
	textRefusedPush          kittui.TextID = "stackchanges.refusedpush"
	textRefusedHead          kittui.TextID = "stackchanges.refusedhead"
	textRefusedList          kittui.TextID = "stackchanges.refusedlist"
	textRefusedUnreadable    kittui.TextID = "stackchanges.refusedunreadable"
	textRefusedRunActive     kittui.TextID = "stackchanges.refusedrun"
	textTitleDiscardChange   kittui.TextID = "stackchanges.titlediscard"
	textAskDiscardChange     kittui.TextID = "stackchanges.askdiscard"
	textWhatDiscardChange    kittui.TextID = "stackchanges.whatdiscard"
	textAskDiscardNew        kittui.TextID = "ask.discardnew"
	textWhatDiscardNew       kittui.TextID = "what.discardnew"
	textAskDiscardDeleted    kittui.TextID = "ask.discarddeleted"
	textWhatDiscardDeleted   kittui.TextID = "what.discarddeleted"
	textDoneDiscarded        kittui.TextID = "done.discarded"
	textDoneDiscardedNew     kittui.TextID = "done.discardednew"
	textDoneDiscardedDeleted kittui.TextID = "done.discardeddeleted"
)

func stackChangesEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyChanges:         "changes",
		textKeyCommit:          "commit",
		textKeyDiff:            "diff",
		textKeyDiscard:         "discard",
		textAnswerRuns:         "show runs",
		textChangesOf:          "Uncommitted changes of %s",
		textChangesNone:        "No uncommitted changes.",
		textChangesFolderClean: "%s has no uncommitted changes.",
		textChangesNote: "A commit takes all files of the list. secrets.env, data folders and new files below a bind mount " +
			"belong to the host and are never part of it.",
		textChangesCut:     "The list is cut. Commit with qhost source commit on the command line.",
		textChangeModified: "modified",
		textChangeNew:      "new",
		textChangeDeleted:  "deleted",
		textDiffCut:        "The diff is cut after 256 KiB.",
		textTitleDiff:      "Diff",
		textAskDiff:        "Show the diff of %s?",
		textWhatDiff:       "The diff can hold values only root may read, so it is read through sudo. Nothing changes.",
		textTitleCommit:    "Commit",
		textAskCommit:      "Commit %d files of %s?",
		textWhatCommitRemote: "qhost checks that every stack of the files still loads and that no secret goes along, commits " +
			"them as they are now and pushes the commit. The normal run then deploys their stacks.",
		textWhatCommitLocal: "qhost checks that every stack of the files still loads and commits them as they are now. The " +
			"normal run then deploys their stacks.",
		textDoneCommitted:      "Committed %d files as %s.",
		textCommitRuns:         "The run of their stacks starts now. Show runs opens the area Runs.",
		textTitleCommitRefused: "Commit refused",
		textRefusedSecrets: "Nothing was committed: the commit would push secret-like files to the remote. Keep secret " +
			"values in secrets.env, which never leaves the host.",
		textRefusedPush: "Nothing was committed: the remote refused the push, as a protected branch does. Change that at " +
			"the Git platform, then commit again.",
		textRefusedHead: "Nothing was committed: the folder moved on since the list was read. The list is read again: " +
			"check it and commit again.",
		textRefusedList: "Nothing was committed: the list no longer matches the changes of the folder. The list is read " +
			"again: check it and commit again.",
		textRefusedUnreadable:    "Nothing was committed: a stack of the files no longer loads. Fix the file, then commit again.",
		textRefusedRunActive:     "Nothing was committed: a run of a stack of the files is under way. Commit again when it has ended.",
		textTitleDiscardChange:   "Discard",
		textAskDiscardChange:     "Discard the changes of %s?",
		textWhatDiscardChange:    "The file goes back to the last commit. The edits are lost and cannot be brought back.",
		textAskDiscardNew:        "Delete the new file %s?",
		textWhatDiscardNew:       "The file is in no commit, so deleting it cannot be undone.",
		textAskDiscardDeleted:    "Bring back the deleted file %s?",
		textWhatDiscardDeleted:   "The file comes back as the last commit has it.",
		textDoneDiscarded:        "Discarded the changes of %s.",
		textDoneDiscardedNew:     "Deleted the new file %s.",
		textDoneDiscardedDeleted: "Brought back %s.",
	}
}

func stackChangesGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyChanges:         "Änderungen",
		textKeyCommit:          "committen",
		textKeyDiff:            "Unterschied",
		textKeyDiscard:         "verwerfen",
		textAnswerRuns:         "Läufe zeigen",
		textChangesOf:          "Nicht committete Änderungen in %s",
		textChangesNone:        "Keine nicht committeten Änderungen.",
		textChangesFolderClean: "%s hat keine nicht committeten Änderungen.",
		textChangesNote: "Ein Commit nimmt alle Dateien der Liste. secrets.env, Datenordner und neue Dateien unter einem " +
			"Bind-Mount gehören dem Host und sind nie Teil davon.",
		textChangesCut:     "Die Liste ist gekürzt. Committe mit qhost source commit auf der Kommandozeile.",
		textChangeModified: "geändert",
		textChangeNew:      "neu",
		textChangeDeleted:  "gelöscht",
		textDiffCut:        "Der Unterschied ist nach 256 KiB gekürzt.",
		textTitleDiff:      "Unterschied",
		textAskDiff:        "Den Unterschied von %s zeigen?",
		textWhatDiff:       "Der Unterschied kann Werte enthalten, die nur root lesen darf, daher wird er über sudo gelesen. Nichts ändert sich.",
		textTitleCommit:    "Committen",
		textAskCommit:      "%d Dateien von %s committen?",
		textWhatCommitRemote: "qhost prüft, dass jeder Stack der Dateien noch lädt und kein Secret mitgeht, committet sie, " +
			"wie sie jetzt sind, und pusht den Commit. Danach rollt der normale Lauf ihre Stacks aus.",
		textWhatCommitLocal: "qhost prüft, dass jeder Stack der Dateien noch lädt, und committet sie, wie sie jetzt sind. " +
			"Danach rollt der normale Lauf ihre Stacks aus.",
		textDoneCommitted:      "%d Dateien als %s committet.",
		textCommitRuns:         "Der Lauf ihrer Stacks beginnt jetzt. Läufe zeigen öffnet den Bereich Läufe.",
		textTitleCommitRefused: "Commit abgelehnt",
		textRefusedSecrets: "Nichts wurde committet: Der Commit würde Secret-artige Dateien ins Remote pushen. Halte " +
			"geheime Werte in secrets.env, die den Host nie verlässt.",
		textRefusedPush: "Nichts wurde committet: Das Remote hat den Push abgelehnt, wie es ein geschützter Branch tut. " +
			"Ändere das auf der Git-Plattform und committe dann erneut.",
		textRefusedHead: "Nichts wurde committet: Der Ordner ist weitergegangen, seit die Liste gelesen wurde. Die Liste " +
			"wird neu gelesen: Prüfe sie und committe erneut.",
		textRefusedList: "Nichts wurde committet: Die Liste passt nicht mehr zu den Änderungen des Ordners. Die Liste " +
			"wird neu gelesen: Prüfe sie und committe erneut.",
		textRefusedUnreadable: "Nichts wurde committet: Ein Stack der Dateien lädt nicht mehr. Korrigiere die Datei und " +
			"committe dann erneut.",
		textRefusedRunActive: "Nichts wurde committet: Ein Lauf eines Stacks der Dateien ist im Gang. Committe erneut, " +
			"wenn er beendet ist.",
		textTitleDiscardChange:   "Verwerfen",
		textAskDiscardChange:     "Die Änderungen an %s verwerfen?",
		textWhatDiscardChange:    "Die Datei geht auf den letzten Commit zurück. Die Änderungen sind verloren und lassen sich nicht zurückholen.",
		textAskDiscardNew:        "Die neue Datei %s löschen?",
		textWhatDiscardNew:       "Die Datei ist in keinem Commit, das Löschen lässt sich daher nicht rückgängig machen.",
		textAskDiscardDeleted:    "Die gelöschte Datei %s zurückholen?",
		textWhatDiscardDeleted:   "Die Datei kommt so zurück, wie der letzte Commit sie hat.",
		textDoneDiscarded:        "Änderungen an %s verworfen.",
		textDoneDiscardedNew:     "Neue Datei %s gelöscht.",
		textDoneDiscardedDeleted: "%s zurückgeholt.",
	}
}
