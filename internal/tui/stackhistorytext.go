package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the view of the history of a stack folder. Author, subject and paths come masked from the service;
// no text quotes a diff.
const (
	textKeyHistory       kittui.TextID = "key.history"
	textColDate          kittui.TextID = "col.date"
	textColAuthor        kittui.TextID = "col.author"
	textColSubject       kittui.TextID = "col.subject"
	textHistoryOf        kittui.TextID = "history.of"
	textHistoryNone      kittui.TextID = "history.none"
	textHistoryCut       kittui.TextID = "history.cut"
	textHistoryFilesOf   kittui.TextID = "history.filesof"
	textHistoryFilesNone kittui.TextID = "history.filesnone"
	textHistoryFilesCut  kittui.TextID = "history.filescut"
	textTitleShow        kittui.TextID = "title.show"
	textAskShow          kittui.TextID = "ask.show"
	textWhatShow         kittui.TextID = "what.show"
)

func stackHistoryEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyHistory:       "history",
		textColDate:          "Date",
		textColAuthor:        "Author",
		textColSubject:       "Subject",
		textHistoryOf:        "History of %s",
		textHistoryNone:      "No commits.",
		textHistoryCut:       "The history is cut: older commits are not listed.",
		textHistoryFilesOf:   "Files of commit %s: %s",
		textHistoryFilesNone: "The commit changed no files.",
		textHistoryFilesCut:  "More files changed than are listed.",
		textTitleShow:        "Diff of commit",
		textAskShow:          "Show the diff of commit %s?",
		textWhatShow:         "The diff can hold values only root may read, so it is read through sudo. Nothing changes.",
	}
}

func stackHistoryGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyHistory:       "Verlauf",
		textColDate:          "Datum",
		textColAuthor:        "Autor",
		textColSubject:       "Betreff",
		textHistoryOf:        "Verlauf von %s",
		textHistoryNone:      "Keine Commits.",
		textHistoryCut:       "Der Verlauf ist gekürzt: Ältere Commits stehen nicht in der Liste.",
		textHistoryFilesOf:   "Dateien des Commits %s: %s",
		textHistoryFilesNone: "Der Commit hat keine Dateien geändert.",
		textHistoryFilesCut:  "Es wurden mehr Dateien geändert, als aufgelistet sind.",
		textTitleShow:        "Diff des Commits",
		textAskShow:          "Den Diff des Commits %s zeigen?",
		textWhatShow:         "Der Diff kann Werte enthalten, die nur root lesen darf, daher wird er über sudo gelesen. Nichts ändert sich.",
	}
}
