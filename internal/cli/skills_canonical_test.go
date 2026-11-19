package cli

import (
	"strings"
	"testing"

	"github.com/bdonaldharris/builder-garage/skills"
)

// canonicalRoles are the Technician roles of the canonical skills (#33
// decision 3, #57 decision 2, #65 decision, #149 decision 4).
var canonicalRoles = map[string]string{
	"application-implementation-workflow": "implementer",
	"engineering-discovery-workflow":      "discovery",
	"implementation-readiness-workflow":   "readiness",
	"independent-implementation-review":   "reviewer",
	"integration-pr-review":               "implementer",
	"issue-authoring-workflow":            "planner",
	"project-spec-workflow":               "planner",
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

const (
	outputFenceOpen  = "```markdown\n"
	outputFenceClose = "\n```\n"
)

// Each output template is a copy of the structure its skill embeds under an
// output section, which stays authoritative so the installed skill stands
// alone: the readiness report (#103) and the project spec (#149). The
// template holds exactly the lines inside that section's first markdown
// fence, each ending in a newline, so it ends with one trailing newline.
func TestOutputTemplatesMatchTheirSkillsOutputStructure(t *testing.T) {
	set, err := loadSkillSet(skills.FS())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		skill, section, template string
	}{
		{"implementation-readiness-workflow", "# Readiness output\n", "templates/readiness-report.md"},
		{"project-spec-workflow", "# Spec output\n", "templates/project-spec.md"},
	} {
		t.Run(tc.skill, func(t *testing.T) {
			var skill *canonicalSkill
			for i := range set {
				if set[i].Name == tc.skill {
					skill = &set[i]
				}
			}
			if skill == nil {
				t.Fatalf("no embedded %s skill", tc.skill)
			}
			text := string(findSkillFile(skill.Files, skillEntryFile).Data)
			section := strings.Index(text, tc.section)
			if section < 0 {
				t.Fatalf("%s: no %q section", tc.skill, strings.TrimSpace(tc.section))
			}
			open := strings.Index(text[section:], outputFenceOpen)
			if open < 0 {
				t.Fatalf("%s: no %q fence under %q", tc.skill, strings.TrimSpace(outputFenceOpen), strings.TrimSpace(tc.section))
			}
			body := text[section+open+len(outputFenceOpen):]
			end := strings.Index(body, outputFenceClose)
			if end < 0 {
				t.Fatalf("%s: the fence under %q is not closed", tc.skill, strings.TrimSpace(tc.section))
			}
			want := body[:end+1]
			tf := findSkillFile(skill.Files, tc.template)
			if tf == nil {
				t.Fatalf("%s: no %s", tc.skill, tc.template)
			}
			if got := string(tf.Data); got != want {
				t.Errorf("%s: %s differs from the structure under %q; keep the two identical", tc.skill, tc.template, strings.TrimSpace(tc.section))
			}
		})
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

// hotfixSkills apply the release branch (E11) to a hotfix-designated issue
// (#139). Each carries the same Hotfixes section, so the designation rule
// has one wording wherever it is checked.
var hotfixSkills = []string{
	"application-implementation-workflow",
	"independent-implementation-review",
	"integration-pr-review",
}

const (
	hotfixSectionStart = "# Hotfixes (E11)\n"
	hotfixSectionEnd   = "\n---\n"
)

func TestHotfixSkillsShareOneHotfixSection(t *testing.T) {
	set, err := loadSkillSet(skills.FS())
	if err != nil {
		t.Fatal(err)
	}
	var section string
	seen := 0
	for _, s := range set {
		text := string(findSkillFile(s.Files, skillEntryFile).Data)
		start := strings.Index(text, hotfixSectionStart)
		applies := false
		for _, name := range hotfixSkills {
			applies = applies || name == s.Name
		}
		if !applies {
			if start >= 0 || strings.Contains(text, "release_branch") {
				t.Errorf("%s does not apply the release branch, but carries the Hotfixes section or E11", s.Name)
			}
			continue
		}
		seen++
		if start < 0 {
			t.Errorf("%s: no %q section", s.Name, strings.TrimSpace(hotfixSectionStart))
			continue
		}
		end := strings.Index(text[start:], hotfixSectionEnd)
		if end < 0 {
			t.Errorf("%s: the Hotfixes section does not end with a rule", s.Name)
			continue
		}
		got := text[start : start+end]
		switch {
		case section == "":
			section = got
		case got != section:
			t.Errorf("%s: the Hotfixes section differs from the other hotfix skills'", s.Name)
		}
		for _, want := range []string{
			"| E11 release branch (`release_branch`) |",
			"[WORKFLOW OWNER — HOTFIX DESIGNATION]",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not contain %q", s.Name, want)
			}
		}
	}
	if seen != len(hotfixSkills) {
		t.Errorf("found %d of the %d hotfix skills", seen, len(hotfixSkills))
	}
}
