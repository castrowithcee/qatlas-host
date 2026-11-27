package control

// SourceLogCommand lists the latest commits of a bound stack folder that its head reaches, newest first, along the
// first parent only. It only reads and shows no file contents. Root and administrators may run it. A folder no
// source is bound to is refused with CodeNoSuchSource.
const SourceLogCommand = "source.log"

// The bounds of SourceLogArgs.Limit: no limit or 0 means DefaultLogLimit, anything above MaxLogLimit is refused
// with CodeInvalid.
const (
	DefaultLogLimit = 20
	MaxLogLimit     = 200
)

// SourceLogArgs are the arguments of SourceLogCommand. Path is the path of the bound stack folder.
type SourceLogArgs struct {
	Path  string `json:"path"`
	Limit int    `json:"limit,omitempty"`
}

// SourceLogPath is one path a commit changed against its first parent. Kind is ChangeNew, ChangeModified or
// ChangeDeleted; a rename is a deleted and a new path.
type SourceLogPath struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// SourceLogCommit is one commit. ID is the full commit id and Date the author date in RFC 3339. Author, Subject
// and Paths are foreign text with the characters replaced that a terminal would act on. PathsTruncated says
// more paths changed than are listed.
type SourceLogCommit struct {
	ID             string          `json:"id"`
	Author         string          `json:"author"`
	Date           string          `json:"date"`
	Subject        string          `json:"subject"`
	Paths          []SourceLogPath `json:"paths"`
	PathsTruncated bool            `json:"paths_truncated,omitempty"`
}

// SourceLogResult answers SourceLogCommand, newest first. The list stops at a byte budget well below
// MaxResponseBytes, and Truncated says commits were left out, also when the git output limit ended the list.
type SourceLogResult struct {
	Path      string            `json:"path"`
	Commits   []SourceLogCommit `json:"commits"`
	Truncated bool              `json:"truncated,omitempty"`
}

// SourceShowCommand returns the diff of one commit against its first parent. It shows file contents that the
// administrator may not be allowed to read, so only root may run it. A commit that the head does not reach on
// its first-parent history is refused with CodeNotFound, a commit that is no hexadecimal id with CodeInvalid.
const SourceShowCommand = "source.show"

// SourceShowArgs are the arguments of SourceShowCommand. Path is the bound stack folder and Commit a
// hexadecimal commit id of at least 7 digits.
type SourceShowArgs struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
}

// SourceShowResult answers SourceShowCommand. Diff is cut at a fixed size, well below MaxResponseBytes, and
// Truncated says so.
type SourceShowResult struct {
	Path      string `json:"path"`
	Commit    string `json:"commit"`
	Diff      string `json:"diff"`
	Truncated bool   `json:"truncated,omitempty"`
}
