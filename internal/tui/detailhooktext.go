package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the Webhooks section of the stack detail. The address of a hook is shown by the one dialog after it
// was added and is never part of another text.
const (
	textDetailHooks       kittui.TextID = "detail.hooks"
	textHooksNoReceiver   kittui.TextID = "hooks.noreceiver"
	textHooksNone         kittui.TextID = "hooks.none"
	textHooksWindow       kittui.TextID = "hooks.window"
	textHooksNoneToRemove kittui.TextID = "hooks.nonetoremove"
	textHookRow           kittui.TextID = "hook.row"
	textKeyHookAdd        kittui.TextID = "key.hookadd"
	textKeyHookRemove     kittui.TextID = "key.hookremove"
	textColHookID         kittui.TextID = "col.hookid"
	textTitleHookAdd      kittui.TextID = "title.hookadd"
	textTitleHookRemove   kittui.TextID = "title.hookremove"
	textTitleHookSecret   kittui.TextID = "title.hooksecret"
	textAskHookAdd        kittui.TextID = "ask.hookadd"
	textWhatHookAdd       kittui.TextID = "what.hookadd"
	textAskHookRemove     kittui.TextID = "ask.hookremove"
	textWhatHookRemove    kittui.TextID = "what.hookremove"
	textHookSecretShown   kittui.TextID = "hook.secretshown"
	textHookSecretOnce    kittui.TextID = "hook.secretonce"
	textDoneHookRemoved   kittui.TextID = "done.hookremoved"
	textHookSecretAddress kittui.TextID = "hook.secretaddress"
)

func detailHookEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textDetailHooks:       "Webhooks",
		textHooksNoReceiver:   "No webhook receiver is set up. Set it up under Settings, Webhooks.",
		textHooksNone:         "No webhook for this stack.",
		textHooksWindow:       "The update trigger is Maintenance window: a hook acts only with the trigger Polling or Webhook only.",
		textHooksNoneToRemove: "This stack has no webhook to remove.",
		textHookRow:           "%s  modified %s",
		textKeyHookAdd:        "add hook",
		textKeyHookRemove:     "remove hook",
		textColHookID:         "ID",
		textTitleHookAdd:      "Add a webhook",
		textTitleHookRemove:   "Remove a webhook",
		textTitleHookSecret:   "Address of the new webhook",
		textAskHookAdd:        "Give the stack %s a webhook?",
		textWhatHookAdd:       "A secret address is created. It is shown once after this and cannot be read again.",
		textAskHookRemove:     "Remove the webhook %s of the stack %s?",
		textWhatHookRemove:    "Its address stops working within ten seconds. This cannot be undone.",
		textHookSecretShown:   "Webhook %s of the stack %s was added. A POST call of this address, without content, makes qhost check the stack.",
		textHookSecretOnce:    "The address is shown only now and does not appear again. Copy it before closing.",
		textHookSecretAddress: "http://%s%s",
		textDoneHookRemoved:   "The webhook %s was removed.",
	}
}

func detailHookGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textDetailHooks:       "Webhooks",
		textHooksNoReceiver:   "Kein Webhook-Empfänger eingerichtet. Richte ihn unter Einstellungen, Webhooks ein.",
		textHooksNone:         "Kein Webhook für diesen Stack.",
		textHooksWindow:       "Der Auslöser ist Wartungsfenster: Ein Hook wirkt erst mit dem Auslöser Abfrage oder Nur Webhook.",
		textHooksNoneToRemove: "Dieser Stack hat keinen Webhook zum Entfernen.",
		textHookRow:           "%s  geändert %s",
		textKeyHookAdd:        "Hook anlegen",
		textKeyHookRemove:     "Hook entfernen",
		textColHookID:         "ID",
		textTitleHookAdd:      "Webhook anlegen",
		textTitleHookRemove:   "Webhook entfernen",
		textTitleHookSecret:   "Adresse des neuen Webhooks",
		textAskHookAdd:        "Dem Stack %s einen Webhook geben?",
		textWhatHookAdd:       "Es entsteht eine geheime Adresse. Sie wird danach einmal gezeigt und ist nicht wieder lesbar.",
		textAskHookRemove:     "Den Webhook %s des Stacks %s entfernen?",
		textWhatHookRemove:    "Seine Adresse wirkt nach spätestens zehn Sekunden nicht mehr. Das lässt sich nicht rückgängig machen.",
		textHookSecretShown:   "Der Webhook %s des Stacks %s wurde angelegt. Ein POST-Aufruf dieser Adresse ohne Inhalt lässt qhost den Stack prüfen.",
		textHookSecretOnce:    "Die Adresse wird nur jetzt gezeigt und erscheint nicht wieder. Kopiere sie vor dem Schließen.",
		textHookSecretAddress: "http://%s%s",
		textDoneHookRemoved:   "Der Webhook %s wurde entfernt.",
	}
}
