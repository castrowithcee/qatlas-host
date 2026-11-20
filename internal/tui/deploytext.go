package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of setting up the deploy service.
const (
	textKeySetupDeploy kittui.TextID = "key.setupdeploy"
	textErrNotSetUp    kittui.TextID = "error.notsetup"
	textSetupTitle     kittui.TextID = "setup.title"
	textSetupBody      kittui.TextID = "setup.body"
	textAskSetup       kittui.TextID = "setup.ask"
	textWhatSetup      kittui.TextID = "setup.what"
	textDoneSetup      kittui.TextID = "setup.done"

	textAskSetupWhich       kittui.TextID = "setup.askwhich"
	textWhatSetupWhich      kittui.TextID = "setup.whatwhich"
	textAnswerSetupSystem   kittui.TextID = "setup.answer.system"
	textAnswerSetupRootless kittui.TextID = "setup.answer.rootless"
	textAnswerSetupSteps    kittui.TextID = "setup.answer.steps"
	textAskSetupUser        kittui.TextID = "setup.askuser"
	textSetupNoUsers        kittui.TextID = "setup.nousers"
	textAskSetupFor         kittui.TextID = "setup.askfor"
	textWhatSetupFor        kittui.TextID = "setup.whatfor"
	textSetupNothing        kittui.TextID = "setup.nothing"
	textDoneSetupUser       kittui.TextID = "setup.doneuser"
	textSetupAsRoot         kittui.TextID = "setup.asroot"
)

func deployEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeySetupDeploy: "set up deploy service",
		textErrNotSetUp:    "the deploy service is not set up",
		textSetupTitle:     "Deploy service",
		textSetupBody: "The deploy service is not set up yet. It rolls out and updates your stacks and runs as its own system user, " +
			"apart from the host service, with access to Docker. {setupdeploy} sets it up; on the command line: sudo qhost deploy-service setup",
		textAskSetup: "Set up the deploy service now?",
		textWhatSetup: "It creates the system user qhost-deploy in the group docker, its channel and its unit, and starts the service. " +
			"The host service gets no access to Docker. These steps run as root:",
		textDoneSetup: "the deploy service is set up and started",

		textAskSetupWhich: "Which deploy service do you want to set up?",
		textWhatSetupWhich: "The system engine gets a service with its own system user. A rootless engine gets a service " +
			"that runs as the user who owns that engine.",
		textAnswerSetupSystem:   "system engine",
		textAnswerSetupRootless: "rootless engine",
		textAnswerSetupSteps:    "show steps",
		textAskSetupUser:        "User of the rootless engine (with an engine: %s)",
		textSetupNoUsers:        "User of the rootless engine (no user with an engine found)",
		textAskSetupFor:         "Set up the deploy service for the rootless engine of %s now?",
		textWhatSetupFor: "It runs as %s with its own unit, channel and state, uses only that user's engine and starts at boot. " +
			"No user is created and no group changes.",
		textSetupAsRoot:   "These steps run as root:",
		textSetupNothing:  "nothing to do: the deploy service is set up already",
		textDoneSetupUser: "the deploy service for the rootless engine of %s is set up and started",
	}
}

func deployGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeySetupDeploy: "Deploy-Dienst einrichten",
		textErrNotSetUp:    "der Deploy-Dienst ist nicht eingerichtet",
		textSetupTitle:     "Deploy-Dienst",
		textSetupBody: "Der Deploy-Dienst ist noch nicht eingerichtet. Er rollt deine Stacks aus und hält sie aktuell, läuft als eigener " +
			"Systembenutzer getrennt vom Host-Dienst und hat Zugriff auf Docker. {setupdeploy} richtet ihn ein; auf der Kommandozeile: sudo qhost deploy-service setup",
		textAskSetup: "Den Deploy-Dienst jetzt einrichten?",
		textWhatSetup: "Das legt den Systembenutzer qhost-deploy in der Gruppe docker, seinen Kanal und seine Unit an und startet den Dienst. " +
			"Der Host-Dienst bekommt keinen Zugriff auf Docker. Diese Schritte laufen als root:",
		textDoneSetup: "der Deploy-Dienst ist eingerichtet und gestartet",

		textAskSetupWhich: "Welchen Deploy-Dienst möchtest du einrichten?",
		textWhatSetupWhich: "Die System-Engine bekommt einen Dienst mit eigenem Systembenutzer. Eine rootless Engine bekommt einen " +
			"Dienst, der als der Benutzer läuft, dem die Engine gehört.",
		textAnswerSetupSystem:   "System-Engine",
		textAnswerSetupRootless: "rootless Engine",
		textAnswerSetupSteps:    "Schritte zeigen",
		textAskSetupUser:        "Benutzer der rootless Engine (mit Engine: %s)",
		textSetupNoUsers:        "Benutzer der rootless Engine (kein Benutzer mit Engine gefunden)",
		textAskSetupFor:         "Den Deploy-Dienst für die rootless Engine von %s jetzt einrichten?",
		textWhatSetupFor: "Er läuft als %s mit eigener Unit, eigenem Kanal und eigenem Zustand, nutzt nur die Engine dieses Benutzers " +
			"und startet beim Boot. Es wird kein Benutzer angelegt und keine Gruppe geändert.",
		textSetupAsRoot:   "Diese Schritte laufen als root:",
		textSetupNothing:  "nichts zu tun: der Deploy-Dienst ist schon eingerichtet",
		textDoneSetupUser: "der Deploy-Dienst für die rootless Engine von %s ist eingerichtet und gestartet",
	}
}
