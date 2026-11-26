package stacks

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	// maxRefDepth bounds the nesting of include and extends.
	maxRefDepth = 8
	// maxRefDocs bounds the compose documents read for the references of one stack.
	maxRefDocs = 64
	// maxUnresolved bounds the unresolved references reported per stack.
	maxUnresolved = 50
)

// Refs is what the compose files of a stack refer to in the working copy, as clean paths from the repository
// root. The compose files are read as data: nothing is executed and no value of .env is kept in it, only
// paths that came out of it. A path that is absolute, uses '..' to leave the repository or still holds a
// variable is never listed.
type Refs struct {
	// Files are the compose files, .env and the files named by env_file, label_file, configs, include, extends,
	// dockerfile and the bind-mounted files that are read-only.
	Files []string
	// EnvFiles are the files named by env_file, also in an include; they are part of Files as well.
	EnvFiles []string
	// Dirs are folders of which every file counts: build contexts and read-only mounts of a folder.
	Dirs []string
	// Writable are existing files that are bind-mounted writable. They may hold data, so they are not taken.
	Writable []string
	// Secrets are the files of the top-level secrets. They may hold secrets, so they are not taken.
	Secrets []string
	// Unresolved describes references whose path uses a variable that has no value in .env. It names the
	// variable, never a value.
	Unresolved []string
}

// refCollector walks the compose documents of one stack.
type refCollector struct {
	root, folder string
	env          map[string]string
	files, dirs  map[string]bool
	writable     map[string]bool
	envFiles     map[string]bool
	secrets      map[string]bool
	unresolved   map[string]bool
	seen         map[string]bool
	docs         int
}

// readRefs collects the references of the stack in folder below root from its compose files, given relative to
// the stack folder, and from the documents they include or extend. A file that cannot be read or parsed adds
// nothing, which leaves its references to the stricter treatment; the stack reports such a file elsewhere.
func readRefs(root, folder string, files []string, env map[string]string) Refs {
	c := &refCollector{
		root: root, folder: folder, env: env,
		files: map[string]bool{}, dirs: map[string]bool{}, writable: map[string]bool{}, envFiles: map[string]bool{},
		secrets: map[string]bool{}, unresolved: map[string]bool{}, seen: map[string]bool{},
	}
	c.addFile(folder, envFile)
	for _, f := range files {
		repoFile := path.Join(folder, f)
		c.files[repoFile] = true
		c.walk(repoFile, folder, 0)
	}
	r := Refs{
		Files: sortedKeys(c.files), EnvFiles: sortedKeys(c.envFiles), Dirs: sortedKeys(c.dirs), Writable: sortedKeys(c.writable),
		Secrets: sortedKeys(c.secrets), Unresolved: sortedKeys(c.unresolved),
	}
	if len(r.Unresolved) > maxUnresolved {
		r.Unresolved = r.Unresolved[:maxUnresolved]
	}
	return r
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// walk reads one compose document. base is the folder its relative paths belong to.
func (c *refCollector) walk(repoFile, base string, depth int) {
	if c.seen[repoFile] || depth > maxRefDepth || c.docs >= maxRefDocs {
		return
	}
	c.seen[repoFile] = true
	c.docs++
	data, err := readBounded(c.root, repoFile)
	if err != nil {
		return
	}
	var doc map[string]any
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&doc); err != nil {
		return
	}
	for _, inc := range list(doc["include"]) {
		c.include(inc, base, depth)
	}
	for _, def := range asMap(doc["configs"]) {
		if p, ok := asMap(def)["file"].(string); ok {
			c.ref("configs.file", base, p, c.files)
		}
	}
	for _, def := range asMap(doc["secrets"]) {
		if p, ok := asMap(def)["file"].(string); ok {
			c.ref("secrets.file", base, p, c.secrets)
		}
	}
	for _, svc := range asMap(doc["services"]) {
		c.service(asMap(svc), base, depth)
	}
}

func (c *refCollector) include(inc any, base string, depth int) {
	var paths []any
	project := ""
	var envFiles any
	switch v := inc.(type) {
	case string:
		paths = []any{v}
	case map[string]any:
		paths, envFiles = list(v["path"]), v["env_file"]
		project, _ = v["project_directory"].(string)
	default:
		return
	}
	for _, e := range list(envFiles) {
		if p, ok := e.(string); ok {
			c.refFile("include.env_file", base, p)
		}
	}
	for _, p := range paths {
		raw, ok := p.(string)
		if !ok {
			continue
		}
		file, ok := c.resolve("include", base, raw)
		if !ok {
			continue
		}
		c.files[file] = true
		sub := path.Dir(file)
		if project != "" {
			if pd, ok := c.resolve("include.project_directory", base, project); ok {
				sub = pd
			}
		}
		c.walk(file, sub, depth+1)
	}
}

func (c *refCollector) service(svc map[string]any, base string, depth int) {
	for _, key := range []string{"env_file", "label_file"} {
		for _, e := range list(svc[key]) {
			switch v := e.(type) {
			case string:
				c.refFile(key, base, v)
			case map[string]any:
				if p, ok := v["path"].(string); ok {
					c.refFile(key, base, p)
				}
			}
		}
	}
	switch ext := svc["extends"].(type) {
	case map[string]any:
		if f, ok := ext["file"].(string); ok {
			if file, ok := c.resolve("extends.file", base, f); ok {
				c.files[file] = true
				c.walk(file, path.Dir(file), depth+1)
			}
		}
	}
	c.build(svc["build"], base)
	for _, v := range list(svc["volumes"]) {
		c.volume(v, base)
	}
}

func (c *refCollector) build(b any, base string) {
	context, dockerfile := ".", ""
	switch v := b.(type) {
	case string:
		context = v
	case map[string]any:
		if s, ok := v["context"].(string); ok {
			context = s
		}
		dockerfile, _ = v["dockerfile"].(string)
		for _, extra := range asMap(v["additional_contexts"]) {
			if s, ok := extra.(string); ok && !remoteContext(s) {
				c.ref("build.additional_contexts", base, s, c.dirs)
			}
		}
	default:
		return
	}
	if remoteContext(context) {
		return
	}
	dir, ok := c.resolve("build.context", base, context)
	if !ok {
		return
	}
	c.dirs[dir] = true
	if dockerfile != "" {
		c.ref("build.dockerfile", dir, dockerfile, c.files)
	}
}

// volume records a bind mount. A read-only file or folder counts. A writable file is unclear, as it may hold
// data. A writable folder is data and stays out.
func (c *refCollector) volume(v any, base string) {
	bind, ok := volumeBind(v, c.env)
	if !ok {
		return
	}
	if bind.unresolved != "" {
		c.unresolved[fmt.Sprintf("volumes %q: variable %s has no value in .env", bind.raw, bind.unresolved)] = true
		return
	}
	p, ok := c.resolvePath(base, bind.src)
	if !ok {
		return
	}
	switch kindOf(c.root, p) {
	case kindFile:
		if bind.readOnly {
			c.files[p] = true
		} else {
			c.writable[p] = true
		}
	case kindDir:
		if bind.readOnly {
			c.dirs[p] = true
		}
	}
}

// ref adds the path named by section, relative to base, to set.
func (c *refCollector) ref(section, base, raw string, set map[string]bool) {
	if p, ok := c.resolve(section, base, raw); ok {
		set[p] = true
	}
}

// addFile adds the file name below the folder dir.
func (c *refCollector) addFile(dir, name string) { c.files[path.Join(dir, name)] = true }

// resolve interpolates raw from .env and turns it into a clean path from the repository root; a variable
// without a value is reported instead.
func (c *refCollector) resolve(section, base, raw string) (string, bool) {
	val, missing := interpolate(raw, c.env)
	if missing != "" {
		c.unresolved[fmt.Sprintf("%s %q: variable %s has no value in .env", section, raw, missing)] = true
		return "", false
	}
	return c.resolvePath(base, val)
}

// resolvePath joins a relative path to base and refuses one that leaves the repository.
func (c *refCollector) resolvePath(base, p string) (string, bool) {
	if p == "" || strings.ContainsAny(p, "$~\\\x00") || path.IsAbs(p) {
		return "", false
	}
	out := path.Join(base, p)
	if out == ".." || strings.HasPrefix(out, "../") {
		return "", false
	}
	return out, true
}

const (
	kindNone = iota
	kindFile
	kindDir
)

// kindOf says what rel is below root without following a link on the way: a link is none.
func kindOf(root, rel string) int {
	cur := root
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return kindNone
		}
		last := i == len(parts)-1
		switch {
		case last && info.Mode().IsRegular():
			return kindFile
		case last && info.IsDir():
			return kindDir
		case last || !info.IsDir():
			return kindNone
		}
	}
	return kindNone
}

// bindVolume is a bind mount of a service as the compose file writes it.
type bindVolume struct {
	// raw is the source as written; src is raw interpolated.
	raw, src string
	readOnly bool
	// unresolved names a variable of raw that has no value.
	unresolved string
}

// volumeBind reads a volume entry of a service in the short and the long syntax. It reports a bind mount only
// when the source is a path: it starts with "." or "/" after interpolation.
func volumeBind(v any, env map[string]string) (bindVolume, bool) {
	var b bindVolume
	switch m := v.(type) {
	case string:
		parts := splitTop(m, ':')
		if len(parts) < 2 {
			return b, false
		}
		b.raw = parts[0]
		if len(parts) > 2 {
			for _, o := range strings.Split(parts[2], ",") {
				if o == "ro" {
					b.readOnly = true
				}
			}
		}
	case map[string]any:
		if typ, _ := m["type"].(string); typ != "bind" {
			return b, false
		}
		b.raw, _ = m["source"].(string)
		b.readOnly, _ = m["read_only"].(bool)
	default:
		return b, false
	}
	if b.raw == "" {
		return b, false
	}
	b.src, b.unresolved = interpolate(b.raw, env)
	if b.unresolved != "" {
		// An unset variable may stand for a named volume as well as a path; it is reported either way.
		return b, true
	}
	if _, isMap := v.(map[string]any); !isMap && !strings.HasPrefix(b.src, ".") && !strings.HasPrefix(b.src, "/") {
		return b, false
	}
	return b, b.src != ""
}

// splitTop splits s at sep outside of ${...}.
func splitTop(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '{':
			depth++
			i++
		case s[i] == '}' && depth > 0:
			depth--
		case s[i] == sep && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// interpolate resolves $VAR, ${VAR} and the forms with :-, -, :?, ?, :+ and + from env, as Compose does for
// a value. $$ is a literal dollar sign. missing names the first variable that has no value and no default;
// the result is then not usable. A default that holds another variable counts as missing.
func interpolate(s string, env map[string]string) (out, missing string) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			b.WriteByte('$')
			continue
		}
		switch n := s[i+1]; {
		case n == '$':
			b.WriteByte('$')
			i++
		case n == '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return "", "?"
			}
			expr := s[i+2 : i+end]
			i += end
			val, name := expandBraces(expr, env)
			if name != "" && missing == "" {
				missing = name
			}
			b.WriteString(val)
		case isNameStart(n):
			j := i + 1
			for j < len(s) && isNameChar(s[j]) {
				j++
			}
			name := s[i+1 : j]
			val, ok := env[name]
			if !ok && missing == "" {
				missing = name
			}
			b.WriteString(val)
			i = j - 1
		default:
			b.WriteByte('$')
		}
	}
	return b.String(), missing
}

func isNameStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isNameChar(c byte) bool  { return isNameStart(c) || c >= '0' && c <= '9' }

// expandBraces resolves the inside of ${...}. The second result names the variable when it cannot be resolved.
func expandBraces(expr string, env map[string]string) (string, string) {
	i := 0
	for i < len(expr) && (i == 0 && isNameStart(expr[0]) || i > 0 && isNameChar(expr[i])) {
		i++
	}
	name := expr[:i]
	if name == "" {
		return "", "?"
	}
	val, set := env[name]
	rest := expr[i:]
	if rest == "" {
		if !set {
			return "", name
		}
		return val, ""
	}
	colon := strings.HasPrefix(rest, ":")
	rest = strings.TrimPrefix(rest, ":")
	if rest == "" || !strings.ContainsRune("-?+", rune(rest[0])) {
		return "", name
	}
	op, arg := rest[0], rest[1:]
	if strings.Contains(arg, "$") {
		return "", name
	}
	have := set && (!colon || val != "")
	switch op {
	case '-':
		if have {
			return val, ""
		}
		return arg, ""
	case '+':
		if have {
			return arg, ""
		}
		return "", ""
	default: // '?': an unset variable is an error in Compose
		if have {
			return val, ""
		}
		return "", name
	}
}

// asMap returns v as a map, or nil.
func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// list returns v as a list; a single value is a list of one, nil is empty.
func list(v any) []any {
	switch l := v.(type) {
	case nil:
		return nil
	case []any:
		return l
	}
	return []any{v}
}
