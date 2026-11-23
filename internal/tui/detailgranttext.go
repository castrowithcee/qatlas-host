package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the Grants section of the stack detail.
const (
	textDetailGrants        kittui.TextID = "detail.grants"
	textGrantsNote          kittui.TextID = "grants.note"
	textGrantsNone          kittui.TextID = "grants.none"
	textGrantsProblem       kittui.TextID = "grants.problem"
	textGrantsNoUsers       kittui.TextID = "grants.nousers"
	textGrantsNoneToRemove  kittui.TextID = "grants.nonetoremove"
	textGrantNoExpiry       kittui.TextID = "grant.noexpiry"
	textGrantUntil          kittui.TextID = "grant.until"
	textGrantLeadNever      kittui.TextID = "grant.leadnever"
	textGrantLeadUntil      kittui.TextID = "grant.leaduntil"
	textGrantValid          kittui.TextID = "grant.valid"
	textGrantExpired        kittui.TextID = "grant.expired"
	textGrantStackMissing   kittui.TextID = "grant.stackmissing"
	textGrantUserChanged    kittui.TextID = "grant.userchanged"
	textGrantActRestart     kittui.TextID = "grant.act.restart"
	textGrantActContainer   kittui.TextID = "grant.act.container"
	textGrantActCheck       kittui.TextID = "grant.act.check"
	textGrantActUpdate      kittui.TextID = "grant.act.update"
	textGrantSetupBoth      kittui.TextID = "grant.setup.both"
	textGrantSetupBackup    kittui.TextID = "grant.setup.backup"
	textGrantSetupCheck     kittui.TextID = "grant.setup.check"
	textGrantSetupNone      kittui.TextID = "grant.setup.none"
	textGrantUpdatePush     kittui.TextID = "grant.updatepush"
	textKeyGrantAdd         kittui.TextID = "key.grantadd"
	textKeyGrantRemove      kittui.TextID = "key.grantremove"
	textTitleGrant          kittui.TextID = "title.grant"
	textTitleGrantRemove    kittui.TextID = "title.grantremove"
	textFieldGrantUser      kittui.TextID = "field.grantuser"
	textHintGrantUser       kittui.TextID = "hint.grantuser"
	textFieldGrantAction    kittui.TextID = "field.grantaction"
	textHintGrantAction     kittui.TextID = "hint.grantaction"
	textFieldGrantExpires   kittui.TextID = "field.grantexpires"
	textHintGrantExpires    kittui.TextID = "hint.grantexpires"
	textProblemGrantExpires kittui.TextID = "problem.grantexpires"
	textAskGrantAdd         kittui.TextID = "ask.grantadd"
	textAskGrantRemove      kittui.TextID = "ask.grantremove"
	textWhatGrantRemove     kittui.TextID = "what.grantremove"
	textDoneGrantAdded      kittui.TextID = "done.grantadded"
	textDoneGrantReplaced   kittui.TextID = "done.grantreplaced"
	textDoneGrantRemoved    kittui.TextID = "done.grantremoved"
)

func detailGrantEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textDetailGrants:        "Grants",
		textGrantsNote:          "A grant applies to the user and to every agent working under the account of the user.",
		textGrantsNone:          "No grants for this stack.",
		textGrantsProblem:       "The grants cannot be read, so none of them counts: %s",
		textGrantsNoUsers:       "No user has access to qhost. Give a user access under System, Users.",
		textGrantsNoneToRemove:  "This stack has no grant to withdraw.",
		textGrantNoExpiry:       "no expiry",
		textGrantUntil:          "until %s",
		textGrantLeadNever:      "The grant does not expire.",
		textGrantLeadUntil:      "The grant holds until the end of %s (UTC).",
		textGrantValid:          "valid",
		textGrantExpired:        "invalid: expired",
		textGrantStackMissing:   "invalid: the stack is gone",
		textGrantUserChanged:    "invalid: the account changed",
		textGrantActRestart:     "restart the stack",
		textGrantActContainer:   "restart a container",
		textGrantActCheck:       "check for updates",
		textGrantActUpdate:      "update the stack",
		textGrantSetupBoth:      "The stack has a backup and a check set up.",
		textGrantSetupBackup:    "The stack has a backup but no check set up.",
		textGrantSetupCheck:     "The stack has a check but no backup set up.",
		textGrantSetupNone:      "The stack has neither a backup nor a check set up.",
		textGrantUpdatePush:     "An update started with this grant commits and pushes to the source of the stack.",
		textKeyGrantAdd:         "grant",
		textKeyGrantRemove:      "withdraw grant",
		textTitleGrant:          "Grant an action",
		textTitleGrantRemove:    "Withdraw a grant",
		textFieldGrantUser:      "User",
		textHintGrantUser:       "Users with access to qhost.",
		textFieldGrantAction:    "Action",
		textHintGrantAction:     "What the user may do for this stack without sudo.",
		textFieldGrantExpires:   "Last day",
		textHintGrantExpires:    "YYYY-MM-DD, valid until the end of that day (UTC). Empty: no expiry.",
		textProblemGrantExpires: "Give the day as YYYY-MM-DD, or leave it empty.",
		textAskGrantAdd:         "Allow %s to %s (%s) without sudo?",
		textAskGrantRemove:      "Withdraw the grant of %s to %s (%s)?",
		textWhatGrantRemove:     "The user and every agent under the account need sudo for this again.",
		textDoneGrantAdded:      "%s may now %s (%s) without sudo.",
		textDoneGrantReplaced:   "The expiry of the grant of %s to %s (%s) was replaced.",
		textDoneGrantRemoved:    "The grant of %s to %s (%s) was withdrawn.",
	}
}

func detailGrantGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textDetailGrants:        "Freigaben",
		textGrantsNote:          "Eine Freigabe gilt für den Benutzer und für jeden Agenten, der unter seinem Konto arbeitet.",
		textGrantsNone:          "Keine Freigaben für diesen Stack.",
		textGrantsProblem:       "Die Freigaben sind nicht lesbar, deshalb zählt keine: %s",
		textGrantsNoUsers:       "Kein Benutzer hat Zugang zu qhost. Gib einem Benutzer Zugang unter System, Benutzer.",
		textGrantsNoneToRemove:  "Dieser Stack hat keine Freigabe zum Entziehen.",
		textGrantNoExpiry:       "ohne Ablauf",
		textGrantUntil:          "bis %s",
		textGrantLeadNever:      "Die Freigabe läuft nicht ab.",
		textGrantLeadUntil:      "Die Freigabe gilt bis zum Ende von %s (UTC).",
		textGrantValid:          "gültig",
		textGrantExpired:        "ungültig: abgelaufen",
		textGrantStackMissing:   "ungültig: der Stack fehlt",
		textGrantUserChanged:    "ungültig: Konto geändert",
		textGrantActRestart:     "Stack neu starten",
		textGrantActContainer:   "Container neu starten",
		textGrantActCheck:       "Updates prüfen",
		textGrantActUpdate:      "Stack aktualisieren",
		textGrantSetupBoth:      "Der Stack hat Backup und Prüfung eingerichtet.",
		textGrantSetupBackup:    "Der Stack hat ein Backup, aber keine Prüfung eingerichtet.",
		textGrantSetupCheck:     "Der Stack hat eine Prüfung, aber kein Backup eingerichtet.",
		textGrantSetupNone:      "Der Stack hat weder Backup noch Prüfung eingerichtet.",
		textGrantUpdatePush:     "Ein Update mit dieser Freigabe committet und pusht in die Quelle des Stacks.",
		textKeyGrantAdd:         "freigeben",
		textKeyGrantRemove:      "Freigabe entziehen",
		textTitleGrant:          "Aktion freigeben",
		textTitleGrantRemove:    "Freigabe entziehen",
		textFieldGrantUser:      "Benutzer",
		textHintGrantUser:       "Benutzer mit Zugang zu qhost.",
		textFieldGrantAction:    "Aktion",
		textHintGrantAction:     "Was der Benutzer für diesen Stack ohne sudo tun darf.",
		textFieldGrantExpires:   "Letzter Tag",
		textHintGrantExpires:    "JJJJ-MM-TT, gültig bis zum Ende dieses Tages (UTC). Leer: kein Ablauf.",
		textProblemGrantExpires: "Gib den Tag als JJJJ-MM-TT an oder lass das Feld leer.",
		textAskGrantAdd:         "%s erlauben: %s (%s), ohne sudo?",
		textAskGrantRemove:      "Die Freigabe von %s entziehen: %s (%s)?",
		textWhatGrantRemove:     "Der Benutzer und jeder Agent unter seinem Konto brauchen dafür wieder sudo.",
		textDoneGrantAdded:      "%s darf jetzt ohne sudo: %s (%s).",
		textDoneGrantReplaced:   "Der Ablauf der Freigabe von %s für %s (%s) wurde ersetzt.",
		textDoneGrantRemoved:    "Die Freigabe von %s für %s (%s) wurde entzogen.",
	}
}
