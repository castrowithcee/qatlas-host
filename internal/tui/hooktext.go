package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the Webhooks page of the Settings area. Technical words (qhost-hook, the unit) stay English.
const (
	textPageHooks         kittui.TextID = "page.hooks"
	textKeyHookSetup      kittui.TextID = "key.hooksetup"
	textKeyHookListen     kittui.TextID = "key.hooklisten"
	textHookNotSetUp      kittui.TextID = "hook.notsetup"
	textHookRunning       kittui.TextID = "hook.running"
	textHookStopped       kittui.TextID = "hook.stopped"
	textHookBody          kittui.TextID = "hook.body"
	textHookProblem       kittui.TextID = "hook.problem"
	textHookAddress       kittui.TextID = "hook.address"
	textHookCount         kittui.TextID = "hook.count"
	textHookProxy         kittui.TextID = "hook.proxy"
	textTitleHook         kittui.TextID = "title.hook"
	textTitleHookListen   kittui.TextID = "title.hooklisten"
	textFieldHookListen   kittui.TextID = "field.hooklisten"
	textHintHookListen    kittui.TextID = "hint.hooklisten"
	textProblemHookListen kittui.TextID = "problem.hooklisten"
	textProblemHookSame   kittui.TextID = "problem.hooksame"
	textAskHookSetup      kittui.TextID = "ask.hooksetup"
	textWhatHookSetup     kittui.TextID = "what.hooksetup"
	textAskHookListen     kittui.TextID = "ask.hooklisten"
	textWhatHookListen    kittui.TextID = "what.hooklisten"
	textDoneHook          kittui.TextID = "done.hook"
)

func hookEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPageHooks:     "Webhooks",
		textKeyHookSetup:  "set up receiver",
		textKeyHookListen: "address",
		textHookNotSetUp:  "not set up",
		textHookRunning:   "running",
		textHookStopped:   "not running",
		textHookBody: "The webhook receiver is not set up. It lets a call of a secret address make qhost check a stack at once. " +
			"A first setup listens only on this host, on %s; the address can be chosen before.",
		textHookProblem:       "The configuration of the receiver cannot be read: %s",
		textHookAddress:       "Address: %s",
		textHookCount:         "Hooks: %d",
		textHookProxy:         "The address is not local and the receiver speaks plain HTTP: a reverse proxy should provide HTTPS.",
		textTitleHook:         "Webhook receiver",
		textTitleHookListen:   "Address of the webhook receiver",
		textFieldHookListen:   "Address",
		textHintHookListen:    "host:port with a port above 1024. 127.0.0.1 reaches only this host.",
		textProblemHookListen: "Give the address as host:port with a port from 1025 to 65535.",
		textProblemHookSame:   "The receiver listens on this address already.",
		textAskHookSetup:      "Set up the webhook receiver on %s?",
		textWhatHookSetup: "This creates the system user qhost-hook, its configuration and the unit qhost-hook.service and starts " +
			"it. It changes nothing for the stacks.",
		textAskHookListen:  "Let the webhook receiver listen on %s?",
		textWhatHookListen: "The receiver restarts on the new address. The hooks stay as they are.",
		textDoneHook:       "The webhook receiver listens on %s.",
	}
}

func hookGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPageHooks:     "Webhooks",
		textKeyHookSetup:  "Empfänger einrichten",
		textKeyHookListen: "Adresse",
		textHookNotSetUp:  "nicht eingerichtet",
		textHookRunning:   "läuft",
		textHookStopped:   "läuft nicht",
		textHookBody: "Der Webhook-Empfänger ist nicht eingerichtet. Er lässt den Aufruf einer geheimen Adresse qhost sofort einen Stack " +
			"prüfen. Eine erste Einrichtung lauscht nur auf diesem Host, auf %s. Die Adresse kann vorher gewählt werden.",
		textHookProblem:       "Die Konfiguration des Empfängers ist nicht lesbar: %s",
		textHookAddress:       "Adresse: %s",
		textHookCount:         "Hooks: %d",
		textHookProxy:         "Die Adresse ist nicht lokal und der Empfänger spricht unverschlüsseltes HTTP: Ein Reverse Proxy sollte HTTPS bieten.",
		textTitleHook:         "Webhook-Empfänger",
		textTitleHookListen:   "Adresse des Webhook-Empfängers",
		textFieldHookListen:   "Adresse",
		textHintHookListen:    "host:port mit einem Port über 1024. 127.0.0.1 erreicht nur diesen Host.",
		textProblemHookListen: "Gib die Adresse als host:port an, mit einem Port von 1025 bis 65535.",
		textProblemHookSame:   "Der Empfänger lauscht schon auf dieser Adresse.",
		textAskHookSetup:      "Den Webhook-Empfänger auf %s einrichten?",
		textWhatHookSetup: "Das legt den Systembenutzer qhost-hook, seine Konfiguration und die Unit qhost-hook.service an und startet sie. " +
			"An den Stacks ändert sich nichts.",
		textAskHookListen:  "Den Webhook-Empfänger auf %s lauschen lassen?",
		textWhatHookListen: "Der Empfänger startet auf der neuen Adresse neu. Die Hooks bleiben, wie sie sind.",
		textDoneHook:       "Der Webhook-Empfänger lauscht auf %s.",
	}
}
