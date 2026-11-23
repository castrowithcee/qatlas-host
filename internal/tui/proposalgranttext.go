package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the proposal kind grant.
const (
	textPropKindGrant          kittui.TextID = "prop.kind.grant"
	textPropGrantSentence      kittui.TextID = "prop.grant.sentence"
	textPropGrantRestart       kittui.TextID = "prop.grant.restart"
	textPropGrantContainer     kittui.TextID = "prop.grant.container"
	textPropGrantCheckUpdates  kittui.TextID = "prop.grant.checkupdates"
	textPropGrantUpdate        kittui.TextID = "prop.grant.update"
	textPropGrantAsked         kittui.TextID = "prop.grant.asked"
	textPropGrantExpires       kittui.TextID = "prop.grant.expires"
	textPropGrantSet           kittui.TextID = "prop.grant.set"
	textPropGrantNever         kittui.TextID = "prop.grant.never"
	textPropGrantUntil         kittui.TextID = "prop.grant.until"
	textPropGrantBackup        kittui.TextID = "prop.grant.backup"
	textPropGrantCheck         kittui.TextID = "prop.grant.check"
	textPropGrantSetUp         kittui.TextID = "prop.grant.setup"
	textPropGrantNotSetUp      kittui.TextID = "prop.grant.notsetup"
	textPropGrantUnknown       kittui.TextID = "prop.grant.unknown"
	textPropGrantPush          kittui.TextID = "prop.grant.push"
	textKeyGrantExpiry         kittui.TextID = "key.grantexpiry"
	textTitleGrantExpiry       kittui.TextID = "title.grantexpiry"
	textFieldGrantExpires      kittui.TextID = "field.grantexpires"
	textHintGrantExpires       kittui.TextID = "hint.grantexpires"
	textProblemGrantExpires    kittui.TextID = "problem.grantexpires"
	textProblemGrantExpiresMax kittui.TextID = "problem.grantexpiresmax"
	textAskProposalGrant       kittui.TextID = "ask.propgrant"
	textWhatProposalGrantSudo  kittui.TextID = "what.propgrant.sudo"
	textDonePropGrant          kittui.TextID = "done.propgrant"
	textDonePropGrantReplaced  kittui.TextID = "done.propgrantreplaced"
)

// grantActionTexts is the phrase of each action a grant can name, with the stack as its argument.
var grantActionTexts = map[string]kittui.TextID{
	"stack.restart":     textPropGrantRestart,
	"container.restart": textPropGrantContainer,
	"update.check":      textPropGrantCheckUpdates,
	"update.run":        textPropGrantUpdate,
}

func proposalGrantEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPropKindGrant:          "standing grant",
		textPropGrantSentence:      "%s (uid %d) may %s without sudo, as a standing permission.",
		textPropGrantRestart:       "restart the stack %s",
		textPropGrantContainer:     "restart containers of the stack %s",
		textPropGrantCheckUpdates:  "check the stack %s for updates",
		textPropGrantUpdate:        "update the stack %s",
		textPropGrantAsked:         "expiry asked for: %s",
		textPropGrantExpires:       "expiry of the grant: %s",
		textPropGrantSet:           "You set this expiry; it is not the one asked for.",
		textPropGrantNever:         "never",
		textPropGrantUntil:         "after the end of %s (UTC)",
		textPropGrantBackup:        "backup of the stack: %s",
		textPropGrantCheck:         "health check of the stack: %s",
		textPropGrantSetUp:         "set up",
		textPropGrantNotSetUp:      "not set up",
		textPropGrantUnknown:       "unknown, the stack settings could not be read",
		textPropGrantPush:          "!! An update started with this grant commits and pushes to the source of the stack.",
		textKeyGrantExpiry:         "expiry",
		textTitleGrantExpiry:       "Expiry of the grant",
		textFieldGrantExpires:      "expires",
		textHintGrantExpires:       "last day the grant holds, as YYYY-MM-DD; empty keeps the expiry asked for",
		textProblemGrantExpires:    "Enter a date as YYYY-MM-DD, or leave the field empty.",
		textProblemGrantExpiresMax: "Enter a date as YYYY-MM-DD that is not later than %s, or leave the field empty.",
		textAskProposalGrant:       "Approve proposal %s and add this grant?",
		textWhatProposalGrantSudo: "Yes runs the command shown. It marks the proposal as approved and then adds the grant, the way qhost grant add adds it. " +
			"It refuses and applies nothing if the proposal or the grant is not the one you see. The grant holds for the user and every agent under that account.",
		textDonePropGrant:         "The grant %s for %s is added.",
		textDonePropGrantReplaced: "It replaced the expiry of an equal grant.",
	}
}

func proposalGrantGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPropKindGrant:          "Dauerfreigabe",
		textPropGrantSentence:      "%s (UID %d) darf ohne sudo %s, als dauerhafte Freigabe.",
		textPropGrantRestart:       "den Stack %s neu starten",
		textPropGrantContainer:     "Container des Stacks %s neu starten",
		textPropGrantCheckUpdates:  "den Stack %s auf Updates prüfen",
		textPropGrantUpdate:        "den Stack %s aktualisieren",
		textPropGrantAsked:         "angefragter Ablauf: %s",
		textPropGrantExpires:       "Ablauf der Freigabe: %s",
		textPropGrantSet:           "Du hast diesen Ablauf gesetzt, er ist nicht der angefragte.",
		textPropGrantNever:         "nie",
		textPropGrantUntil:         "nach Ende von %s (UTC)",
		textPropGrantBackup:        "Backup des Stacks: %s",
		textPropGrantCheck:         "Prüfung des Stacks: %s",
		textPropGrantSetUp:         "eingerichtet",
		textPropGrantNotSetUp:      "nicht eingerichtet",
		textPropGrantUnknown:       "unbekannt, die Stack-Einstellungen ließen sich nicht lesen",
		textPropGrantPush:          "!! Ein Update, das mit dieser Freigabe startet, committet und pusht in die Quelle des Stacks.",
		textKeyGrantExpiry:         "Ablauf",
		textTitleGrantExpiry:       "Ablauf der Freigabe",
		textFieldGrantExpires:      "läuft ab",
		textHintGrantExpires:       "letzter Tag der Freigabe als JJJJ-MM-TT; leer behält den angefragten Ablauf",
		textProblemGrantExpires:    "Gib ein Datum als JJJJ-MM-TT ein oder lass das Feld leer.",
		textProblemGrantExpiresMax: "Gib ein Datum als JJJJ-MM-TT ein, das nicht nach %s liegt, oder lass das Feld leer.",
		textAskProposalGrant:       "Vorschlag %s freigeben und diese Freigabe anlegen?",
		textWhatProposalGrantSudo: "Ja führt den gezeigten Befehl aus. Er markiert den Vorschlag als freigegeben und legt dann die Freigabe an, so wie qhost grant add sie anlegt. " +
			"Er lehnt ab und wendet nichts an, wenn der Vorschlag oder die Freigabe nicht die gezeigte ist. Die Freigabe gilt für den Benutzer und jeden Agenten unter diesem Konto.",
		textDonePropGrant:         "Die Freigabe %s für %s ist angelegt.",
		textDonePropGrantReplaced: "Sie hat den Ablauf einer gleichen Freigabe ersetzt.",
	}
}
