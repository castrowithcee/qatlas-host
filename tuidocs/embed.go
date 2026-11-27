// Package tuidocs carries the Markdown help pages of the terminal interface, a folder per topic, in the page
// model of the kit's helppages package. The pages explain the screens only; the interface test requires a page
// for every screen that is not on its list of exceptions.
package tuidocs

import (
	"embed"
	"io/fs"
)

//go:embed */*.md
var files embed.FS

// FS is the root of the pages: every folder is a topic.
var FS fs.FS = files
