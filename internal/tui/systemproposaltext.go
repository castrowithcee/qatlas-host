package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of System > Proposals. The sentence and the commands of a proposal are what the host administration
// reports; they come in as arguments in English.
const (
	textPageProposals kittui.TextID = "system.page.proposals"
	textColProposal   kittui.TextID = "col.proposal"

	textHostPropReading   kittui.TextID = "hostprop.reading"
	textHostPropEmpty     kittui.TextID = "hostprop.empty"
	textHostPropNoChannel kittui.TextID = "hostprop.nochannel"
	textHostPropFailed    kittui.TextID = "hostprop.failed"
	textHostPropStale     kittui.TextID = "hostprop.stale"
	textHostPropQuestion  kittui.TextID = "hostprop.question"
	textTitleHostProposal kittui.TextID = "title.hostproposal"

	textHostPropHeadChange   kittui.TextID = "hostprop.head.change"
	textHostPropHeadCommands kittui.TextID = "hostprop.head.commands"
	textHostPropHeadKeys     kittui.TextID = "hostprop.head.keys"
	textHostPropKey          kittui.TextID = "hostprop.key"
	textHostPropBad          kittui.TextID = "hostprop.bad"
	textHostPropPassword     kittui.TextID = "hostprop.password"

	textRiskSetDocker kittui.TextID = "host.risksetdocker"
	textRiskEnable    kittui.TextID = "host.riskenable"

	textTitleHostPropPassword kittui.TextID = "title.hostproppassword"
	textAskHostProposal       kittui.TextID = "ask.hostproposal"
	textWhatHostPropReject    kittui.TextID = "what.hostpropreject"
	textHostPropRejected      kittui.TextID = "hostprop.rejected"
	textHostPropWhat          kittui.TextID = "hostprop.what"
	textHostPropNotApprovable kittui.TextID = "hostprop.notapprovable"
)

func proposalHostEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPageProposals: "Proposals",
		textColProposal:   "proposal",

		textHostPropReading:   "Reading the proposals of the host service...",
		textHostPropEmpty:     "No administrator proposed a change to the host.",
		textHostPropNoChannel: "The proposals cannot be read here: the interface does not reach the host service.",
		textHostPropFailed:    "The proposals cannot be read: %s",
		textHostPropStale:     "The host or a proposal changed since the list was read. Press {reload} to read it again.",
		textHostPropQuestion:  "Proposal %s from %s",
		textTitleHostProposal: "Proposal of a change to the host",

		textHostPropHeadChange:   "The change",
		textHostPropHeadCommands: "Commands, as typed by hand",
		textHostPropHeadKeys:     "Proposed SSH keys",
		textHostPropKey:          "%s, comment: %s, fingerprint: %s",
		textHostPropBad:          "This proposal cannot be shown or applied: %s",
		textHostPropPassword:     "The initial password is never part of the proposal: you enter it when you approve.",

		textRiskSetDocker: "The Docker right means full control over this host.",
		textRiskEnable:    "Enabling gives this administrator access to qhost, which controls the host.",

		textTitleHostPropPassword: "Initial password",
		textAskHostProposal:       "Apply proposal %s to the host: %s",
		textWhatHostPropReject:    "The proposal is closed as rejected and nothing on the host changes. This runs with sudo.",
		textHostPropRejected:      "Proposal %s is rejected.",
		textHostPropWhat:          "Yes goes on to the question that shows the commands. Nothing is applied before you answer that one.",
		textHostPropNotApprovable: "This proposal cannot be approved: it is not open or does not pass the check.",
	}
}

func proposalHostGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPageProposals: "Vorschläge",
		textColProposal:   "Vorschlag",

		textHostPropReading:   "Die Vorschläge des Host-Dienstes werden gelesen...",
		textHostPropEmpty:     "Kein Administrator hat eine Änderung am Host vorgeschlagen.",
		textHostPropNoChannel: "Die Vorschläge lassen sich hier nicht lesen: Die Oberfläche erreicht den Host-Dienst nicht.",
		textHostPropFailed:    "Die Vorschläge lassen sich nicht lesen: %s",
		textHostPropStale:     "Der Host oder ein Vorschlag hat sich geändert, seit die Liste gelesen wurde. Drücke {reload}, um sie neu zu lesen.",
		textHostPropQuestion:  "Vorschlag %s von %s",
		textTitleHostProposal: "Vorschlag einer Änderung am Host",

		textHostPropHeadChange:   "Die Änderung",
		textHostPropHeadCommands: "Befehle, wie von Hand eingegeben",
		textHostPropHeadKeys:     "Vorgeschlagene SSH-Schlüssel",
		textHostPropKey:          "%s, Kommentar: %s, Fingerprint: %s",
		textHostPropBad:          "Dieser Vorschlag lässt sich nicht anzeigen oder anwenden: %s",
		textHostPropPassword:     "Das Startpasswort ist nie Teil des Vorschlags: Du gibst es bei der Freigabe ein.",

		textRiskSetDocker: "Das Docker-Recht bedeutet volle Kontrolle über diesen Host.",
		textRiskEnable:    "Das Freischalten gibt diesem Administrator Zugang zu qhost, das den Host steuert.",

		textTitleHostPropPassword: "Startpasswort",
		textAskHostProposal:       "Vorschlag %s auf den Host anwenden: %s",
		textWhatHostPropReject:    "Der Vorschlag wird als abgelehnt geschlossen und auf dem Host ändert sich nichts. Das läuft mit sudo.",
		textHostPropRejected:      "Vorschlag %s ist abgelehnt.",
		textHostPropWhat:          "Ja führt zur Rückfrage mit den Befehlen. Vorher wird nichts angewendet.",
		textHostPropNotApprovable: "Dieser Vorschlag lässt sich nicht freigeben: Er ist nicht offen oder besteht die Prüfung nicht.",
	}
}
