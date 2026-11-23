package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the return to before an update run. The command words stay English in every language.
const (
	textKeyRollback         kittui.TextID = "key.rollback"
	textTitleRollback       kittui.TextID = "title.rollback"
	textAskRollback         kittui.TextID = "ask.rollback"
	textAskRollbackRun      kittui.TextID = "ask.rollbackrun"
	textWhatRollback        kittui.TextID = "what.rollback"
	textWhatRollbackRestore kittui.TextID = "what.rollbackrestore"
	textWhatRollbackKeep    kittui.TextID = "what.rollbackkeep"
	textAnswerRollbackData  kittui.TextID = "answer.rollbackdata"
	textAnswerRollbackKeep  kittui.TextID = "answer.rollbackkeep"
	textRollbackData        kittui.TextID = "rollback.data"
	textRollbackKeep        kittui.TextID = "rollback.keep"
	textRollbackNoBackup    kittui.TextID = "rollback.nobackup"
	textRollbackNotRun      kittui.TextID = "rollback.notrun"
	textRollbackNoRun       kittui.TextID = "rollback.norun"
	textRollbackVersions    kittui.TextID = "rollback.versions"
	textRollbackStep        kittui.TextID = "rollback.step"
	textRollbackPinned      kittui.TextID = "rollback.pinned"
	textRollbackFailed      kittui.TextID = "rollback.failed"
	textDoneRollback        kittui.TextID = "done.rollback"
)

func rollbackEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyRollback:         "back to before update",
		textTitleRollback:       "Back to the state before",
		textAskRollback:         "Set %s back to the state before the update run %s?",
		textAskRollbackRun:      "Return %s to the versions before the update run %s?",
		textWhatRollback:        "The stack is pinned afterwards, so no update or window moves it on. Choose what happens to its data.",
		textWhatRollbackRestore: "The stack is stopped, its current data is backed up if it has backup rules, the backup of that update is restored and the stack starts on the old versions. The stack is pinned afterwards.",
		textWhatRollbackKeep:    "The stack starts on the old versions and keeps its data as it is. The stack is pinned afterwards.",
		textAnswerRollbackData:  "with data",
		textAnswerRollbackKeep:  "version only",
		textRollbackData:        "With data: Everything written since this update is lost.",
		textRollbackKeep:        "Version only: The data stays, the old version may not understand migrated data.",
		textRollbackNoBackup:    "This run made no backup, so only the version can go back.",
		textRollbackNotRun:      "Only a successful update run can be returned to the state before it.",
		textRollbackNoRun:       "This stack has no successful update run to go back from.",
		textRollbackVersions:    "Versions: %s",
		textRollbackStep:        "%s: %s -> %s",
		textRollbackPinned:      "The stack is pinned on these versions: the maintenance window, the update check and webhooks leave it alone. Release it with unpin in Stacks.",
		textRollbackFailed:      "The return of %s did not succeed (%s). Run %s has the log.",
		textDoneRollback:        "Stack %s is back on the versions before the update run %s.",
	}
}

func rollbackGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyRollback:         "zurück auf Stand davor",
		textTitleRollback:       "Zurück auf den Stand davor",
		textAskRollback:         "%s auf den Stand vor dem Update-Lauf %s zurücksetzen?",
		textAskRollbackRun:      "%s auf die Versionen vor dem Update-Lauf %s zurücksetzen?",
		textWhatRollback:        "Danach ist der Stack festgehalten, kein Update und kein Fenster bewegt ihn weiter. Wähle, was mit seinen Daten geschieht.",
		textWhatRollbackRestore: "Der Stack wird gestoppt, seine aktuellen Daten werden gesichert, wenn er Backup-Regeln hat, das Backup dieses Updates wird eingespielt und der Stack startet auf den alten Versionen. Danach ist der Stack festgehalten.",
		textWhatRollbackKeep:    "Der Stack startet auf den alten Versionen und behält seine Daten, wie sie sind. Danach ist der Stack festgehalten.",
		textAnswerRollbackData:  "mit Daten",
		textAnswerRollbackKeep:  "nur Version",
		textRollbackData:        "Mit Daten: Alles seit diesem Update Geschriebene geht verloren.",
		textRollbackKeep:        "Nur Version: Die Daten bleiben, die alte Version versteht migrierte Daten womöglich nicht.",
		textRollbackNoBackup:    "Dieser Lauf hat kein Backup gemacht, daher kann nur die Version zurück.",
		textRollbackNotRun:      "Nur ein erfolgreicher Update-Lauf lässt sich auf den Stand davor zurücksetzen.",
		textRollbackNoRun:       "Dieser Stack hat keinen erfolgreichen Update-Lauf, von dem es zurückgeht.",
		textRollbackVersions:    "Versionen: %s",
		textRollbackStep:        "%s: %s -> %s",
		textRollbackPinned:      "Der Stack ist auf diesen Versionen festgehalten: Wartungsfenster, Update-Prüfung und Webhooks lassen ihn in Ruhe. Gib ihn mit unpin in Stacks wieder frei.",
		textRollbackFailed:      "Das Zurücksetzen von %s ist nicht gelungen (%s). Der Lauf %s hat das Log.",
		textDoneRollback:        "Stack %s läuft wieder auf den Versionen vor dem Update-Lauf %s.",
	}
}
