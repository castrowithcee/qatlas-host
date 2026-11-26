package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the file browser and the preview of System > Folders. Paths and link targets come in as arguments.
const (
	textKeyRights       kittui.TextID = "key.rights"
	textTitleReadRoot   kittui.TextID = "title.readroot"
	textAskReadRoot     kittui.TextID = "ask.readroot"
	textWhatReadRoot    kittui.TextID = "what.readroot"
	textFileNoFolder    kittui.TextID = "files.nofolder"
	textFileIsFolder    kittui.TextID = "files.isfolder"
	textFileBinary      kittui.TextID = "files.binary"
	textFileTooLarge    kittui.TextID = "files.toolarge"
	textFileLink        kittui.TextID = "files.link"
	textFileOther       kittui.TextID = "files.other"
	textFileGitReadOnly kittui.TextID = "files.gitreadonly"
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
	}
}
