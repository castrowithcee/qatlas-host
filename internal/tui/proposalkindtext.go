package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the proposal kinds stack-settings and window.
const (
	textPropKindStackSettings    kittui.TextID = "prop.kind.stacksettings"
	textPropKindWindow           kittui.TextID = "prop.kind.window"
	textPropStack                kittui.TextID = "prop.stack"
	textPropBackupCommand        kittui.TextID = "prop.backupcommand"
	textPropSettingsNew          kittui.TextID = "prop.settings.new"
	textPropSettingsChanges      kittui.TextID = "prop.settings.changes"
	textPropSettingsSame         kittui.TextID = "prop.settings.same"
	textPropWindowChanges        kittui.TextID = "prop.window.changes"
	textPropWindowSame           kittui.TextID = "prop.window.same"
	textPropWindowChangesHost    kittui.TextID = "prop.window.changes.host"
	textPropWindowSameHost       kittui.TextID = "prop.window.same.host"
	textPropBefore               kittui.TextID = "prop.before"
	textPropAfter                kittui.TextID = "prop.after"
	textPropNoStack              kittui.TextID = "prop.nostack"
	textAskProposalSettings      kittui.TextID = "ask.propsettings"
	textAskProposalWindow        kittui.TextID = "ask.propwindow"
	textWhatProposalSettings     kittui.TextID = "what.propsettings"
	textWhatProposalWindow       kittui.TextID = "what.propwindow"
	textDonePropSettings         kittui.TextID = "done.propsettings"
	textDonePropSettingsSame     kittui.TextID = "done.propsettingssame"
	textPropKindChannel          kittui.TextID = "prop.kind.channel"
	textPropChannelNew           kittui.TextID = "prop.channel.new"
	textPropChannelTaken         kittui.TextID = "prop.channel.taken"
	textAskProposalChannel       kittui.TextID = "ask.propchannel"
	textWhatProposalChannel      kittui.TextID = "what.propchannel"
	textDonePropChannel          kittui.TextID = "done.propchannel"
	textTitleProposalSecrets     kittui.TextID = "title.propsecrets"
	textWhatProposalSettingsSudo kittui.TextID = "what.propsettings.sudo"
	textWhatProposalWindowSudo   kittui.TextID = "what.propwindow.sudo"
	textWhatProposalChannelSudo  kittui.TextID = "what.propchannel.sudo"
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
		textPropWindowChanges:     "The own maintenance window of this instance would change:",
		textPropWindowChangesHost: "The maintenance window of the host would change:",
		textPropWindowSameHost:    "The window equals the host window in force. Nothing would change.",
		textPropWindowSame:        "The window equals the own window of this instance in force. Nothing would change.",
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
		textPropKindChannel:    "notification channel",
		textPropChannelNew:     "The channel would be added; no channel of this name exists now. The proposal holds no secret parts: you enter them on approving.",
		textPropChannelTaken:   "A channel named %s exists now. The proposal cannot be approved; reject it, or remove the channel first.",
		textAskProposalChannel: "Approve proposal %s and add the channel %s (%s)?",
		textWhatProposalChannel: "Yes marks the proposal as approved and then adds the channel with the secret parts you entered, the way the channel form and qhost notify add add it. " +
			"Right before that the proposal and the channels are read again, and nothing is applied if either differs from what you saw. " +
			"The secret parts are never kept on the proposal.",
		textWhatProposalSettingsSudo: "Yes runs the command shown. It marks the proposal as approved and then saves the settings shown, the way qhost stack settings set saves them. " +
			"It refuses and applies nothing if the proposal is not the one you see. The result is kept on the proposal.",
		textWhatProposalWindowSudo: "Yes runs the command shown. It marks the proposal as approved and then sets the window shown, the way qhost window set sets it. " +
			"It refuses and applies nothing if the proposal is not the one you see. The result is kept on the proposal.",
		textWhatProposalChannelSudo: "Yes runs the command shown. It marks the proposal as approved and then adds the channel with the secret parts you entered, the way qhost notify add adds it. " +
			"The secret parts go to the command on standard input only, never into the command line, and are never kept on the proposal. It refuses and applies nothing if the proposal is not the one you see.",
		textDonePropChannel:      "The channel %s is added.",
		textTitleProposalSecrets: "Secret parts of the channel",
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
		textPropWindowChanges:     "Das eigene Wartungsfenster dieser Instanz würde sich ändern:",
		textPropWindowChangesHost: "Das Wartungsfenster des Hosts würde sich ändern:",
		textPropWindowSameHost:    "Das Fenster entspricht dem geltenden Host-Fenster. Nichts würde sich ändern.",
		textPropWindowSame:        "Das Fenster entspricht dem geltenden eigenen Fenster dieser Instanz. Nichts würde sich ändern.",
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
		textPropKindChannel:    "Benachrichtigungskanal",
		textPropChannelNew:     "Der Kanal würde angelegt, einen Kanal dieses Namens gibt es jetzt nicht. Der Vorschlag enthält keine geheimen Teile: Du gibst sie bei der Freigabe ein.",
		textPropChannelTaken:   "Einen Kanal namens %s gibt es jetzt. Der Vorschlag kann nicht freigegeben werden, lehne ihn ab oder entferne zuerst den Kanal.",
		textAskProposalChannel: "Vorschlag %s freigeben und den Kanal %s (%s) anlegen?",
		textWhatProposalChannel: "Ja markiert den Vorschlag als freigegeben und legt dann den Kanal mit den eingegebenen geheimen Teilen an, so wie das Kanalformular und qhost notify add ihn anlegen. " +
			"Direkt davor werden der Vorschlag und die Kanäle neu gelesen, und nichts wird angewendet, wenn eines von beiden vom Gezeigten abweicht. " +
			"Die geheimen Teile bleiben nie am Vorschlag.",
		textWhatProposalSettingsSudo: "Ja führt den gezeigten Befehl aus. Er markiert den Vorschlag als freigegeben und speichert dann die gezeigten Einstellungen, so wie qhost stack settings set sie speichert. " +
			"Er lehnt ab und wendet nichts an, wenn der Vorschlag nicht der gezeigte ist. Das Ergebnis bleibt am Vorschlag.",
		textWhatProposalWindowSudo: "Ja führt den gezeigten Befehl aus. Er markiert den Vorschlag als freigegeben und setzt dann das gezeigte Fenster, so wie qhost window set es setzt. " +
			"Er lehnt ab und wendet nichts an, wenn der Vorschlag nicht der gezeigte ist. Das Ergebnis bleibt am Vorschlag.",
		textWhatProposalChannelSudo: "Ja führt den gezeigten Befehl aus. Er markiert den Vorschlag als freigegeben und legt dann den Kanal mit den eingegebenen geheimen Teilen an, so wie qhost notify add ihn anlegt. " +
			"Die geheimen Teile gehen nur über die Standardeingabe an den Befehl, nie in die Befehlszeile, und bleiben nie am Vorschlag. Er lehnt ab und wendet nichts an, wenn der Vorschlag nicht der gezeigte ist.",
		textDonePropChannel:      "Der Kanal %s ist angelegt.",
		textTitleProposalSecrets: "Geheime Teile des Kanals",
		textDonePropSettings:     "Die Einstellungen des Stacks %s sind gespeichert.",
		textDonePropSettingsSame: "Der Stack %s hatte diese Einstellungen schon.",
	}
}
