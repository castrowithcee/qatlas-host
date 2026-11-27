package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of securing the .git of a stack folder.
const (
	textKeySecure        kittui.TextID = "key.secure"
	textFolderGitUnsafe  kittui.TextID = "folders.gitunsafe"
	textDetailGitUnsafe  kittui.TextID = "detail.gitunsafe"
	textDetailGitSecure  kittui.TextID = "detail.gitsecure"
	textFolderSecureNone kittui.TextID = "folders.securenone"
	textAskSecure        kittui.TextID = "ask.secure"
	textWhatSecure       kittui.TextID = "what.secure"
	textDoneSecured      kittui.TextID = "done.secured"
)

func secureEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeySecure:        "secure",
		textFolderGitUnsafe:  "git not run: %s",
		textDetailGitUnsafe:  "Git is not run in this folder: %s",
		textDetailGitSecure:  "Nothing is fetched or deployed from it until its .git is secured.",
		textFolderSecureNone: "The .git of this stack folder needs no securing.",
		textAskSecure:        "Make the .git of %s root-only?",
		textWhatSecure: "Every entry of .git gets the owner and group root and loses write for group and others, and " +
			"core.sharedRepository is removed. The work tree stays as it is. Afterwards the deploy service runs git in the folder again.",
		textDoneSecured: "The .git of %s is root-only now.",
	}
}

func secureGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeySecure:        "absichern",
		textFolderGitUnsafe:  "Git läuft nicht: %s",
		textDetailGitUnsafe:  "In diesem Ordner läuft kein Git: %s",
		textDetailGitSecure:  "Bis sein .git abgesichert ist, wird daraus nichts abgerufen oder ausgerollt.",
		textFolderSecureNone: "Das .git dieses Stack-Ordners muss nicht abgesichert werden.",
		textAskSecure:        "Das .git von %s nur für root beschreibbar machen?",
		textWhatSecure: "Jeder Eintrag in .git bekommt Eigentümer und Gruppe root und verliert das Schreibrecht für Gruppe und " +
			"andere, und core.sharedRepository wird entfernt. Der Arbeitsbaum bleibt, wie er ist. Danach führt der Deploy-Dienst Git im Ordner wieder aus.",
		textDoneSecured: "Das .git von %s ist jetzt nur für root beschreibbar.",
	}
}
