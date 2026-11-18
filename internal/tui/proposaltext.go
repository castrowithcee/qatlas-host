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

	textPropEmpty   kittui.TextID = "prop.empty"
	textPropLoading kittui.TextID = "prop.loading"
	textPropNotice  kittui.TextID = "prop.notice"

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

		textPropEmpty:   "No proposals.",
		textPropLoading: "Reading the proposals…",
		textPropNotice:  "%d open proposals wait for you. Press %s to review them.",

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

		textPropEmpty:   "Keine Vorschläge.",
		textPropLoading: "Vorschläge werden gelesen…",
		textPropNotice:  "%d offene Vorschläge warten auf dich. Mit %s prüfst du sie.",

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
