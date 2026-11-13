package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// commentedConfigYAML is a valid configuration with comments on settings the
// tests change and on settings they leave alone.
const commentedConfigYAML = `# Project header comment.
schema_version: 2
project:
  # Comment on the integration branch key.
  integration_branch: main # line comment on the branch
  discovery:
    mode: implementer-judgment
  authorities:
    # Comment on an untouched authority.
    workflow_owner:
      role: lead maintainer
    change_acceptor:
      role: maintainer
    product_intent_acceptor:
      role: Workflow Owner
  guidance:
    manuals:
      # Comment on the go manual.
      - name: go
        version: "0.1"
`

// extensionPointID matches an extension-point identifier, which no
// configuration command may show.
var extensionPointID = regexp.MustCompile(`\bE[0-9]+\b`)

// schemaField matches a configuration field name, such as applies_when.
var schemaField = regexp.MustCompile(`\b[a-z]+_[a-z_]+\b|\[[0-9]+\]`)

func assertNoSchemaTerms(t *testing.T, what, text string) {
	t.Helper()
	if extensionPointID.MatchString(text) {
		t.Errorf("%s shows an extension-point identifier:\n%s", what, text)
	}
	for _, term := range []string{"project.", "tooling."} {
		if strings.Contains(text, term) {
			t.Errorf("%s shows the schema term %q:\n%s", what, term, text)
		}
	}
	if field := schemaField.FindString(text); field != "" {
		t.Errorf("%s shows the schema field %q:\n%s", what, field, text)
	}
}

func writeConfig(t *testing.T, content string) (dir, path string) {
	t.Helper()
	dir = newProjectDir(t)
	path = configPathIn(dir)
	writeFile(t, path, content)
	return dir, path
}

func assertValidFile(t *testing.T, path string) projectConfig {
	t.Helper()
	cfg, err := decodeConfig([]byte(readFile(t, path)))
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if problems := validateConfig(cfg); problems != nil {
		t.Fatalf("%s is invalid: %v", path, problems)
	}
	return cfg
}

func TestConfigShowNamesEverySetting(t *testing.T) {
	writeConfig(t, validConfigYAML)

	stdout, _, err := runCLI(t, "dev", "config", "show")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	for _, want := range []string{
		"Integration branch (integration-branch)",
		"staging (default)",
		"Workflow Owner (workflow-owner)",
		"Manuals (manual)",
		"Skills installed for (skill-target)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("show output lacks %q:\n%s", want, stdout)
		}
	}
	assertNoSchemaTerms(t, "show", stdout)
}

func TestConfigGetPrintsFlagFormats(t *testing.T) {
	writeConfig(t, validConfigYAML+`  release_conditions:
    - name: migration
      applies_when: a schema change
      required_evidence: a tested rollback
    - name: data
      applies_when: a backfill
      required_evidence: a dry run
  suggestions_backlog: "#24"
`)
	writeFile(t, configPathIn(mustGetwd(t)), strings.Replace(readFile(t, configPathIn(mustGetwd(t))),
		"      role: maintainer\n", "      role: maintainer\n      members: [alice, bob]\n", 1))

	cases := map[string]string{
		"integration-branch":  "staging (default)\n",
		"release-condition":   "migration|a schema change|a tested rollback\ndata|a backfill|a dry run\n",
		"suggestions-backlog": "#24\n",
		"change-acceptor":     "maintainer:alice,bob\n",
		"workflow-owner":      "lead maintainer\n",
		"manual":              "none (default)\n",
		"discovery":           "implementer-judgment\n",
		"small-corrections":   "disabled (default)\n",
	}
	for setting, want := range cases {
		stdout, _, err := runCLI(t, "dev", "config", "get", setting)
		if err != nil {
			t.Errorf("get %s: %v", setting, err)
			continue
		}
		if stdout != want {
			t.Errorf("get %s = %q, want %q", setting, stdout, want)
		}
	}
	if _, _, err := runCLI(t, "dev", "config", "get", "integration_branch"); err == nil ||
		!strings.Contains(err.Error(), "unknown setting") {
		t.Errorf("get with a schema field name: error = %v, want an unknown-setting error", err)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestConfigSetKeepsCommentsOnUntouchedNodesAndTheReplacedKey(t *testing.T) {
	_, path := writeConfig(t, commentedConfigYAML)

	stdout, _, err := runCLI(t, "dev", "config", "set", "integration-branch", "develop", "--yes")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !strings.Contains(stdout, "integration-branch: main -> develop") {
		t.Errorf("output does not show the change:\n%s", stdout)
	}
	got := readFile(t, path)
	want := strings.Replace(commentedConfigYAML, "integration_branch: main #", "integration_branch: develop #", 1)
	if got != want {
		t.Errorf("file after set:\n%s\nwant:\n%s", got, want)
	}
	assertValidFile(t, path)
}

func TestConfigRemoveReportsTheRemovedComments(t *testing.T) {
	_, path := writeConfig(t, commentedConfigYAML)

	stdout, _, err := runCLI(t, "dev", "config", "remove", "manual", "go", "--yes")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	for _, want := range []string{"manual: remove go@0.1", "will be removed with them", "# Comment on the go manual."} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
	got := readFile(t, path)
	if strings.Contains(got, "guidance") || strings.Contains(got, "go manual") {
		t.Errorf("the last manual's removal left its setting or comment:\n%s", got)
	}
	for _, kept := range []string{"# Project header comment.", "# Comment on an untouched authority.", "# line comment on the branch"} {
		if !strings.Contains(got, kept) {
			t.Errorf("an untouched comment %q was lost:\n%s", kept, got)
		}
	}
	assertValidFile(t, path)
}

func TestConfigChangesNeedConfirmationOutsideATerminal(t *testing.T) {
	for _, args := range [][]string{
		{"set", "integration-branch", "develop"},
		{"unset", "integration-branch"},
		{"add", "pr-review-trigger", "schema|a schema change"},
		{"remove", "manual", "go"},
	} {
		t.Run(args[0], func(t *testing.T) {
			_, path := writeConfig(t, commentedConfigYAML)
			_, _, err := runCLI(t, "dev", append([]string{"config"}, args...)...)
			if !errors.Is(err, errConfirmationRequired) {
				t.Fatalf("error = %v, want errConfirmationRequired", err)
			}
			if !strings.Contains(err.Error(), "--yes") {
				t.Errorf("the refusal does not mention --yes: %v", err)
			}
			if got := readFile(t, path); got != commentedConfigYAML {
				t.Errorf("an unconfirmed change was written:\n%s", got)
			}
		})
	}
}

func TestConfigRefusesAnInvalidResult(t *testing.T) {
	cases := []struct {
		name   string
		config string
		args   []string
		want   string
	}{
		{"classifier without small corrections", validConfigYAML + `  small_corrections:
    enabled: true
`, []string{"set", "small-corrections", "enabled"},
			"small-correction-classifier is required when small-corrections is enabled"},
		{"classifier left behind", validConfigYAML + `  small_corrections:
    enabled: true
`, []string{"set", "small-correction-classifier", "maintainer"}, ""},
		{"triggers without a trigger", validConfigYAML,
			[]string{"set", "discovery", "triggers"}, "discovery \"triggers\" needs at least one discovery-trigger"},
		{"trigger without the triggers mode", validConfigYAML,
			[]string{"add", "discovery-trigger", "schema|a schema change"},
			"discovery-trigger applies only when discovery is \"triggers\""},
		{"reviewer without a request", validConfigYAML,
			[]string{"set", "automated-reviewer", "codex"}, "automated-reviewer-request must not be empty"},
		{"bad branch", validConfigYAML,
			[]string{"set", "integration-branch", "a..b"}, "integration-branch: \"a..b\" is not a valid branch name"},
		{"bad backlog", validConfigYAML,
			[]string{"set", "suggestions-backlog", "24"}, "suggestions-backlog must be an issue reference"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, path := writeConfig(t, c.config)
			_, _, err := runCLI(t, "dev", append(append([]string{"config"}, c.args...), "--yes")...)
			if c.want == "" {
				// The first case's own config is invalid; this one is
				// refused for the same reason, before any change.
				if err == nil || !strings.Contains(err.Error(), "not a valid configuration") {
					t.Fatalf("error = %v, want a refusal of the invalid file", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want it to contain %q", err, c.want)
			}
			assertNoSchemaTerms(t, "the refusal", err.Error())
			if got := readFile(t, path); got != c.config {
				t.Errorf("a refused change was written:\n%s", got)
			}
		})
	}
}

func TestConfigSetsDependentSettingsTogether(t *testing.T) {
	_, path := writeConfig(t, validConfigYAML)
	steps := [][]string{
		{"set", "small-corrections", "enabled", "--small-correction-classifier", "maintainer:alice", "--small-corrections-without-issue"},
		{"set", "discovery", "triggers", "--discovery-trigger", "schema|a schema change", "--discovery-trigger", "auth|an authorization change"},
		{"set", "automated-reviewer", "codex", "--automated-reviewer-request", "comment @codex review"},
	}
	for _, args := range steps {
		if _, _, err := runCLI(t, "dev", append(append([]string{"config"}, args...), "--yes")...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	cfg := assertValidFile(t, path)
	pc := cfg.Project
	if !*pc.SmallCorrections.Enabled || !*pc.SmallCorrections.WithoutIssue ||
		formatAuthority(*pc.Authorities.SmallCorrectionClassifier) != "maintainer:alice" {
		t.Errorf("small corrections = %+v, classifier = %+v", pc.SmallCorrections, pc.Authorities.SmallCorrectionClassifier)
	}
	if pc.Discovery.Mode != discoveryTriggers || len(*pc.Discovery.Triggers) != 2 {
		t.Errorf("discovery = %+v", pc.Discovery)
	}
	if pc.AutomatedReviewer.Name != "codex" || pc.AutomatedReviewer.Request != "comment @codex review" {
		t.Errorf("automated reviewer = %+v", pc.AutomatedReviewer)
	}

	// Switching discovery back removes its triggers, and says so.
	stdout, _, err := runCLI(t, "dev", "config", "set", "discovery", "implementer-judgment", "--yes")
	if err != nil {
		t.Fatalf("set discovery implementer-judgment: %v", err)
	}
	if !strings.Contains(stdout, "discovery-trigger: remove schema|a schema change") {
		t.Errorf("the removed triggers are not shown:\n%s", stdout)
	}
	// Disabling small corrections needs the classifier removed with it.
	if _, _, err := runCLI(t, "dev", "config", "set", "small-corrections", "disabled", "--yes"); err == nil {
		t.Errorf("disabling small corrections while a classifier is configured was accepted")
	}
	if _, _, err := runCLI(t, "dev", "config", "unset", "small-corrections", "small-correction-classifier", "small-corrections-without-issue", "--yes"); err != nil {
		t.Errorf("unsetting small corrections with its classifier: %v", err)
	}
	cfg = assertValidFile(t, path)
	if cfg.Project.SmallCorrections != nil || cfg.Project.Authorities.SmallCorrectionClassifier != nil ||
		cfg.Project.Discovery.Triggers != nil {
		t.Errorf("project = %+v", cfg.Project)
	}

	// A companion option belongs to one setting.
	if _, _, err := runCLI(t, "dev", "config", "set", "integration-branch", "main", "--discovery-trigger", "a|b", "--yes"); err == nil ||
		!strings.Contains(err.Error(), "is set together with discovery") {
		t.Errorf("a companion option with the wrong setting: error = %v", err)
	}
}

func TestConfigUnsetRefusesARequiredSetting(t *testing.T) {
	for _, setting := range []string{"discovery", "workflow-owner", "change-acceptor", "product-intent-acceptor"} {
		_, path := writeConfig(t, validConfigYAML)
		_, _, err := runCLI(t, "dev", "config", "unset", setting, "--yes")
		if err == nil || !strings.Contains(err.Error(), "is required and cannot be unset") {
			t.Errorf("unset %s: error = %v, want a refusal", setting, err)
		}
		if got := readFile(t, path); got != validConfigYAML {
			t.Errorf("unset %s changed the file", setting)
		}
	}
}

func TestConfigAddWritesDeliveredAndConfirmedVersions(t *testing.T) {
	_, path := writeConfig(t, validConfigYAML)
	catalog, err := embeddedManualCatalog()
	if err != nil || len(catalog) == 0 {
		t.Fatalf("manual catalog: %v", err)
	}
	manual := catalog[0]

	if _, _, err := runCLI(t, "dev", "config", "add", "manual", manual.Name, "--yes"); err != nil {
		t.Fatalf("add manual: %v", err)
	}
	if _, _, err := runCLI(t, "dev", "config", "add", "skill-target", "codex", "--yes"); err != nil {
		t.Fatalf("add skill-target: %v", err)
	}
	cfg := assertValidFile(t, path)
	if got := cfg.Project.Guidance.Manuals; len(got) != 1 || got[0] != (manualChoice{manual.Name, manual.Version}) {
		t.Errorf("manuals = %+v, want %s@%s", got, manual.Name, manual.Version)
	}
	targets := *cfg.Tooling.SkillTargets
	confirmed := confirmedSkillTargets["codex"]
	if len(targets) != 1 || targets[0].Harness != "codex" || targets[0].ConfirmedVersion != confirmed.Version ||
		targets[0].ConfirmedOn != confirmed.On || targets[0].Path != supportedSkillTargets["codex"] {
		t.Errorf("skill targets = %+v", targets)
	}

	for _, args := range [][]string{
		{"add", "manual", "no-such-manual"},
		{"add", "manual", manual.Name},
		{"add", "skill-target", "vim"},
		{"remove", "manual", "no-such-manual"},
		{"add", "integration-branch", "main"},
		{"set", "manual", "go"},
		{"set", "conditional-guidance", "x"},
	} {
		before := readFile(t, path)
		if _, _, err := runCLI(t, "dev", append(append([]string{"config"}, args...), "--yes")...); err == nil {
			t.Errorf("%v was accepted", args)
		}
		if readFile(t, path) != before {
			t.Errorf("%v changed the file", args)
		}
	}
}

func TestConfigReportsNothingToChange(t *testing.T) {
	_, path := writeConfig(t, commentedConfigYAML)
	stdout, _, err := runCLI(t, "dev", "config", "set", "integration-branch", "main")
	if err != nil {
		t.Fatalf("set to the current value: %v", err)
	}
	if !strings.Contains(stdout, "Nothing to change") {
		t.Errorf("output = %q", stdout)
	}
	if readFile(t, path) != commentedConfigYAML {
		t.Errorf("the file changed")
	}
}

const retiredConfigYAML = validConfigYAML + `  guidance:
    manuals:
      - name: go
        version: "0.1"
    # A retired setting.
    conditional:
      - supabase-data-api-grants
`

func TestConfigUnsetRemovesTheRetiredSettingOnly(t *testing.T) {
	t.Run("the retired setting alone", func(t *testing.T) {
		_, path := writeConfig(t, retiredConfigYAML)
		stdout, _, err := runCLI(t, "dev", "config", "unset", "conditional-guidance", "--yes")
		if err != nil {
			t.Fatalf("unset conditional-guidance: %v", err)
		}
		if !strings.Contains(stdout, "# A retired setting.") {
			t.Errorf("the removed comment is not reported:\n%s", stdout)
		}
		cfg := assertValidFile(t, path)
		if len(cfg.Project.Guidance.Manuals) != 1 || cfg.Project.Guidance.Conditional != nil {
			t.Errorf("guidance = %+v", cfg.Project.Guidance)
		}
	})
	refused := []struct {
		name   string
		config string
		args   []string
	}{
		{"another change", retiredConfigYAML, []string{"set", "integration-branch", "main"}},
		{"with another setting", retiredConfigYAML, []string{"unset", "conditional-guidance", "integration-branch"}},
		{"a result still invalid", retiredConfigYAML + "  integration_branch: \"a..b\"\n", []string{"unset", "conditional-guidance"}},
	}
	for _, c := range refused {
		t.Run(c.name, func(t *testing.T) {
			_, path := writeConfig(t, c.config)
			_, _, err := runCLI(t, "dev", append(append([]string{"config"}, c.args...), "--yes")...)
			if err == nil {
				t.Fatalf("%v on an invalid file was accepted", c.args)
			}
			assertNoSchemaTerms(t, "the refusal", err.Error())
			if readFile(t, path) != c.config {
				t.Errorf("the invalid file was changed")
			}
		})
	}
	t.Run("the refusal names the command", func(t *testing.T) {
		writeConfig(t, retiredConfigYAML)
		_, _, err := runCLI(t, "dev", "config", "get", "manual")
		if err == nil || !strings.Contains(err.Error(), `garage config unset conditional-guidance`) {
			t.Errorf("error = %v", err)
		}
	})
}

func TestConfigRefusesAnOldSchemaVersion(t *testing.T) {
	_, path := writeConfig(t, "schema_version: 1\n")
	_, _, err := runCLI(t, "dev", "config", "set", "integration-branch", "main", "--yes")
	if err == nil || !strings.Contains(err.Error(), `run "garage init" to upgrade it`) {
		t.Fatalf("error = %v, want a pointer to garage init", err)
	}
	if readFile(t, path) != "schema_version: 1\n" {
		t.Errorf("the file was changed")
	}
}

func TestConfigChangeIsConfirmedInATerminal(t *testing.T) {
	for _, c := range []struct {
		answer string
		write  bool
	}{{"y", true}, {"", false}, {"n", false}} {
		t.Run("answer "+c.answer, func(t *testing.T) {
			dir, path := writeConfig(t, commentedConfigYAML)
			ask := &scriptedPrompter{answers: []string{c.answer}}
			var out bytes.Buffer
			err := changeConfiguration(dir, configChange{apply: func(cfg *projectConfig) error {
				o, _ := optionByName("integration-branch")
				return o.set(cfg, "develop")
			}}, ask, false, &out)
			if err != nil {
				t.Fatalf("change: %v", err)
			}
			if !strings.Contains(ask.transcript(), "integration-branch: main -> develop") ||
				!strings.Contains(ask.transcript(), "Write this change? [y/N]") {
				t.Errorf("the confirmation does not show the change:\n%s", ask.transcript())
			}
			written := readFile(t, path) != commentedConfigYAML
			if written != c.write {
				t.Errorf("written = %v, want %v", written, c.write)
			}
		})
	}
}

func TestConfigChangeNeverOverwritesAConcurrentChange(t *testing.T) {
	dir, path := writeConfig(t, commentedConfigYAML)
	concurrent := strings.Replace(commentedConfigYAML, "main #", "trunk #", 1)
	ask := &scriptedPrompter{answers: []string{"y"}, onAsk: func(int) { writeFile(t, path, concurrent) }}
	err := changeConfiguration(dir, configChange{apply: func(cfg *projectConfig) error {
		o, _ := optionByName("integration-branch")
		return o.set(cfg, "develop")
	}}, ask, false, &bytes.Buffer{})
	if !errors.Is(err, errConfigEditChanged) {
		t.Fatalf("error = %v, want errConfigEditChanged", err)
	}
	if readFile(t, path) != concurrent {
		t.Errorf("the concurrent change was overwritten")
	}
	assertNoDirectoryLeftovers(t, dir)
}

func assertNoDirectoryLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(configPathIn(dir)))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != configFileName {
			t.Errorf("unexpected file left in %s: %s", garageDirName, e.Name())
		}
	}
}

func TestConfigSettingsAreInitsFlags(t *testing.T) {
	var flags []string
	newInitCommand().Flags().VisitAll(func(f *pflag.Flag) { flags = append(flags, f.Name) })
	var names []string
	for _, o := range allOptions() {
		if o.kind != optionRetired {
			names = append(names, o.name)
		}
		switch {
		case o.get == nil || o.unset == nil:
			t.Errorf("%s cannot be read or unset", o.name)
		case o.kind == optionValue && o.set == nil:
			t.Errorf("%s cannot be set", o.name)
		case o.kind == optionList && (o.add == nil || o.remove == nil):
			t.Errorf("%s cannot be added to or removed from", o.name)
		case len(o.fields) == 0:
			t.Errorf("%s names no configuration field", o.name)
		}
	}
	slices.Sort(flags)
	slices.Sort(names)
	if !slices.Equal(flags, names) {
		t.Errorf("setting names differ from init's flags:\n settings %v\n flags    %v", names, flags)
	}
	for flag := range setCompanions {
		if _, ok := optionByName(flag); !ok {
			t.Errorf("companion option --%s is not a setting", flag)
		}
	}
}

func TestConfigCommandHelpShowsNoIdentifiers(t *testing.T) {
	for _, cmd := range []string{"show", "get", "set", "unset", "add", "remove"} {
		stdout, _, err := runCLI(t, "dev", "config", cmd, "--help")
		if err != nil {
			t.Fatalf("%s --help: %v", cmd, err)
		}
		if extensionPointID.MatchString(stdout) {
			t.Errorf("config %s --help shows an extension-point identifier:\n%s", cmd, stdout)
		}
	}
}

func TestTranslatedProblemsUseSettingNames(t *testing.T) {
	invalid := []string{
		validConfigYAML + "  pr_review_triggers:\n    - {name: a, applies_when: \"\"}\n",
		validConfigYAML + "  small_corrections:\n    without_issue: true\n",
		validConfigYAML + "  release_conditions:\n    - {name: a, applies_when: x, required_evidence: \"\"}\n",
		validConfigYAML + "  user_facing_inspection:\n    additional_conditions: [a, a]\n",
		validConfigYAML + "  guidance:\n    manuals:\n      - {name: nope, version: \"1\"}\n",
		validConfigYAML + "tooling:\n  skill_targets:\n    - {harness: vim, scope: team, confirmed_version: \"\", confirmed_on: x}\n",
		strings.Replace(validConfigYAML, "      role: maintainer\n", "      role: \"\"\n      members: [a, a]\n", 1),
		strings.Replace(validConfigYAML, "implementer-judgment", "sometimes", 1),
	}
	for _, content := range invalid {
		problems := validateConfig(decodeOrFail(t, content))
		if problems == nil {
			t.Errorf("expected problems for:\n%s", content)
			continue
		}
		assertNoSchemaTerms(t, "translated problems", formatProblems(problems))
	}
}
