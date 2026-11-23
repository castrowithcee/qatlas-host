package control

// SourceMoveCommand previews the move of a running Compose project from a folder outside every source into a
// source that exists. It only reads: no file is written, no container is touched and nothing is fetched.
const SourceMoveCommand = "source.move"

// The codes of SourceMoveIssue. A code names one condition, in Blocks or in Warnings.
const (
	// MoveRootStack: the source is one root stack, so no second stack fits.
	MoveRootStack = "root-stack"
	// MoveSourceUnreadable: the stacks of the source cannot be read.
	MoveSourceUnreadable = "source-unreadable"
	// MoveTargetExists: the target folder exists already, or cannot be told to exist.
	MoveTargetExists = "target-exists"
	// MoveTargetOverlaps: the target folder overlaps the stack folder of another source.
	MoveTargetOverlaps = "target-overlaps"
	// MoveTargetName: the folder name cannot be the name of a stack folder.
	MoveTargetName = "target-name"
	// MoveOtherFilesystem: the project folder and the source are on different file systems.
	MoveOtherFilesystem = "other-filesystem"
	// MoveEngineDiffers: the project runs in another Docker engine than the source.
	MoveEngineDiffers = "engine-differs"
	// MoveNameTaken: a stack of that name exists already.
	MoveNameTaken = "name-taken"
	// MoveProjectName: the Compose project name would change.
	MoveProjectName = "project-name-changes"
	// MoveAbsoluteBind: a bind mount names a path inside the old folder as an absolute path.
	MoveAbsoluteBind = "absolute-bind-mount"
	// MoveSecretsInEnv: .env holds values of secret-like keys.
	MoveSecretsInEnv = "secrets-in-env"
	// MoveFolderRejected: the folder cannot be looked at as a project folder (overlaps a source, lies in a repository...).
	MoveFolderRejected = "folder-rejected"
	// MoveNotAProject: the folder is not one Compose project with its compose file at the root.
	MoveNotAProject = "not-a-project"
	// MoveIsRepository: the folder has a repository of its own.
	MoveIsRepository = "project-is-repository"
	// MoveStackUnreadable: the project cannot be read.
	MoveStackUnreadable = "stack-unreadable"
	// MoveFixedTags: images with a fixed tag, which qhost cannot update.
	MoveFixedTags = "fixed-tags"
	// MoveDowntime: the containers are recreated.
	MoveDowntime = "downtime"
	// MoveNotChecked: an engine or a deploy service could not be asked, or the containers were not read.
	MoveNotChecked = "not-checked"
)

// SourceMoveArgs are the arguments of SourceMoveCommand. Path is the folder of the project and Into the folder
// of a source of this service.
type SourceMoveArgs struct {
	Path string `json:"path"`
	Into string `json:"into"`
}

// SourceMoveIssue is a block or a warning: the condition, why it matters and what to do. Items are the keys,
// paths or names the condition is about; they are data of the folder, and never hold a value of an env file.
type SourceMoveIssue struct {
	Code   string   `json:"code"`
	Reason string   `json:"reason"`
	Fix    string   `json:"fix"`
	Items  []string `json:"items,omitempty"`
}

// SourceMoveRunning is a container that runs from the folder of the project, in the engine of the service.
type SourceMoveRunning struct {
	Container string `json:"container"`
	Project   string `json:"project"`
}

// SourceMoveResult answers SourceMoveCommand. Target is the folder the project would get in the source. Project
// is the Compose project name now, from the running containers, else from the folder; NewProject is the name the
// target would give it, empty when it cannot be told. PlannedFiles, Excluded and Ignore are what the commit
// into the source would hold and what it would leave out, as source.inspect lists them for a folder of files,
// relative to the project folder. SourceKind is SourceKindRemote or SourceKindLocal. Running lists the
// containers of the folder in the engine of the service; ContainersChecked and ContainersError say whether the
// engine could be asked. The client adds Engine, the Docker engine of the source, ProjectEngines, the engines in
// which containers of the folder run, and EnginesNotChecked.
//
// A project can be moved when Blocks is empty. Every block and warning names its reason and the fix.
type SourceMoveResult struct {
	Path              string              `json:"path"`
	Into              string              `json:"into"`
	Target            string              `json:"target,omitempty"`
	SourceKind        string              `json:"source_kind,omitempty"`
	Engine            string              `json:"engine,omitempty"`
	Project           string              `json:"project,omitempty"`
	NewProject        string              `json:"new_project,omitempty"`
	Blocks            []SourceMoveIssue   `json:"blocks"`
	Warnings          []SourceMoveIssue   `json:"warnings"`
	PlannedFiles      []string            `json:"planned_files,omitempty"`
	Excluded          []SourceExcluded    `json:"excluded,omitempty"`
	Ignore            *SourceIgnore       `json:"ignore,omitempty"`
	Running           []SourceMoveRunning `json:"running,omitempty"`
	ProjectEngines    []string            `json:"project_engines,omitempty"`
	ContainersChecked bool                `json:"containers_checked"`
	ContainersError   string              `json:"containers_error,omitempty"`
	EnginesNotChecked []EngineNotChecked  `json:"engines_not_checked,omitempty"`
}

// SourceFolderCommitCommand commits a project folder that lies in a source and is not versioned yet, with a
// push when the source has a remote. The service plans the files of the folder as the preview of SourceMoveCommand
// does and refuses unless Files is exactly that plan, and unless Head is still the head of the working copy
// (CodeHeadMoved). It writes the lines of the plan into the .gitignore of the folder, which the commit holds.
// The folder is a plain name below the source. A push the remote did not take gives CodePushRejected and
// secret-like files CodeSecrets; a refusal leaves the folder as it was. Only root may run it.
const SourceFolderCommitCommand = "source.commit-folder"

// SourceFolderCommitArgs are the arguments of SourceFolderCommitCommand. Path is the folder of the source,
// Folder the name of the project folder in it, Files the planned files relative to Folder, as the preview lists
// them. SudoUID is the person behind the sudo call, named in the commit message.
type SourceFolderCommitArgs struct {
	Path    string   `json:"path"`
	Folder  string   `json:"folder"`
	Files   []string `json:"files"`
	Head    string   `json:"head"`
	SudoUID uint32   `json:"sudo_uid,omitempty"`
}

// SourceFolderCommitResult answers SourceFolderCommitCommand with the new commit, the committed files relative
// to the source, and the change to the .gitignore of the folder, nil when there was none.
type SourceFolderCommitResult struct {
	Path      string        `json:"path"`
	Folder    string        `json:"folder"`
	Commit    string        `json:"commit"`
	Committed []string      `json:"committed"`
	Ignore    *SourceIgnore `json:"ignore,omitempty"`
}
