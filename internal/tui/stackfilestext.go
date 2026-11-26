package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the file browser over a stack folder and of the uncommitted changes it leaves. No text quotes the
// content of a file.
const (
	textKeyFiles             kittui.TextID = "key.files"
	textStackFilesNoFolder   kittui.TextID = "stackfiles.nofolder"
	textStackFileLocked      kittui.TextID = "stackfiles.locked"
	textStackFileDataRunning kittui.TextID = "stackfiles.datarunning"
	textFolderUncommitted    kittui.TextID = "folders.uncommitted"
	textDetailUncommitted    kittui.TextID = "detail.uncommitted"
	textTitleSecretsSaved    kittui.TextID = "title.secretssaved"
	textAskDeploySecrets     kittui.TextID = "ask.deploysecrets"
	textWhatDeploySecrets    kittui.TextID = "what.deploysecrets"
	textAnswerDeployNow      kittui.TextID = "answer.deploynow"
	textAnswerLater          kittui.TextID = "answer.later"
	textSecretsNextDeploy    kittui.TextID = "stackfiles.nextdeploy"
)

func stackFilesEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyFiles:           "files",
		textStackFilesNoFolder: "The service names no stack folder for %s.",
		textStackFileLocked: "%s is only read here: .git belongs to Git, and .qatlas/qhost holds the settings of the stacks, " +
			"which the forms of the stack detail change.",
		textStackFileDataRunning: "%s is the data folder of the running container %s. The change is not refused, but the " +
			"container may overwrite it or read it half written. If in doubt, stop the stack first.",
		textFolderUncommitted: "uncommitted changes, no updates",
		textDetailUncommitted: "This folder has uncommitted changes: updates leave it out until they are committed or discarded.",
		textTitleSecretsSaved: "secrets.env saved",
		textAskDeploySecrets:  "The change of %s takes effect when the stack %s is deployed next. Deploy it now?",
		textWhatDeploySecrets: "Deploying starts a run of the stack, which creates its containers again with the new values.",
		textAnswerDeployNow:   "deploy now",
		textAnswerLater:       "later",
		textSecretsNextDeploy: "The change of %s takes effect when its stack is deployed next.",
	}
}

func stackFilesGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyFiles:           "Dateien",
		textStackFilesNoFolder: "Der Dienst nennt keinen Stack-Ordner für %s.",
		textStackFileLocked: "%s wird hier nur gelesen: .git gehört Git, und .qatlas/qhost enthält die Einstellungen der " +
			"Stacks, die die Formulare im Stack-Detail ändern.",
		textStackFileDataRunning: "%s ist der Datenordner des laufenden Containers %s. Die Änderung wird nicht verweigert, " +
			"aber der Container kann sie überschreiben oder halb geschrieben lesen. Stoppe im Zweifel vorher den Stack.",
		textFolderUncommitted: "nicht committet, keine Updates",
		textDetailUncommitted: "Dieser Ordner hat nicht committete Änderungen: Updates lassen ihn aus, bis sie committet oder verworfen sind.",
		textTitleSecretsSaved: "secrets.env gespeichert",
		textAskDeploySecrets:  "Die Änderung an %s wirkt erst, wenn der Stack %s das nächste Mal ausgerollt wird. Jetzt ausrollen?",
		textWhatDeploySecrets: "Das Ausrollen startet einen Lauf des Stacks, der seine Container mit den neuen Werten neu erstellt.",
		textAnswerDeployNow:   "jetzt ausrollen",
		textAnswerLater:       "später",
		textSecretsNextDeploy: "Die Änderung an %s wirkt erst beim nächsten Ausrollen ihres Stacks.",
	}
}
