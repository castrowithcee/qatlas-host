package tui

import kittui "github.com/castrowithcee/qatlas-kit/tui"

// The texts of the System area and of the changes of the host. Names of the host, commands and what the root
// command reports come in as arguments and are escaped where they are drawn; sudo, root, qhost and Docker stay
// as they are in every language.
const (
	textAreaSystem kittui.TextID = "area.system"
	textPageUsers  kittui.TextID = "system.page.users"
	textPageGroups kittui.TextID = "system.page.groups"

	textColUID     kittui.TextID = "col.uid"
	textColGID     kittui.TextID = "col.gid"
	textColAdmin   kittui.TextID = "col.admin"
	textColDocker  kittui.TextID = "col.docker"
	textColLogin   kittui.TextID = "col.login"
	textColGroups  kittui.TextID = "col.groups"
	textColMembers kittui.TextID = "col.members"
	textColChange  kittui.TextID = "col.change"

	textUsersEmpty         kittui.TextID = "system.users.empty"
	textGroupsEmpty        kittui.TextID = "system.groups.empty"
	textGroupRegularUsers  kittui.TextID = "system.group.users"
	textGroupSystemUsers   kittui.TextID = "system.group.sysusers"
	textGroupRegularGroups kittui.TextID = "system.group.groups"
	textGroupSystemGroups  kittui.TextID = "system.group.sysgroups"
	textSystemLoading      kittui.TextID = "system.loading"
	textSystemReadFailed   kittui.TextID = "system.readfailed"

	textAdminNoAccess kittui.TextID = "system.admin.noaccess"
	textLoginActive   kittui.TextID = "system.login.active"
	textLoginLocked   kittui.TextID = "system.login.locked"
	textLoginExpired  kittui.TextID = "system.login.expired"
	textLoginUnknown  kittui.TextID = "system.login.unknown"

	textDetailHome         kittui.TextID = "system.detail.home"
	textDetailPrimary      kittui.TextID = "system.detail.primary"
	textDetailGroups       kittui.TextID = "system.detail.groups"
	textDetailAccess       kittui.TextID = "system.detail.access"
	textDetailLogin        kittui.TextID = "system.detail.login"
	textDetailKeys         kittui.TextID = "system.detail.keys"
	textDetailKeysUnknown  kittui.TextID = "system.detail.keysunknown"
	textAccessAdmin        kittui.TextID = "system.access.admin"
	textAccessAdminNoQhost kittui.TextID = "system.access.adminnoqhost"
	textAccessNone         kittui.TextID = "system.access.none"

	textKeyNewUser  kittui.TextID = "key.newuser"
	textKeyChange   kittui.TextID = "key.change"
	textKeyDelete   kittui.TextID = "key.delete"
	textKeyNewGroup kittui.TextID = "key.newgroup"
	textKeyMembers  kittui.TextID = "key.members"
	textKeyRename   kittui.TextID = "key.rename"

	textTitleNewUser      kittui.TextID = "title.newuser"
	textTitleChangeUser   kittui.TextID = "title.changeuser"
	textTitleAddGroups    kittui.TextID = "title.addgroups"
	textTitleRemoveGroups kittui.TextID = "title.removegroups"
	textTitleUserSecret   kittui.TextID = "title.usersecret"
	textTitleAddKey       kittui.TextID = "title.addkey"
	textTitleRemoveKey    kittui.TextID = "title.removekey"
	textTitleDeleteUser   kittui.TextID = "title.deleteuser"
	textLeadNewUser       kittui.TextID = "lead.newuser"
	textLeadDeleteUser    kittui.TextID = "lead.deleteuser"

	textTitleNewGroup      kittui.TextID = "title.newgroup"
	textTitleMembers       kittui.TextID = "title.members"
	textTitleRenameGroup   kittui.TextID = "title.renamegroup"
	textLeadNewGroup       kittui.TextID = "lead.newgroup"
	textLeadMembers        kittui.TextID = "lead.members"
	textLeadMembersSudo    kittui.TextID = "lead.memberssudo"
	textLeadMembersDocker  kittui.TextID = "lead.membersdocker"
	textLeadRenameGroup    kittui.TextID = "lead.renamegroup"
	textFieldNewName       kittui.TextID = "field.newname"
	textFieldMembersAdd    kittui.TextID = "field.membersadd"
	textFieldMembersRemove kittui.TextID = "field.membersremove"
	textHintMembers        kittui.TextID = "hint.members"
	textProblemGroupName   kittui.TextID = "problem.groupname"
	textProblemGroupTaken  kittui.TextID = "problem.grouptaken"
	textProblemTickUser    kittui.TextID = "problem.tickuser"
	textAskCreateGroup     kittui.TextID = "ask.creategroup"
	textAskMembersAdd      kittui.TextID = "ask.membersadd"
	textAskMembersRemove   kittui.TextID = "ask.membersremove"
	textAskRenameGroup     kittui.TextID = "ask.renamegroup"
	textAskDeleteGroup     kittui.TextID = "ask.deletegroup"
	textMembersSudoNote    kittui.TextID = "host.memberssudo"
	textRiskGroupRoot      kittui.TextID = "host.riskgrouproot"
	textRiskGroupDocker    kittui.TextID = "host.riskgroupdocker"
	textRiskRename         kittui.TextID = "host.riskrename"
	textRiskMembers        kittui.TextID = "host.riskmembers"
	textGroupKeepName      kittui.TextID = "host.groupkeepname"
	textGroupKeepDelete    kittui.TextID = "host.groupkeepdelete"
	textGroupPrimary       kittui.TextID = "host.groupprimary"
	textNoMemberChange     kittui.TextID = "host.nomemberchange"

	textFieldUserName   kittui.TextID = "field.username"
	textHintUserName    kittui.TextID = "hint.username"
	textFieldInitial    kittui.TextID = "field.initial"
	textHintInitial     kittui.TextID = "hint.initial"
	textFieldNewSecret  kittui.TextID = "field.newsecret"
	textHintNewSecret   kittui.TextID = "hint.newsecret"
	textFieldRepeat     kittui.TextID = "field.repeat"
	textHintRepeat      kittui.TextID = "hint.repeat"
	textFieldMust       kittui.TextID = "field.must"
	textHintMust        kittui.TextID = "hint.must"
	textFieldAdmin      kittui.TextID = "field.admin"
	textHintAdmin       kittui.TextID = "hint.admin"
	textFieldDocker     kittui.TextID = "field.docker"
	textHintDocker      kittui.TextID = "hint.docker"
	textFieldGroups     kittui.TextID = "field.groups"
	textHintGroups      kittui.TextID = "hint.groups"
	textFieldKeys       kittui.TextID = "field.keys"
	textHintKeys        kittui.TextID = "hint.keys"
	textFieldKey        kittui.TextID = "field.key"
	textHintKey         kittui.TextID = "hint.key"
	textFieldKeyOf      kittui.TextID = "field.keyof"
	textHintKeyOf       kittui.TextID = "hint.keyof"
	textFieldAddTo      kittui.TextID = "field.addto"
	textHintAddTo       kittui.TextID = "hint.addto"
	textFieldRemoveFrom kittui.TextID = "field.removefrom"
	textHintRemoveFrom  kittui.TextID = "hint.removefrom"
	textFieldHome       kittui.TextID = "field.home"
	textHintHome        kittui.TextID = "hint.home"
	textHomeKeep        kittui.TextID = "home.keep"
	textHomeDelete      kittui.TextID = "home.delete"

	textProblemUserName       kittui.TextID = "problem.username"
	textProblemUserTaken      kittui.TextID = "problem.usertaken"
	textProblemSecretEmpty    kittui.TextID = "problem.secretempty"
	textProblemSecretBad      kittui.TextID = "problem.secretbad"
	textProblemSecretMismatch kittui.TextID = "problem.secretmismatch"
	textProblemKeyBad         kittui.TextID = "problem.keybad"
	textProblemKeyTwice       kittui.TextID = "problem.keytwice"
	textProblemKeysMax        kittui.TextID = "problem.keysmax"
	textProblemKeyOne         kittui.TextID = "problem.keyone"
	textProblemTickOne        kittui.TextID = "problem.tickone"

	textChgEnable       kittui.TextID = "change.enable"
	textChgAdminOn      kittui.TextID = "change.adminon"
	textChgAdminOff     kittui.TextID = "change.adminoff"
	textChgDockerOn     kittui.TextID = "change.dockeron"
	textChgDockerOff    kittui.TextID = "change.dockeroff"
	textChgAddGroups    kittui.TextID = "change.addgroups"
	textChgRemoveGroups kittui.TextID = "change.removegroups"
	textChgSecret       kittui.TextID = "change.secret"
	textChgAddKey       kittui.TextID = "change.addkey"
	textChgRemoveKey    kittui.TextID = "change.removekey"
	textChgLock         kittui.TextID = "change.lock"
	textChgUnlock       kittui.TextID = "change.unlock"

	textAskCreateUser   kittui.TextID = "ask.createuser"
	textAskEnable       kittui.TextID = "ask.enable"
	textAskAdminOn      kittui.TextID = "ask.adminon"
	textAskAdminOff     kittui.TextID = "ask.adminoff"
	textAskDockerOn     kittui.TextID = "ask.dockeron"
	textAskDockerOff    kittui.TextID = "ask.dockeroff"
	textAskAddGroups    kittui.TextID = "ask.addgroups"
	textAskRemoveGroups kittui.TextID = "ask.removegroups"
	textAskSecret       kittui.TextID = "ask.secret"
	textAskSecretMust   kittui.TextID = "ask.secretmust"
	textAskAddKey       kittui.TextID = "ask.addkey"
	textAskRemoveKey    kittui.TextID = "ask.removekey"
	textAskLock         kittui.TextID = "ask.lock"
	textAskUnlock       kittui.TextID = "ask.unlock"
	textAskDeleteKeep   kittui.TextID = "ask.deletekeep"
	textAskDeleteHome   kittui.TextID = "ask.deletehome"

	textHostPowerNote kittui.TextID = "host.powernote"
	textLockStays     kittui.TextID = "host.lockstays"
	textDeleteKeeps   kittui.TextID = "host.deletekeeps"
	textDeleteFinal   kittui.TextID = "host.deletefinal"
	textHomeSystem    kittui.TextID = "host.homesystem"
	textHomeHolds     kittui.TextID = "host.homeholds"
	textHomeLink      kittui.TextID = "host.homelink"
	textHomeNotDir    kittui.TextID = "host.homenotdir"
	textHomeOwner     kittui.TextID = "host.homeowner"
	textHomeUnread    kittui.TextID = "host.homeunread"
	textRiskRoot      kittui.TextID = "host.riskroot"
	textRiskAccount   kittui.TextID = "host.riskaccount"
	textRiskGroup     kittui.TextID = "host.riskgroup"
	textSelfAdmin     kittui.TextID = "host.selfadmin"
	textSelfLock      kittui.TextID = "host.selflock"
	textSelfDelete    kittui.TextID = "host.selfdelete"
	textLastAdmin     kittui.TextID = "host.lastadmin"
	textNoKeys        kittui.TextID = "host.nokeys"

	textTitleHostChange  kittui.TextID = "title.hostchange"
	textTitleHostResult  kittui.TextID = "title.hostresult"
	textAnswerRunChange  kittui.TextID = "answer.runchange"
	textHostReversible   kittui.TextID = "host.reversible"
	textHostIrreversible kittui.TextID = "host.irreversible"
	textHostDone         kittui.TextID = "host.done"
	textHostDoneUndo     kittui.TextID = "host.doneundo"
	textHostDoneFinal    kittui.TextID = "host.donefinal"
	textHostRefused      kittui.TextID = "host.refused"
	textHostRefusedNote  kittui.TextID = "host.refusednote"
	textHostRestored     kittui.TextID = "host.restored"
	textHostRestoredNote kittui.TextID = "host.restorednote"
	textHostFailed       kittui.TextID = "host.failed"
	textHostFailedNote   kittui.TextID = "host.failednote"
	textHostCheckPassed  kittui.TextID = "host.checkpassed"
	textHostCheckFailed  kittui.TextID = "host.checkfailed"
	textHostAuditWarning kittui.TextID = "host.auditwarning"
	textHostRegroup      kittui.TextID = "host.regroup"

	textHostChangeFailed     kittui.TextID = "host.changefailed"
	textHostReadFailed       kittui.TextID = "host.readfailed"
	textHostErrNoSudo        kittui.TextID = "host.err.nosudo"
	textHostErrStart         kittui.TextID = "host.err.start"
	textHostErrDenied        kittui.TextID = "host.err.denied"
	textHostErrAbortedChange kittui.TextID = "host.err.abortedchange"
	textHostErrAbortedRead   kittui.TextID = "host.err.abortedread"
	textHostErrExitChange    kittui.TextID = "host.err.exitchange"
	textHostErrExitRead      kittui.TextID = "host.err.exitread"
	textHostErrOutput        kittui.TextID = "host.err.output"
	textHostErrNoCredential  kittui.TextID = "host.err.nocredential"
	textHostErrReported      kittui.TextID = "host.err.reported"
)

func systemEnglish() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textAreaSystem: "System",
		textPageUsers:  "Users",
		textPageGroups: "Groups",

		textColUID:     "UID",
		textColGID:     "GID",
		textColAdmin:   "Admin",
		textColDocker:  "Docker",
		textColLogin:   "Login",
		textColGroups:  "Groups",
		textColMembers: "Members",
		textColChange:  "Change",

		textUsersEmpty:         "No local user. Press {new} to create one.",
		textGroupsEmpty:        "No local group.",
		textGroupRegularUsers:  "Users",
		textGroupSystemUsers:   "System accounts",
		textGroupRegularGroups: "Groups",
		textGroupSystemGroups:  "System groups",
		textSystemLoading:      "Reading the users and groups of the host...",
		textSystemReadFailed:   "the users and groups cannot be read: %s",

		textAdminNoAccess: "yes, without qhost",
		textLoginActive:   "active",
		textLoginLocked:   "locked",
		textLoginExpired:  "expired",
		textLoginUnknown:  "unknown",

		textDetailHome:         "Home: %s",
		textDetailPrimary:      "Primary group: %s",
		textDetailGroups:       "Further groups: %s",
		textDetailAccess:       "Access: %s",
		textDetailLogin:        "Login: %s",
		textDetailKeys:         "SSH keys: %s",
		textDetailKeysUnknown:  "Login state and SSH keys are shown to root only.",
		textAccessAdmin:        "administrator with access to qhost",
		textAccessAdminNoQhost: "administrator without access to qhost",
		textAccessNone:         "no administrator",

		textKeyNewUser:  "new user",
		textKeyChange:   "change",
		textKeyDelete:   "delete",
		textKeyNewGroup: "new group",
		textKeyMembers:  "members",
		textKeyRename:   "rename",

		textTitleNewUser:      "New user",
		textTitleChangeUser:   "Change %s",
		textTitleAddGroups:    "Add to groups",
		textTitleRemoveGroups: "Remove from groups",
		textTitleUserSecret:   "New password",
		textTitleAddKey:       "Add an SSH key",
		textTitleRemoveKey:    "Remove an SSH key",
		textTitleDeleteUser:   "Delete user",
		textLeadNewUser:       "The user must change the initial password at the first login.",
		textLeadDeleteUser:    "Locking is the usual way when someone leaves. Keeping the home keeps the delete undoable.",

		textTitleNewGroup:      "New group",
		textTitleMembers:       "Members of %s",
		textTitleRenameGroup:   "Rename %s",
		textLeadNewGroup:       "The system gives the group a regular group number.",
		textLeadMembers:        "One confirmation covers all changes. A user whose primary group this is cannot be added or removed.",
		textLeadMembersSudo:    "Adding to sudo also gives access to qhost; removing takes both. Admin rights allow practically everything on this host.",
		textLeadMembersDocker:  "Members of the group docker use Docker, which is practically root. One confirmation covers all changes.",
		textLeadRenameGroup:    "Configurations such as sudoers address a group by its name; they keep the old name.",
		textFieldNewName:       "New name",
		textFieldMembersAdd:    "Add",
		textFieldMembersRemove: "Remove",
		textHintMembers:        "Space ticks a user; several at once are fine.",
		textProblemGroupName:   "Enter a name of lower-case letters, digits, _ and -, at most 32.",
		textProblemGroupTaken:  "This group exists already.",
		textProblemTickUser:    "Tick at least one user.",
		textAskCreateGroup:     "Create the group %s.",
		textAskMembersAdd:      "Add %s to the group %s.",
		textAskMembersRemove:   "Remove %s from the group %s.",
		textAskRenameGroup:     "Rename the group %s to %s.",
		textAskDeleteGroup:     "Delete the group %s.",
		textMembersSudoNote:    "Administrators gain or lose access to qhost (group qhost) with it.",
		textRiskGroupRoot:      "The group root belongs to the root account and to system files.",
		textRiskGroupDocker:    "Members of the group docker use Docker, which is practically root.",
		textRiskRename:         "Configurations such as sudoers address a group by its name and keep the old one; they stop working until they are changed too.",
		textRiskMembers:        "Packages or services may depend on a system account of this change.",
		textGroupKeepName:      "%s keeps its name: administrator rights and access to qhost depend on it.",
		textGroupKeepDelete:    "%s is never deleted: administrator rights and access to qhost depend on it.",
		textGroupPrimary:       "%s is the primary group of %s and cannot be deleted.",
		textNoMemberChange:     "There is no user to add to or remove from %s.",

		textFieldUserName:   "Name",
		textHintUserName:    "Lower-case letters, digits, _ and -, at most 32.",
		textFieldInitial:    "Initial password",
		textHintInitial:     "Hidden while typed. It never shows and never goes into a command line.",
		textFieldNewSecret:  "New password",
		textHintNewSecret:   "Hidden while typed. It never shows and never goes into a command line.",
		textFieldRepeat:     "Repeat",
		textHintRepeat:      "The same password again.",
		textFieldMust:       "Change at login",
		textHintMust:        "The user must choose a new password at the next login.",
		textFieldAdmin:      "Admin",
		textHintAdmin:       "Administrator with sudo and access to qhost (groups sudo and qhost).",
		textFieldDocker:     "Docker",
		textHintDocker:      "Access to Docker (group docker).",
		textFieldGroups:     "Further groups",
		textHintGroups:      "Space ticks a group.",
		textFieldKeys:       "SSH keys",
		textHintKeys:        "Optional. Paste public keys, one per line. They are not shown.",
		textFieldKey:        "SSH key",
		textHintKey:         "Paste one public key in authorized_keys format. It is not shown.",
		textFieldKeyOf:      "Key",
		textHintKeyOf:       "The key to remove, by its fingerprint.",
		textFieldAddTo:      "Add to",
		textHintAddTo:       "Space ticks a group; several at once are fine.",
		textFieldRemoveFrom: "Remove from",
		textHintRemoveFrom:  "Space ticks a group; several at once are fine.",
		textFieldHome:       "Home",
		textHintHome:        "Deleting the home cannot be undone.",
		textHomeKeep:        "keep the home and its files",
		textHomeDelete:      "delete the home and the mail spool",

		textProblemUserName:       "Enter a name of lower-case letters, digits, _ and -, at most 32, not root.",
		textProblemUserTaken:      "This user exists already.",
		textProblemSecretEmpty:    "Enter a password.",
		textProblemSecretBad:      "The password may have up to %d bytes, without control characters.",
		textProblemSecretMismatch: "The two passwords differ.",
		textProblemKeyBad:         "Not a public key in authorized_keys format (one line each, without options).",
		textProblemKeyTwice:       "A key is given twice.",
		textProblemKeysMax:        "At most %d keys.",
		textProblemKeyOne:         "Paste exactly one key.",
		textProblemTickOne:        "Tick at least one group.",

		textChgEnable:       "Give access to qhost",
		textChgAdminOn:      "Make administrator",
		textChgAdminOff:     "Take administrator rights",
		textChgDockerOn:     "Give access to Docker",
		textChgDockerOff:    "Take access to Docker",
		textChgAddGroups:    "Add to groups",
		textChgRemoveGroups: "Remove from groups",
		textChgSecret:       "Set a new password",
		textChgAddKey:       "Add an SSH key",
		textChgRemoveKey:    "Remove an SSH key",
		textChgLock:         "Lock",
		textChgUnlock:       "Unlock",

		textAskCreateUser:   "Create the user %s with an initial password that must be changed at the first login.",
		textAskEnable:       "Give the administrator %s access to qhost.",
		textAskAdminOn:      "Make %s an administrator with access to qhost.",
		textAskAdminOff:     "Take administrator rights and access to qhost from %s.",
		textAskDockerOn:     "Give %s access to Docker.",
		textAskDockerOff:    "Take access to Docker from %s.",
		textAskAddGroups:    "Add %s to %s.",
		textAskRemoveGroups: "Remove %s from %s.",
		textAskSecret:       "Set a new password for %s.",
		textAskSecretMust:   "Set a new password for %s that must be changed at the next login.",
		textAskAddKey:       "Add the SSH key to %s.",
		textAskRemoveKey:    "Remove the SSH key %s from %s.",
		textAskLock:         "Lock %s: no login is possible any more, with a password or an SSH key.",
		textAskUnlock:       "Unlock %s.",
		textAskDeleteKeep:   "Delete the user %s. The home and its files stay.",
		textAskDeleteHome:   "Delete the user %s together with the home and the mail spool.",

		textHostPowerNote: "Admin and Docker rights allow practically everything on this host.",
		textLockStays:     "Running sessions stay open until they end.",
		textDeleteKeeps:   "A user with running processes is not deleted.",
		textDeleteFinal:   "This cannot be undone: the home %s and the mail spool are deleted with all their files.",
		textHomeSystem:    "%s is a system path.",
		textHomeHolds:     "%s holds the home of %s.",
		textHomeLink:      "%s is a link.",
		textHomeNotDir:    "%s is not a directory.",
		textHomeOwner:     "%s belongs to %s.",
		textHomeUnread:    "What else %s holds could not be checked: %s.",
		textRiskRoot:      "root is the account with all rights on this host.",
		textRiskAccount:   "Packages or services may depend on this system account.",
		textRiskGroup:     "Packages or services may depend on a system group of this change.",
		textSelfAdmin:     "You cannot take administrator rights from yourself.",
		textSelfLock:      "You cannot lock yourself.",
		textSelfDelete:    "You cannot delete yourself.",
		textLastAdmin:     "%s is the last administrator with access to qhost and stays one.",
		textNoKeys:        "%s has no SSH key.",

		textTitleHostChange:  "Change the host",
		textTitleHostResult:  "Host",
		textAnswerRunChange:  "yes, with sudo",
		textHostReversible:   "Runs with sudo, goes into the journal and can be undone.",
		textHostIrreversible: "Runs with sudo. This cannot be undone.",
		textHostDone:         "Done: %s",
		textHostDoneUndo:     "Journal entry %s. System > Changes takes it back.",
		textHostDoneFinal:    "This change cannot be undone.",
		textHostRefused:      "Not changed: %s",
		textHostRefusedNote:  "A rule of the host refused this change before anything was done.",
		textHostRestored:     "Failed, previous state restored: %s",
		textHostRestoredNote: "The change or its check failed and the previous state is back.",
		textHostFailed:       "Failed: %s",
		textHostFailedNote:   "The change did not succeed. Read sudo qhost host journal for what ran.",
		textHostCheckPassed:  "Check: passed.",
		textHostCheckFailed:  "Check failed: %s",
		textHostAuditWarning: "Warning: %s",
		textHostRegroup:      "This session does not have the group qhost yet. Start qhost tui again to use it.",

		textHostChangeFailed:     "The host was not changed as asked",
		textHostReadFailed:       "Reading as root failed",
		textHostErrNoSudo:        "sudo is not installed on this host, and a change needs root. Nothing was changed.",
		textHostErrStart:         "sudo could not be started: %s. Nothing was changed.",
		textHostErrDenied:        "sudo did not run it: the password was wrong or this account may not use sudo. Nothing was changed.",
		textHostErrAbortedChange: "The run was interrupted before it reported a result. Read sudo qhost host journal for what was done.",
		textHostErrAbortedRead:   "The run was interrupted. Nothing was changed.",
		textHostErrExitChange:    "The command ended with exit code %d and no report. Read sudo qhost host journal for what was done.",
		textHostErrExitRead:      "The command ended with exit code %d and no answer. Nothing was changed.",
		textHostErrOutput:        "The answer of qhost could not be read. Nothing was changed.",
		textHostErrNoCredential:  "sudo no longer holds your password and cannot ask again here, for example because its timeout is zero. Nothing was changed.",
		textHostErrReported:      "qhost answered without a report: %s. Read sudo qhost host journal for whether anything was done.",
	}
}

func systemGerman() map[kittui.TextID]string {
	return map[kittui.TextID]string{
		textAreaSystem: "System",
		textPageUsers:  "Benutzer",
		textPageGroups: "Gruppen",

		textColUID:     "UID",
		textColGID:     "GID",
		textColAdmin:   "Admin",
		textColDocker:  "Docker",
		textColLogin:   "Login",
		textColGroups:  "Gruppen",
		textColMembers: "Mitglieder",
		textColChange:  "Änderung",

		textUsersEmpty:         "Kein lokaler Benutzer. {new} legt einen an.",
		textGroupsEmpty:        "Keine lokale Gruppe.",
		textGroupRegularUsers:  "Benutzer",
		textGroupSystemUsers:   "Systemkonten",
		textGroupRegularGroups: "Gruppen",
		textGroupSystemGroups:  "Systemgruppen",
		textSystemLoading:      "Benutzer und Gruppen des Hosts werden gelesen...",
		textSystemReadFailed:   "Benutzer und Gruppen lassen sich nicht lesen: %s",

		textAdminNoAccess: "ja, ohne qhost",
		textLoginActive:   "aktiv",
		textLoginLocked:   "gesperrt",
		textLoginExpired:  "abgelaufen",
		textLoginUnknown:  "unbekannt",

		textDetailHome:         "Home: %s",
		textDetailPrimary:      "Primäre Gruppe: %s",
		textDetailGroups:       "Weitere Gruppen: %s",
		textDetailAccess:       "Zugang: %s",
		textDetailLogin:        "Login: %s",
		textDetailKeys:         "SSH-Schlüssel: %s",
		textDetailKeysUnknown:  "Login-Zustand und SSH-Schlüssel sieht nur root.",
		textAccessAdmin:        "Administrator mit Zugang zu qhost",
		textAccessAdminNoQhost: "Administrator ohne Zugang zu qhost",
		textAccessNone:         "kein Administrator",

		textKeyNewUser:  "neuer Benutzer",
		textKeyChange:   "ändern",
		textKeyDelete:   "löschen",
		textKeyNewGroup: "neue Gruppe",
		textKeyMembers:  "Mitglieder",
		textKeyRename:   "umbenennen",

		textTitleNewUser:      "Neuer Benutzer",
		textTitleChangeUser:   "%s ändern",
		textTitleAddGroups:    "Zu Gruppen hinzufügen",
		textTitleRemoveGroups: "Aus Gruppen entfernen",
		textTitleUserSecret:   "Neues Passwort",
		textTitleAddKey:       "SSH-Schlüssel hinzufügen",
		textTitleRemoveKey:    "SSH-Schlüssel entfernen",
		textTitleDeleteUser:   "Benutzer löschen",
		textLeadNewUser:       "Der Benutzer muss das Startpasswort beim ersten Login ändern.",
		textLeadDeleteUser:    "Sperren ist der übliche Weg, wenn jemand geht. Bleibt das Home, lässt sich das Löschen rückgängig machen.",

		textTitleNewGroup:      "Neue Gruppe",
		textTitleMembers:       "Mitglieder von %s",
		textTitleRenameGroup:   "%s umbenennen",
		textLeadNewGroup:       "Das System gibt der Gruppe eine reguläre Gruppennummer.",
		textLeadMembers:        "Eine Rückfrage deckt alle Änderungen ab. Wessen primäre Gruppe dies ist, kann weder hinzugefügt noch entfernt werden.",
		textLeadMembersSudo:    "Wer zu sudo kommt, bekommt auch Zugang zu qhost; wer entfernt wird, verliert beides. Adminrechte erlauben praktisch alles auf diesem Host.",
		textLeadMembersDocker:  "Mitglieder der Gruppe docker nutzen Docker, und das ist praktisch root. Eine Rückfrage deckt alle Änderungen ab.",
		textLeadRenameGroup:    "Konfigurationen wie sudoers sprechen eine Gruppe über ihren Namen an; sie behalten den alten Namen.",
		textFieldNewName:       "Neuer Name",
		textFieldMembersAdd:    "Hinzufügen",
		textFieldMembersRemove: "Entfernen",
		textHintMembers:        "Leertaste wählt einen Benutzer, auch mehrere auf einmal.",
		textProblemGroupName:   "Gib einen Namen aus Kleinbuchstaben, Ziffern, _ und - ein, höchstens 32.",
		textProblemGroupTaken:  "Diese Gruppe gibt es schon.",
		textProblemTickUser:    "Wähle mindestens einen Benutzer.",
		textAskCreateGroup:     "Die Gruppe %s anlegen.",
		textAskMembersAdd:      "%s zur Gruppe %s hinzufügen.",
		textAskMembersRemove:   "%s aus der Gruppe %s entfernen.",
		textAskRenameGroup:     "Die Gruppe %s in %s umbenennen.",
		textAskDeleteGroup:     "Die Gruppe %s löschen.",
		textMembersSudoNote:    "Administratoren bekommen oder verlieren damit den Zugang zu qhost (Gruppe qhost).",
		textRiskGroupRoot:      "Die Gruppe root gehört zum Konto root und zu Systemdateien.",
		textRiskGroupDocker:    "Mitglieder der Gruppe docker nutzen Docker, und das ist praktisch root.",
		textRiskRename:         "Konfigurationen wie sudoers sprechen eine Gruppe über ihren Namen an und behalten den alten; sie wirken nicht mehr, bis sie ebenfalls geändert sind.",
		textRiskMembers:        "Pakete oder Dienste können von einem Systemkonto dieser Änderung abhängen.",
		textGroupKeepName:      "%s behält ihren Namen: Adminrechte und der Zugang zu qhost hängen daran.",
		textGroupKeepDelete:    "%s wird nie gelöscht: Adminrechte und der Zugang zu qhost hängen daran.",
		textGroupPrimary:       "%s ist die primäre Gruppe von %s und lässt sich nicht löschen.",
		textNoMemberChange:     "Es gibt keinen Benutzer, der zu %s hinzukommen oder daraus entfernt werden könnte.",

		textFieldUserName:   "Name",
		textHintUserName:    "Kleinbuchstaben, Ziffern, _ und -, höchstens 32.",
		textFieldInitial:    "Startpasswort",
		textHintInitial:     "Beim Tippen verborgen. Es erscheint nie und steht in keiner Befehlszeile.",
		textFieldNewSecret:  "Neues Passwort",
		textHintNewSecret:   "Beim Tippen verborgen. Es erscheint nie und steht in keiner Befehlszeile.",
		textFieldRepeat:     "Wiederholen",
		textHintRepeat:      "Dasselbe Passwort noch einmal.",
		textFieldMust:       "Beim Login ändern",
		textHintMust:        "Der Benutzer muss beim nächsten Login ein neues Passwort wählen.",
		textFieldAdmin:      "Admin",
		textHintAdmin:       "Administrator mit sudo und Zugang zu qhost (Gruppen sudo und qhost).",
		textFieldDocker:     "Docker",
		textHintDocker:      "Zugang zu Docker (Gruppe docker).",
		textFieldGroups:     "Weitere Gruppen",
		textHintGroups:      "Leertaste wählt eine Gruppe.",
		textFieldKeys:       "SSH-Schlüssel",
		textHintKeys:        "Optional. Öffentliche Schlüssel einfügen, einer je Zeile. Sie werden nicht angezeigt.",
		textFieldKey:        "SSH-Schlüssel",
		textHintKey:         "Einen öffentlichen Schlüssel im Format authorized_keys einfügen. Er wird nicht angezeigt.",
		textFieldKeyOf:      "Schlüssel",
		textHintKeyOf:       "Der zu entfernende Schlüssel, nach seinem Fingerabdruck.",
		textFieldAddTo:      "Hinzufügen zu",
		textHintAddTo:       "Leertaste wählt eine Gruppe, auch mehrere auf einmal.",
		textFieldRemoveFrom: "Entfernen aus",
		textHintRemoveFrom:  "Leertaste wählt eine Gruppe, auch mehrere auf einmal.",
		textFieldHome:       "Home",
		textHintHome:        "Das Löschen des Homes lässt sich nicht rückgängig machen.",
		textHomeKeep:        "Home und Dateien behalten",
		textHomeDelete:      "Home und Mail-Spool löschen",

		textProblemUserName:       "Gib einen Namen aus Kleinbuchstaben, Ziffern, _ und - ein, höchstens 32, nicht root.",
		textProblemUserTaken:      "Diesen Benutzer gibt es schon.",
		textProblemSecretEmpty:    "Gib ein Passwort ein.",
		textProblemSecretBad:      "Das Passwort darf bis zu %d Bytes haben, ohne Steuerzeichen.",
		textProblemSecretMismatch: "Die beiden Passwörter sind verschieden.",
		textProblemKeyBad:         "Kein öffentlicher Schlüssel im Format authorized_keys (je eine Zeile, ohne Optionen).",
		textProblemKeyTwice:       "Ein Schlüssel steht doppelt da.",
		textProblemKeysMax:        "Höchstens %d Schlüssel.",
		textProblemKeyOne:         "Füge genau einen Schlüssel ein.",
		textProblemTickOne:        "Wähle mindestens eine Gruppe.",

		textChgEnable:       "Zugang zu qhost geben",
		textChgAdminOn:      "Zum Administrator machen",
		textChgAdminOff:     "Administratorrechte nehmen",
		textChgDockerOn:     "Zugang zu Docker geben",
		textChgDockerOff:    "Zugang zu Docker nehmen",
		textChgAddGroups:    "Zu Gruppen hinzufügen",
		textChgRemoveGroups: "Aus Gruppen entfernen",
		textChgSecret:       "Neues Passwort setzen",
		textChgAddKey:       "SSH-Schlüssel hinzufügen",
		textChgRemoveKey:    "SSH-Schlüssel entfernen",
		textChgLock:         "Sperren",
		textChgUnlock:       "Entsperren",

		textAskCreateUser:   "Den Benutzer %s mit einem Startpasswort anlegen, das beim ersten Login zu ändern ist.",
		textAskEnable:       "Dem Administrator %s Zugang zu qhost geben.",
		textAskAdminOn:      "%s zum Administrator mit Zugang zu qhost machen.",
		textAskAdminOff:     "%s die Administratorrechte und den Zugang zu qhost nehmen.",
		textAskDockerOn:     "%s Zugang zu Docker geben.",
		textAskDockerOff:    "%s den Zugang zu Docker nehmen.",
		textAskAddGroups:    "%s zu %s hinzufügen.",
		textAskRemoveGroups: "%s aus %s entfernen.",
		textAskSecret:       "Ein neues Passwort für %s setzen.",
		textAskSecretMust:   "Ein neues Passwort für %s setzen, das beim nächsten Login zu ändern ist.",
		textAskAddKey:       "Den SSH-Schlüssel zu %s hinzufügen.",
		textAskRemoveKey:    "Den SSH-Schlüssel %s von %s entfernen.",
		textAskLock:         "%s sperren: Danach ist kein Login mehr möglich, weder mit Passwort noch mit SSH-Schlüssel.",
		textAskUnlock:       "%s entsperren.",
		textAskDeleteKeep:   "Den Benutzer %s löschen. Home und Dateien bleiben.",
		textAskDeleteHome:   "Den Benutzer %s mit Home und Mail-Spool löschen.",

		textHostPowerNote: "Admin- und Docker-Rechte erlauben praktisch alles auf diesem Host.",
		textLockStays:     "Laufende Sitzungen bleiben offen, bis sie enden.",
		textDeleteKeeps:   "Ein Benutzer mit laufenden Prozessen wird nicht gelöscht.",
		textDeleteFinal:   "Das lässt sich nicht rückgängig machen: Das Home %s und der Mail-Spool werden mit allen Dateien gelöscht.",
		textHomeSystem:    "%s ist ein Systempfad.",
		textHomeHolds:     "%s enthält das Home von %s.",
		textHomeLink:      "%s ist ein Link.",
		textHomeNotDir:    "%s ist kein Verzeichnis.",
		textHomeOwner:     "%s gehört %s.",
		textHomeUnread:    "Was %s sonst enthält, ließ sich nicht prüfen: %s.",
		textRiskRoot:      "root ist das Konto mit allen Rechten auf diesem Host.",
		textRiskAccount:   "Pakete oder Dienste können von diesem Systemkonto abhängen.",
		textRiskGroup:     "Pakete oder Dienste können von einer Systemgruppe dieser Änderung abhängen.",
		textSelfAdmin:     "Du kannst dir die Administratorrechte nicht selbst nehmen.",
		textSelfLock:      "Du kannst dich nicht selbst sperren.",
		textSelfDelete:    "Du kannst dich nicht selbst löschen.",
		textLastAdmin:     "%s ist der letzte Administrator mit Zugang zu qhost und bleibt es.",
		textNoKeys:        "%s hat keinen SSH-Schlüssel.",

		textTitleHostChange:  "Host ändern",
		textTitleHostResult:  "Host",
		textAnswerRunChange:  "ja, mit sudo",
		textHostReversible:   "Läuft mit sudo, steht im Journal und lässt sich rückgängig machen.",
		textHostIrreversible: "Läuft mit sudo. Das lässt sich nicht rückgängig machen.",
		textHostDone:         "Erledigt: %s",
		textHostDoneUndo:     "Journaleintrag %s. System > Änderungen nimmt ihn zurück.",
		textHostDoneFinal:    "Diese Änderung lässt sich nicht rückgängig machen.",
		textHostRefused:      "Nicht geändert: %s",
		textHostRefusedNote:  "Eine Regel des Hosts hat die Änderung abgelehnt, bevor etwas geschah.",
		textHostRestored:     "Fehlgeschlagen, vorheriger Zustand zurückgestellt: %s",
		textHostRestoredNote: "Die Änderung oder ihre Prüfung ist fehlgeschlagen, der vorherige Zustand ist wieder da.",
		textHostFailed:       "Fehlgeschlagen: %s",
		textHostFailedNote:   "Die Änderung ist nicht gelungen. sudo qhost host journal zeigt, was lief.",
		textHostCheckPassed:  "Prüfung: bestanden.",
		textHostCheckFailed:  "Prüfung fehlgeschlagen: %s",
		textHostAuditWarning: "Warnung: %s",
		textHostRegroup:      "Dieser Sitzung fehlt die Gruppe qhost noch. Starte qhost tui neu, um sie zu nutzen.",

		textHostChangeFailed:     "Der Host wurde nicht wie gewünscht geändert",
		textHostReadFailed:       "Lesen als root fehlgeschlagen",
		textHostErrNoSudo:        "sudo ist auf diesem Host nicht installiert, und eine Änderung braucht root. Es wurde nichts geändert.",
		textHostErrStart:         "sudo ließ sich nicht starten: %s. Es wurde nichts geändert.",
		textHostErrDenied:        "sudo hat es nicht ausgeführt: Das Passwort war falsch oder dieses Konto darf sudo nicht nutzen. Es wurde nichts geändert.",
		textHostErrAbortedChange: "Der Lauf wurde unterbrochen, bevor er ein Ergebnis meldete. sudo qhost host journal zeigt, was geschah.",
		textHostErrAbortedRead:   "Der Lauf wurde unterbrochen. Es wurde nichts geändert.",
		textHostErrExitChange:    "Der Befehl endete mit Exit-Code %d ohne Bericht. sudo qhost host journal zeigt, was geschah.",
		textHostErrExitRead:      "Der Befehl endete mit Exit-Code %d ohne Antwort. Es wurde nichts geändert.",
		textHostErrOutput:        "Die Antwort von qhost ließ sich nicht lesen. Es wurde nichts geändert.",
		textHostErrNoCredential:  "sudo hält dein Passwort nicht mehr und kann hier nicht erneut fragen, zum Beispiel wegen eines Timeouts von null. Es wurde nichts geändert.",
		textHostErrReported:      "qhost antwortete ohne Bericht: %s. sudo qhost host journal zeigt, ob etwas geschah.",
	}
}
