package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the Proposals area. What a submitter wrote never becomes part of a catalog text; it comes in
// as an argument and is escaped where it is drawn.
const (
	textAreaProposals kittui.TextID = "area.proposals"

	textColKind      kittui.TextID = "col.kind"
	textColSubmitter kittui.TextID = "col.submitter"
	textColSubmitted kittui.TextID = "col.submitted"
	textColExpires   kittui.TextID = "col.expires"

	textPropEmpty     kittui.TextID = "prop.empty"
	textPropLoading   kittui.TextID = "prop.loading"
	textPropNotice    kittui.TextID = "prop.notice"
	textPropNoticeOne kittui.TextID = "prop.notice.one"

	textPropKindSourceAdd kittui.TextID = "prop.kind.sourceadd"
	textPropOpen          kittui.TextID = "prop.state.open"
	textPropApproving     kittui.TextID = "prop.state.approving"
	textPropApplied       kittui.TextID = "prop.state.applied"
	textPropFailed        kittui.TextID = "prop.state.failed"
	textPropRejected      kittui.TextID = "prop.state.rejected"
	textPropWithdrawn     kittui.TextID = "prop.state.withdrawn"
	textPropExpired       kittui.TextID = "prop.state.expired"

	textPropFieldSubmitter kittui.TextID = "prop.field.submitter"
	textPropFieldSubmitted kittui.TextID = "prop.field.submitted"
	textPropFieldExpires   kittui.TextID = "prop.field.expires"
	textPropFieldState     kittui.TextID = "prop.field.state"
	textPropMessage        kittui.TextID = "prop.message"
	textPropReason         kittui.TextID = "prop.reason"
	textPropFailure        kittui.TextID = "prop.failure"
	textPropHeadContent    kittui.TextID = "prop.head.content"
	textPropHeadEffect     kittui.TextID = "prop.head.effect"
	textPropPath           kittui.TextID = "prop.path"
	textPropAddress        kittui.TextID = "prop.address"
	textPropNoAddress      kittui.TextID = "prop.noaddress"
	textPropKindUnknown    kittui.TextID = "prop.kind.unknown"
	textPropContentBad     kittui.TextID = "prop.contentbad"
	textPropNeedsSudo      kittui.TextID = "prop.needssudo"
	textPropEffectFailed   kittui.TextID = "prop.effectfailed"
)

func proposalEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textAreaProposals: "Proposals",

		textColKind:      "kind",
		textColSubmitter: "from",
		textColSubmitted: "submitted",
		textColExpires:   "expires",

		textPropEmpty:     "No proposals.",
		textPropLoading:   "Reading the proposals…",
		textPropNotice:    "%d open proposals wait for you. Press %s to review them.",
		textPropNoticeOne: "%d open proposal waits for you. Press %s to review it.",

		textPropKindSourceAdd: "stack folder",
		textPropOpen:          "open",
		textPropApproving:     "approving",
		textPropApplied:       "applied",
		textPropFailed:        "failed",
		textPropRejected:      "rejected",
		textPropWithdrawn:     "withdrawn",
		textPropExpired:       "expired",

		textPropFieldSubmitter: "from",
		textPropFieldSubmitted: "submitted",
		textPropFieldExpires:   "expires",
		textPropFieldState:     "state",
		textPropMessage:        "note: %s",
		textPropReason:         "reason: %s",
		textPropFailure:        "failed: %s",
		textPropHeadContent:    "Proposed",
		textPropHeadEffect:     "Effect if applied now",
		textPropPath:           "path: %s",
		textPropAddress:        "address: %s",
		textPropNoAddress:      "none proposed",
		textPropKindUnknown:    "This version cannot show or apply this kind of proposal.",
		textPropContentBad:     "The content of the proposal cannot be read.",
		textPropNeedsSudo:      "The effect and approving a proposal need sudo. Run qhost with sudo to see what applying it changes.",
		textPropEffectFailed:   "The effect could not be worked out: %s",
	}
}

func proposalGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textAreaProposals: "Vorschläge",

		textColKind:      "Art",
		textColSubmitter: "von",
		textColSubmitted: "eingereicht",
		textColExpires:   "läuft ab",

		textPropEmpty:     "Keine Vorschläge.",
		textPropLoading:   "Vorschläge werden gelesen…",
		textPropNotice:    "%d offene Vorschläge warten auf dich. Mit %s prüfst du sie.",
		textPropNoticeOne: "%d offener Vorschlag wartet auf dich. Mit %s prüfst du ihn.",

		textPropKindSourceAdd: "Stack-Ordner",
		textPropOpen:          "offen",
		textPropApproving:     "wird freigegeben",
		textPropApplied:       "angewendet",
		textPropFailed:        "gescheitert",
		textPropRejected:      "abgelehnt",
		textPropWithdrawn:     "zurückgezogen",
		textPropExpired:       "abgelaufen",

		textPropFieldSubmitter: "von",
		textPropFieldSubmitted: "eingereicht",
		textPropFieldExpires:   "läuft ab",
		textPropFieldState:     "Zustand",
		textPropMessage:        "Hinweis: %s",
		textPropReason:         "Grund: %s",
		textPropFailure:        "gescheitert: %s",
		textPropHeadContent:    "Vorgeschlagen",
		textPropHeadEffect:     "Wirkung bei Anwendung jetzt",
		textPropPath:           "Pfad: %s",
		textPropAddress:        "Adresse: %s",
		textPropNoAddress:      "keine vorgeschlagen",
		textPropKindUnknown:    "Diese Version kann diese Art von Vorschlag weder anzeigen noch anwenden.",
		textPropContentBad:     "Der Inhalt des Vorschlags ist nicht lesbar.",
		textPropNeedsSudo:      "Die Wirkung und die Freigabe eines Vorschlags brauchen sudo. Starte qhost mit sudo, um zu sehen, was die Anwendung ändert.",
		textPropEffectFailed:   "Die Wirkung konnte nicht berechnet werden: %s",
	}
}

// The texts of approving and rejecting a proposal.
const (
	textKeyApprove kittui.TextID = "key.propapprove"
	textKeyReject  kittui.TextID = "key.propreject"

	textTitleProposalApprove kittui.TextID = "title.propapprove"
	textTitleProposalReject  kittui.TextID = "title.propreject"
	textAskProposalApprove   kittui.TextID = "ask.propapprove"
	textWhatProposalApprove  kittui.TextID = "what.propapprove"
	textAskProposalReject    kittui.TextID = "ask.propreject"
	textWhatProposalReject   kittui.TextID = "what.propreject"
	textFieldPropReason      kittui.TextID = "field.propreason"
	textHintPropReason       kittui.TextID = "hint.propreason"
	textProblemPropReason    kittui.TextID = "problem.propreason"
	textDonePropRejected     kittui.TextID = "done.proprejected"
	textPropChanged          kittui.TextID = "prop.changed"
	textPropNotOpen          kittui.TextID = "prop.notopen"
	textPropCannotApprove    kittui.TextID = "prop.cannotapprove"
	textPropResultLost       kittui.TextID = "prop.resultlost"
)

func proposalDecideEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyApprove: "approve",
		textKeyReject:  "reject",

		textTitleProposalApprove: "Approve a proposal",
		textTitleProposalReject:  "Reject a proposal",
		textAskProposalApprove:   "Approve proposal %s and apply these changes to %s?",
		textWhatProposalApprove: "Yes marks the proposal as approved and then makes the changes shown, the way adding the folder by hand makes them. " +
			"Right before that the proposal and its effect are read again, and nothing is applied if either differs from what you saw. " +
			"The result is kept on the proposal.",
		textAskProposalReject:  "Reject proposal %s?",
		textWhatProposalReject: "The proposal is closed as rejected and nothing on the host changes. The reason, if you give one, is shown to the submitter.",
		textFieldPropReason:    "reason",
		textHintPropReason:     "optional, at most 300 characters; the submitter reads it with qhost proposal show",
		textProblemPropReason:  "The reason has at most 300 characters and no control characters.",
		textDonePropRejected:   "Proposal %s is rejected.",
		textPropChanged:        "The proposal or its effect changed since it was shown; nothing was applied. The view shows it as it is now.",
		textPropNotOpen:        "This proposal is no longer open.",
		textPropCannotApprove:  "This version cannot approve this proposal.",
		textPropResultLost:     "The result could not be kept on the proposal: %s. Look at its state in the view.",
	}
}

func proposalDecideGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textKeyApprove: "freigeben",
		textKeyReject:  "ablehnen",

		textTitleProposalApprove: "Vorschlag freigeben",
		textTitleProposalReject:  "Vorschlag ablehnen",
		textAskProposalApprove:   "Vorschlag %s freigeben und diese Änderungen an %s anwenden?",
		textWhatProposalApprove: "Ja markiert den Vorschlag als freigegeben und nimmt dann die gezeigten Änderungen vor, so wie das Hinzufügen des Ordners von Hand. " +
			"Direkt davor werden der Vorschlag und seine Wirkung neu gelesen, und nichts wird angewendet, wenn eines von beiden vom Gezeigten abweicht. " +
			"Das Ergebnis bleibt am Vorschlag.",
		textAskProposalReject:  "Vorschlag %s ablehnen?",
		textWhatProposalReject: "Der Vorschlag wird als abgelehnt geschlossen, am Host ändert sich nichts. Der Grund, falls du einen nennst, wird dem Einreicher gezeigt.",
		textFieldPropReason:    "Grund",
		textHintPropReason:     "optional, höchstens 300 Zeichen; der Einreicher liest ihn mit qhost proposal show",
		textProblemPropReason:  "Der Grund hat höchstens 300 Zeichen und keine Steuerzeichen.",
		textDonePropRejected:   "Vorschlag %s ist abgelehnt.",
		textPropChanged:        "Der Vorschlag oder seine Wirkung hat sich seit der Anzeige geändert, nichts wurde angewendet. Die Ansicht zeigt den jetzigen Stand.",
		textPropNotOpen:        "Dieser Vorschlag ist nicht mehr offen.",
		textPropCannotApprove:  "Diese Version kann diesen Vorschlag nicht freigeben.",
		textPropResultLost:     "Das Ergebnis konnte nicht am Vorschlag festgehalten werden: %s. Sieh den Zustand in der Ansicht an.",
	}
}
