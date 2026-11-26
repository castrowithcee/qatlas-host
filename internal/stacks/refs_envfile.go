package stacks

import (
	"strings"
)

// refFile adds the file named by section, relative to base, to the files; one named by env_file is also noted
// as an env file, which the service reads on every deploy.
func (c *refCollector) refFile(section, base, raw string) {
	if p, ok := c.resolve(section, base, raw); ok {
		c.files[p] = true
		if strings.HasSuffix(section, "env_file") {
			c.envFiles[p] = true
		}
	}
}

// remoteContext tells whether a build context is a URL or another source than a path of the host.
func remoteContext(s string) bool {
	if strings.Contains(s, "://") || strings.HasPrefix(s, "git@") || strings.HasPrefix(s, "github.com/") {
		return true
	}
	if i := strings.IndexByte(s, ':'); i > 0 && !strings.ContainsAny(s[:i], "/.$") {
		return true // service:name, a docker-image: reference
	}
	return false
}
