package tui

import (
	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// The texts of the command line that are not per command, and the table of the commands. A name text holds the
// name and then its aliases, separated by "|".
const (
	textCmdOpenArea kittui.TextID = "cmd.openarea"
	textCmdNotHere  kittui.TextID = "cmd.nothere"
	textCmdArea     kittui.TextID = "cmd.area"
)

// commandRow is one command: the action it runs, the area where that action usually acts, and its names and
// description in English and in German.
type commandRow struct {
	id                     kittui.ActionID
	home                   kittui.AreaID
	en, enDesc, de, deDesc string
}

var commandRows = []commandRow{
	{actReload, "", "reload", "Read everything from the service again", "neu laden|reload", "Alles erneut vom Dienst lesen"},
	{actSearch, "", "search", "Search the rows of the list", "suchen|search", "Die Zeilen der Liste durchsuchen"},
	{actMenu, "", "menu|overview", "Show the areas and pages of the path to choose one", "menü|menu|übersicht",
		"Die Bereiche und Seiten des Pfads zur Wahl zeigen"},
	{actStackOnly, areaRuns, "stack filter", "Limit the runs to the stack marked in Stacks, or lift the limit",
		"stack-filter|stack filter", "Die Läufe auf den in Stacks markierten Stack begrenzen oder die Grenze aufheben"},
	{actUpdate, areaStacks, "update", "Update the marked or ticked stacks", "aktualisieren|update", "Die markierten oder angehakten Stacks aktualisieren"},
	{actDeploy, areaStacks, "deploy", "Roll out the marked or ticked stacks", "ausrollen|deploy", "Die markierten oder angehakten Stacks ausrollen"},
	{actPause, areaStacks, "pause", "Pause the marked or ticked stacks", "pausieren|pause", "Die markierten oder angehakten Stacks pausieren"},
	{actUnpause, areaStacks, "unpause", "End the pause of the marked or ticked stacks", "pause beenden|unpause", "Die Pause der markierten oder angehakten Stacks beenden"},
	{actPin, areaStacks, "pin", "Pin the marked or ticked stacks on their versions", "festhalten|pin", "Die markierten oder angehakten Stacks auf ihren Versionen festhalten"},
	{actUnpin, areaStacks, "unpin", "Release the pin of the marked or ticked stacks", "festhalten aufheben|unpin", "Das Festhalten der markierten oder angehakten Stacks aufheben"},
	{actResume, areaStacks, "resume", "Resume the marked or ticked stacks after a failed update", "wiederaufnehmen|resume", "Die markierten oder angehakten Stacks nach einem fehlgeschlagenen Update wiederaufnehmen"},
	{actCheck, areaStacks, "check registries|check", "Check the registries for new versions", "registries prüfen|check registries|check", "Die Registries auf neue Versionen prüfen"},
	{actStackStart, areaStacks, "start stack|start", "Start the containers of the marked or ticked stacks", "stack starten|start", "Die Container der markierten oder angehakten Stacks starten"},
	{actStackStop, areaStacks, "stop stack|stop", "Stop the containers of the marked or ticked stacks", "stack stoppen|stop", "Die Container der markierten oder angehakten Stacks stoppen"},
	{actStackRestart, areaStacks, "restart stack|restart", "Restart the containers of the marked or ticked stacks", "stack neu starten|restart", "Die Container der markierten oder angehakten Stacks neu starten"},
	{actFetch, areaStacks, "fetch", "Fetch the marked folder from its remote now", "abrufen|fetch", "Den markierten Ordner jetzt von seinem Remote abrufen"},
	{actRemote, areaStacks, "add remote", "Give the marked local folder a remote", "remote hinzufügen|add remote", "Dem markierten lokalen Ordner ein Remote geben"},
	{actAdd, "", "add", "Add an entry in this view", "hinzufügen|add", "In dieser Ansicht einen Eintrag hinzufügen"},
	{actRemove, "", "remove", "Remove the marked entry", "entfernen|remove", "Den markierten Eintrag entfernen"},
	{actEdit, "", "edit", "Change the marked entry", "ändern|edit", "Den markierten Eintrag ändern"},
	{actRename, "", "rename", "Rename the marked entry", "umbenennen|rename", "Den markierten Eintrag umbenennen"},
	{actTest, "", "send test", "Send a test notification", "test senden|send test", "Eine Testbenachrichtigung senden"},
	{actSave, "", "save", "Save the open form", "speichern|save", "Das offene Formular speichern"},
	{actRollback, areaRuns, "rollback", "Return a stack to the state before an update run", "zurücksetzen|rollback", "Einen Stack auf den Stand vor einem Update-Lauf zurücksetzen"},
	{actRestore, areaDetail, "restore backup", "Choose a backup of the stack to restore", "backup einspielen|restore backup", "Ein Backup des Stacks zum Einspielen wählen"},
	{actDataDirs, areaDetail, "create data directories", "Create the missing data directories of the stack", "datenordner anlegen|create data directories", "Die fehlenden Datenordner des Stacks anlegen"},
	{actContainerLogs, areaContainers, "container log", "Open the output of the marked container", "container-log|container log", "Die Ausgabe des markierten Containers öffnen"},
	{actServiceLog, areaRuns, "service log", "Open the log of the deploy service", "dienstlog|service log", "Das Log des Deploy-Dienstes öffnen"},
	{actLogLevel, areaRuns, "log level", "Choose the lowest level the service log shows", "log-stufe|log level", "Die niedrigste Stufe wählen, die das Dienstlog zeigt"},
	{actSaveLog, "", "save log", "Save the lines of the open log as a file", "log speichern|save log", "Die Zeilen des offenen Logs als Datei speichern"},
	{actDiagnose, areaRuns, "diagnose", "Write the diagnostic package from the service log", "diagnose", "Das Diagnosepaket aus dem Dienstlog schreiben"},
	{actBackup, areaDetail, "backup settings", "Edit the backup rules of the stack", "backup-einstellungen|backup settings", "Die Backup-Regeln des Stacks bearbeiten"},
	{actCompose, areaDetail, "compose file", "Edit the compose file of the stack", "compose-datei|compose file", "Die Compose-Datei des Stacks bearbeiten"},
	{actCheckForm, areaDetail, "check settings", "Edit the checks of the stack", "prüfung einstellen|check settings", "Die Prüfung des Stacks bearbeiten"},
	{actRemoveSettings, areaDetail, "clear settings", "Remove all settings of the stack", "einstellungen entfernen|clear settings", "Alle Einstellungen des Stacks entfernen"},
	{actMoveUp, "", "move up", "Move the marked entry up", "nach oben|move up", "Den markierten Eintrag nach oben verschieben"},
	{actMoveDown, "", "move down", "Move the marked entry down", "nach unten|move down", "Den markierten Eintrag nach unten verschieben"},
	{actSetupDeploy, areaStacks, "set up deploy service", "Set up the deploy service where it does not exist yet", "deploy-dienst einrichten|set up deploy service", "Den Deploy-Dienst einrichten, wo es ihn noch nicht gibt"},
	{actApprove, areaProposals, "approve", "Approve the open proposal", "freigeben|approve", "Den geöffneten Vorschlag freigeben"},
	{actReject, areaProposals, "reject", "Reject the open proposal", "ablehnen|reject", "Den geöffneten Vorschlag ablehnen"},
	{actInstance, areaSettings, "instance", "Choose the deploy service whose settings are shown", "instanz|instance", "Den Deploy-Dienst wählen, dessen Einstellungen gezeigt werden"},
	{actHookAdd, areaDetail, "add hook", "Add a webhook for the stack", "hook anlegen|add hook", "Einen Webhook für den Stack anlegen"},
	{actHookRemove, areaDetail, "remove hook", "Remove a webhook of the stack", "hook entfernen|remove hook", "Einen Webhook des Stacks entfernen"},
	{actGrantAdd, areaDetail, "grant", "Grant an action on the stack to a user", "freigabe erteilen|grant", "Einem Benutzer eine Aktion auf dem Stack freigeben"},
	{actGrantRemove, areaDetail, "withdraw grant", "Withdraw a grant on the stack", "freigabe entziehen|withdraw grant", "Eine Freigabe auf dem Stack entziehen"},
	{actPromote, areaDetail, "take versions", "Take over the image versions of another stack", "versionen übernehmen|take versions", "Die Image-Versionen eines anderen Stacks übernehmen"},
	{kittui.ActionLogFollow, "", "follow log", "Follow the end of the log as it grows", "log mitlesen|follow log", "Dem Ende des Logs folgen, während es wächst"},
	{kittui.ActionLogWrap, "", "wrap lines", "Wrap long lines of the log or cut them", "zeilen umbrechen|wrap lines", "Lange Zeilen des Logs umbrechen oder abschneiden"},
	{kittui.ActionLogTimestamps, "", "timestamps", "Show or hide the timestamps of the log", "zeitstempel|timestamps", "Die Zeitstempel des Logs zeigen oder ausblenden"},
	{kittui.ActionLogFilter, "", "log period", "Choose the period the log shows", "log-zeitraum|log period", "Den Zeitraum wählen, den das Log zeigt"},
}

func commandCatalog(row func(commandRow) (name, desc string)) map[kittui.TextID]string {
	c := map[kittui.TextID]string{}
	for _, r := range commandRows {
		c[commandText(r.id, "name")], c[commandText(r.id, "desc")] = row(r)
	}
	return c
}

func commandEnglish() map[kittui.TextID]string {
	c := commandCatalog(func(r commandRow) (string, string) { return r.en, r.enDesc })
	c[textCmdOpenArea], c[textCmdNotHere], c[textCmdArea] = "open the area %s first", "not available in this view", "Open the area %s"
	return c
}

func commandGerman() map[kittui.TextID]string {
	c := commandCatalog(func(r commandRow) (string, string) { return r.de, r.deDesc })
	c[textCmdOpenArea], c[textCmdNotHere], c[textCmdArea] = "öffne zuerst den Bereich %s", "in dieser Ansicht nicht verfügbar", "Den Bereich %s öffnen"
	return c
}
