package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the proposal kinds stack-settings and window.
const (
	textPropKindStackSettings kittui.TextID = "prop.kind.stacksettings"
	textPropKindWindow        kittui.TextID = "prop.kind.window"
	textPropStack             kittui.TextID = "prop.stack"
	textPropBackupCommand     kittui.TextID = "prop.backupcommand"
	textPropSettingsNew       kittui.TextID = "prop.settings.new"
	textPropSettingsChanges   kittui.TextID = "prop.settings.changes"
	textPropSettingsSame      kittui.TextID = "prop.settings.same"
	textPropWindowChanges     kittui.TextID = "prop.window.changes"
	textPropWindowSame        kittui.TextID = "prop.window.same"
	textPropBefore            kittui.TextID = "prop.before"
	textPropAfter             kittui.TextID = "prop.after"
	textPropNoStack           kittui.TextID = "prop.nostack"
	textAskProposalSettings   kittui.TextID = "ask.propsettings"
	textAskProposalWindow     kittui.TextID = "ask.propwindow"
	textWhatProposalSettings  kittui.TextID = "what.propsettings"
	textWhatProposalWindow    kittui.TextID = "what.propwindow"
	textDonePropSettings      kittui.TextID = "done.propsettings"
	textDonePropSettingsSame  kittui.TextID = "done.propsettingssame"
)

func proposalKindEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPropKindStackSettings: "stack settings",
		textPropKindWindow:        "maintenance window",
		textPropStack:             "stack: %s",
		textPropBackupCommand:     "!! The backup command runs on the host later, before each update. Read every word of it.",
		textPropSettingsNew:       "The stack has no settings yet. These would be set:",
		textPropSettingsChanges:   "These settings would change:",
		textPropSettingsSame:      "The settings equal those in force. Nothing would change.",
		textPropWindowChanges:     "The maintenance window would change:",
		textPropWindowSame:        "The window equals the one in force. Nothing would change.",
		textPropBefore:            "before: %s",
		textPropAfter:             "after:  %s",
		textPropNoStack:           "The stack %s does not exist. The proposal cannot be approved; reject it.",
		textAskProposalSettings:   "Approve proposal %s and apply these settings to the stack %s?",
		textAskProposalWindow:     "Approve proposal %s and set this maintenance window?",
		textWhatProposalSettings: "Yes marks the proposal as approved and then saves the settings shown, the way the settings forms and qhost stack settings save them. " +
			"Right before that the proposal and the settings in force are read again, and nothing is applied if either differs from what you saw. " +
			"The result is kept on the proposal.",
		textWhatProposalWindow: "Yes marks the proposal as approved and then sets the window shown, the way the window form and qhost window set it. " +
			"Right before that the proposal and the window in force are read again, and nothing is applied if either differs from what you saw. " +
			"The result is kept on the proposal.",
		textDonePropSettings:     "The settings of the stack %s are saved.",
		textDonePropSettingsSame: "The stack %s already had these settings.",
	}
}

func proposalKindGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textPropKindStackSettings: "Stack-Einstellungen",
		textPropKindWindow:        "Wartungsfenster",
		textPropStack:             "Stack: %s",
		textPropBackupCommand:     "!! Der Backup-Befehl läuft später auf dem Host, vor jedem Update. Lies jedes Wort davon.",
		textPropSettingsNew:       "Der Stack hat noch keine Einstellungen. Diese würden gesetzt:",
		textPropSettingsChanges:   "Diese Einstellungen würden sich ändern:",
		textPropSettingsSame:      "Die Einstellungen entsprechen den geltenden. Nichts würde sich ändern.",
		textPropWindowChanges:     "Das Wartungsfenster würde sich ändern:",
		textPropWindowSame:        "Das Fenster entspricht dem geltenden. Nichts würde sich ändern.",
		textPropBefore:            "vorher:  %s",
		textPropAfter:             "nachher: %s",
		textPropNoStack:           "Den Stack %s gibt es nicht. Der Vorschlag kann nicht freigegeben werden, lehne ihn ab.",
		textAskProposalSettings:   "Vorschlag %s freigeben und diese Einstellungen auf den Stack %s anwenden?",
		textAskProposalWindow:     "Vorschlag %s freigeben und dieses Wartungsfenster setzen?",
		textWhatProposalSettings: "Ja markiert den Vorschlag als freigegeben und speichert dann die gezeigten Einstellungen, so wie die Formulare und qhost stack settings sie speichern. " +
			"Direkt davor werden der Vorschlag und die geltenden Einstellungen neu gelesen, und nichts wird angewendet, wenn eines von beiden vom Gezeigten abweicht. " +
			"Das Ergebnis bleibt am Vorschlag.",
		textWhatProposalWindow: "Ja markiert den Vorschlag als freigegeben und setzt dann das gezeigte Fenster, so wie das Fensterformular und qhost window es setzen. " +
			"Direkt davor werden der Vorschlag und das geltende Fenster neu gelesen, und nichts wird angewendet, wenn eines von beiden vom Gezeigten abweicht. " +
			"Das Ergebnis bleibt am Vorschlag.",
		textDonePropSettings:     "Die Einstellungen des Stacks %s sind gespeichert.",
		textDonePropSettingsSame: "Der Stack %s hatte diese Einstellungen schon.",
	}
}
