package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of proposals of free commands: the switch in Settings > General, the marks in the list, the view, the
// question and the result of running one. What a submitter wrote comes in as an argument and is never a text of
// this catalog.
const (
	textFieldFreeCmds kittui.TextID = "field.freecmds"
	textHintFreeCmds  kittui.TextID = "hint.freecmds"
	textFreeOn        kittui.TextID = "free.on"
	textFreeOff       kittui.TextID = "free.off"

	textTitleFreeSwitch kittui.TextID = "title.freeswitch"
	textAskFreeOn       kittui.TextID = "ask.freeon"
	textAskFreeOff      kittui.TextID = "ask.freeoff"
	textWhatFreeOn      kittui.TextID = "what.freeon"
	textWhatFreeOff     kittui.TextID = "what.freeoff"
	textFreeSwitchedOn  kittui.TextID = "free.switchedon"
	textFreeSwitchedOff kittui.TextID = "free.switchedoff"

	textPropKindHostChange kittui.TextID = "prop.kind.hostchange"
	textPropKindFree       kittui.TextID = "prop.kind.free"
	textColKindHost        kittui.TextID = "col.kindhost"

	textFreeNotice       kittui.TextID = "free.notice"
	textFreeHeadReason   kittui.TextID = "free.head.reason"
	textFreeHeadCommands kittui.TextID = "free.head.commands"
	textFreeSwitchOff    kittui.TextID = "free.switchoff"
	textFreeWhat         kittui.TextID = "free.what"
	textFreeAnswerRun    kittui.TextID = "free.answerrun"
	textFreeKeyRun       kittui.TextID = "free.keyrun"

	textTitleFreeRun      kittui.TextID = "title.freerun"
	textAskFreeRun        kittui.TextID = "ask.freerun"
	textWhatFreeRun       kittui.TextID = "what.freerun"
	textTitleFreeResult   kittui.TextID = "title.freeresult"
	textFreeRunDone       kittui.TextID = "free.rundone"
	textFreeRunDoneNote   kittui.TextID = "free.rundonenote"
	textFreeRunFailed     kittui.TextID = "free.runfailed"
	textFreeRunFailedNote kittui.TextID = "free.runfailednote"
	textFreeExit          kittui.TextID = "free.exit"
	textFreeNotRun        kittui.TextID = "free.notrun"
	textFreeTimedOut      kittui.TextID = "free.timedout"
	textFreeNoOutput      kittui.TextID = "free.nooutput"
	textFreeCut           kittui.TextID = "free.cut"
	textNoUndoFree        kittui.TextID = "journal.noundo.free"
)

func freeEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textFieldFreeCmds: "Free proposals",
		textHintFreeCmds:  "agents may propose commands that qhost does not check and cannot undo",
		textFreeOn:        "on",
		textFreeOff:       "off",

		textTitleFreeSwitch: "Free proposals",
		textAskFreeOn:       "Allow agents to propose free commands?",
		textAskFreeOff:      "Stop accepting proposals of free commands?",
		textWhatFreeOn: "Agents may then propose commands that qhost does not check and cannot undo. Nothing runs before an administrator " +
			"runs a proposal. Runs with sudo.",
		textWhatFreeOff:     "New proposals of free commands are refused and open ones can no longer be run. Runs with sudo.",
		textFreeSwitchedOn:  "Proposals of free commands are on.",
		textFreeSwitchedOff: "Proposals of free commands are off.",

		textPropKindHostChange: "host change",
		textPropKindFree:       "free commands",
		textColKindHost:        "kind",

		textFreeNotice:       "unchecked, no computed effect, no undo",
		textFreeHeadReason:   "Reason of the submitter, as written",
		textFreeHeadCommands: "Commands of the submitter, as written",
		textFreeSwitchOff:    "Running is off: turn on Free proposals under Settings, General.",
		textFreeWhat:         "Yes goes on to the question that names the number of commands. Nothing runs before you answer that one.",
		textFreeAnswerRun:    "run",
		textFreeKeyRun:       "run",

		textTitleFreeRun: "Run free commands",
		textAskFreeRun:   "Run the %d commands of proposal %s as root?",
		textWhatFreeRun: "The submitter wrote them and nothing checks them. They run one after the other, the run stops at the " +
			"first one that fails, and nothing can be undone. Runs with sudo.",
		textTitleFreeResult:   "Result of the commands",
		textFreeRunDone:       "Proposal %s: all %d commands ran with exit code 0.",
		textFreeRunDoneNote:   "Journal entry %s. This cannot be undone.",
		textFreeRunFailed:     "Proposal %s: command %d of %d did not succeed, %d did not run.",
		textFreeRunFailedNote: "Nothing was undone and the commands after the failed one did not run. Journal entry %s.",
		textFreeExit:          "exit code %d",
		textFreeNotRun:        "did not run",
		textFreeTimedOut:      "ran out of time",
		textFreeNoOutput:      "no output",
		textFreeCut:           "The output is cut.",
		textNoUndoFree:        "Free commands are not checked and nothing records what they changed, so this cannot be undone.",
	}
}

func freeGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textFieldFreeCmds: "Freie Vorschläge",
		textHintFreeCmds:  "Agenten dürfen Befehle vorschlagen, die qhost nicht prüft und nicht rückgängig machen kann",
		textFreeOn:        "an",
		textFreeOff:       "aus",

		textTitleFreeSwitch: "Freie Vorschläge",
		textAskFreeOn:       "Agenten erlauben, freie Befehle vorzuschlagen?",
		textAskFreeOff:      "Vorschläge freier Befehle nicht mehr annehmen?",
		textWhatFreeOn: "Agenten dürfen dann Befehle vorschlagen, die qhost nicht prüft und nicht rückgängig machen kann. Nichts läuft, " +
			"bevor ein Administrator einen Vorschlag ausführt. Das läuft mit sudo.",
		textWhatFreeOff:     "Neue Vorschläge freier Befehle werden abgelehnt und offene lassen sich nicht mehr ausführen. Das läuft mit sudo.",
		textFreeSwitchedOn:  "Vorschläge freier Befehle sind an.",
		textFreeSwitchedOff: "Vorschläge freier Befehle sind aus.",

		textPropKindHostChange: "Host-Änderung",
		textPropKindFree:       "freie Befehle",
		textColKindHost:        "Art",

		textFreeNotice:       "ungeprüft, keine berechnete Wirkung, kein Rückgängig",
		textFreeHeadReason:   "Begründung des Einreichers, wie geschrieben",
		textFreeHeadCommands: "Befehle des Einreichers, wie geschrieben",
		textFreeSwitchOff:    "Das Ausführen ist aus: Schalte Freie Vorschläge unter Einstellungen, Allgemein ein.",
		textFreeWhat:         "Ja führt zur Rückfrage mit der Zahl der Befehle. Vorher läuft nichts.",
		textFreeAnswerRun:    "ausführen",
		textFreeKeyRun:       "ausführen",

		textTitleFreeRun: "Freie Befehle ausführen",
		textAskFreeRun:   "Die %d Befehle von Vorschlag %s als root ausführen?",
		textWhatFreeRun: "Der Einreicher hat sie geschrieben und nichts prüft sie. Sie laufen nacheinander, der Lauf endet beim " +
			"ersten, der scheitert, und nichts lässt sich rückgängig machen. Das läuft mit sudo.",
		textTitleFreeResult:   "Ergebnis der Befehle",
		textFreeRunDone:       "Vorschlag %s: Alle %d Befehle liefen mit Exit-Code 0.",
		textFreeRunDoneNote:   "Journal-Eintrag %s. Das lässt sich nicht rückgängig machen.",
		textFreeRunFailed:     "Vorschlag %s: Befehl %d von %d war nicht erfolgreich, %d liefen nicht mehr.",
		textFreeRunFailedNote: "Nichts wurde rückgängig gemacht und die Befehle nach dem gescheiterten liefen nicht. Journal-Eintrag %s.",
		textFreeExit:          "Exit-Code %d",
		textFreeNotRun:        "lief nicht",
		textFreeTimedOut:      "Zeit abgelaufen",
		textFreeNoOutput:      "keine Ausgabe",
		textFreeCut:           "Die Ausgabe ist gekürzt.",
		textNoUndoFree:        "Freie Befehle werden nicht geprüft und nichts hält fest, was sie geändert haben. Das lässt sich nicht rückgängig machen.",
	}
}
