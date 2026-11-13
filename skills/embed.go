// Package skills embeds the canonical Builder Garage skills, so that a garage
// binary installs exactly the skill bytes it was built from, without fetching
// anything at run time.
//
// Each skill is one directory beneath this one. Only the contents of those
// directories are embedded; this file is not part of any skill.
package skills

import (
	"embed"
	"io/fs"
)

//go:embed */*
var files embed.FS

// FS returns the embedded skills: one top-level directory per skill, each
// holding the skill's files, beginning with SKILL.md.
func FS() fs.FS {
	return files
}
