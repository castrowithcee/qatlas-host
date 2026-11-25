package stacks

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/changelog"
)

// validateUpdateMaps checks the update members that name image variables or a stack and sets them in out.
func validateUpdateMaps(u *UpdateSettings, out *Update) error {
	keys := make([]string, 0, len(u.Tags))
	for k := range u.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !envKey.MatchString(k) {
			return fmt.Errorf("update.tags.%s: not a variable name", k)
		}
		re, err := regexp.Compile(u.Tags[k])
		if err != nil {
			return fmt.Errorf("update.tags.%s: invalid regular expression: %v", k, err)
		}
		if out.Tags == nil {
			out.Tags = map[string]*regexp.Regexp{}
		}
		out.Tags[k] = re
	}
	keys = keys[:0]
	for k := range u.Changelog {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !envKey.MatchString(k) {
			return fmt.Errorf("update.changelog.%s: not a variable name", k)
		}
		src := changelog.ParseSource(u.Changelog[k])
		if !src.GitHub {
			return fmt.Errorf("update.changelog.%s: must be a GitHub repository as owner/name or a github.com address", k)
		}
		if out.Changelog == nil {
			out.Changelog = map[string]string{}
		}
		out.Changelog[k] = src.Repo.String()
	}
	if p := u.PromoteSource; p != "" {
		if !stackName.MatchString(p) {
			return fmt.Errorf("update.promote_source: %q is not a stack name", p)
		}
		out.PromoteSource = p
	}
	return nil
}

func validateBackup(paths, volumes, command []string) (*Backup, error) {
	if len(paths) == 0 && len(volumes) == 0 && len(command) == 0 {
		return nil, errors.New("backup: needs at least one of paths, volumes, command")
	}
	for i, p := range paths {
		if err := checkRelative(p); err != nil {
			return nil, fmt.Errorf("backup.paths[%d]: %w", i, err)
		}
	}
	for i, v := range volumes {
		if !volumeName.MatchString(v) {
			return nil, fmt.Errorf("backup.volumes[%d]: %q is not a volume name", i, v)
		}
	}
	if len(command) > 0 && command[0] == "" {
		return nil, errors.New("backup.command[0]: the program must not be empty")
	}
	return &Backup{Paths: paths, Volumes: volumes, Command: command}, nil
}

func validateHTTP(rawURL string, status *int, timeout *string) (*HTTPCheck, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("check.http.url: must be an absolute http or https URL (got %q)", rawURL)
	}
	hc := &HTTPCheck{URL: rawURL, Status: 200, Timeout: defaultTimeout}
	if status != nil {
		if *status < 100 || *status > 599 {
			return nil, fmt.Errorf("check.http.status: must be between 100 and 599 (got %d)", *status)
		}
		hc.Status = *status
	}
	if timeout != nil {
		d, err := time.ParseDuration(*timeout)
		if err != nil || d < time.Second || d > 10*time.Minute {
			return nil, fmt.Errorf("check.http.timeout: must be a duration between 1s and 10m (got %q)", *timeout)
		}
		hc.Timeout = d
	}
	return hc, nil
}

// ownSettings returns the settings of the stack called name; a stack named as its own promote source is
// refused here, where the name is known.
func ownSettings(settingsOf func(string) (*Rules, error), name string) (*Rules, error) {
	rules, err := settingsOf(name)
	if err == nil && rules != nil && rules.Update.PromoteSource == name {
		return rules, errors.New("stack settings: update.promote_source: a stack cannot be its own source")
	}
	return rules, err
}
