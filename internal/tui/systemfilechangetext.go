package tui

import (
	"maps"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// The texts of creating, renaming, copying and removing in the browser of System > Folders. Paths and sizes come in
// as arguments.
const (
	textTitleFileNew      kittui.TextID = "title.filenew"
	textTitleFileRename   kittui.TextID = "title.filerename"
	textTitleFileCopy     kittui.TextID = "title.filecopy"
	textLeadFileNew       kittui.TextID = "lead.filenew"
	textLeadFileRename    kittui.TextID = "lead.filerename"
	textLeadFileCopy      kittui.TextID = "lead.filecopy"
	textFieldFileName     kittui.TextID = "field.filename"
	textFieldFileKind     kittui.TextID = "field.filekind"
	textFieldCopyName     kittui.TextID = "field.copyname"
	textHintFileName      kittui.TextID = "hint.filename"
	textHintFileKind      kittui.TextID = "hint.filekind"
	textProblemFileEmpty  kittui.TextID = "problem.fileempty"
	textProblemFileDots   kittui.TextID = "problem.filedots"
	textProblemFileChars  kittui.TextID = "problem.filechars"
	textProblemFileTaken  kittui.TextID = "problem.filetaken"
	textFileReadFirst     kittui.TextID = "files.readfirst"
	textFileReadAgain     kittui.TextID = "files.readagain"
	textFileTooMany       kittui.TextID = "files.toomany"
	textFileNothingChange kittui.TextID = "files.nothingchanged"
	textFileCount         kittui.TextID = "files.count"
	textFileSizeUnknown   kittui.TextID = "files.sizeunknown"
	textFileFinal         kittui.TextID = "files.final"
	textAskFileNew        kittui.TextID = "ask.filenew"
	textAskFolderNew      kittui.TextID = "ask.foldernew"
	textAskFileRename     kittui.TextID = "ask.filerename"
	textAskFileCopy       kittui.TextID = "ask.filecopy"
	textAskFolderCopy     kittui.TextID = "ask.foldercopy"
	textAskFileRemove     kittui.TextID = "ask.fileremove"
	textAskFolderRemove   kittui.TextID = "ask.folderremove"
)

// joinTexts is the texts of several catalogs as one.
func joinTexts(parts ...map[kittui.TextID]string) map[kittui.TextID]string {
	all := map[kittui.TextID]string{}
	for _, p := range parts {
		maps.Copy(all, p)
	}
	return all
}

func fileChangeEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textTitleFileNew:      "New in %s",
		textTitleFileRename:   "Rename %s",
		textTitleFileCopy:     "Copy %s",
		textLeadFileNew:       "Creates an empty file or a folder. Nothing that exists is replaced.",
		textLeadFileRename:    "The entry stays in its folder. Nothing that exists is replaced.",
		textLeadFileCopy:      "The copy lies in the same folder and gets the owner and group of that folder.",
		textFieldFileName:     "Name",
		textFieldFileKind:     "Create",
		textFieldCopyName:     "Name of the copy",
		textHintFileName:      "One name in this folder, without /.",
		textHintFileKind:      "A file is created empty.",
		textProblemFileEmpty:  "The name is empty.",
		textProblemFileDots:   "The name is . or .., which are no names of an entry.",
		textProblemFileChars:  "The name holds a / or a control character.",
		textProblemFileTaken:  "%s exists already. Nothing is replaced.",
		textFileReadFirst:     "The folder %s is not read yet or must be read again, press R. Nothing is created before its entries are known.",
		textFileReadAgain:     "Changed. This folder was read through sudo, so it is not read again by itself. Press R to read it again.",
		textFileTooMany:       "%s holds more than %d entries. It is not changed in one step.",
		textFileNothingChange: "Nothing was changed.",
		textFileCount:         "Entries: %d, size of the files: %s.",
		textFileSizeUnknown: "The entries and their size cannot be read here (%s). If it holds more than %d entries or %d MiB, " +
			"the command refuses and nothing changes.",
		textFileFinal:       "This is final: beyond %d entries or %d MiB the journal keeps nothing of it, so it cannot be undone.",
		textAskFileNew:      "Create the empty file %s.",
		textAskFolderNew:    "Create the folder %s.",
		textAskFileRename:   "Rename %s to %s.",
		textAskFileCopy:     "Copy %s to %s.",
		textAskFolderCopy:   "Copy the folder %s with everything in it to %s.",
		textAskFileRemove:   "Remove %s.",
		textAskFolderRemove: "Remove the folder %s with everything in it.",
	}
}

func fileChangeGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textTitleFileNew:      "Neu in %s",
		textTitleFileRename:   "%s umbenennen",
		textTitleFileCopy:     "%s kopieren",
		textLeadFileNew:       "Legt eine leere Datei oder einen Ordner an. Nichts Vorhandenes wird ersetzt.",
		textLeadFileRename:    "Der Eintrag bleibt in seinem Ordner. Nichts Vorhandenes wird ersetzt.",
		textLeadFileCopy:      "Die Kopie liegt im selben Ordner und erhält Besitzer und Gruppe dieses Ordners.",
		textFieldFileName:     "Name",
		textFieldFileKind:     "Anlegen",
		textFieldCopyName:     "Name der Kopie",
		textHintFileName:      "Ein Name in diesem Ordner, ohne /.",
		textHintFileKind:      "Eine Datei wird leer angelegt.",
		textProblemFileEmpty:  "Der Name ist leer.",
		textProblemFileDots:   "Der Name ist . oder .., das sind keine Namen eines Eintrags.",
		textProblemFileChars:  "Der Name enthält ein / oder ein Steuerzeichen.",
		textProblemFileTaken:  "%s gibt es schon. Nichts wird ersetzt.",
		textFileReadFirst:     "Der Ordner %s ist noch nicht gelesen oder muss neu gelesen werden, drücke R. Vorher wird nichts angelegt.",
		textFileReadAgain:     "Geändert. Dieser Ordner wurde über sudo gelesen und wird nicht von selbst neu gelesen. Drücke R, um ihn neu zu lesen.",
		textFileTooMany:       "%s enthält mehr als %d Einträge. Das wird nicht in einem Schritt geändert.",
		textFileNothingChange: "Es wurde nichts geändert.",
		textFileCount:         "Einträge: %d, Größe der Dateien: %s.",
		textFileSizeUnknown: "Die Einträge und ihre Größe lassen sich hier nicht lesen (%s). Enthält der Ordner mehr als %d Einträge " +
			"oder %d MiB, lehnt der Befehl ab und nichts ändert sich.",
		textFileFinal:       "Das ist endgültig: Über %d Einträge oder %d MiB hinaus behält das Journal nichts davon, es lässt sich nicht rückgängig machen.",
		textAskFileNew:      "Die leere Datei %s anlegen.",
		textAskFolderNew:    "Den Ordner %s anlegen.",
		textAskFileRename:   "%s in %s umbenennen.",
		textAskFileCopy:     "%s nach %s kopieren.",
		textAskFolderCopy:   "Den Ordner %s mit allem darin nach %s kopieren.",
		textAskFileRemove:   "%s löschen.",
		textAskFolderRemove: "Den Ordner %s mit allem darin löschen.",
	}
}
