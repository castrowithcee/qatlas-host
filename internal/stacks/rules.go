package stacks

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

// Update modes, ceilings and major strategies.
const (
	ModeNone      = "none" // no stack settings: deployed, never updated
	ModeManual    = "manual"
	ModeAutomatic = "automatic"

	CeilingPatch = "patch"
	CeilingMinor = "minor"
	CeilingMajor = "major"

	MajorDirect   = "direct"
	MajorStepwise = "stepwise"
)

// Update triggers: what makes qhost check a stack's registries. TriggerWindow is the maintenance window,
// TriggerInterval a regular query every update.interval, TriggerWebhook an external call.
const (
	TriggerWindow   = "window"
	TriggerInterval = "interval"
	TriggerWebhook  = "webhook"
)

// The query interval of the trigger interval, in seconds: its default and its bounds. The lower bound
// follows from the request limits of the registries.
const (
	DefaultIntervalSeconds = 60
	MinIntervalSeconds     = 30
	MaxIntervalSeconds     = 86400
)

// The observation time after an update, in minutes: its default and its upper bound, a day.
const (
	DefaultObserveMinutes = 60
	MaxObserveMinutes     = 1440
)

// Rules are the validated settings of a stack with defaults applied.
type Rules struct {
	Update         Update
	Backup         *Backup // nil: no backup configured
	Check          Check
	PromoteCompare bool // a promotion into the stack compares the image fingerprints with the source's
	// ComposeFiles are the compose files in merge order from compose.files; nil leaves the choice to
	// COMPOSE_FILE and the default names.
	ComposeFiles []string
}

// Update says whether and how far a stack is updated.
type Update struct {
	Mode string
	// Trigger is what makes qhost check the registries for the stack: TriggerWindow, TriggerInterval or
	// TriggerWebhook. Interval is the time between two checks; it is set whatever the trigger is, but only
	// TriggerInterval gives it a meaning. Use Stack.UpdateTrigger for the effective pair.
	Trigger  string
	Interval time.Duration
	Ceiling  string
	// Major is how a major jump is taken: directly, or one major version at a time.
	Major string
	// Observe is how long after an update by qhost a failure of the stack rolls that update back; zero
	// turns it off.
	Observe time.Duration
	// Tags restricts the candidate tags per image variable.
	Tags map[string]*regexp.Regexp
	// Changelog names the GitHub repository ("owner/name") whose releases describe the versions of an image
	// variable. It takes precedence over the image label.
	Changelog map[string]string
	// PromoteSource names the stack this one takes its versions over from; "" when none. Nothing follows it
	// by itself, and the stack need not exist.
	PromoteSource string
}

// Backup describes what is saved before an update. Nothing here is executed by this package.
type Backup struct {
	// Paths are data directories relative to the stack folder.
	Paths   []string
	Volumes []string
	// Command is an argument vector, never a shell line.
	Command []string
	// Keep is the number of newest backups to keep; MaxAgeDays the age limit in days. A backup is kept only
	// while it satisfies every limit that is set.
	Keep       int
	MaxAgeDays int
}

// Check says when a deployed stack counts as healthy.
type Check struct {
	// Containers requires every container of the stack to be running.
	Containers bool
	HTTP       *HTTPCheck
}

// HTTPCheck is an HTTP probe.
type HTTPCheck struct {
	URL     string
	Status  int
	Timeout time.Duration
}

// Settings is the file form of a stack's settings: the schema of .qatlas/qhost/<stack>.yaml and of the
// settings that travel over the service channel. A nil member is absent; ParseSettings validates it and
// applies the defaults.
type Settings struct {
	Version *int             `yaml:"version,omitempty" json:"version,omitempty"`
	Compose *ComposeSettings `yaml:"compose,omitempty" json:"compose,omitempty"`
	Update  *UpdateSettings  `yaml:"update,omitempty" json:"update,omitempty"`
	Backup  *BackupSettings  `yaml:"backup,omitempty" json:"backup,omitempty"`
	Check   *CheckSettings   `yaml:"check,omitempty" json:"check,omitempty"`
	Promote *PromoteSettings `yaml:"promote,omitempty" json:"promote,omitempty"`
}

// ComposeSettings is the compose section of Settings.
type ComposeSettings struct {
	Files []string `yaml:"files,omitempty" json:"files,omitempty"`
}

// UpdateSettings is the update section of Settings.
type UpdateSettings struct {
	Mode      *string           `yaml:"mode,omitempty" json:"mode,omitempty"`
	Trigger   *string           `yaml:"trigger,omitempty" json:"trigger,omitempty"`
	Interval  *int              `yaml:"interval,omitempty" json:"interval,omitempty"`
	Ceiling   *string           `yaml:"ceiling,omitempty" json:"ceiling,omitempty"`
	Major     *string           `yaml:"major,omitempty" json:"major,omitempty"`
	Observe   *int              `yaml:"observe,omitempty" json:"observe,omitempty"`
	Tags      map[string]string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Changelog map[string]string `yaml:"changelog,omitempty" json:"changelog,omitempty"`
	// PromoteSource is the name of the stack whose versions promote takes over by default.
	PromoteSource string `yaml:"promote_source,omitempty" json:"promote_source,omitempty"`
}

// BackupSettings is the backup section of Settings.
type BackupSettings struct {
	Paths     []string           `yaml:"paths,omitempty" json:"paths,omitempty"`
	Volumes   []string           `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Command   []string           `yaml:"command,omitempty" json:"command,omitempty"`
	Retention *RetentionSettings `yaml:"retention,omitempty" json:"retention,omitempty"`
}

const (
	defaultKeep    = 7
	defaultTimeout = 10 * time.Second
)

var (
	stackName    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`) // as backup.ValidStack
	volumeName   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	unknownField = regexp.MustCompile(`field (\S+) not found in type \S+`)
)

// settingsLabel starts every error about the settings of a stack.
const settingsLabel = "stack settings"

// settingsVersion is the format number of the settings file.
const settingsVersion = 1

// readSettings loads the settings of the stack called name from the working copy at root; nil, nil when the
// stack has none.
func readSettings(root, name string) (*Rules, error) {
	data, err := readBounded(root, SettingsPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", settingsLabel, err)
	}
	return parseSettings(SettingsPath(name), data)
}

// ParseSettings decodes and validates settings given as YAML or JSON, which is YAML too: one document, no
// unknown field, no repeated key. Errors start with "stack settings" and name the field path. The checks
// that need the stack itself, such as image variables and compose files, are made when the stack is read.
func ParseSettings(data []byte) (*Rules, error) { return parseSettings("the given settings", data) }

// CheckSettingsFormat refuses settings whose format number this version does not read, without looking at any
// other field. Empty or malformed content passes: the full parse reports it.
func CheckSettingsFormat(file string, data []byte) error {
	var head struct {
		Version *int `yaml:"version"`
	}
	if yaml.Unmarshal(data, &head) != nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	n := 0
	if head.Version != nil {
		n = *head.Version
	}
	return formatnum.Check(file, "version", n, settingsVersion)
}

func parseSettings(file string, data []byte) (*Rules, error) {
	if err := CheckSettingsFormat(file, data); err != nil {
		return nil, fmt.Errorf("%s: %w", settingsLabel, err)
	}
	var raw Settings
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: empty; at least 'version: 1' is required", settingsLabel)
		}
		msg := unknownField.ReplaceAllString(err.Error(), `unknown field $1`)
		msg = strings.TrimPrefix(msg, "yaml: ")
		return nil, fmt.Errorf("%s: %s", settingsLabel, strings.ReplaceAll(msg, "\n", "; "))
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: only one YAML document is allowed", settingsLabel)
	}
	rules, err := validateRules(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", settingsLabel, err)
	}
	return rules, nil
}

// Settings returns the rules in their file form with every default written out, so that equal rules give
// equal settings and ParseSettings gives the rules back.
func (r *Rules) Settings() Settings {
	one := settingsVersion
	s := Settings{Version: &one, Update: &UpdateSettings{
		Mode: ptr(r.Update.Mode), Trigger: ptr(r.Update.Trigger), Ceiling: ptr(r.Update.Ceiling), Major: ptr(r.Update.Major),
		Observe: ptr(int(r.Update.Observe / time.Minute)),
	}}
	if r.Update.Trigger == TriggerInterval {
		s.Update.Interval = ptr(int(r.Update.Interval / time.Second))
	}
	if r.ComposeFiles != nil {
		s.Compose = &ComposeSettings{Files: slices.Clone(r.ComposeFiles)}
	}
	for k, re := range r.Update.Tags {
		if s.Update.Tags == nil {
			s.Update.Tags = map[string]string{}
		}
		s.Update.Tags[k] = re.String()
	}
	if len(r.Update.Changelog) > 0 {
		s.Update.Changelog = maps.Clone(r.Update.Changelog)
	}
	s.Update.PromoteSource = r.Update.PromoteSource
	if b := r.Backup; b != nil {
		bs := &BackupSettings{Paths: slices.Clone(b.Paths), Volumes: slices.Clone(b.Volumes), Command: slices.Clone(b.Command),
			Retention: &RetentionSettings{}}
		if b.Keep > 0 {
			bs.Retention.Keep = ptr(b.Keep)
		}
		if b.MaxAgeDays > 0 {
			bs.Retention.MaxAgeDays = ptr(b.MaxAgeDays)
		}
		s.Backup = bs
	}
	s.Promote = r.promoteSettings()
	s.Check = &CheckSettings{Containers: ptr(r.Check.Containers)}
	if h := r.Check.HTTP; h != nil {
		s.Check.HTTP = &HTTPSettings{URL: h.URL, Status: ptr(h.Status), Timeout: ptr(h.Timeout.String())}
	}
	return s
}

// EncodeSettings returns the file content of the rules: r.Settings() as YAML in block form, indented by two
// spaces. Equal rules give equal bytes.
func EncodeSettings(r *Rules) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(r.Settings()); err != nil {
		return nil, fmt.Errorf("encode the stack settings: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode the stack settings: %w", err)
	}
	return b.Bytes(), nil
}

func ptr[T any](v T) *T { return &v }

func oneOf(path, got string, allowed ...string) error {
	for _, a := range allowed {
		if got == a {
			return nil
		}
	}
	return fmt.Errorf("%s: must be one of %s (got %q)", path, strings.Join(allowed, ", "), got)
}

func validateRules(raw Settings) (*Rules, error) {
	if raw.Version == nil || *raw.Version != settingsVersion {
		return nil, errors.New("version: must be 1")
	}
	r := &Rules{Update: Update{Mode: ModeManual, Trigger: TriggerWindow, Interval: DefaultIntervalSeconds * time.Second, Ceiling: CeilingMinor, Major: MajorStepwise, Observe: DefaultObserveMinutes * time.Minute},
		Check: Check{Containers: true}, PromoteCompare: raw.Promote.compare()}

	if c := raw.Compose; c != nil {
		if len(c.Files) == 0 {
			return nil, errors.New("compose.files: needs at least one file")
		}
		if len(c.Files) > MaxComposeFiles {
			return nil, fmt.Errorf("compose.files: more than %d files", MaxComposeFiles)
		}
		seen := map[string]bool{}
		for i, f := range c.Files {
			if err := checkRelative(f); err != nil {
				return nil, fmt.Errorf("compose.files[%d]: %w", i, err)
			}
			if strings.HasPrefix(f, "-") {
				return nil, fmt.Errorf("compose.files[%d]: a file name must not start with '-'", i)
			}
			if seen[f] {
				return nil, fmt.Errorf("compose.files[%d]: %q is listed twice", i, f)
			}
			seen[f] = true
		}
		r.ComposeFiles = c.Files
	}

	if u := raw.Update; u != nil {
		for _, f := range []struct {
			path    string
			in      *string
			out     *string
			allowed []string
		}{
			{"update.mode", u.Mode, &r.Update.Mode, []string{ModeAutomatic, ModeManual}},
			{"update.ceiling", u.Ceiling, &r.Update.Ceiling, []string{CeilingPatch, CeilingMinor, CeilingMajor}},
			{"update.major", u.Major, &r.Update.Major, []string{MajorDirect, MajorStepwise}},
		} {
			if f.in == nil {
				continue
			}
			if err := oneOf(f.path, *f.in, f.allowed...); err != nil {
				return nil, err
			}
			*f.out = *f.in
		}
		if u.Trigger != nil {
			if err := oneOf("update.trigger", *u.Trigger, TriggerWindow, TriggerInterval, TriggerWebhook); err != nil {
				return nil, err
			}
			r.Update.Trigger = *u.Trigger
		}
		if i := u.Interval; i != nil {
			if r.Update.Trigger != TriggerInterval {
				return nil, fmt.Errorf("update.interval: only allowed with update.trigger: interval (the trigger is %s)", r.Update.Trigger)
			}
			if *i < MinIntervalSeconds || *i > MaxIntervalSeconds {
				return nil, fmt.Errorf("update.interval: must be seconds from %d to %d (got %d)", MinIntervalSeconds, MaxIntervalSeconds, *i)
			}
			r.Update.Interval = time.Duration(*i) * time.Second
		}
		if o := u.Observe; o != nil {
			if *o < 0 || *o > MaxObserveMinutes {
				return nil, fmt.Errorf("update.observe: must be minutes from 0 to %d, 0 turns it off (got %d)", MaxObserveMinutes, *o)
			}
			r.Update.Observe = time.Duration(*o) * time.Minute
		}
		if err := validateUpdateMaps(u, &r.Update); err != nil {
			return nil, err
		}
	}

	if b := raw.Backup; b != nil {
		out, err := validateBackup(b.Paths, b.Volumes, b.Command)
		if err != nil {
			return nil, err
		}
		out.Keep = 0
		if rt := b.Retention; rt != nil {
			if rt.Keep != nil {
				if *rt.Keep < 1 {
					return nil, errors.New("backup.retention.keep: must be at least 1")
				}
				out.Keep = *rt.Keep
			}
			if rt.MaxAgeDays != nil {
				if *rt.MaxAgeDays < 1 {
					return nil, errors.New("backup.retention.max_age_days: must be at least 1")
				}
				out.MaxAgeDays = *rt.MaxAgeDays
			}
		}
		if out.Keep == 0 && out.MaxAgeDays == 0 {
			out.Keep = defaultKeep
		}
		r.Backup = out
	}

	if c := raw.Check; c != nil {
		if c.Containers != nil {
			r.Check.Containers = *c.Containers
		}
		if h := c.HTTP; h != nil {
			hc, err := validateHTTP(h.URL, h.Status, h.Timeout)
			if err != nil {
				return nil, err
			}
			r.Check.HTTP = hc
		}
	}
	return r, nil
}
