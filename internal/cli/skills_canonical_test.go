package cli

import (
	"strings"
	"testing"

	"github.com/bdonaldharris/builder-garage/skills"
)

// canonicalRoles are the Technician roles of the canonical skills (#33
// decision 3, #57 decision 2, #65 decision).
var canonicalRoles = map[string]string{
	"application-implementation-workflow": "implementer",
	"engineering-discovery-workflow":      "discovery",
	"implementation-readiness-workflow":   "readiness",
	"independent-implementation-review":   "reviewer",
	"integration-pr-review":               "implementer",
	"issue-authoring-workflow":            "planner",
}

// authoringSkills may author issues, each from its own evidence, and the
// planner from consolidated evidence or a decision (#57 decision 1). Each
// records this position in its authoring record.
var authoringSkills = map[string]string{
	"engineering-discovery-workflow":      "none",
	"application-implementation-workflow": "Agent #1",
	"independent-implementation-review":   "Agent #2",
	"issue-authoring-workflow":            "none",
}

const (
	authoringSectionStart = "# Issue authoring\n"
	authoringSectionEnd   = "## What this skill may author\n"
	authoringTemplate     = "templates/issue.md"
)

func TestCanonicalSkillsDeclareTheirTechnicianRoles(t *testing.T) {
	set, err := loadSkillSet(skills.FS())
	if err != nil {
		t.Fatal(err)
	}
	if got := skillNames(set); len(got) != len(canonicalRoles) {
		t.Fatalf("embedded skills = %v, want the %d canonical skills", got, len(canonicalRoles))
	}
	for _, s := range set {
		want, ok := canonicalRoles[s.Name]
		if !ok {
			t.Errorf("unexpected embedded skill %s", s.Name)
			continue
		}
		fm, err := parseSkillFrontmatter(findSkillFile(s.Files, skillEntryFile).Data)
		if err != nil {
			t.Errorf("%s: %v", s.Name, err)
			continue
		}
		if fm.Name != s.Name || fm.Metadata["garage-role"] != want {
			t.Errorf("%s: name %q, garage-role %q; want %q", s.Name, fm.Name, fm.Metadata["garage-role"], want)
		}
	}
}

// The "Issue authoring" section and the fallback template are byte-identical
// in every skill that may author, and absent from the others, because each
// installed skill must stand alone.
func TestAuthoringSkillsShareOneProcedureAndTemplate(t *testing.T) {
	set, err := loadSkillSet(skills.FS())
	if err != nil {
		t.Fatal(err)
	}
	var section, template string
	for _, s := range set {
		text := string(findSkillFile(s.Files, skillEntryFile).Data)
		position, authors := authoringSkills[s.Name]
		start := strings.Index(text, authoringSectionStart)
		if !authors {
			if start >= 0 || findSkillFile(s.Files, authoringTemplate) != nil {
				t.Errorf("%s may not author issues, but carries the authoring section or template", s.Name)
			}
			continue
		}
		end := strings.Index(text, authoringSectionEnd)
		if start < 0 || end < start {
			t.Errorf("%s: no %q section followed by %q", s.Name, strings.TrimSpace(authoringSectionStart), strings.TrimSpace(authoringSectionEnd))
			continue
		}
		got := text[start:end]
		switch {
		case section == "":
			section = got
		case got != section:
			t.Errorf("%s: the Issue authoring section differs from the other authoring skills'", s.Name)
		}
		tf := findSkillFile(s.Files, authoringTemplate)
		if tf == nil {
			t.Errorf("%s: no %s", s.Name, authoringTemplate)
		} else {
			switch {
			case template == "":
				template = string(tf.Data)
			case string(tf.Data) != template:
				t.Errorf("%s: %s differs from the other authoring skills'", s.Name, authoringTemplate)
			}
		}
		// Its own table names its role and position for the authoring record.
		table := text[end:]
		want := "`technician: " + canonicalRoles[s.Name] + "` and `position: " + position + "`"
		if !strings.Contains(table, want) {
			t.Errorf("%s: its \"What this skill may author\" section does not state %s", s.Name, want)
		}
	}
	if section == "" || template == "" {
		t.Fatal("no authoring skill was found")
	}
}

func TestCanonicalSkillsDoNotNameAWorkflowOrchestrator(t *testing.T) {
	set, err := loadSkillSet(skills.FS())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range set {
		for _, f := range s.Files {
			if strings.Contains(string(f.Data), "workflow orchestrator") {
				t.Errorf("%s/%s still says \"workflow orchestrator\"; the reader is the Builder", s.Name, f.Path)
			}
		}
	}
}
