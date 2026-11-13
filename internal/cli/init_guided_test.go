package cli

import (
	"regexp"
	"strings"
	"testing"
)

// defaultAnswers accept every default of a new configuration and name the
// three required authorities, the last two by reuse.
var defaultAnswers = []string{
	"", "", "", "", "", "", "", // branch, PR reviews, reviewer, small corrections, discovery, release, suggestions
	"lead maintainer", "alice", // Workflow Owner
	"1",        // change acceptor: same as the Workflow Owner
	"1",        // product-intent acceptor: same as the Workflow Owner
	"", "", "", // inspection, guidance, agent tools
}

func TestGuidedSetupWithDefaultsWritesOnlyChoicesMade(t *testing.T) {
	noDefaultBranch(t)
	dir := t.TempDir()
	ask := &scriptedPrompter{answers: append(append([]string(nil), defaultAnswers...), "y")}

	result, err := initProject(dir, projectConfig{}, ask)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != initCreated || len(ask.answers) != 0 {
		t.Fatalf("action = %v, unused answers %q", result.Action, ask.answers)
	}
	cfg := mustLoadValid(t, dir)
	pc := cfg.Project
	if pc.IntegrationBranch != nil || pc.AutomatedReviewer != nil || pc.SmallCorrections != nil ||
		pc.SuggestionsBacklog != nil || pc.Guidance != nil || cfg.Tooling != nil {
		t.Errorf("a default was written explicitly: %+v, tooling %+v", pc, cfg.Tooling)
	}
	if pc.Discovery.Mode != discoveryJudgment {
		t.Errorf("discovery = %+v", pc.Discovery)
	}
	a := pc.Authorities
	if a.ChangeAcceptor.Role != "lead maintainer" || a.ProductIntentAcceptor.Members[0] != "alice" {
		t.Errorf("reused authorities = %+v, %+v", a.ChangeAcceptor, a.ProductIntentAcceptor)
	}
}

func TestGuidedSetupKeepsFrameworkIdentifiersInternal(t *testing.T) {
	noDefaultBranch(t)
	ask := &scriptedPrompter{answers: append(append([]string(nil), defaultAnswers...), "y")}
	if _, err := initProject(t.TempDir(), projectConfig{}, ask); err != nil {
		t.Fatal(err)
	}
	all := ask.transcript()
	if regexp.MustCompile(`\bE(10|[1-9])\b`).MatchString(all) {
		t.Errorf("the setup flow shows an extension-point identifier:\n%s", all)
	}
	for _, internal := range []string{"ROLE", "MEMBERS", "NAME|WHEN", "implementer-judgment"} {
		if strings.Contains(all, internal) {
			t.Errorf("the setup flow shows internal syntax %q", internal)
		}
	}
	if key := regexp.MustCompile(`\b(project|tooling)\.[a-z]`).FindString(all); key != "" {
		t.Errorf("the setup flow shows a configuration key (%q)", key)
	}
	for _, explained := range []string{"The Workflow Owner makes", "The change acceptor decides", "The product-intent acceptor decides"} {
		if !strings.Contains(all, explained) {
			t.Errorf("the setup flow does not explain %q", explained)
		}
	}
	if !strings.Contains(all, "Integration branch") || !strings.Contains(all, "staging (default)") {
		t.Errorf("the summary does not show the defaults that apply:\n%s", all)
	}
}

func TestGuidedSetupDeclinedWritesNothing(t *testing.T) {
	for _, answer := range []string{"", "n", "no"} {
		t.Run("answer "+answer, func(t *testing.T) {
			noDefaultBranch(t)
			dir := t.TempDir()
			ask := &scriptedPrompter{answers: append(append([]string(nil), defaultAnswers...), answer)}
			result, err := initProject(dir, projectConfig{}, ask)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Action != initDeclined {
				t.Errorf("action = %v, want declined", result.Action)
			}
			if got := entryNames(t, dir); len(got) != 0 {
				t.Errorf("project directory contains %v, want nothing written", got)
			}
		})
	}
}

func TestGuidedSetupRejectsPlaceholders(t *testing.T) {
	noDefaultBranch(t)
	dir := t.TempDir()
	answers := []string{"", "", "", "", "", "", ""}
	answers = append(answers,
		"ROLE", "<role>", "TBD", "role:members", "maintainer, owner", // each rejected
		"lead maintainer",
		"MEMBERS", // rejected placeholder member
		"alice",
		"1", "1", "", "", "", "y")
	ask := &scriptedPrompter{answers: answers}
	if _, err := initProject(dir, projectConfig{}, ask); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ask.answers) != 0 {
		t.Fatalf("unused answers %q", ask.answers)
	}
	wo := mustLoadValid(t, dir).Project.Authorities.WorkflowOwner
	if wo.Role != "lead maintainer" || len(wo.Members) != 1 || wo.Members[0] != "alice" {
		t.Errorf("workflow owner = %+v", wo)
	}
	if !strings.Contains(ask.transcript(), "looks like a placeholder") {
		t.Error("the placeholder rejection was not explained")
	}
}

func TestGuidedSetupSuggestsTheRepositoryDefaultBranch(t *testing.T) {
	saved := detectDefaultBranch
	detectDefaultBranch = func(string) string { return "main" }
	t.Cleanup(func() { detectDefaultBranch = saved })
	dir := t.TempDir()
	ask := &scriptedPrompter{answers: append(append([]string(nil), defaultAnswers...), "y")}
	if _, err := initProject(dir, projectConfig{}, ask); err != nil {
		t.Fatal(err)
	}
	if b := mustLoadValid(t, dir).Project.IntegrationBranch; b == nil || *b != "main" {
		t.Errorf("integration branch = %v, want the suggested main", b)
	}
}

// Every choice made through the menus produces exactly the file that the
// equivalent flags produce.
func TestGuidedSetupMatchesTheEquivalentFlags(t *testing.T) {
	noDefaultBranch(t)
	guided := t.TempDir()
	ask := &scriptedPrompter{answers: []string{
		"2", "develop", // another integration branch
		"2", "security-sensitive", "the change touches authentication", "n",
		"2", "reviewbot", "it reviews each pull request when it is opened",
		"2", "y", // small corrections, without an issue
		"2", "schema-change", "the change alters a persisted schema", "n",
		"2", "payments", "the change touches payment processing", "a security sign-off", "n",
		"2", "24", // suggestions in issue #24
		"lead maintainer", "alice, bob", // Workflow Owner
		"2", "maintainer", "", // change acceptor: someone else
		"1", // product-intent acceptor: same as the Workflow Owner
		"2", // classifier: same as the change acceptor
		"2", "the change alters a user-visible flow", "n",
		"2", "y", "go", "0.1", "", "y", "supabase-data-api", "n",
		"1,3", // Claude Code and GitHub Copilot CLI
		"yes",
	}}
	if _, err := initProject(guided, projectConfig{}, ask); err != nil {
		t.Fatalf("guided setup: %v", err)
	}
	if len(ask.answers) != 0 {
		t.Fatalf("unused answers %q", ask.answers)
	}

	flagged := t.TempDir()
	desired := desiredFrom(t,
		"--integration-branch", "develop",
		"--pr-review-trigger", "security-sensitive|the change touches authentication",
		"--automated-reviewer", "reviewbot",
		"--automated-reviewer-request", "it reviews each pull request when it is opened",
		"--small-corrections", "enabled", "--small-corrections-without-issue",
		"--discovery", "triggers", "--discovery-trigger", "schema-change|the change alters a persisted schema",
		"--release-condition", "payments|the change touches payment processing|a security sign-off",
		"--suggestions-backlog", "#24",
		"--workflow-owner", "lead maintainer:alice,bob",
		"--change-acceptor", "maintainer",
		"--product-intent-acceptor", "lead maintainer:alice,bob",
		"--small-correction-classifier", "maintainer",
		"--inspection-condition", "the change alters a user-visible flow",
		"--manual", "go@0.1",
		"--conditional-guidance", "supabase-data-api",
		"--skill-target", "claude-code@2.1.277@2026-11-12",
		"--skill-target", "copilot-cli@1.0.93-1@2026-11-12",
	)
	if _, err := initProject(flagged, desired, nil); err != nil {
		t.Fatalf("flag setup: %v", err)
	}
	got, want := readFile(t, configPathIn(guided)), readFile(t, configPathIn(flagged))
	if got != want {
		t.Errorf("guided setup wrote:\n%s\nthe equivalent flags write:\n%s", got, want)
	}
}

// An interactive upgrade asks only for missing required values, never turns
// a default into an explicit value, and confirms before writing.
func TestGuidedUpgradeAsksOnlyMissingRequiredValues(t *testing.T) {
	const existing = `schema_version: 2
project:
  discovery:
    mode: implementer-judgment
  authorities:
    workflow_owner:
      role: lead maintainer
    product_intent_acceptor:
      role: lead maintainer
`
	t.Run("confirmed", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, configPathIn(dir), existing)
		ask := &scriptedPrompter{answers: []string{"1", "y"}} // change acceptor: same as the Workflow Owner
		result, err := initProject(dir, projectConfig{}, ask)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Action != initUpdated || len(ask.answers) != 0 {
			t.Fatalf("action = %v, unused answers %q", result.Action, ask.answers)
		}
		all := ask.transcript()
		if strings.Contains(all, "Integration branch\nWhich") || strings.Contains(all, "Small corrections\nMay") {
			t.Error("an upgrade asked about an optional choice")
		}
		if !strings.Contains(all, "will add to .garage/config.yaml: change acceptor") {
			t.Errorf("the confirmation does not say what will be added:\n%s", all)
		}
		cfg := mustLoadValid(t, dir)
		if cfg.Project.IntegrationBranch != nil || cfg.Project.SmallCorrections != nil {
			t.Errorf("the upgrade wrote an optional value: %+v", cfg.Project)
		}
		if cfg.Project.Authorities.ChangeAcceptor.Role != "lead maintainer" {
			t.Errorf("change acceptor = %+v", cfg.Project.Authorities.ChangeAcceptor)
		}
	})
	t.Run("declined", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, configPathIn(dir), existing)
		ask := &scriptedPrompter{answers: []string{"1", "n"}}
		result, err := initProject(dir, projectConfig{}, ask)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Action != initDeclined {
			t.Errorf("action = %v, want declined", result.Action)
		}
		if got := readFile(t, configPathIn(dir)); got != existing {
			t.Errorf("a declined upgrade changed the file:\n%s", got)
		}
	})
}

func TestGuideMenusAskAgainOnInvalidAnswers(t *testing.T) {
	ask := &scriptedPrompter{answers: []string{"0", "3", "x", "2"}}
	n, err := guide{p: ask}.choose("Pick", []choice{{label: "a"}, {label: "b"}}, 0)
	if err != nil || n != 1 {
		t.Fatalf("choose = %d, %v; want 1", n, err)
	}
	many := &scriptedPrompter{answers: []string{"1,9", "3, 1"}}
	got, err := guide{p: many}.chooseMany("Pick", []choice{{label: "a"}, {label: "b"}, {label: "c"}})
	if err != nil || len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("chooseMany = %v, %v; want [0 2]", got, err)
	}
	yn := &scriptedPrompter{answers: []string{"maybe", "Y"}}
	if ok, err := (guide{p: yn}).yesNo("Sure?", false); err != nil || !ok {
		t.Fatalf("yesNo = %v, %v; want true", ok, err)
	}
}
