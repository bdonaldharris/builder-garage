package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/bdonaldharris/builder-garage/manuals"
)

// manualCatalogFS returns the manuals this garage binary carries. It is a
// variable only so tests can substitute a catalog.
var manualCatalogFS = manuals.FS

// manualSkillPrefix begins the name of every skill that delivers a manual,
// so a manual skill can never take the name of a canonical skill.
const manualSkillPrefix = "builder-garage-manual-"

// manualsReadme is the one file in the manuals directory that is not a
// manual.
const manualsReadme = "README.md"

// catalogManual is one manual this binary carries: its catalog entry, read
// from its frontmatter, and its text without the frontmatter.
type catalogManual struct {
	Name        string
	Version     string
	Status      string
	Description string
	File        string
	Body        []byte
	// Source is the manual file exactly as embedded.
	Source []byte
}

// manualFrontmatter is a manual's catalog entry.
type manualFrontmatter struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Status      string `yaml:"status"`
	Description string `yaml:"description"`
}

// manualStatuses are the statuses a manual may declare.
var manualStatuses = []string{"provisional", "stable"}

// loadManualCatalog reads every manual in fsys, sorted by name. A manual
// whose frontmatter is missing or invalid is an error: the catalog is the
// manuals' own frontmatter, so it cannot be partly unreadable.
func loadManualCatalog(fsys fs.FS) ([]catalogManual, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("cannot read the embedded manuals: %w", err)
	}
	var catalog []catalogManual
	for _, entry := range entries {
		file := entry.Name()
		if entry.IsDir() || path.Ext(file) != ".md" || file == manualsReadme {
			continue
		}
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("cannot read the embedded manual %s: %w", file, err)
		}
		m, err := parseCatalogManual(file, data)
		if err != nil {
			return nil, fmt.Errorf("the embedded manual %s: %w", file, err)
		}
		catalog = append(catalog, m)
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].Name < catalog[j].Name })
	return catalog, nil
}

func parseCatalogManual(file string, data []byte) (catalogManual, error) {
	fm, closeAt, err := splitFrontmatter(data)
	if err != nil {
		return catalogManual{}, err
	}
	var entry manualFrontmatter
	decoder := yaml.NewDecoder(strings.NewReader(fm))
	decoder.KnownFields(true)
	if err := decoder.Decode(&entry); err != nil {
		return catalogManual{}, fmt.Errorf("its frontmatter is not a valid catalog entry: %w", err)
	}
	var problems configProblems
	checkToken(&problems, "name", entry.Name)
	checkToken(&problems, "version", entry.Version)
	checkLine(&problems, "description", entry.Description)
	if strings.TrimSuffix(file, ".md") != entry.Name {
		problems = append(problems, fmt.Sprintf("name %q must match its file name", entry.Name))
	}
	if !containsString(manualStatuses, entry.Status) {
		problems = append(problems, fmt.Sprintf("status must be one of %s, not %q", strings.Join(manualStatuses, ", "), entry.Status))
	}
	if len(problems) > 0 {
		return catalogManual{}, problems
	}
	// The body begins after the closing "---" line and the blank line
	// that follows it.
	body := bytes.TrimLeft(data[closeAt+len("---\n"):], "\n")
	return catalogManual{
		Name:        entry.Name,
		Version:     entry.Version,
		Status:      entry.Status,
		Description: entry.Description,
		File:        file,
		Body:        body,
		Source:      data,
	}, nil
}

// embeddedManualCatalog loads the catalog this binary carries.
func embeddedManualCatalog() ([]catalogManual, error) {
	return loadManualCatalog(manualCatalogFS())
}

func findManual(catalog []catalogManual, name string) *catalogManual {
	for i := range catalog {
		if catalog[i].Name == name {
			return &catalog[i]
		}
	}
	return nil
}

func manualNames(catalog []catalogManual) []string {
	names := make([]string, len(catalog))
	for i, m := range catalog {
		names[i] = m.Name
	}
	return names
}

// manualSkillName is the name of the skill that delivers manual name.
func manualSkillName(name string) string {
	return manualSkillPrefix + name
}

// describeManual is a manual's one-line label: name, version, status, and
// description.
func describeManual(m catalogManual) string {
	return fmt.Sprintf("%s %s (%s): %s", m.Name, m.Version, m.Status, m.Description)
}

// manualSkill returns the skill that delivers m to agents: a SKILL.md whose
// frontmatter names the skill and says when it applies, followed by the
// manual's text. Installing it never makes it applicable; the description
// says that only a project that adopts the manual applies it.
func manualSkill(m catalogManual) (canonicalSkill, error) {
	name := manualSkillName(m.Name)
	description := fmt.Sprintf("Builder Garage %s manual, version %s (%s). %s "+
		"Guidance only. Apply it only in a project whose .garage/config.yaml adopts the %s manual "+
		"(project.guidance.manuals); being installed does not make it apply.",
		m.Name, m.Version, m.Status, m.Description, m.Name)
	var b bytes.Buffer
	fmt.Fprintf(&b, "---\nname: %s\ndescription: %s\nmetadata:\n  garage-manual: %s\n  garage-manual-version: %s\n---\n\n",
		name, strconv.Quote(description), strconv.Quote(m.Name), strconv.Quote(m.Version))
	fmt.Fprintf(&b, "> This skill delivers the Builder Garage `%s` manual, version %s (%s). A manual is guidance only: "+
		"it never relaxes a review or approval gate. It applies only in a project whose `.garage/config.yaml` "+
		"adopts it. Links in the manual point into the Builder Garage repository.\n\n", m.Name, m.Version, m.Status)
	b.Write(m.Body)

	parsed, err := parseSkillFrontmatter(b.Bytes())
	if err != nil {
		return canonicalSkill{}, fmt.Errorf("cannot prepare the %s manual skill: %w", m.Name, err)
	}
	if parsed.Name != name || parsed.Metadata["garage-manual"] != m.Name || parsed.Metadata["garage-manual-version"] != m.Version {
		return canonicalSkill{}, fmt.Errorf("cannot prepare the %s manual skill: its frontmatter did not round-trip", m.Name)
	}
	return canonicalSkill{Name: name, Files: []skillFile{{Path: skillEntryFile, Data: b.Bytes()}}}, nil
}

// embeddedSetDigest is the skill-set digest of a binary: every embedded
// skill file, by "<skill>/<path>", and every embedded manual, by
// "manuals/<file>". A development revision is bound to it, so it changes
// whenever a skill or a manual does.
func embeddedSetDigest(set []canonicalSkill, catalog []catalogManual) string {
	return skillSetDigest(append(append([]canonicalSkill(nil), set...), manualSourceSkill(catalog)))
}

// manualSourceSkill lists the embedded manual files as one pseudo-skill, so
// the skill-set digest covers the manuals too.
func manualSourceSkill(catalog []catalogManual) canonicalSkill {
	s := canonicalSkill{Name: "manuals"}
	for _, m := range catalog {
		s.Files = append(s.Files, skillFile{Path: m.File, Data: m.Source})
	}
	sortSkillFiles(s.Files)
	return s
}
