package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the deploy services in System > Log.
const (
	textColFunction kittui.TextID = "col.function"
	textColOrigin   kittui.TextID = "col.origin"

	textKeyLogService   kittui.TextID = "key.logservice"
	textLogServiceHost  kittui.TextID = "deploylog.service.host"
	textLogServiceDepl  kittui.TextID = "deploylog.service.deploy"
	textDeployLogEmpty  kittui.TextID = "deploylog.empty"
	textDeployLogDown   kittui.TextID = "deploylog.down"
	textDeployLogNotRun kittui.TextID = "deploylog.notrunning"
	textDeployLogNoAcc  kittui.TextID = "deploylog.noaccess"
)

func deployLogEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textColFunction:     "function",
		textColOrigin:       "origin",
		textKeyLogService:   "service",
		textLogServiceHost:  "Service: host",
		textLogServiceDepl:  "Service: deploy, engine %s",
		textDeployLogEmpty:  "The action log of this deploy service holds no entry yet.",
		textDeployLogDown:   "The deploy service of engine %s cannot be asked: %s",
		textDeployLogNotRun: "The deploy service of engine %s is not set up or not running.",
		textDeployLogNoAcc:  "The deploy service of engine %s is reserved for administrators (sudo).",
	}
}

func deployLogGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textColFunction:     "Funktion",
		textColOrigin:       "Herkunft",
		textKeyLogService:   "Dienst",
		textLogServiceHost:  "Dienst: Host",
		textLogServiceDepl:  "Dienst: Deploy, Engine %s",
		textDeployLogEmpty:  "Das Aktionsprotokoll dieses Deploy-Dienstes enthält noch keinen Eintrag.",
		textDeployLogDown:   "Der Deploy-Dienst der Engine %s lässt sich nicht fragen: %s",
		textDeployLogNotRun: "Der Deploy-Dienst der Engine %s ist nicht eingerichtet oder läuft nicht.",
		textDeployLogNoAcc:  "Der Deploy-Dienst der Engine %s ist Administratoren vorbehalten (sudo).",
	}
}
