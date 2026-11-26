package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the file browser, the preview and the edit of System > Folders. Paths, link targets, editors and reasons
// come in as arguments; no text quotes the content of a file.
const (
	textKeyRights           kittui.TextID = "key.rights"
	textTitleReadRoot       kittui.TextID = "title.readroot"
	textAskReadRoot         kittui.TextID = "ask.readroot"
	textWhatReadRoot        kittui.TextID = "what.readroot"
	textFileNoFolder        kittui.TextID = "files.nofolder"
	textFileIsFolder        kittui.TextID = "files.isfolder"
	textFileBinary          kittui.TextID = "files.binary"
	textFileTooLarge        kittui.TextID = "files.toolarge"
	textFileLink            kittui.TextID = "files.link"
	textFileOther           kittui.TextID = "files.other"
	textFileGitReadOnly     kittui.TextID = "files.gitreadonly"
	textKeyEditFile         kittui.TextID = "key.editfile"
	textEditFolder          kittui.TextID = "edit.folder"
	textEditLink            kittui.TextID = "edit.link"
	textEditOther           kittui.TextID = "edit.other"
	textEditBinary          kittui.TextID = "edit.binary"
	textEditTooLarge        kittui.TextID = "edit.toolarge"
	textEditAsRoot          kittui.TextID = "edit.asroot"
	textEditNoUser          kittui.TextID = "edit.nouser"
	textEditNoEditor        kittui.TextID = "edit.noeditor"
	textEditNoDir           kittui.TextID = "edit.nodir"
	textEditNoRAM           kittui.TextID = "edit.noram"
	textEditFailed          kittui.TextID = "edit.failed"
	textTitleEditTraces     kittui.TextID = "title.edittraces"
	textAskEditTraces       kittui.TextID = "ask.edittraces"
	textWhatEditTraces      kittui.TextID = "what.edittraces"
	textAnswerOpenEditor    kittui.TextID = "answer.openeditor"
	textTitleEditInvalid    kittui.TextID = "title.editinvalid"
	textAskEditInvalid      kittui.TextID = "ask.editinvalid"
	textAskEditInvalidLine  kittui.TextID = "ask.editinvalidline"
	textWhatEditInvalid     kittui.TextID = "what.editinvalid"
	textAnswerEditAgain     kittui.TextID = "answer.editagain"
	textAnswerDiscardEdit   kittui.TextID = "answer.discardedit"
	textAskFileWrite        kittui.TextID = "ask.filewrite"
	textRiskFileWriteSystem kittui.TextID = "host.riskfilewritesystem"
)

func fileEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyRights:       "permissions",
		textTitleReadRoot:   "Read as root",
		textAskReadRoot:     "Only root may read %s. Read it with sudo?",
		textWhatReadRoot:    "sudo asks for your password. Nothing is changed.",
		textFileNoFolder:    "this is no longer a folder",
		textFileIsFolder:    "%s is a folder; open it in the list.",
		textFileBinary:      "%s is a binary file. It is not shown.",
		textFileTooLarge:    "%s is larger than %d MiB. It is not shown.",
		textFileLink:        "%s is a link to %s. Links are not followed; open the target itself.",
		textFileOther:       "%s is no regular file. It has no content to show.",
		textFileGitReadOnly: "%s lies in a .git folder, which is only read here.",
		textKeyEditFile:     "edit",
		textEditFolder:      "%s is a folder. Only files are edited.",
		textEditLink:        "%s is a link. Links are never followed; edit the target itself.",
		textEditOther:       "%s is no regular file. It is not edited.",
		textEditBinary:      "%s is a binary file. Only text files are edited.",
		textEditTooLarge:    "%s is larger than %d MiB. It is not edited here.",
		textEditAsRoot: "The editor never runs as root, and qhost runs as root without sudo here. Start it with sudo from " +
			"your own account to edit files.",
		textEditNoUser:   "The account with UID %d is not on this host, so the editor has no user to run as.",
		textEditNoEditor: "No editor found: %s. Choose one under Settings > General.",
		textEditNoDir:    "There is no private folder for the copy: %s",
		textEditNoRAM: "Only root may read %s, and no private folder lies in memory (tmpfs): its copy would reach the " +
			"disk, so it is not edited.",
		textEditFailed:         "The edit of %s failed and nothing was written: %s",
		textTitleEditTraces:    "Unknown editor",
		textAskEditTraces:      "%s is an editor qhost does not know. Open %s in it anyway?",
		textWhatEditTraces:     "It may write backup or swap files of the text, which qhost cannot remove.",
		textAnswerOpenEditor:   "open the editor",
		textTitleEditInvalid:   "Check failed",
		textAskEditInvalid:     "The check of %s as %s failed.",
		textAskEditInvalidLine: "The check of %s as %s failed at line %d.",
		textWhatEditInvalid:    "Nothing has been written. Edit it again to correct it, or discard the change.",
		textAnswerEditAgain:    "edit again",
		textAnswerDiscardEdit:  "discard",
		textAskFileWrite:       "Write the edited file %s back in place.",
		textRiskFileWriteSystem: "This is a system path: a wrong file here can stop services or keep the host from " +
			"starting.",
	}
}

func fileGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyRights:       "Rechte",
		textTitleReadRoot:   "Als root lesen",
		textAskReadRoot:     "Nur root darf %s lesen. Mit sudo lesen?",
		textWhatReadRoot:    "sudo fragt nach deinem Passwort. Es wird nichts geändert.",
		textFileNoFolder:    "das ist kein Ordner mehr",
		textFileIsFolder:    "%s ist ein Ordner. Öffne ihn in der Liste.",
		textFileBinary:      "%s ist eine Binärdatei. Sie wird nicht angezeigt.",
		textFileTooLarge:    "%s ist größer als %d MiB. Sie wird nicht angezeigt.",
		textFileLink:        "%s ist ein Link auf %s. Links werden nicht verfolgt. Öffne das Ziel selbst.",
		textFileOther:       "%s ist keine normale Datei. Es gibt keinen Inhalt zu zeigen.",
		textFileGitReadOnly: "%s liegt in einem .git-Ordner, der hier nur gelesen wird.",
		textKeyEditFile:     "bearbeiten",
		textEditFolder:      "%s ist ein Ordner. Bearbeitet werden nur Dateien.",
		textEditLink:        "%s ist ein Link. Links werden nie verfolgt. Bearbeite das Ziel selbst.",
		textEditOther:       "%s ist keine normale Datei. Sie wird nicht bearbeitet.",
		textEditBinary:      "%s ist eine Binärdatei. Bearbeitet werden nur Textdateien.",
		textEditTooLarge:    "%s ist größer als %d MiB. Sie wird hier nicht bearbeitet.",
		textEditAsRoot: "Der Editor läuft nie als root, und qhost läuft hier als root ohne sudo. Starte es mit sudo aus " +
			"deinem eigenen Konto, um Dateien zu bearbeiten.",
		textEditNoUser:   "Das Konto mit der UID %d gibt es auf diesem Host nicht. Der Editor hat keinen Benutzer, als der er läuft.",
		textEditNoEditor: "Kein Editor gefunden: %s. Wähle einen unter Einstellungen > Allgemein.",
		textEditNoDir:    "Es gibt keinen privaten Ordner für die Kopie: %s",
		textEditNoRAM: "Nur root darf %s lesen, und kein privater Ordner liegt im Arbeitsspeicher (tmpfs). Die Kopie " +
			"käme auf die Platte, deshalb wird die Datei nicht bearbeitet.",
		textEditFailed:         "Das Bearbeiten von %s ist fehlgeschlagen, es wurde nichts geschrieben: %s",
		textTitleEditTraces:    "Unbekannter Editor",
		textAskEditTraces:      "%s ist ein Editor, den qhost nicht kennt. %s trotzdem darin öffnen?",
		textWhatEditTraces:     "Er kann Sicherungs- oder Auslagerungsdateien des Textes schreiben, die qhost nicht entfernen kann.",
		textAnswerOpenEditor:   "Editor öffnen",
		textTitleEditInvalid:   "Prüfung fehlgeschlagen",
		textAskEditInvalid:     "Die Prüfung von %s als %s ist fehlgeschlagen.",
		textAskEditInvalidLine: "Die Prüfung von %s als %s scheitert in Zeile %d.",
		textWhatEditInvalid:    "Es wurde nichts geschrieben. Bearbeite die Datei erneut, um sie zu korrigieren, oder verwirf die Änderung.",
		textAnswerEditAgain:    "erneut bearbeiten",
		textAnswerDiscardEdit:  "verwerfen",
		textAskFileWrite:       "Die bearbeitete Datei %s an ihrem Ort zurückschreiben.",
		textRiskFileWriteSystem: "Das ist ein Systempfad: Eine falsche Datei hier kann Dienste anhalten oder den Start des " +
			"Hosts verhindern.",
	}
}
