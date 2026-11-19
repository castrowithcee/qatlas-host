package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of System > Changes and System > Log. The sentences and commands of the journal and the action words of
// the action log are what the root command and the host service report; they come in as arguments in English.
const (
	textPageChanges kittui.TextID = "system.page.changes"
	textPageLog     kittui.TextID = "system.page.log"

	textColTime   kittui.TextID = "col.time"
	textColBy     kittui.TextID = "col.by"
	textColUndo   kittui.TextID = "col.undo"
	textColUser   kittui.TextID = "col.user"
	textColAction kittui.TextID = "col.action"

	textJournalHint    kittui.TextID = "journal.hint"
	textJournalReading kittui.TextID = "journal.reading"
	textJournalEmpty   kittui.TextID = "journal.empty"
	textJournalStale   kittui.TextID = "journal.stale"
	textJrnApplied     kittui.TextID = "journal.applied"
	textJrnFailed      kittui.TextID = "journal.failed"
	textJrnRestored    kittui.TextID = "journal.restored"
	textJrnUndone      kittui.TextID = "journal.undone"
	textUndoPossible   kittui.TextID = "journal.undopossible"
	textJournalUndo    kittui.TextID = "journal.undo"
	textNoUndoUndone   kittui.TextID = "journal.noundo.undone"
	textNoUndoFailed   kittui.TextID = "journal.noundo.failed"
	textNoUndoFinal    kittui.TextID = "journal.noundo.final"
	textNoUndoTitle    kittui.TextID = "journal.noundo.title"
	textKeyUndo        kittui.TextID = "key.undo"
	textAskUndo        kittui.TextID = "ask.undo"
	textUndoNote       kittui.TextID = "host.undonote"
	textUndoDone       kittui.TextID = "host.undodone"

	textLogReading      kittui.TextID = "hostlog.reading"
	textLogEmpty        kittui.TextID = "hostlog.empty"
	textLogNoChannel    kittui.TextID = "hostlog.nochannel"
	textLogNotRunning   kittui.TextID = "hostlog.notrunning"
	textLogNoAccess     kittui.TextID = "hostlog.noaccess"
	textLogFailed       kittui.TextID = "hostlog.failed"
	textLogStale        kittui.TextID = "hostlog.stale"
	textLogSuccess      kittui.TextID = "hostlog.success"
	textLogFailure      kittui.TextID = "hostlog.failure"
	textLogFailureCode  kittui.TextID = "hostlog.failurecode"
	textLogUndoOf       kittui.TextID = "hostlog.undoof"
	textChainChecking   kittui.TextID = "chain.checking"
	textChainNone       kittui.TextID = "chain.none"
	textChainIntact     kittui.TextID = "chain.intact"
	textChainDamaged    kittui.TextID = "chain.damaged"
	textChainBroken     kittui.TextID = "chain.broken"
	textChainMore       kittui.TextID = "chain.more"
	textChainFailed     kittui.TextID = "chain.failed"
	textChainUnparsable kittui.TextID = "chain.unparsable"
	textChainGenesis    kittui.TextID = "chain.genesis"
	textChainMissing    kittui.TextID = "chain.missing"
	textChainLink       kittui.TextID = "chain.link"
)

func changesEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPageChanges: "Changes",
		textPageLog:     "Log",

		textColTime:   "time",
		textColBy:     "by",
		textColUndo:   "undo",
		textColUser:   "user",
		textColAction: "action",

		textJournalHint:    "The latest changes of the host are in a journal that only root can read. Press {reload} to read it through sudo.",
		textJournalReading: "Reading the journal...",
		textJournalEmpty:   "The journal holds no change yet.",
		textJournalStale:   "The host was changed since the journal was read. Press {reload} to read it again.",
		textJrnApplied:     "done",
		textJrnFailed:      "failed",
		textJrnRestored:    "restored",
		textJrnUndone:      "undone",
		textUndoPossible:   "possible",
		textJournalUndo:    "Undo: %s",
		textNoUndoUndone:   "This change was undone already.",
		textNoUndoFailed:   "This change did not stand, so there is nothing to undo.",
		textNoUndoFinal:    "This change cannot be undone, as a deleted home or an undo itself.",
		textNoUndoTitle:    "Not undone: %s",
		textKeyUndo:        "undo",
		textAskUndo:        "Undo the change: %s",
		textUndoNote:       "Runs with sudo and goes into the journal. The undo itself cannot be undone.",
		textUndoDone:       "Journal entry %s. An undo is not undone.",

		textLogReading:      "Reading the action log...",
		textLogEmpty:        "The action log holds no entry yet.",
		textLogNoChannel:    "The action log cannot be read here: the interface has no way to reach the host service.",
		textLogNotRunning:   "The host service is not running. Start it with sudo systemctl start qhost.",
		textLogNoAccess:     "qhost is reserved for administrators (sudo).",
		textLogFailed:       "The action log cannot be read: %s",
		textLogStale:        "The host was changed since the action log was read. Press {reload} to read it again.",
		textLogSuccess:      "success",
		textLogFailure:      "failure",
		textLogFailureCode:  "failure: %s",
		textLogUndoOf:       "%s, undo of %s",
		textChainChecking:   "Checking the chain of hashes...",
		textChainNone:       "Chain of hashes: no day to check yet.",
		textChainIntact:     "Chain of hashes intact: %d days with %d entries checked.",
		textChainDamaged:    "%d unreadable lines are documented and leave it intact.",
		textChainBroken:     "Chain of hashes broken on %s, line %d: %s.",
		textChainMore:       "Further days are broken too.",
		textChainFailed:     "The chain of hashes cannot be checked: %s",
		textChainUnparsable: "the line is no log entry",
		textChainGenesis:    "the first entry does not start the chain",
		textChainMissing:    "entries are missing without a retention cut",
		textChainLink:       "the entry does not follow from the one before",
	}
}

func changesGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPageChanges: "Änderungen",
		textPageLog:     "Protokoll",

		textColTime:   "Zeit",
		textColBy:     "von",
		textColUndo:   "Rückgängig",
		textColUser:   "Benutzer",
		textColAction: "Aktion",

		textJournalHint:    "Die letzten Änderungen am Host stehen in einem Journal, das nur root lesen kann. Drücke {reload}, um es über sudo zu lesen.",
		textJournalReading: "Das Journal wird gelesen...",
		textJournalEmpty:   "Das Journal enthält noch keine Änderung.",
		textJournalStale:   "Der Host wurde geändert, seit das Journal gelesen wurde. Drücke {reload}, um es neu zu lesen.",
		textJrnApplied:     "erledigt",
		textJrnFailed:      "fehlgeschlagen",
		textJrnRestored:    "zurückgesetzt",
		textJrnUndone:      "rückgängig gemacht",
		textUndoPossible:   "möglich",
		textJournalUndo:    "Rückgängig: %s",
		textNoUndoUndone:   "Diese Änderung wurde schon rückgängig gemacht.",
		textNoUndoFailed:   "Diese Änderung ist nicht wirksam geworden, es gibt nichts rückgängig zu machen.",
		textNoUndoFinal:    "Diese Änderung lässt sich nicht rückgängig machen, etwa ein gelöschtes Home oder ein Rückgängigmachen selbst.",
		textNoUndoTitle:    "Nicht rückgängig gemacht: %s",
		textKeyUndo:        "rückgängig",
		textAskUndo:        "Änderung rückgängig machen: %s",
		textUndoNote:       "Läuft mit sudo und steht im Journal. Das Rückgängigmachen selbst lässt sich nicht rückgängig machen.",
		textUndoDone:       "Journaleintrag %s. Ein Rückgängigmachen wird nicht rückgängig gemacht.",

		textLogReading:      "Das Aktionsprotokoll wird gelesen...",
		textLogEmpty:        "Das Aktionsprotokoll enthält noch keinen Eintrag.",
		textLogNoChannel:    "Das Aktionsprotokoll lässt sich hier nicht lesen: Die Oberfläche erreicht den Host-Dienst nicht.",
		textLogNotRunning:   "Der Host-Dienst läuft nicht. Starte ihn mit sudo systemctl start qhost.",
		textLogNoAccess:     "qhost ist Administratoren vorbehalten (sudo).",
		textLogFailed:       "Das Aktionsprotokoll lässt sich nicht lesen: %s",
		textLogStale:        "Der Host wurde geändert, seit das Aktionsprotokoll gelesen wurde. Drücke {reload}, um es neu zu lesen.",
		textLogSuccess:      "Erfolg",
		textLogFailure:      "Fehler",
		textLogFailureCode:  "Fehler: %s",
		textLogUndoOf:       "%s, Rückgängig von %s",
		textChainChecking:   "Die Kette der Hashes wird geprüft...",
		textChainNone:       "Kette der Hashes: noch kein Tag zu prüfen.",
		textChainIntact:     "Kette der Hashes intakt: %d Tage mit %d Einträgen geprüft.",
		textChainDamaged:    "%d unlesbare Zeilen sind dokumentiert und lassen sie intakt.",
		textChainBroken:     "Kette der Hashes gebrochen am %s, Zeile %d: %s.",
		textChainMore:       "Weitere Tage sind ebenfalls gebrochen.",
		textChainFailed:     "Die Kette der Hashes lässt sich nicht prüfen: %s",
		textChainUnparsable: "Die Zeile ist kein Protokolleintrag",
		textChainGenesis:    "Der erste Eintrag beginnt die Kette nicht",
		textChainMissing:    "Einträge fehlen ohne dokumentierten Schnitt der Aufbewahrung",
		textChainLink:       "Der Eintrag folgt nicht aus dem davor",
	}
}
