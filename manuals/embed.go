// Package manuals embeds Builder Garage's manuals, so that a garage binary
// carries exactly the manual versions it was built from and can deliver them
// without fetching anything at run time.
//
// Every Markdown file in this directory except README.md is one manual. Its
// YAML frontmatter is its catalog entry: name, version, status, and a
// one-line description.
package manuals

import (
	"embed"
	"io/fs"
)

//go:embed *.md
var files embed.FS

// FS returns the embedded manuals directory: README.md and one file per
// manual.
func FS() fs.FS {
	return files
}
