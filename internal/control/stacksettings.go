package control

// StackSettings are the settings of a stack. Version must be 1. A member left out takes its default: update
// mode manual, ceiling minor, major stepwise; no backup; containers checked, no HTTP probe; the compose files
// Compose finds itself.
type StackSettings struct {
	Version int                   `json:"version"`
	Compose *StackComposeSettings `json:"compose,omitempty"`
	Update  *StackUpdateSettings  `json:"update,omitempty"`
	Backup  *StackBackupSettings  `json:"backup,omitempty"`
	Check   *StackCheckSettings   `json:"check,omitempty"`
}

// StackComposeSettings lists the compose files in merge order, relative to the stack folder. They must not
// change the project name of the stack.
type StackComposeSettings struct {
	Files []string `json:"files"`
}

// StackUpdateSettings say whether and how far the stack is updated. Mode is automatic or manual, Ceiling
// patch, minor or major, Major direct or stepwise. Trigger is window (default), interval or webhook; Interval
// is the seconds between two registry queries, from 30 to 86400, 60 when left out, and only allowed with the
// trigger interval. Observe is the observation time after an update in
// minutes, from 0 (off) to 1440, 60 when left out. Tags maps an image version variable to a regular
// expression its candidate tags must match; Changelog maps it to a GitHub repository (owner/name).
type StackUpdateSettings struct {
	Mode      string            `json:"mode,omitempty"`
	Trigger   string            `json:"trigger,omitempty"`
	Interval  *int              `json:"interval,omitempty"`
	Ceiling   string            `json:"ceiling,omitempty"`
	Major     string            `json:"major,omitempty"`
	Observe   *int              `json:"observe,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	Changelog map[string]string `json:"changelog,omitempty"`
	// PromoteSource is the stack whose versions 'qhost stack promote <stack>' takes over; not the stack itself.
	PromoteSource string `json:"promote_source,omitempty"`
}

// StackBackupSettings say what is saved before an update: data directories relative to the stack folder,
// named volumes and a command as an argument vector. At least one of them is needed.
type StackBackupSettings struct {
	Paths     []string                `json:"paths,omitempty"`
	Volumes   []string                `json:"volumes,omitempty"`
	Command   []string                `json:"command,omitempty"`
	Retention *StackRetentionSettings `json:"retention,omitempty"`
}

// StackRetentionSettings keep a backup while it is among the newest Keep and younger than MaxAgeDays, for
// each limit that is set; Keep is 7 when neither is set.
type StackRetentionSettings struct {
	Keep       int `json:"keep,omitempty"`
	MaxAgeDays int `json:"max_age_days,omitempty"`
}
