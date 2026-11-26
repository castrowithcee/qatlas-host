package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The actions of the key table.
const (
	actHelp                      = kittui.ActionHelp
	actCommand                   = kittui.ActionCommand
	actBack                      = kittui.ActionBack
	actForceQuit                 = kittui.ActionForceQuit
	actQuit                      = kittui.ActionQuit
	actReload                    = kittui.ActionReload
	actUp        kittui.ActionID = "up"
	actDown      kittui.ActionID = "down"
	actLeft      kittui.ActionID = "left"
	actRight     kittui.ActionID = "right"
	actFirst     kittui.ActionID = "first"
	actLast      kittui.ActionID = "last"
	actPageUp    kittui.ActionID = "pageup"
	actPageDown  kittui.ActionID = "pagedown"
	actEnter     kittui.ActionID = "enter"
	actSearch                    = kittui.ActionSearch
	actArea      kittui.ActionID = "area"
	// actStackOnly limits the Runs area to the stack chosen in Stacks, and lifts the limit again.
	actStackOnly = kittui.ActionFilter
	// actToggle ticks a stack in the Stacks list.
	actToggle kittui.ActionID = "toggle"
	// The actions on stacks; each opens a confirmation first.
	actUpdate                  = kittui.ActionUpdate
	actDeploy  kittui.ActionID = "deploy"
	actPause   kittui.ActionID = "pause"
	actUnpause kittui.ActionID = "unpause"
	actPin     kittui.ActionID = "pin"
	actUnpin   kittui.ActionID = "unpin"
	actResume  kittui.ActionID = "resume"
	// actRollback opens the return to before an update run, from Runs and the stack detail; its two answers choose
	// whether the data goes back too.
	actRollback     kittui.ActionID = "rollback"
	actRollbackData kittui.ActionID = "rollbackdata"
	actRollbackKeep kittui.ActionID = "rollbackkeep"
	actCheck        kittui.ActionID = "check"
	// The actions on the containers of a whole stack.
	actStackStart   kittui.ActionID = "stackstart"
	actStackStop    kittui.ActionID = "stackstop"
	actStackRestart kittui.ActionID = "stackrestart"
	// The answers of a question, and of a result that started runs.
	actYes kittui.ActionID = "yes"
	// The keys of the Settings area and of its forms.
	actNext   kittui.ActionID = "next"
	actPrev   kittui.ActionID = "prev"
	actSave                   = kittui.ActionSave
	actFetch  kittui.ActionID = "fetch"
	actAdd                    = kittui.ActionNew
	actRemove                 = kittui.ActionDelete
	actEdit                   = kittui.ActionEdit
	// actRename renames the group marked on System > Groups.
	actRename = kittui.ActionRename
	// actCopy copies the entry marked in the file browser of System > Folders.
	actCopy = kittui.ActionCopy
	// actRemote gives a local stack folder a remote; actWithRemote is the answer that adds a folder of files
	// together with the repository at an address.
	actRemote     kittui.ActionID = "remote"
	actWithRemote kittui.ActionID = "withremote"
	actTest       kittui.ActionID = "test"
	actNo         kittui.ActionID = "no"
	// actRestore opens the choice of a backup in the stack detail.
	actRestore kittui.ActionID = "restore"
	actShowLog kittui.ActionID = "showlog"
	// actDataDirs opens the creation of the missing data directories in the stack detail.
	actDataDirs kittui.ActionID = "datadirs"
	// actContainerLogs opens the output of a container, or of a whole stack in the stack detail.
	actContainerLogs kittui.ActionID = "containerlogs"
	// actServiceLog opens the log of the service from the Runs list, actLogLevel chooses its minimum level.
	actServiceLog kittui.ActionID = "servicelog"
	actLogLevel   kittui.ActionID = "loglevel"
	// actRights shows the permissions of the entry marked in the file browser of System > Folders.
	actRights = kittui.ActionInfo
	// actDiagnose writes the diagnostic package from the service log.
	actDiagnose kittui.ActionID = "diagnose"
	// actSaveLog saves the lines a log view holds as a file.
	actSaveLog kittui.ActionID = "savelog"
	// actBackup and actCompose open the backup and the compose file forms in the stack detail.
	actBackup  kittui.ActionID = "backupform"
	actCompose kittui.ActionID = "composeform"
	// actCheckForm opens the check form and actRemoveSettings the removal of all settings in the stack detail.
	actCheckForm      kittui.ActionID = "checkform"
	actRemoveSettings kittui.ActionID = "removesettings"
	// actMoveUp and actMoveDown move an entry of an ordered list in its editor.
	actMoveUp   kittui.ActionID = "moveup"
	actMoveDown kittui.ActionID = "movedown"
	// actSetupDeploy asks to set up the deploy service where it does not exist yet.
	actSetupDeploy kittui.ActionID = "setupdeploy"
	// actRootless is the answer that sets up the service of a rootless engine.
	actRootless kittui.ActionID = "rootless"
	// actApprove and actReject decide the proposal shown in the Proposals area.
	actApprove kittui.ActionID = "approve"
	actReject  kittui.ActionID = "reject"
	// actInstance chooses the deploy service whose credentials or notification channels Settings shows.
	actInstance kittui.ActionID = "instance"

	actHookAdd    kittui.ActionID = "hookadd"
	actHookRemove kittui.ActionID = "hookremove"

	// actGrantAdd and actGrantRemove grant an action of the stack detail to a user and withdraw a grant.
	actGrantAdd    kittui.ActionID = "grantadd"
	actGrantRemove kittui.ActionID = "grantremove"
)

// areaKeys are the keys of actArea, one per area in the order of the tab line.
var areaKeys = []string{"1", "2", "3", "4", "5", "6", "7"}

func act(id kittui.ActionID, level kittui.KeyLevel, help kittui.TextID, keys ...string) kittui.Action {
	return kittui.Action{ID: id, Level: level, DefaultKeys: keys, Help: help}
}

// std is the standard action id of the kit with its level and key, under a help text of qhost.
func std(id kittui.ActionID, help kittui.TextID) kittui.Action {
	for _, a := range kittui.StandardActions() {
		if a.ID == id {
			a.Help = help
			return a
		}
	}
	panic("tui: no standard action " + string(id))
}

func keyTable() []kittui.Action {
	table := []kittui.Action{
		std(actHelp, kittui.TextKeyHelp), std(actCommand, kittui.TextKeyCommands),
		std(actBack, kittui.TextKeyBack),
		std(actForceQuit, kittui.TextKeyQuit),
		std(actQuit, kittui.TextKeyQuit),
		std(actReload, kittui.TextKeyReload),
		act(actUp, kittui.LevelMove, kittui.TextKeyUp, "up"),
		act(actDown, kittui.LevelMove, kittui.TextKeyDown, "down"),
		act(actLeft, kittui.LevelMove, kittui.TextKeyLeft, "left"),
		act(actRight, kittui.LevelMove, kittui.TextKeyRight, "right"),
		act(actFirst, kittui.LevelMove, kittui.TextKeyFirst, "home"),
		act(actLast, kittui.LevelMove, kittui.TextKeyLast, "end"),
		act(actPageUp, kittui.LevelMove, kittui.TextKeyPageUp, "pgup"),
		act(actPageDown, kittui.LevelMove, kittui.TextKeyPageDown, "pgdown"),
		act(actEnter, kittui.LevelMove, kittui.TextKeyEnter, "enter"),
		std(actSearch, kittui.TextKeySearch),
		act(actArea, kittui.LevelMove, textKeyArea, areaKeys...),
		std(actStackOnly, textKeyStackOnly),
		act(actToggle, kittui.LevelMove, textKeyTick, "space"),
		std(actUpdate, textKeyUpdate),
		act(actDeploy, kittui.LevelLetter, textKeyDeploy, "a"),
		act(actPause, kittui.LevelLetter, textKeyPause, "p"),
		act(actUnpause, kittui.LevelLetter, textKeyUnpause, "P"),
		act(actPin, kittui.LevelLetter, textKeyPin, "y"),
		act(actUnpin, kittui.LevelLetter, textKeyUnpin, "Y"),
		act(actResume, kittui.LevelLetter, textKeyResume, "g"),
		act(actCheck, kittui.LevelLetter, textKeyCheck, "u"),
		act(actStackStart, kittui.LevelLetter, textKeyStart, "S"),
		act(actStackStop, kittui.LevelLetter, textKeyStop, "X"),
		act(actStackRestart, kittui.LevelLetter, textKeyRestart, "Z"),
		act(actNext, kittui.LevelMove, textKeyNext, "tab"),
		act(actPrev, kittui.LevelMove, textKeyPrev, "shift+tab"),
		std(actSave, textKeySave),
		act(actFetch, kittui.LevelLetter, textKeyFetch, "F"),
		std(actAdd, textKeyAdd),
		std(actRemove, textKeyRemove),
		std(actEdit, textKeyEdit),
		std(actRename, textKeyRename),
		act(actTest, kittui.LevelLetter, textKeyTest, "s"),
		act(actRemote, kittui.LevelLetter, textKeyRemote, "O"),
		act(actWithRemote, kittui.LevelConfirm, textAnswerWithRemote, "w"),
		act(actChoose, kittui.LevelConfirm, textAnswerChoose, "c"),
		act(actRollback, kittui.LevelLetter, textKeyRollback, "B"),
		act(actRollbackData, kittui.LevelConfirm, textAnswerRollbackData, "d"),
		act(actRollbackKeep, kittui.LevelConfirm, textAnswerRollbackKeep, "v"),
		act(actRestore, kittui.LevelLetter, textKeyRestore, "l"),
		act(actDataDirs, kittui.LevelLetter, textKeyDataDirs, "+"),
		act(actContainerLogs, kittui.LevelLetter, textKeyLog, "o"),
		act(actServiceLog, kittui.LevelLetter, textKeySvcLog, "v"),
		act(actLogLevel, kittui.LevelLetter, textKeyLogLevel, "!"),
		act(actSaveLog, kittui.LevelLetter, textKeySaveLog, "W"),
		act(actDiagnose, kittui.LevelLetter, textKeyDiagnose, "D"),
		act(actBackup, kittui.LevelLetter, textKeyBackupForm, "b"),
		act(actCompose, kittui.LevelLetter, textKeyComposeForm, "I"),
		act(actCheckForm, kittui.LevelLetter, textKeyCheckForm, "k"),
		act(actRemoveSettings, kittui.LevelLetter, textKeyRemoveSettings, "E"),
		act(actMoveUp, kittui.LevelLetter, textKeyMoveUp, "K"),
		act(actMoveDown, kittui.LevelLetter, textKeyMoveDown, "J"),
		act(actSetupDeploy, kittui.LevelLetter, textKeySetupDeploy, "G"),
		act(actApprove, kittui.LevelLetter, textKeyApprove, "A"),
		act(actReject, kittui.LevelLetter, textKeyReject, "N"),
		act(actInstance, kittui.LevelLetter, textKeyInstance, "H"),
		act(actHookAdd, kittui.LevelLetter, textKeyHookAdd, "h"),
		act(actHookRemove, kittui.LevelLetter, textKeyHookRemove, "j"),
		act(actGrantAdd, kittui.LevelLetter, textKeyGrantAdd, "z"),
		act(actGrantRemove, kittui.LevelLetter, textKeyGrantRemove, "x"),
		act(actRootless, kittui.LevelConfirm, textAnswerSetupRootless, "r"),
		act(actYes, kittui.LevelConfirm, textAnswerYes, "y"),
		act(actNo, kittui.LevelConfirm, textAnswerCancel, "n", "esc"),
		act(actShowLog, kittui.LevelConfirm, textAnswerLog, "l"),
	}
	return append(append(append(table, promoteKeys()...), browserKeys()...), append(kittui.LogViewActions(), navActions()...)...)
}

// browserKeys are the actions of the file browser and its text view beyond those of a list and a log.
func browserKeys() []kittui.Action {
	return append([]kittui.Action{std(actRights, textKeyRights), std(actCopy, kittui.TextKeyCopy)}, kittui.TextViewActions()...)
}

func textKeys() kittui.TextViewKeys {
	return kittui.TextViewKeys{Up: actUp, Down: actDown, PageUp: actPageUp, PageDown: actPageDown, First: actFirst,
		Last: actLast, Search: actSearch, Enter: actEnter, Back: actBack, NextMatch: kittui.ActionLogNextMatch,
		PrevMatch: kittui.ActionLogPrevMatch, Wrap: kittui.ActionLogWrap, Numbers: kittui.ActionTextNumbers}
}

func fileBrowserKeys() kittui.FileBrowserKeys {
	return kittui.FileBrowserKeys{Up: actUp, Down: actDown, PageUp: actPageUp, PageDown: actPageDown, First: actFirst,
		Last: actLast, Left: actLeft, Right: actRight, Enter: actEnter, Search: actSearch, Back: actBack, Reload: actReload}
}

func logKeys() kittui.LogViewKeys {
	return kittui.LogViewKeys{Up: actUp, Down: actDown, PageUp: actPageUp, PageDown: actPageDown, First: actFirst,
		Last: actLast, Search: actSearch, Enter: actEnter, Back: actBack, Follow: kittui.ActionLogFollow,
		NextMatch: kittui.ActionLogNextMatch, PrevMatch: kittui.ActionLogPrevMatch, Wrap: kittui.ActionLogWrap,
		Timestamps: kittui.ActionLogTimestamps,
		Filter:     kittui.ActionLogFilter, FilterForm: formKeys()}
}

func listKeys() kittui.ListKeys {
	return kittui.ListKeys{Up: actUp, Down: actDown, PageUp: actPageUp, PageDown: actPageDown, First: actFirst,
		Last: actLast, Left: actLeft, Right: actRight, Enter: actEnter, Search: actSearch, Back: actBack}
}

// stackListKeys are the keys of the Stacks list, which ticks stacks.
func stackListKeys() kittui.ListKeys {
	k := listKeys()
	k.Toggle = actToggle
	return k
}

func helpKeys() kittui.HelpKeys {
	return kittui.HelpKeys{Left: actLeft, Right: actRight, Up: actUp, Down: actDown, PageUp: actPageUp,
		PageDown: actPageDown, First: actFirst, Last: actLast, Back: actBack, Help: actHelp}
}

// formKeys are the actions of the key table a form reads.
func formKeys() kittui.FormKeys {
	return kittui.FormKeys{Up: actUp, Down: actDown, Next: actNext, Prev: actPrev, PageUp: actPageUp,
		PageDown: actPageDown, Left: actLeft, Right: actRight, First: actFirst, Last: actLast, Enter: actEnter,
		Toggle: actToggle, Save: actSave, Back: actBack}
}
