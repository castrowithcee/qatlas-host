package stacks

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ProjectNameIn is the Compose project name s would have if its folder were called folder: the name of .env
// or of the compose files when they set one, else the folder name. It reads the files of s again and fails as
// Load fails for a name it cannot resolve.
func ProjectNameIn(s Stack, folder string) (string, error) {
	fileName, err := readComposeName(s.Dir, s.ComposeFiles)
	if err != nil {
		return "", err
	}
	return projectNameOf(folder, s.Env, fileName)
}

// AbsoluteBinds lists the sources of the bind mounts of s that are written as absolute paths and are the folder
// of s or lie below it, as written and cleaned, sorted. Such a path keeps pointing at the old place when the
// folder moves. Variables are resolved from .env; a source with an unset variable is left out.
func AbsoluteBinds(s Stack) []string {
	set := map[string]bool{}
	for _, f := range s.ComposeFiles {
		data, err := readBounded(s.Dir, f)
		if err != nil {
			continue
		}
		var doc struct {
			Services map[string]struct {
				Volumes []any `yaml:"volumes"`
			} `yaml:"services"`
		}
		if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&doc); err != nil {
			continue
		}
		for _, svc := range doc.Services {
			for _, v := range svc.Volumes {
				b, ok := volumeBind(v, s.Env)
				if !ok || b.unresolved != "" || !filepath.IsAbs(b.src) {
					continue
				}
				if p := filepath.Clean(b.src); p == s.Dir || strings.HasPrefix(p, s.Dir+"/") {
					set[p] = true
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
