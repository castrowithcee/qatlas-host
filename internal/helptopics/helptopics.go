// Package helptopics holds the short guides that ship inside the binary. The command line prints them as help
// topics.
package helptopics

import "github.com/castrowithcee/qatlas-kit/helptopics"

// All returns the topics in the order a newcomer reads them. The text is plain ASCII with one line per
// paragraph, list item, or command; the kit fits it to the width where it is shown.
func All() []helptopics.Topic {
	return []helptopics.Topic{
		{Name: "qhost", Short: "What qhost is and what it is for", Text: overview},
		{Name: "deploy-setup", Short: "The deploy service as its own service: what the setup creates and why", Text: deployServiceTopic},
		{Name: "credentials", Short: "The deploy keys and stored credentials", Text: credentialsTopic},
		{Name: "repository", Short: "Binding the stacks repository and trusting its host key", Text: repositoryTopic},
		{Name: "sources", Short: "Several stack folders on one host: taking over a folder, ids, deploy keys and unique stack names", Text: sourcesTopic},
		{Name: "stack-capabilities", Short: "What qhost can do with a stack's files, and what qhost stack check reports", Text: stackCapabilitiesTopic},
		{Name: "stack-settings", Short: "Stack settings", Text: stackSettingsTopic},
		{Name: "backups", Short: "Backing up and restoring the data of a stack", Text: backupsTopic},
		{Name: "data-directories", Short: "Missing data directories of a stack: the data-dir-missing finding and how to resolve it", Text: dataDirectoriesTopic},
		{Name: "notifications", Short: "Notification channels: events, types and secrets", Text: notificationsTopic},
		{Name: "proposals", Short: "Proposals: asking for a change of the setup without sudo, the kinds source-add, stack-settings, window and notify-channel, limits and states", Text: proposalsTopic},
		{Name: "containers", Short: "The containers of the host: kinds, states, metrics, start, stop, restart", Text: containersTopic},
	}
}

const overview = `qhost runs as a service on a Linux host and keeps the Docker stacks of that host rolled out and up to date from its stacks repository.

Commands:
- qhost run: run the host service in the foreground until it receives SIGTERM or SIGINT; it serves root and the administrators of the group qhost, checks the administrator right in /etc/group on every request and keeps the action log; the deploy service is started by its own unit with qhost deploy-service run
- qhost deploy-service setup [--confirm] [--yes]: set up the deploy service as its own system user qhost-deploy with the group docker, after showing what it does and the exact commands and asking; only the steps still needed run (needs sudo; see qhost help deploy-setup)
- qhost tui, or qhost without a command at a terminal: open the terminal interface with the areas Stacks, Stack detail, Runs, Settings and Containers; it shows each stack's versions, state, last run and findings, follows the log of a running run, v in Runs opens the log of the service (V chooses its minimum level), and offers update, deploy, pause, pin, resume, start, stop and restart (a stopped stack shows as stopped) and the registry query on the cursor stack or on stacks ticked with space; the stack detail sets and deletes the stack's secrets (value hidden) and restores an older backup; Stacks groups the stacks under a row for each stack folder, and a adds a folder, F fetches the folder on the cursor, x releases it and e generates, stores and shows its deploy key; Settings shows what applies to the whole host or device on four sub-tabs under the tab line (arrow down leads to them, left and right choose one): General sets the preferences of this device, Credentials sets credentials, Maintenance window changes the maintenance window and Notifications adds, tests and removes notification channels; Containers lists every container of the host by own stack, foreign project and single container with state and a detail, and reads again every 5 seconds while it is shown, and S, X and Z there start, stop and restart the container on the cursor or in the detail after a question, also a foreign or single one, and a on a foreign project takes it over as a stack folder (see qhost help sources); the stack detail lists the containers of its stack; o opens the output of the container on the cursor, or in the stack detail of all containers of the stack, which follows once a second and takes a period with T and a search with /; every change comes after a confirmation and needs sudo qhost; without sudo the interface only reads; where the deploy service is not set up the areas around Docker say so and G sets it up after a question (needs sudo qhost)
- qhost status: show the host service and the deploy service, which is shown as not set up until its setup; for the deploy service its version, start time, last poll and the state of each stack (runs without sudo for an administrator in the group qhost)
- qhost key generate|set|show [--dir <path>]: create or store the SSH deploy key of a stack folder and show its public part (show runs without sudo for an administrator in the group qhost; generate and set need sudo)
- qhost repo set <ssh-url> [--dir <path>]: bind a stacks repository to a stack folder, confirm its host key and clone it; a further folder becomes a further source (run with sudo; see qhost help sources)
- qhost repo fetch: fetch new commits of every stack folder without touching the working copies (run with sudo)
- qhost source inspect <path> [--url <ssh-url>]: classify a folder before it is taken over as a stack folder (clone, adopt-clone, local, files or reject), with its permissions, the files and stacks it would bring and container conflicts; changes nothing (run with sudo; see qhost help sources)
- qhost source add <path> [--url <ssh-url>] [--fingerprint <fp>] [--yes]: take over a folder as a stack folder (a clone to make, an existing clone, a repository without a remote or a folder of files, which --url can join with a remote) and give the service the access it needs, after a preview and a confirmation (run with sudo; see qhost help sources)
- qhost source remote <path> <ssh-url> [--fingerprint <fp>] [--yes]: give a local source a remote repository after comparing the folder with it; secret-like files in what would be pushed stop it (run with sudo; see qhost help sources)
- qhost source list: list the bound stack folders with kind, address, head and the state of the last poll (runs without sudo for an administrator in the group qhost)
- qhost source remove <path> [--yes]: release a stack folder after a question; no file is deleted and no container is stopped (run with sudo; see qhost help sources)
- qhost stack list: list the stacks of the working copy with image versions, update mode and whether the stack is stopped (runs without sudo for an administrator in the group qhost)
- qhost stack check [stack]: show per stack of the working copy whether deploy, update, rollback with data, backup and the redaction of secrets work with its files, why not and what to change, and the findings; exit code 1 on errors (runs without sudo for an administrator in the group qhost; see qhost help stack-capabilities)
- qhost stack deploy <stack>: start a manual run of a stack at the commit the working copy has; the stack is named by its Compose project (see qhost help stack-capabilities), and for a stack with the finding project-renamed this decides to start the new project, next to the old one, which keeps running (run with sudo)
- qhost stack runs [stack]: list the recorded runs, newest first (runs without sudo for an administrator in the group qhost)
- qhost stack log <run>: print the log of a run (runs without sudo for an administrator in the group qhost)
- qhost stack resume <stack>: clear the pause a failed rollback left on a stack, and release a halted automatic repair; it starts no run (run with sudo)
- qhost stack pause|unpause <stack>: keep the maintenance window from running a stack, or lift that; unrelated to the pause of a failed rollback (run with sudo)
- qhost stack pin|unpin <stack>: hold the image versions of a stack, so the maintenance window and qhost stack update leave them alone, or release them (run with sudo)
- qhost stack start|stop|restart <stack> [--yes]: start, stop or restart the containers of a stack after a question; a stop keeps the stack stopped against new commits and the maintenance window until a start, and qhost status and stack list show it (run with sudo; see qhost help containers)
- qhost window show|set: show the maintenance window, in which the service checks the registries and updates the stacks in automatic mode, or change its weekdays, start, duration and time zone; the running service follows at once (show runs without sudo for an administrator in the group qhost; set needs sudo)
- qhost stack update <stack>... [--version <v>]: update the image versions of stacks, each version step a commit of the stack's .env pushed to the branch and a run with backup, check and rollback (run with sudo)
- qhost stack updates [stack] [--refresh]: show the current and the available image versions per stack and variable, with newer major versions flagged as not automatic; --refresh asks the registries now (without --refresh it runs without sudo for an administrator in the group qhost; --refresh needs sudo)
- qhost stack changelog <stack>: show the release notes between the current and the available version of each variable with an update; qhost stack updates --changelog shows them for all stacks (run with sudo, as it asks GitHub)
- qhost stack backup create|list <stack>, qhost stack backup restore <stack> <backup> [--yes]: back up, list and restore the data of a stack (list runs without sudo for an administrator in the group qhost, the rest needs sudo; see qhost help backups)
- qhost stack datadir <stack> [--owner UID:GID] [--mode 0750] --yes: create the missing data directories of a stack and deploy it (run with sudo; see qhost help data-directories)
- qhost stack secret set <stack> <KEY>, qhost stack secret list <stack>, qhost stack secret delete <stack> <KEY>: write, list by key and remove the secrets in a stack's secrets.env; the value is read without echo at a terminal or from standard input, never from an argument, and nothing is redeployed (list runs without sudo for an administrator in the group qhost, set and delete need sudo)
- qhost credential set|list|delete: manage registry tokens and notification credentials (list runs without sudo for an administrator in the group qhost, set and delete need sudo)
- qhost notify list|add|remove|test: manage the notification channels (webhook, Slack, Telegram, e-mail) and send a test message through one (list runs without sudo for an administrator in the group qhost, the rest needs sudo; see qhost help notifications)
- qhost proposal submit|list|show|withdraw: ask for a change of the setup (a stack folder, the settings of a stack, the maintenance window or a notification channel), without sudo for an administrator in the group qhost; nothing on the host changes. qhost proposal approve|reject: decide on a proposal, only root at a terminal (see qhost help proposals)
- qhost update [version], qhost update status: update qhost itself to the latest stable release or to a given newer one from its signed GitHub releases and restart the service; status shows the installed version and the latest release and changes nothing (update runs with sudo and needs --confirm; stack updates are qhost stack update)
- qhost container list [--metrics], qhost container show <name>: show every container of the host, own stacks, foreign Compose projects and single containers, with state; changes nothing (runs without sudo for an administrator in the group qhost; see qhost help containers)
- qhost container logs <name>|--stack <stack> [--since T] [--until T] [--tail N] [--follow]: show the output of a container or of a whole stack (runs without sudo for an administrator in the group qhost; see qhost help containers)
- qhost service log [--since T] [--level debug|info|warn|error] [--tail N] [--follow] [--source S]: show the log of the qhost service itself, with start and stop, maintenance windows, source calls, warnings and errors; the service keeps a bounded copy of its log with secret values masked (runs without sudo for an administrator in the group qhost; the terminal interface shows it with v in Runs)
- qhost audit [--since T] [--until T] [--action A] [--result success|failure] [--limit N]: show the action log of the host service, newest first: one entry per request on its channel and per change of the host, never with an argument or a secret value. qhost audit verify: check the hash chain of the action log day by day; a broken chain ends with exit code 1 (both run without sudo for an administrator in the group qhost)
- qhost diagnose [--out <path>]: write a diagnostic package, an archive (tar.gz) with a text file and a JSON file per part, with the versions, the state of the service and the sources, the stacks and their check, the last 20 runs with the logs of the 5 newest and of every failed one, the service log of the last 24 hours and the list of the containers, but no container logs, env files or compose files; secret values are masked by the service, a part that cannot be collected is in the package with its reason. Without --out it goes to the folder qhost in your home; the file is new, mode 0600 and yours. It holds the host name and paths: check it before you pass it on (runs without sudo for an administrator in the group qhost)
- qhost container start|stop|restart <name> [--yes]: start, stop or restart one container of the host after a question (run with sudo; see qhost help containers)
- qhost version: print the version
- qhost help <topic>: show a help topic`

const deployServiceTopic = `qhost runs as two services. The host service runs as the system user qhost without access to the container engine. The deploy service rolls out the stacks and runs as the system user qhost-deploy in the group docker, which is practically root, with its own channel in /run/qhost-deploy and its own state in /var/lib/qhost-deploy. The deploy service cannot read the state of the host service or reach its channel.

The installer sets up the host service only. The deploy service exists after a person sets it up:
- qhost deploy-service setup [--confirm] [--yes]: run as root, it shows what it does and the exact commands, asks, and then creates the system group and user qhost-deploy without login, adds the user to docker, writes /etc/tmpfiles.d/qhost-deploy.conf for the channel directory and the unit qhost-deploy.service, and enables and starts it. The user never joins the group qhost. A repeated setup changes nothing, and one that stopped at a step continues from there. --confirm and the question (--yes) are needed as for qhost update.
- The terminal interface offers the same with G, after a question, when it runs as root; without sudo it says that it needs sudo qhost.
- Until then the commands that need the deploy service end with exit code 3 and the message that it is not set up, and qhost status shows it as not set up.
- qhost update replaces the unit of the deploy service and restarts it only where it is set up.

The setup needs the groups docker and qhost: install the container engine and qhost with its installer first.`

const credentialsTopic = `The service keeps its own secrets in a private store that only the service user can read. Listing the credentials runs without sudo for an administrator in the group qhost; setting and deleting need sudo, because they change the service.

Deploy keys:
- Every remote stack folder has its own key, because a git platform accepts a deploy key in one repository only. The first folder uses the key without --dir; every further one needs its own before qhost repo set binds it.
- qhost key generate [--dir <path>]: create an Ed25519 key in the service for the stack folder and print its public part and SHA256 fingerprint; the folder need not be bound yet
- qhost key generate --replace: replace an existing key; without --replace an existing key is kept and the command refuses
- qhost key set [--dir <path>] [--file <path>]: store an existing key, for example from a password manager, instead of generating one; paste it at the prompt (hidden, ends with the END line), pipe it on standard input, or name a key file. It must be an OpenSSH private key without passphrase, Ed25519 or RSA of at least 3072 bits, at most 16 KiB. It prints the public part like generate; --replace replaces an existing key.
- qhost key show [--dir <path>]: print the public part again
- Enter the public part as a deploy key in the git platform of that folder's repository. A folder whose key pushes (a version change or revert, qhost source remote) needs write access to the repository. The private key never leaves the service.

Credentials:
- qhost credential set <name>: store a registry token or notification credential; the value is read without echo at a terminal, or from standard input when piped, and never from an argument
- qhost credential list: show names and modification times, never values
- qhost credential delete <name>: remove a credential
- A name has 1 to 64 characters: lowercase letters, digits, '.', '_' or '-', starting with a letter.
- Registry login for update checks: name the credential "registry." plus the registry host in lowercase with ':' written as '-' (Docker Hub: registry.docker.io), with the value <user>:<token>. Without such a credential the registry is asked anonymously.
- GitHub token for changelogs: name the credential "github.com", with the token alone as value. It is sent only to the GitHub API and raises the rate limit; without it GitHub is asked anonymously.
- A stored value is not shown again and is removed from the service's log output.`

const sourcesTopic = `A host can deploy from several stack folders, each a source with its own clone, engine state, poll and deploy key. Commands on a stack need no source: a stack is named by its Compose project, and the service finds its folder.

Sources:
- qhost repo set <ssh-url> --dir <path> binds a further folder next to those bound earlier; a bound folder is never rebound to another URL, and folders may not lie inside each other (see qhost help repository).
- The id of a source comes from its path and never changes: /opt/stacks becomes opt-stacks, /srv/services/stacks becomes srv-services-stacks.
- Each subfolder of a source with a compose file is a stack. When the root of the source holds a compose file, found as in a subfolder, the source is exactly one stack instead: the whole folder except .qatlas/, named by its Compose project, else by the folder's name. Its subfolders are then part of it, and those with a compose file of their own get the finding ignored-stack-folder.
- Every remote source has its own deploy key: qhost key generate --dir <path> or qhost key set --dir <path> before qhost repo set (see qhost help credentials).
- A source whose fetch or poll fails keeps its state; the other sources go on. qhost status shows each source with its last poll, error and commits, and each stack with its source.
- The maintenance window and qhost stack updates cover the stacks of every source.

Taking over a folder:
- qhost source inspect <path> [--url <ssh-url>] classifies a folder and changes nothing: no file, permission, repository or container is touched, and no value of an env file is shown. --url is the SSH address to clone, or the one the folder must have as origin. It exits 0 for every case, a rejection included; read the case, or the case field of --output json.
- clone: the folder is missing or empty and would get a clone of the URL; without --url the answer says an address is needed.
- adopt-clone: the folder is a clone with an origin remote, and is taken over as it is; a --url must equal that origin.
- local: the folder is a repository without a remote and runs locally, never pushed. A --url is rejected here, as for a folder with files.
- files: the folder holds files and no repository. The answer lists the files a first commit would take and what it would leave out with the reason: bind-mount (the data folder of a stack), too-large, not-regular (not a plain file or folder), unreadable and nested-repo (a folder with a repository of its own). source add takes it over (see below).
- reject: the folder cannot be taken over. The answer gives the reason and what to do, for example a symbolic link in the path, a folder inside another repository or overlapping another source, a repository with remotes but no origin, an address that differs from the origin, or a folder on a read-only file system.
- Every answer except a rejection shows the owner, group and mode of the folder (or of its nearest existing parent while it is missing), whether the service can write there, and whether permissions would have to be set. For a repository it also says whether the service can write its .git folder.
- For each stack the folder would bring it shows the functions and findings as qhost stack check does, and the containers of other Compose projects that already run in the stack's folder: deploying the stack would create duplicates of them. When the stacks or the container engine cannot be read, the answer says why and the rest still holds.

Source add:
- qhost source add <path> [--url <ssh-url>] [--fingerprint <fp>] [--yes] inspects the folder, shows the case and the changes it would make, and applies them only after you confirm: at a terminal by answering the question, without a terminal with --yes. A rejected folder is refused and nothing changes.
- clone and adopt-clone work like qhost repo set (see qhost help repository): the host key is confirmed at the terminal or with --fingerprint, and the folder needs its deploy key. --yes never confirms a host key.
- local binds the repository without a remote and without a deploy key. The service deploys the commits that people make there and rolls a failing one back with a revert commit in the same folder; nothing is pushed until qhost source remote gives it a remote.
- files shows the list of files and what is left out with the reason (the first 50 of each), and starts a repository only after you confirm: git init with branch main and core.sharedRepository=group, the files left out only in .git/info/exclude (never .gitignore), a first commit of the listed files as qhost, and no file in the folder written. The folder is then bound as local, or as remote with --url (see below). Permissions are set for the folder, the folders of listed files and the listed files. Before it changes anything, source add looks at the folder again and stops if the list or the left-out files differ from what was shown. Files left out are not in the repository: one a stack needs to read stays in the folder and must be readable by the service.
- Permissions are set only when the service cannot write the folder or its .git, as root and without changing an owner: the group of the service account qhost-deploy for the folder, .git and every folder with versioned files; group write and setgid on those folders; group read and write on versioned files (for files, the listed ones); core.sharedRepository=group. Never changed: the sources of bind mounts and everything below them, unversioned files, symbolic links, files with further hard links and paths outside the folder. An unversioned file the stacks need, such as secrets.env, must therefore already be readable by the service, for example with the group qhost-deploy and mode 0640.
- The folder is then added to the systemd drop-in and the service restarts once, as repo set does.
- qhost source list shows every bound folder with path, id, kind, address (none for a local one), head, last poll, poll error and note.
- qhost source remove <path> [--yes] asks first. It releases only the binding and drops the engine state of the source: no file in the folder is deleted, no container is stopped, and the permissions, the drop-in entry and the deploy key stay, so source add binds the folder again. A source with an active run is refused.

Remote for a folder:
- qhost source remote <path> <ssh-url> [--fingerprint <fp>] [--yes] gives a local source a remote repository. qhost source add <path> --url <ssh-url> does it for a folder of files in one step, but only for a repository that already holds commits; take the folder over without --url first for an empty one. The host key is confirmed as for repo set and --yes never confirms it. The folder needs its deploy key first, also while it is a local source (qhost key generate --dir <path> or qhost key set --dir <path>), with write access to the repository (see qhost help credentials).
- Both compare the folder with the branch of the remote and show the state before anything changes. empty (source remote only): the local history is pushed as the branch. equal (the same files): the history of the remote is taken over and no commit is made; the former local history stays under refs/qhost/local-before-remote. different: the paths that differ are listed (the first 50 and their number), and the state of the host becomes one commit on the remote branch and is pushed, or you cancel. The host never overwrites the remote and nothing is forced.
- After your confirmation the service acts only on the state and commit that were shown and changes nothing if the remote moved. The source is then remote; for a folder of files the repository starts on the history of the remote.
- Secret check: every commit the push would publish is checked for secrets files, env files with values and private keys. A finding stops the command with secrets-found before anything changes. It names commit, path and kind, never a value, and cannot be overridden: clean the history yourself, then run the command again.

Foreign Compose projects:
- A Compose project that was started outside qhost shows in the Containers area of the TUI as a foreign project, with the folder and the Compose and env files its containers were started with. a on its group, one of its containers or its detail opens the adoption: it only inspects the folder, shows the project, the files from the labels and what works with them, and goes on with the questions of the case (local, files or adopt-clone) as qhost source add does. Nothing changes before the last yes.
- The folder becomes a source of exactly one stack: its compose file must lie directly in the folder. The adoption is refused, with the cause, when the containers carry no absolute working directory, when a Compose or env file lies outside that folder, when the inspection rejects the folder or does not find exactly one stack.
- A warning, which does not block, appears when the project name or the Compose or env files of the running containers differ from those a deploy would pass to Compose: the first deploy then creates new containers instead of taking over the running ones.
- Without the TUI, qhost container list shows the folder and the Compose files, and qhost source add <folder> takes the folder over.

Commits of people in a local folder:
- A person commits as a member of the group qhost-deploy (usermod -aG qhost-deploy <user>, then a new login) or as the service account with sudo -u qhost-deploy git -c safe.directory=<path> .... The folder belongs to another user, so git refuses it until it is marked safe; a group member does that once with git config --global --add safe.directory <path>.
- A person who creates new folders should work with umask 002: a folder made with the usual umask 022 is not group-writable, and the service could not write into it when a revert has to restore files there.

Stack names:
- Compose tells stacks apart only by project name, so a name is deployed from one source alone. When two sources list the same name, the stack already deployed keeps it, else the one of the source bound first; the other gets the finding name-collision and is never deployed until one of them gets another project name.
- qhost status lists such a stack as source.<id>.stack.<name>; the stack that holds the name stays stack.<name>.

Earlier installations:
- A host bound to one repository by an earlier version keeps running: at its first start the service makes the binding its first source, moves the engine state below it and keeps the existing deploy key. Nothing needs to be done.`

const repositoryTopic = `The service clones the stacks repository of the host over SSH with its own deploy key and checks the server with its own known_hosts file; the system's SSH configuration and known hosts are not used. Host key checking is always strict.

Binding:
- qhost key generate (or qhost key set for a key you already have), then enter the public key as a deploy key with write access in the git platform; qhost pushes its version and revert commits with it
- qhost repo set <ssh-url> --dir <path>: the URL is user@host:path or ssh://[user@]host[:port]/path; --dir names the stack folder, is required and has no default
- On first contact the host key and its SHA256 fingerprint are shown. At a terminal you confirm it by answering the question (default no). Without a terminal, pass the expected fingerprint: --fingerprint SHA256:...
- Compare the fingerprint with the one the git platform publishes before you confirm it.
- The stack folder is any absolute path, for example on another disk; each host chooses its own. It must be empty or already a clone of the same URL. Another folder becomes a further stack folder next to the earlier ones, with its own deploy key from qhost key generate --dir <path> or qhost key set --dir <path>; a bound folder is never rebound to another URL.
- A folder that does not exist is created only after you confirmed its exact path: answer the question at a terminal, or pass --create without one. It gets the owner qhost-deploy:qhost-deploy and mode 0755. Neither the installer, the service nor an update creates a stack folder.
- The service runs sandboxed and may write only where the systemd drop-in /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf allows it. repo set adds the folder there as ReadWritePaths=-<path> (entries are only ever added), reloads systemd and restarts the service once. A bound folder without that entry, as after an update from an older release, is added when you next run qhost as root.
- The same steps are offered by sudo qhost in the Settings area.
- qhost repo set is the short form of qhost source add for cloning into an empty or missing folder and for taking over an existing clone; source add also binds a repository without a remote (see qhost help sources).

Afterwards:
- qhost repo fetch: fetch new commits into the remote-tracking branch and report whether the working copy is behind; the working copy itself is not changed
- qhost stack list: list each stack with its image version variables, their values and its update mode (none without settings)
- A host key that differs from the pinned one aborts every command. The pinned key is never replaced by a command; verify the change with the git platform's operator first.`

const stackSettingsTopic = `The settings of a stack say how qhost updates, backs up and checks it. They live in the stacks repository as .qatlas/qhost/<stack>.yaml in its root, one file per stack, named by the stack's Compose project name. A stack without settings is deployed but never updated.

The settings are shown and changed only in the terminal interface or with qhost stack settings <stack>, qhost stack settings set <stack> and qhost stack settings remove <stack>; nobody edits the file by hand. The stack detail of the terminal interface has the form Update rules (key e): mode, ceiling, major jump, the observation time in minutes (0 off) and, per image version variable, a tag filter and a changelog repository. The form Backup (key b) sets the data directories, the volumes ticked from the named volumes of the stack's compose files (a saved volume the files no longer have stays and is marked), the command as a list of arguments, and the retention (keep, maximum age in days). The form Compose files (key C) sets the compose files as a list that can be reordered, at most 16; an empty list removes the entry and leaves the files to the stack's own detection. The form Check (key k) sets the container check on or off and the HTTP check: with it on, the address, the expected status (100 to 599, 200 by default) and the timeout (1s to 10m, 10s by default); with it off the three fields are ignored and the section holds no probe. Each form saves its own section only: saving asks first, commits, and in a remote stack folder pushes, and leaves the other sections as they are. Remove deletes the settings file by a commit, with all its sections, and leaves the stack deployed but never updated; the stack detail offers it as the action Remove settings (key E) after a question marked as far-reaching, and cancelling changes nothing. A stack without settings is reported and nothing is committed. qhost checks them against the stack, writes the file, commits it and pushes the commit. A settings commit does not touch the stack folder and starts no run; the settings act on the next update or deploy. The file is read as data only: unknown fields, a missing version, or a path that is absolute or leaves the stack folder are rejected.

Fields (all optional except version):
- version: 1
- compose.files: the compose files of the stack as a list of relative paths inside the stack folder, in merge order like COMPOSE_FILE; it takes precedence over COMPOSE_FILE in .env and over the default names
- update.mode: manual (default) or automatic
- update.ceiling: patch, minor (default) or major; the largest version step taken; versions are MAJOR.MINOR.PATCH (SemVer or CalVer), and with CalVer the first part, for example the year, counts as major
- update.major: stepwise (default, one major version at a time) or direct
- update.observe: observation time in minutes after an update by qhost (default 60, 0 off, at most 1440); a failure of the stack in it rolls the update back (see qhost help containers); a commit that did not come from qhost starts none; qhost status shows a running one as observe with its end and the update commit
- update.tags.<VARIABLE>: regular expression a candidate tag must match; VARIABLE is the image version variable in .env
- update.changelog.<VARIABLE>: GitHub repository (owner/name or a github.com address) whose releases are shown as changelog; it takes precedence over the image label org.opencontainers.image.source
- backup.paths: data directories below data/ in the stack folder, for example data/db; other paths are refused when a backup runs
- backup.volumes: volume keys of the Compose file (the top-level volumes: names), not Docker volume names
- backup.command: a command as a list of arguments, never a shell line (see qhost help backups)
- backup.retention.keep: number of newest backups to keep (default 7 when no retention is set)
- backup.retention.max_age_days: age limit in days; a backup is kept only while it satisfies every limit that is set
- check.containers: require all containers to be running (default true)
- check.http.url: http or https URL to probe
- check.http.status: expected status code (default 200)
- check.http.timeout: probe timeout from 1s to 10m (default 10s)
- backup needs at least one of paths, volumes, command

Example:
version: 1
update:
  mode: automatic
  ceiling: minor
  tags:
    DEMO_APP_VERSION: '^[0-9]+\.[0-9]+\.[0-9]+$'
backup:
  paths: [data/db]
  retention: {keep: 5}
check:
  http: {url: 'http://localhost:8080/health', status: 200, timeout: 5s}

Image versions stand as variables in the stack's .env, for example image: registry.example.test/demo/app:${DEMO_APP_VERSION}. A fixed or floating tag such as latest leaves the image deploy only; qhost stack check names it (see qhost help stack-capabilities).`

const backupsTopic = `A backup saves the data of one stack while the stack is stopped, so the copy is consistent. What is saved is listed in the backup section of the stack settings (see qhost help stack-settings). Backups are kept in the service's state directory under backups/<stack>/<backup>, readable by the service user only.

Commands (list runs without sudo for an administrator in the group qhost, the rest needs sudo; the service must run):
- qhost stack backup create <stack>: stop the stack if it runs, save every listed data directory and volume, run the backup command, start the stack again, then remove backups beyond the retention limits
- qhost stack backup list <stack>: the backups of a stack, newest first, with time, deployed commit, size and number of parts
- qhost stack backup restore <stack> <backup> [--yes]: replace the data with the content of one backup; the stack is stopped and started again if it ran

How it works:
- A data directory or volume is packed into a tar archive with owners and permissions by a short-lived helper container without network. The helper image is set with qhost deploy-service run --backup-image; it needs sh, find, rm and a tar that knows --numeric-owner.
- backup.command runs on the host, not in a container, as a program in the search path or an absolute path, in the stack folder and without a shell; its standard output is saved as command.out in the backup. A relative path such as ./dump.sh is refused.
- The backup records the stack, the time, the commit deployed at that time and the image versions.
- A failed backup is removed and a stack that ran is started again.
- A backup and a deployment of the same stack never run at the same time.
- Every deployment run of a stack whose settings have a backup section makes a backup after it stopped the stack; when the run is rolled back, that backup is restored. These backups count for retention like any other.

Retention:
- backup.retention.keep and backup.retention.max_age_days apply after each successful backup; a backup is kept only while it satisfies every limit that is set. The newest backup is never removed.

Restore:
- The backup id comes from qhost stack backup list; the newest is never chosen for you.
- At a terminal the command asks first (default no). Without a terminal, and with --agent, it refuses unless --yes is given.
- Every archive is read through before the first target is emptied. Each target is then emptied and unpacked inside the helper container, on the mounted path only.
- A backup that holds a data directory or volume the settings no longer list is refused as a whole.
- The command output is not restored; the settings have no restore command.
- A restore that fails after the first target was emptied leaves the stack stopped, because it would run on incomplete data.
- A volume must exist before it is backed up or restored; deploy the stack first.`

const stackCapabilitiesTopic = `qhost stack check reads the working copy of the stacks repository and reports for each stack which functions of qhost work with its files, which do not, why, and what a person would change. qhost never changes these files itself. The TUI shows the same report at the end of the stack detail.

Functions, in this order:
- deploy: works when qhost can read the stack: its compose files exist and are safe, its settings are valid, secrets.env, if there is one, is a regular file of at most 1 MiB, and every image version variable has a value. An .env with lines qhost cannot parse still deploys, unless such a line is a COMPOSE_ setting, because Compose reads it itself; the stack is then deploy-only.
- update, with the mode automatic, manual or none: works when the stack deploys, qhost can read .env, at least one image takes its version from a variable in .env whose value has a MAJOR.MINOR.PATCH core (SemVer or CalVer, x.y.z), and the stack settings exist. Without settings the mode is none and updates are not switched on. An image with a fixed tag, a digest, no tag or a floating tag such as latest is deploy only and is left alone; a version that only comes from a default in the compose file, or a variable that is the tag of two different images, stops the update of the whole stack.
- rollback: the rollback with data. A failed run of a stack that was deployed before goes back to the previous commit; it restores the data only when the stack settings have a backup section, from the backup the run made before the change.
- backup: works when the stack settings have a backup section, every backup path lies below data/, and the stack name has lowercase letters, digits, - and _ only and at most 63 characters (see qhost help backups).
- redact: the values of secrets.env and of the .env keys whose name contains PASSWORD, SECRET, TOKEN or KEY are removed from the service log, the run logs and the notifications. It does not work for a secrets.env that is a link or too large, and finds nothing when the stack has neither.

Findings (rule ID, severity):
- missing-compose-file (error): a file named in compose.files or COMPOSE_FILE does not exist
- invalid-compose-file (error): a compose.files or COMPOSE_FILE entry is unsafe or not a regular file
- invalid-rules-file (error): the stack settings are invalid
- unknown-rules-variable (error): the stack settings name an image variable the stack does not have
- unreadable-stack (error, or warning for an .env qhost cannot parse): a file of the stack cannot be read
- unreadable-secrets-env (error): secrets.env is a link or too large
- unset-image-variable (error): an image version variable has no value, so the image has an empty tag
- tracked-secrets-env (error with a remote, else warning): git tracks secrets.env, so its secrets reach the remote
- duplicate-key (warning): a key stands in both .env and secrets.env; keep it in one file
- secrets-env-not-ignored (warning): .gitignore does not cover secrets.env
- tracked-data (warning): git tracks files below data/
- old-rules-file (warning): the stack folder holds a qhost.yaml, which is no longer read; set the stack settings instead and delete it
- ignored-stack-folder (warning): a subfolder holds a compose file but is part of the one stack whose compose file is in the root of the stack folder

A finding is an error only when the stack cannot be deployed or a secret would reach a remote; everything else is a warning or only part of the functions. The exit code is 1 when there is at least one error.

Layout of a stack folder:
- one or more compose files: the list compose.files in the stack settings, else COMPOSE_FILE in .env (entries separated by ':', or by COMPOSE_PATH_SEPARATOR), else the first of compose.yaml, compose.yml, docker-compose.yaml, docker-compose.yml
- .env (optional): non-secret values such as image versions, domains, ports, COMPOSE_FILE and COMPOSE_PROJECT_NAME
- secrets.env (optional): the secrets, set with qhost stack secret set or in the stack detail of the TUI; kept out of git with .gitignore
- data/ (optional): the data directories of the host, kept out of git

A stack is named by its Compose project name, which is not necessarily its folder: COMPOSE_PROJECT_NAME in .env, else the top-level name: of the compose files, else the folder name. This name is used by every command. When the project name of a deployed stack changes, both names get the finding project-renamed and nothing is deployed automatically: the old project keeps running and is not stopped. qhost stack deploy <new name> is the decision to start the new project next to the old one, with new volumes prefixed by the new name; the old project must be stopped by a person.

Secret values are never shown or logged; the check reads only the key names of secrets.env. Compose runs with --env-file for each of .env and secrets.env that exists.`

const dataDirectoriesTopic = `A data directory is a bind mount source inside the stack folder that git does not track, for example ./data/db. If it is missing when a container starts, Docker creates it owned by root, and an image that runs as another user then cannot write to it.

The finding data-dir-missing:
- Before a run touches the stack, the service reads the bind mount sources inside the stack folder from the rendered Compose configuration and finds the user each image runs as. A missing, unversioned directory of a service that does not run as root stops the run with this finding, which lists the paths relative to the stack folder and the owner (UID:GID) the image user suggests, when it is known.
- The finding clears and the same commit runs by itself as soon as all listed directories exist. A manual run (qhost stack deploy) checks again. Nothing is created without you.
- Directories of services that run as root, named volumes, files and tracked paths are not part of the finding.

Resolving it (run with sudo):
- In the terminal interface, open the stack detail, check the owner and mode, and confirm the creation.
- qhost stack datadir <stack> [--owner UID:GID] [--mode 0750] --yes creates the listed directories and then starts a run of the stack. Without --yes it asks at a terminal and otherwise only prints what it would create. The owner comes from --owner or from the finding; the mode is 0750 unless given.
- Existing directories are never changed, missing parents are created like the listed directories, and nothing outside the stack folder is touched. A symlink on the way refuses the creation.
- If you create the directories yourself, set the owner the container user needs; the service takes them as they are.`

const notificationsTopic = `A notification channel receives the events it selected. Channels belong to the host, not to a stack. Commands (list runs without sudo for an administrator in the group qhost, the rest needs sudo; the service must run):
- qhost notify list: the channels with type, events and settings, never their secrets
- qhost notify add <name> --type webhook|slack|telegram|smtp [--events <list>] [settings]: add a channel
- qhost notify remove <name>: remove a channel and its stored secrets
- qhost notify test <name>: send one test message once, without retries; a failure carries the error class of the delivery (auth, permission, not-found, rate-limited, unreachable, timeout, tls, config-invalid, invalid-request or runtime) in its message and exit code

Events (--events takes a comma list; without it a new channel receives all; a stored channel keeps the events it selected):
- succeeded: a run deployed a commit
- failed: a run failed and no rollback was made
- rolled-back: a run failed and the stack was rolled back to the previous state; with cause observation, the stack failed in the observation time after an update and qhost rolled that update back
- rollback-failed: a run failed and the rollback failed too; the stack is stopped and paused
- container-failed: the failure monitor found a container of an own stack failed (see qhost help containers); one message per failure with stack, kind, container and the time since which it failed
- recovered: that failure ended; names the same container and how long it lasted, and whether qhost restarted it
- recovery-failed: three repair attempts did not end the failure (see qhost help containers); sent once, names stack, kind, container, the time since which it failed and the number of attempts; with cause backup-missing, the stack failed in the observation time but the backup of the update is missing, so the update was not rolled back and nothing was changed
- update-available: a newer image version was found for a stack in manual mode, once per version

Channel types and settings:
- webhook: posts JSON (event, stack, run, commit, error, items; for container-failed, recovered and recovery-failed also kind, container and since; recovered and recovery-failed also at; recovered also restarted, recovery-failed also attempts; cause is observation for rolled-back and rollback-failed after a failure in the observation time, backup-missing with error for recovery-failed) to an https URL (plain http only to the local machine), signed with a key. The headers X-Qhost-Event, X-Qhost-Timestamp and X-Qhost-Signature carry the event, the Unix time and the signature over the timestamp and the body.
- slack: posts text to an incoming-webhook URL.
- telegram: sends text to a chat; needs --chat-id, and --base-url for a Bot API server of your own.
- smtp: sends an e-mail; needs --host, --from and --to (comma list or repeated); --port (default 587 with starttls, 465 with tls), --security starttls (default) or tls, and --user for a login. The connection is always encrypted.

Secrets: the webhook URL and its signing key, the Slack URL, the Telegram bot token and the SMTP password. They are never an argument or a flag value. At a terminal they are asked for without echo; piped on standard input they are lines in that order (webhook: URL, then key; smtp: the password only with --user). The service stores them as credentials, shows them nowhere and removes them from its log output.

Delivery runs in the background after the event; a failing channel is retried a few times and never delays or fails a run. A delivery that still fails is noted in the log of its run (qhost stack log <run>) and in the service log.`

const containersTopic = `qhost container list and qhost container show read the containers of the whole host from the container engine and change nothing. qhost container start, stop and restart act on one container. Listing, showing and logs run without sudo for an administrator in the group qhost; start, stop and restart need sudo. The service must run.

Commands:
- qhost container list [--metrics]: every container, grouped; add --output json for the service's result as it is
- qhost container show <name>: one container by name or ID, with its group, state, ports, mounts, networks, labels and the names of its environment variables
- qhost container logs <name>: the output (stdout and stderr) of one container by name or ID; with --stack <stack> the merged output of all containers of an own stack, each line with its container. Not the protocol of a run: that is qhost stack log <run>
- qhost container start|stop|restart <name> [--yes]: act on one container by name or ID, also a foreign or single one

Kinds of a group:
- own: a Compose project that is a stack of the bound stacks repository and was started from that stack's folder
- foreign: any other Compose project; it shows its working directory and compose files
- single: a container that belongs to no Compose project
- conflict: a foreign project that has the name of a stack of this host but was started from another folder; it is not the stack and is marked in the list

Logs: --since and --until take a duration back from now (90s, 15m, 2h) or an RFC 3339 time; --tail shows the last n lines (default 100, at most 5000, 0 for all); --follow asks about once a second for new lines until Ctrl-C (exit code 0). Secret values of the stacks are masked and control characters replaced; --output json prints one object per line. The service answers in pages and the command continues at the time of the last line, so no line is repeated or missed; lines with the same time are never split between pages.

State of a group is the worst state of its containers, in this order: unhealthy, restarting, stopped, starting, ok. The uptime of a group is that of its youngest running container; restarts are summed.

Actions: the command names the container and its kind and asks first (default no); --yes answers the question, and without a terminal --yes is required. In the terminal interface (qhost tui), S, X and Z in the area Containers do the same for the container on the cursor or in the open detail, after a question that names the container and its kind (default cancel) and act by the ID the container has at that moment; they do nothing on a group header. The result shows the state afterwards, and a container that is not ready counts as failed. A stop signals the container and kills it after ten seconds; stopping a stopped or starting a running container changes nothing. After a start or restart the service waits up to a minute until the container runs and, if it has a health check, is healthy; if it is not ready by then the command shows the state and fails. A container of a stack of this host is refused while a run of that stack is active.

Stacks: qhost stack start|stop|restart <stack> [--yes] act on all containers of an own stack of the working copy, ask first like the container actions and are refused while a run of the stack is active. A stop stops the containers without removing them and sets the mark "stopped": new commits of the stacks repository start no run (the stack shows the finding stopped and its pending commit), the maintenance window skips it, and qhost status and qhost stack list show "stopped since <time>". Only qhost stack start clears the mark; it starts the existing containers, never creates any (a stack without containers fails; deploy it with qhost stack deploy), clears the mark once Compose has started them and then runs the stack's check. qhost stack restart restarts the containers and runs the check; on a stack stopped through qhost it is refused with the hint to use start. Stopping a single container with qhost container stop sets no mark. A stopped stack is refused by every command that would run it (qhost stack deploy, qhost stack update, and likewise the maintenance window, which skips it); start it first.

Metrics: --metrics adds CPU (100 per fully used CPU) and memory of the running containers. Reading them takes about two seconds per running container, in parallel and within a time limit, so the command may wait up to two minutes. If a reading fails or the time runs out, the list says that the metrics are incomplete and the container shows none.

Container engine: the service reads the engine's socket, by default /var/run/docker.sock; qhost deploy-service run --docker-socket <path> names another one. If the engine cannot be reached, the service user may not use it, or it does not answer in time, the command says so and shows nothing.

Failure monitor: every 30 seconds the service checks the containers of own stacks and reports a failure once, as the finding container-failed in qhost status and as the notification event container-failed; when the failure ends it sends recovered. The kinds:
- exited: a container exited with a non-zero code or is dead (an exit with code 0, such as an init job, is fine)
- unhealthy: its health check has been failing for more than 60 seconds
- restart-loop: its restart count rose by 3 or more within 5 minutes
- missing: a service that was running at the last good reading is gone
Nothing is checked while a run or an action of the stack is active, nor for stopped (qhost stack stop), paused (either pause) and foreign stacks. A container stopped with qhost container stop is no failure.
Repair: the monitor also repairs a failure by restarts, each under the run slot of the stack. Attempt 1 comes one minute after the failure began and restarts the failed containers; attempts 2 and 3 come 5 and 15 minutes after the previous one and restart the stack. In the observation time after an update by qhost (qhost help stack-settings, update.observe), a failure gets only attempt 1; if it lasts until attempt 2 would be due, qhost rolls that update back instead of attempts 2 and 3: it stops the stack, restores the backup of the update, reverts the update commit and starts and checks the stack again. Data written since the update is lost. A failed rollback stops and pauses the stack; a missing backup halts the repair without any change. If the failure persists after the third, the repair is halted: qhost status shows "repair" as halted, recovery-failed is sent once and no further attempt is made until qhost stack resume releases it. The attempts count until the stack has run an hour without a failure. Stopped (qhost stack stop), paused (either pause) and foreign stacks are not repaired; a stop or pause ends a repair under way, its count stays.

Environment: only the names of the variables are shown, never their values.`

const proposalsTopic = `A proposal asks for a change of the setup. An administrator in the group qhost, who needs no sudo to read, submits it; the service checks and stores it. Submitting changes nothing on the host: the service does not touch the proposed folder, uses no network and runs no git. Only a person decides: root at a terminal approves or rejects it.

Commands submit, list, show and withdraw run without sudo for an administrator in the group qhost:
- qhost proposal submit --kind source-add|stack-settings|window|notify-channel [--file <path>|-]: read the content as JSON from the file or from standard input and print the id of the new proposal. The file is read as the calling user, at most a little over 64 KiB.
- qhost proposal list: list the proposals, newest first, with id, kind, state, submitter and times
- qhost proposal show <id>: show one proposal with its fingerprint, submitter and content
- qhost proposal withdraw <id>: close an open proposal as withdrawn; only its submitter or root may do that

Commands approve and reject need sudo and a terminal; they refuse --agent and input that is no terminal, and there is no --yes:
- sudo qhost proposal approve <id> [--url <ssh-url>]: show the proposal and, below it, its effect. For source-add it is worked out with the same check as qhost source add (case, permissions, conflicts, file list). You confirm the host key of the server yourself, because a proposal never does, add the deploy key if the folder has none, and answer a last question. Right before anything changes the proposal is read again and the effect is worked out again; if either differs from what you saw, nothing is applied and the new effect is shown. The change is then made the way qhost source add makes it. --url replaces the proposed address. The other kinds are described below.
- sudo qhost proposal reject <id> [--reason <text>]: close an open proposal as rejected; the reason is kept at most 300 characters long

Kind source-add proposes a folder as a stack folder (see qhost help sources). Its content is a JSON object with these fields and no others:
- path: the absolute, clean path of the folder (required)
- url: the SSH address of its repository, for example ssh://git.example.test/stacks.git (optional)
Example: {"path": "/srv/stacks", "url": "ssh://git.example.test/stacks.git"}

Kind stack-settings proposes the settings of a stack (see qhost stack settings --help). Its content is a JSON object with these fields and no others:
- stack: the name of a stack the service deploys; an unknown stack is refused when submitting and when approving
- settings: the settings object of qhost stack settings, with the members version (1, required), compose, update, backup and check; it is checked with the same rules as qhost stack settings set
Example: {"stack": "web", "settings": {"version": 1, "update": {"mode": "manual"}, "backup": {"paths": ["data"]}}}
The schema has no secret member, so a secret is not proposable: a proposal with a field the schema does not name is refused, never trimmed. Approving shows the difference to the settings that apply now, every value in full and broken into lines, a backup command marked because it runs on the host later; the settings are then written the way qhost stack settings set writes them (a commit, pushed when the folder has a remote).
Kind window proposes the maintenance window of the host (see qhost window set --help). Its content is a JSON object with these fields and no others, at least one of them; a part left out keeps its value:
- days: weekday names such as ["mon", "wed"], or ["daily"]
- start: the start time HH:MM on the 24-hour clock
- duration: whole minutes such as 2h or 1h30m, from 1m to 12h
- zone: local or an IANA time zone name
Example: {"days": ["mon", "thu"], "start": "02:30", "duration": "90m", "zone": "UTC"}
The values are checked with the same rules as qhost window set, so a wrong weekday, time, duration or zone is refused when submitting, and a field the schema does not name, a secret included, is refused, never trimmed. Approving shows the difference to the window that applies now, every value in full; the window is then set the way qhost window set sets it.
Kind notify-channel proposes a notification channel (see qhost help notifications). Its content is a JSON object with these fields and no others: the fields of qhost notify add without its secret parts.
- name: 1 to 32 characters, lowercase letters, digits or '-', starting with a letter (required); a name that is taken is refused when submitting and when approving
- type: webhook, slack, telegram or smtp (required)
- events: the events the channel receives; none means all
- smtp only: host, port, security (starttls or tls), user, from and to (a list of recipients)
- telegram only: chat_id and optionally base_url
The secret parts are the webhook or Slack address with its token, the signing key of a webhook, the bot token and the SMTP password. The schema has no member for them: a proposal that names secret or key, or any other field the schema does not name, is refused, never trimmed. The fields are checked with the same rules as qhost notify add, without secrets and without any network access, so a wrong type, event or setting of another type is refused when submitting.
Example: {"name": "ops-alerts", "type": "telegram", "events": ["failed", "rolled-back"], "chat_id": "-1001234567890"}
Approving shows the type, the name, the events and every field in full, broken into lines. You then enter the secret parts as qhost notify add asks for them, hidden at the terminal; they are never kept on the proposal or in its result. Right before the channel is added, the proposal is read again and the channels are checked again; if the proposal changed or a channel of that name exists now, nothing is added. The channel is then added the way qhost notify add adds it.
A proposal holds configuration only, never a command, a secret or a host key fingerprint; a field the kind does not name, and an unknown kind, are refused.

Limits: at most 20 open proposals per submitter, at most 64 KiB per proposal, and an open proposal expires after 14 days.

States: open, withdrawn, rejected, approving, applied, failed and expired. An open proposal past its time shows as expired and cannot be approved. The fingerprint identifies exactly the content, and the service accepts an approval only with the fingerprint that was shown. Once accepted, the proposal is approving: it can no longer be withdrawn, rejected or approved again, and it ends as applied or as failed with a message. An approval that gets no result within 15 minutes, for example after an interrupted command, shows as failed; the submitter then submits a new proposal. The reason of a rejection and the message of a failure show in qhost proposal show.

Text of a submitter is shown with control characters replaced; --output json keeps the text as submitted.`
