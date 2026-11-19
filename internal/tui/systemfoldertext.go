package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of System > Folders. Paths and names of the host come in as arguments; setuid, setgid, sticky and
// ACL stay as they are in every language.
const (
	textPageFolders kittui.TextID = "system.page.folders"

	textFoldersHint      kittui.TextID = "folders.hint"
	textFolderReading    kittui.TextID = "folders.reading"
	textFolderNotFound   kittui.TextID = "folders.notfound"
	textFolderDenied     kittui.TextID = "folders.denied"
	textFolderFailed     kittui.TextID = "folders.failed"
	textFolderNotAbs     kittui.TextID = "folders.notabs"
	textFolderResolved   kittui.TextID = "folders.resolved"
	textFolderKind       kittui.TextID = "folders.kind"
	textKindFolder       kittui.TextID = "folders.kind.folder"
	textKindFile         kittui.TextID = "folders.kind.file"
	textKindOther        kittui.TextID = "folders.kind.other"
	textFolderOwnerMay   kittui.TextID = "folders.ownermay"
	textFolderGroupMay   kittui.TextID = "folders.groupmay"
	textFolderOthersMay  kittui.TextID = "folders.othersmay"
	textFolderMode       kittui.TextID = "folders.mode"
	textFolderACL        kittui.TextID = "folders.acl"
	textFolderACLUnknown kittui.TextID = "folders.aclunknown"
	textFolderSpecial    kittui.TextID = "folders.special"
	textFolderSystem     kittui.TextID = "folders.system"
	textLevelNothing     kittui.TextID = "level.nothing"
	textLevelRead        kittui.TextID = "level.read"
	textLevelReadWrite   kittui.TextID = "level.readwrite"

	textKeyChooseFolder kittui.TextID = "key.choosefolder"
	textTitleFolderPath kittui.TextID = "title.folderpath"
	textAskFolderPath   kittui.TextID = "ask.folderpath"
	textAnswerShow      kittui.TextID = "answer.show"

	textTitleChangeFolder kittui.TextID = "title.changefolder"
	textLeadFolder        kittui.TextID = "lead.folder"
	textLeadFolderSystem  kittui.TextID = "lead.foldersystem"
	textFieldFolderOwner  kittui.TextID = "field.folderowner"
	textFieldFolderGroup  kittui.TextID = "field.foldergroup"
	textFieldOwnerMay     kittui.TextID = "field.ownermay"
	textFieldGroupMay     kittui.TextID = "field.groupmay"
	textFieldOthersMay    kittui.TextID = "field.othersmay"
	textFieldBelow        kittui.TextID = "field.below"
	textHintFolderOwner   kittui.TextID = "hint.folderowner"
	textHintFolderGroup   kittui.TextID = "hint.foldergroup"
	textHintFolderLevel   kittui.TextID = "hint.folderlevel"
	textHintFolderBelow   kittui.TextID = "hint.folderbelow"

	textAskFolder         kittui.TextID = "ask.folder"
	textAskFolderBelow    kittui.TextID = "ask.folderbelow"
	textFolderCount       kittui.TextID = "folders.count"
	textFolderCountOne    kittui.TextID = "folders.countone"
	textRiskFolderSystem  kittui.TextID = "host.riskfoldersystem"
	textFolderOnlyDir     kittui.TextID = "folders.onlydir"
	textFolderNone        kittui.TextID = "folders.none"
	textFolderTooMany     kittui.TextID = "folders.toomany"
	textFolderCountFailed kittui.TextID = "folders.countfailed"
	textFolderSmaller     kittui.TextID = "folders.smaller"
)

func folderEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPageFolders: "Folders",

		textFoldersHint:      "Press {enter} to choose a folder and see who may read and change it.",
		textFolderReading:    "Reading %s...",
		textFolderNotFound:   "%s does not exist.",
		textFolderDenied:     "%s cannot be read: permission denied.",
		textFolderFailed:     "%s cannot be read: %s",
		textFolderNotAbs:     "%s is not an absolute path. Type one such as /srv/data.",
		textFolderResolved:   "Real path: %s",
		textFolderKind:       "Type: %s",
		textKindFolder:       "folder",
		textKindFile:         "file",
		textKindOther:        "other",
		textFolderOwnerMay:   "Owner %s may: %s",
		textFolderGroupMay:   "Group %s may: %s",
		textFolderOthersMay:  "Everyone else may: %s",
		textFolderMode:       "Mode: %s",
		textFolderACL:        "ACL: present. qhost does not change ACLs; they may grant more than shown here.",
		textFolderACLUnknown: "ACL: unknown",
		textFolderSpecial:    "Special bits: %s. A change leaves them as they are.",
		textFolderSystem:     "This is a system path: a wrong owner or permission here can break the host.",
		textLevelNothing:     "nothing",
		textLevelRead:        "read",
		textLevelReadWrite:   "read and write",

		textKeyChooseFolder: "choose folder",
		textTitleFolderPath: "Choose a folder",
		textAskFolderPath:   "Type the absolute path of the folder.",
		textAnswerShow:      "show",

		textTitleChangeFolder: "Change %s",
		textLeadFolder:        "Owner, group and what each may do. Read lets them open and list it, read and write also change what it holds.",
		textLeadFolderSystem:  "This is a system path. Wrong owners or permissions here can stop services and block logins.",
		textFieldFolderOwner:  "Owner",
		textFieldFolderGroup:  "Group",
		textFieldOwnerMay:     "Owner may",
		textFieldGroupMay:     "Group may",
		textFieldOthersMay:    "Everyone else may",
		textFieldBelow:        "Everything below it",
		textHintFolderOwner:   "The user the folder belongs to.",
		textHintFolderGroup:   "The group whose members get the rights of the group.",
		textHintFolderLevel:   "nothing, read, or read and write.",
		textHintFolderBelow:   "Yes changes every file and folder inside it too; their number is shown before anything runs.",

		textAskFolder:         "Change %s: owner %s, group %s; the owner may %s, the group %s, everyone else %s.",
		textAskFolderBelow:    "Change %s and everything below it: owner %s, group %s; the owner may %s, the group %s, everyone else %s.",
		textFolderCount:       "%d entries change: the folder and everything below it.",
		textFolderCountOne:    "1 entry changes: the folder itself, nothing lies below it.",
		textRiskFolderSystem:  "This is a system path: wrong owners or permissions here can stop services and block logins on this host.",
		textFolderOnlyDir:     "Only a folder can be changed here, not a file.",
		textFolderNone:        "Choose a folder first with {enter}.",
		textFolderTooMany:     "%s holds more than %d entries.",
		textFolderCountFailed: "The entries below the folder cannot be counted: %s",
		textFolderSmaller:     "Change a smaller folder, or the folder alone. Nothing was changed.",
	}
}

func folderGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPageFolders: "Ordner",

		textFoldersHint:      "Drücke {enter}, um einen Ordner zu wählen und zu sehen, wer ihn lesen und ändern darf.",
		textFolderReading:    "%s wird gelesen...",
		textFolderNotFound:   "%s gibt es nicht.",
		textFolderDenied:     "%s lässt sich nicht lesen: keine Berechtigung.",
		textFolderFailed:     "%s lässt sich nicht lesen: %s",
		textFolderNotAbs:     "%s ist kein absoluter Pfad. Gib einen wie /srv/data ein.",
		textFolderResolved:   "Echter Pfad: %s",
		textFolderKind:       "Art: %s",
		textKindFolder:       "Ordner",
		textKindFile:         "Datei",
		textKindOther:        "anderes",
		textFolderOwnerMay:   "Besitzer %s darf: %s",
		textFolderGroupMay:   "Gruppe %s darf: %s",
		textFolderOthersMay:  "Alle anderen dürfen: %s",
		textFolderMode:       "Modus: %s",
		textFolderACL:        "ACL: vorhanden. qhost ändert keine ACLs, sie können mehr erlauben als hier steht.",
		textFolderACLUnknown: "ACL: unbekannt",
		textFolderSpecial:    "Sonderbits: %s. Eine Änderung lässt sie, wie sie sind.",
		textFolderSystem:     "Das ist ein Systempfad: Falsche Besitzer oder Rechte können hier den Host beschädigen.",
		textLevelNothing:     "nichts",
		textLevelRead:        "lesen",
		textLevelReadWrite:   "lesen und schreiben",

		textKeyChooseFolder: "Ordner wählen",
		textTitleFolderPath: "Ordner wählen",
		textAskFolderPath:   "Gib den absoluten Pfad des Ordners ein.",
		textAnswerShow:      "anzeigen",

		textTitleChangeFolder: "%s ändern",
		textLeadFolder:        "Besitzer, Gruppe und was jeder darf. Lesen erlaubt Öffnen und Auflisten, lesen und schreiben auch das Ändern des Inhalts.",
		textLeadFolderSystem:  "Das ist ein Systempfad. Falsche Besitzer oder Rechte können hier Dienste stoppen und Anmeldungen blockieren.",
		textFieldFolderOwner:  "Besitzer",
		textFieldFolderGroup:  "Gruppe",
		textFieldOwnerMay:     "Besitzer darf",
		textFieldGroupMay:     "Gruppe darf",
		textFieldOthersMay:    "Alle anderen dürfen",
		textFieldBelow:        "Alles darunter",
		textHintFolderOwner:   "Der Benutzer, dem der Ordner gehört.",
		textHintFolderGroup:   "Die Gruppe, deren Mitglieder die Rechte der Gruppe erhalten.",
		textHintFolderLevel:   "nichts, lesen oder lesen und schreiben.",
		textHintFolderBelow:   "Ja ändert auch jede Datei und jeden Ordner darin. Ihre Anzahl steht da, bevor etwas läuft.",

		textAskFolder:         "%s ändern: Besitzer %s, Gruppe %s. Der Besitzer darf %s, die Gruppe %s, alle anderen %s.",
		textAskFolderBelow:    "%s und alles darunter ändern: Besitzer %s, Gruppe %s. Der Besitzer darf %s, die Gruppe %s, alle anderen %s.",
		textFolderCount:       "%d Einträge ändern sich: der Ordner und alles darunter.",
		textFolderCountOne:    "1 Eintrag ändert sich: der Ordner selbst, darunter liegt nichts.",
		textRiskFolderSystem:  "Das ist ein Systempfad: Falsche Besitzer oder Rechte können hier Dienste stoppen und Anmeldungen auf diesem Host blockieren.",
		textFolderOnlyDir:     "Hier lässt sich nur ein Ordner ändern, keine Datei.",
		textFolderNone:        "Wähle zuerst mit {enter} einen Ordner.",
		textFolderTooMany:     "%s enthält mehr als %d Einträge.",
		textFolderCountFailed: "Die Einträge unter dem Ordner lassen sich nicht zählen: %s",
		textFolderSmaller:     "Ändere einen kleineren Ordner oder nur den Ordner selbst. Es wurde nichts geändert.",
	}
}
