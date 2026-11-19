package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of setting up the deploy service.
const (
	textKeySetupDeploy kittui.TextID = "key.setupdeploy"
	textErrNotSetUp    kittui.TextID = "error.notsetup"
	textSetupTitle     kittui.TextID = "setup.title"
	textSetupBody      kittui.TextID = "setup.body"
	textSetupNeedsRoot kittui.TextID = "setup.needsroot"
	textAskSetup       kittui.TextID = "setup.ask"
	textWhatSetup      kittui.TextID = "setup.what"
	textDoneSetup      kittui.TextID = "setup.done"
)

func deployEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeySetupDeploy: "set up deploy service",
		textErrNotSetUp:    "the deploy service is not set up",
		textSetupTitle:     "Deploy service",
		textSetupBody: "The deploy service is not set up yet. It rolls out and updates your stacks and runs as its own system user, " +
			"apart from the host service, with access to Docker. {setupdeploy} sets it up; on the command line: sudo qhost deploy-service setup",
		textSetupNeedsRoot: "Setting up needs root: quit and run sudo qhost.",
		textAskSetup:       "Set up the deploy service now?",
		textWhatSetup: "It creates the system user qhost-deploy in the group docker, its channel and its unit, and starts the service. " +
			"The host service gets no access to Docker. These steps run as root:",
		textDoneSetup: "the deploy service is set up and started",
	}
}

func deployGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeySetupDeploy: "Deploy-Dienst einrichten",
		textErrNotSetUp:    "der Deploy-Dienst ist nicht eingerichtet",
		textSetupTitle:     "Deploy-Dienst",
		textSetupBody: "Der Deploy-Dienst ist noch nicht eingerichtet. Er rollt deine Stacks aus und hält sie aktuell, läuft als eigener " +
			"Systembenutzer getrennt vom Host-Dienst und hat Zugriff auf Docker. {setupdeploy} richtet ihn ein; auf der Kommandozeile: sudo qhost deploy-service setup",
		textSetupNeedsRoot: "Das Einrichten braucht root: beende und starte sudo qhost.",
		textAskSetup:       "Den Deploy-Dienst jetzt einrichten?",
		textWhatSetup: "Das legt den Systembenutzer qhost-deploy in der Gruppe docker, seinen Kanal und seine Unit an und startet den Dienst. " +
			"Der Host-Dienst bekommt keinen Zugriff auf Docker. Diese Schritte laufen als root:",
		textDoneSetup: "der Deploy-Dienst ist eingerichtet und gestartet",
	}
}
