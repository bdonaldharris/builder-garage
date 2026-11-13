package cli

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"runtime/debug"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// skillEntryFile is the file every skill directory must contain.
const skillEntryFile = "SKILL.md"

// developmentRevisionPrefix marks a development or unversioned install: one
// made from a build without clean version-control metadata. Such an install
// is bound to its skill-set digest instead of a commit.
const developmentRevisionPrefix = "development+"

// skillFile is one file of a skill. Path is slash-separated and relative to
// the skill's directory.
type skillFile struct {
	Path string
	Data []byte
}

// canonicalSkill is one embedded skill and its files, sorted by path.
type canonicalSkill struct {
	Name  string
	Files []skillFile
}

// loadSkillSet reads the embedded skills: each top-level directory of fsys
// is one skill. Skills are returned sorted by name.
func loadSkillSet(fsys fs.FS) ([]canonicalSkill, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("cannot read the embedded skills: %w", err)
	}
	var skills []canonicalSkill
	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("the embedded skills contain %q, which is not a skill directory", entry.Name())
		}
		skill := canonicalSkill{Name: entry.Name()}
		err := fs.WalkDir(fsys, entry.Name(), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("%s is not a regular file", p)
			}
			data, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			skill.Files = append(skill.Files, skillFile{Path: strings.TrimPrefix(p, entry.Name()+"/"), Data: data})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("cannot read the embedded skill %s: %w", entry.Name(), err)
		}
		sortSkillFiles(skill.Files)
		if findSkillFile(skill.Files, skillEntryFile) == nil {
			return nil, fmt.Errorf("the embedded skill %s has no %s", skill.Name, skillEntryFile)
		}
		skills = append(skills, skill)
	}
	if len(skills) == 0 {
		return nil, errors.New("this garage binary embeds no skills")
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

func sortSkillFiles(files []skillFile) {
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
}

func findSkillFile(files []skillFile, name string) *skillFile {
	for i := range files {
		if files[i].Path == name {
			return &files[i]
		}
	}
	return nil
}

// digestSkillFiles returns the digest of a set of files, written
// "sha256:<64 hex>". It is the SHA-256 of one line per file, in path order:
// the slash-separated path, a NUL byte, the lowercase hex SHA-256 of the
// file's bytes, and a newline. Paths are those the files are listed under, so
// the digest of a skill uses paths within the skill directory, and the digest
// of the skill set prefixes each with the skill's name.
func digestSkillFiles(files []skillFile) string {
	sorted := append([]skillFile(nil), files...)
	sortSkillFiles(sorted)
	h := sha256.New()
	for _, f := range sorted {
		fmt.Fprintf(h, "%s\x00%x\n", f.Path, sha256.Sum256(f.Data))
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

// skillSetDigest is the digest of every canonical file of every embedded
// skill, each listed as "<skill>/<path>".
func skillSetDigest(skills []canonicalSkill) string {
	var all []skillFile
	for _, s := range skills {
		for _, f := range s.Files {
			all = append(all, skillFile{Path: s.Name + "/" + f.Path, Data: f.Data})
		}
	}
	return digestSkillFiles(all)
}

// buildRevision is the version-control state the binary was built from.
type buildRevision struct {
	// Revision is the full commit object name; empty when unknown.
	Revision string
	// Clean is true only when the build recorded a revision and no local
	// modifications.
	Clean bool
}

// readBuildRevision reads the version-control metadata that the Go toolchain
// embeds in a binary built from a repository checkout.
func readBuildRevision() buildRevision {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return buildRevision{}
	}
	var rev buildRevision
	modified := ""
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev.Revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	rev.Clean = rev.Revision != "" && modified == "false"
	return rev
}

// errUnversionedBuild reports that the binary was not built from a clean
// commit, so its skills have no commit identity.
var errUnversionedBuild = errors.New("this garage binary was not built from a clean commit")

// installRevision returns the revision installed skills carry. A clean build
// carries its commit. Any other build is refused unless development is set;
// it then carries a development revision bound to the skill-set digest.
func installRevision(build buildRevision, setDigest string, development bool) (string, bool, error) {
	if build.Clean {
		return build.Revision, false, nil
	}
	if !development {
		reason := "its version-control metadata is unavailable"
		if build.Revision != "" {
			reason = "it was built from a working tree with local modifications"
		}
		return "", false, fmt.Errorf("%w: %s. Rebuild garage from a clean checkout, "+
			"or pass --development to install skills marked as development and unversioned", errUnversionedBuild, reason)
	}
	return developmentRevision(setDigest), true, nil
}

func developmentRevision(setDigest string) string {
	return developmentRevisionPrefix + setDigest
}

// currentRevision is the revision this binary's skills are compared against
// by verification: its commit when the build is clean, and otherwise its
// development revision.
func currentRevision(build buildRevision, setDigest string) string {
	if build.Clean {
		return build.Revision
	}
	return developmentRevision(setDigest)
}

// renderSkill returns the files to install for skill: the canonical files,
// with metadata.garage-revision added to the SKILL.md frontmatter.
func renderSkill(skill canonicalSkill, revision string) ([]skillFile, error) {
	out := make([]skillFile, 0, len(skill.Files))
	for _, f := range skill.Files {
		data := f.Data
		if f.Path == skillEntryFile {
			var err error
			data, err = injectSkillRevision(f.Data, revision)
			if err != nil {
				return nil, fmt.Errorf("cannot prepare %s/%s: %w", skill.Name, f.Path, err)
			}
		}
		out = append(out, skillFile{Path: f.Path, Data: data})
	}
	return out, nil
}

// skillFrontmatter is the part of a skill's frontmatter Garage reads.
type skillFrontmatter struct {
	Name     string            `yaml:"name"`
	Metadata map[string]string `yaml:"metadata"`
}

// splitFrontmatter returns the YAML between a leading "---" line and the
// next "---" line, and the byte offset where that closing line starts.
func splitFrontmatter(data []byte) (string, int, error) {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return "", 0, errors.New("it does not begin with a frontmatter block")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return "", 0, errors.New("its frontmatter block is not closed")
	}
	return text[4 : 4+end+1], 4 + end + 1, nil
}

func parseSkillFrontmatter(data []byte) (skillFrontmatter, error) {
	fm, _, err := splitFrontmatter(data)
	if err != nil {
		return skillFrontmatter{}, err
	}
	var parsed skillFrontmatter
	if err := yaml.Unmarshal([]byte(fm), &parsed); err != nil {
		return skillFrontmatter{}, fmt.Errorf("its frontmatter is not valid YAML: %w", err)
	}
	return parsed, nil
}

// injectSkillRevision adds `garage-revision: "<revision>"` as the last key of
// the frontmatter's top-level metadata map. The canonical source never
// carries the key, so a source that already has it is refused. The result is
// parsed again to confirm the value.
func injectSkillRevision(data []byte, revision string) ([]byte, error) {
	before, err := parseSkillFrontmatter(data)
	if err != nil {
		return nil, err
	}
	if before.Metadata == nil {
		return nil, errors.New("its frontmatter has no metadata map")
	}
	if _, ok := before.Metadata["garage-revision"]; ok {
		return nil, errors.New("its frontmatter already sets metadata.garage-revision")
	}

	fm, closeAt, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	lines := strings.SplitAfter(fm, "\n")
	insertAt := -1
	for i, line := range lines {
		if strings.TrimRight(line, "\n") != "metadata:" {
			continue
		}
		insertAt = i + 1
		for insertAt < len(lines) && strings.HasPrefix(lines[insertAt], "  ") {
			insertAt++
		}
		break
	}
	if insertAt < 0 {
		return nil, errors.New("its metadata map is not a block mapping")
	}
	added := "  garage-revision: " + quoteRevision(revision) + "\n"
	var b bytes.Buffer
	b.WriteString("---\n")
	for _, line := range lines[:insertAt] {
		b.WriteString(line)
	}
	b.WriteString(added)
	for _, line := range lines[insertAt:] {
		b.WriteString(line)
	}
	b.Write(data[closeAt:])
	result := b.Bytes()

	after, err := parseSkillFrontmatter(result)
	if err != nil {
		return nil, err
	}
	if after.Metadata["garage-revision"] != revision || after.Name != before.Name ||
		len(after.Metadata) != len(before.Metadata)+1 {
		return nil, errors.New("adding metadata.garage-revision did not produce the expected frontmatter")
	}
	return result, nil
}

// quoteRevision double-quotes a revision for YAML, so that a commit name made
// only of digits stays a string. Revisions contain only hex digits, letters,
// '+', and ':', so no escaping is needed; the caller parses the result again
// to confirm the value.
func quoteRevision(s string) string {
	return `"` + s + `"`
}

// skillNames lists the names of skills.
func skillNames(skills []canonicalSkill) []string {
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	return names
}
