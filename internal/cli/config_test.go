package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validConfigYAML is a complete, valid configuration that the cases below
// modify one setting at a time.
const validConfigYAML = `schema_version: 2
project:
  discovery:
    mode: implementer-judgment
  authorities:
    workflow_owner:
      role: lead maintainer
    change_acceptor:
      role: maintainer
    product_intent_acceptor:
      role: Workflow Owner
`

func decodeOrFail(t *testing.T, content string) projectConfig {
	t.Helper()
	cfg, err := decodeConfig([]byte(content))
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, content)
	}
	return cfg
}

func TestDecodeConfigIsStrict(t *testing.T) {
	cases := map[string]string{
		"unknown top-level key": "schema_version: 2\nextra: true\n",
		"unknown project key":   "schema_version: 2\nproject:\n  branch: main\n",
		"wrong type":            "schema_version: two\n",
		"two documents":         "schema_version: 2\n---\nschema_version: 2\n",
		"empty":                 "",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeConfig([]byte(content)); err == nil {
				t.Errorf("expected %q to be rejected", content)
			}
		})
	}
}

func TestValidateAcceptsCompleteConfiguration(t *testing.T) {
	full := validConfigYAML + `  integration_branch: release/main-2
  pr_review_triggers:
    - name: security-sensitive
      applies_when: the change touches authentication
  automated_reviewer:
    name: codex
    request: automatic review when a pull request opens
  small_corrections:
    enabled: true
    without_issue: true
  release_conditions:
    - name: production
      applies_when: the change is deployed
      required_evidence: a post-deployment check
  suggestions_backlog: "#24"
  user_facing_inspection:
    additional_conditions:
      - the change alters printed output
  guidance:
    manuals:
      - name: go
        version: "0.1"
    conditional:
      - supabase
tooling:
  skill_targets:
    - harness: codex
      scope: user
      path:
        override: $CODEX_HOME/skills
        default: ~/.codex/skills
      confirmed_version: 0.137.0
      confirmed_on: "2026-11-12"
`
	full = strings.Replace(full, "      role: Workflow Owner\n",
		"      role: Workflow Owner\n    small_correction_classifier:\n      role: maintainer\n      members: [alice]\n", 1)
	if problems := validateConfig(decodeOrFail(t, full)); problems != nil {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if problems := validateConfig(decodeOrFail(t, validConfigYAML)); problems != nil {
		t.Fatalf("minimal configuration: unexpected problems: %v", problems)
	}
}

func TestValidateReportsEachInvalidValue(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"schema version 1", "schema_version: 1\n", "run \"garage init\" to upgrade"},
		{"unsupported version", "schema_version: 7\n", "schema_version must be 2"},
		{"no project", "schema_version: 2\n", "project.discovery (E5) is required"},
		{"E1 branch", validConfigYAML + "  integration_branch: \"a..b\"\n", "project.integration_branch (E1)"},
		{"E1 empty", validConfigYAML + "  integration_branch: \"\"\n", "project.integration_branch (E1)"},
		{"E2 duplicate", validConfigYAML + "  pr_review_triggers:\n    - {name: a, applies_when: x}\n    - {name: a, applies_when: y}\n", "duplicate name"},
		{"E2 empty", validConfigYAML + "  pr_review_triggers:\n    - {name: a, applies_when: \"\"}\n", "applies_when must not be empty"},
		{"E3 request", validConfigYAML + "  automated_reviewer:\n    name: codex\n    request: \"\"\n", "automated_reviewer.request (E3)"},
		{"E4 enabled missing", validConfigYAML + "  small_corrections:\n    without_issue: true\n", "enabled (E4) is required"},
		{"E4 without issue", validConfigYAML + "  small_corrections:\n    enabled: false\n    without_issue: true\n", "applies only when small corrections are enabled"},
		{"E4 classifier missing", validConfigYAML + "  small_corrections:\n    enabled: true\n", "small_correction_classifier (E8) is required"},
		{"E5 mode", strings.Replace(validConfigYAML, "implementer-judgment", "sometimes", 1), "project.discovery.mode (E5)"},
		{"E5 triggers missing", strings.Replace(validConfigYAML, "implementer-judgment", "triggers", 1), "at least one trigger"},
		{"E5 triggers with judgment", strings.Replace(validConfigYAML, "    mode: implementer-judgment\n",
			"    mode: implementer-judgment\n    triggers:\n      - {name: a, applies_when: x}\n", 1), "apply only when mode is"},
		{"E6 evidence", validConfigYAML + "  release_conditions:\n    - {name: a, applies_when: x, required_evidence: \"\"}\n", "required_evidence must not be empty"},
		{"E7 reference", validConfigYAML + "  suggestions_backlog: issue-24\n", "project.suggestions_backlog (E7)"},
		{"E8 workflow owner", strings.Replace(validConfigYAML, "    workflow_owner:\n      role: lead maintainer\n", "", 1), "workflow_owner (E8) is required"},
		{"E8 empty role", strings.Replace(validConfigYAML, "role: maintainer", "role: \"\"", 1), "change_acceptor (E8).role must not be empty"},
		{"E8 duplicate member", strings.Replace(validConfigYAML, "role: maintainer", "role: maintainer\n      members: [a, a]", 1), "listed more than once"},
		{"E8 classifier without E4", strings.Replace(validConfigYAML, "      role: Workflow Owner\n",
			"      role: Workflow Owner\n    small_correction_classifier:\n      role: maintainer\n", 1), "applies only when small corrections are enabled"},
		{"E9 duplicate", validConfigYAML + "  user_facing_inspection:\n    additional_conditions: [x, x]\n", "(E9)"},
		{"E10 duplicate manual", validConfigYAML + "  guidance:\n    manuals:\n      - {name: go, version: \"0.1\"}\n      - {name: go, version: \"0.2\"}\n", "listed more than once"},
		{"E10 version", validConfigYAML + "  guidance:\n    manuals:\n      - {name: go, version: \"zero one\"}\n", "single word"},
		{"tooling harness", validConfigYAML + "tooling:\n  skill_targets:\n    - {harness: cursor, scope: user, path: {override: x, default: y}, confirmed_version: \"1\", confirmed_on: \"2026-11-12\"}\n", "harness must be one of"},
		{"tooling path", validConfigYAML + "tooling:\n  skill_targets:\n    - {harness: codex, scope: user, path: {override: /tmp, default: /tmp}, confirmed_version: \"1\", confirmed_on: \"2026-11-12\"}\n", "confirmed rule for codex"},
		{"tooling scope", validConfigYAML + "tooling:\n  skill_targets:\n    - {harness: codex, scope: project, path: {override: $CODEX_HOME/skills, default: ~/.codex/skills}, confirmed_version: \"1\", confirmed_on: \"2026-11-12\"}\n", "scope must be \"user\""},
		{"tooling date", validConfigYAML + "tooling:\n  skill_targets:\n    - {harness: codex, scope: user, path: {override: $CODEX_HOME/skills, default: ~/.codex/skills}, confirmed_version: \"1\", confirmed_on: \"12/11/2026\"}\n", "YYYY-MM-DD"},
		{"tooling duplicate", validConfigYAML + "tooling:\n  skill_targets:\n" +
			"    - {harness: codex, scope: user, path: {override: $CODEX_HOME/skills, default: ~/.codex/skills}, confirmed_version: \"1\", confirmed_on: \"2026-11-12\"}\n" +
			"    - {harness: codex, scope: user, path: {override: $CODEX_HOME/skills, default: ~/.codex/skills}, confirmed_version: \"2\", confirmed_on: \"2026-11-12\"}\n", "listed more than once"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems := validateConfig(decodeOrFail(t, c.content))
			if problems == nil {
				t.Fatalf("expected a problem containing %q", c.want)
			}
			if !strings.Contains(problems.Error(), c.want) {
				t.Errorf("problems do not mention %q:\n%v", c.want, problems)
			}
		})
	}
}

func TestValidateReportsAllProblemsTogether(t *testing.T) {
	problems := validateConfig(decodeOrFail(t, "schema_version: 2\nproject:\n  integration_branch: \"-x\"\n  suggestions_backlog: bad\n"))
	for _, want := range []string{"(E1)", "(E5) is required", "(E7)", "workflow_owner (E8)", "change_acceptor (E8)", "product_intent_acceptor (E8)"} {
		if !strings.Contains(problems.Error(), want) {
			t.Errorf("problems do not include %q:\n%v", want, problems)
		}
	}
}

func TestCheckBranchName(t *testing.T) {
	for _, ok := range []string{"main", "staging", "release/2026-11", "feat_x"} {
		if err := checkBranchName(ok); err != nil {
			t.Errorf("checkBranchName(%q) = %v, want valid", ok, err)
		}
	}
	for _, bad := range []string{"", "-main", "a..b", "a b", "a~b", "a:b", "x.lock", "a/", "/a", ".a", "@", "a@{b", "a//b"} {
		if err := checkBranchName(bad); err == nil {
			t.Errorf("checkBranchName(%q) accepted an invalid name", bad)
		}
	}
}

func TestConfigValidateReportsAValidConfiguration(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, configPathIn(dir), validConfigYAML)

	stdout, stderr, err := runCLI(t, "dev", "config", "validate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"is valid (schema version 2)",
		"E1   integration branch           staging (default)",
		"E5   discovery",
		"Workflow Owner               lead maintainer",
		"skill targets: none",
		"Not certified: that configured authorities are human",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want it empty", stderr)
	}
}

func TestConfigValidateFailsWithoutConfiguration(t *testing.T) {
	newProjectDir(t)

	stdout, stderr, err := runCLI(t, "dev", "config", "validate")
	if !errors.Is(err, errNoConfig) {
		t.Fatalf("error = %v, want errNoConfig", err)
	}
	if !strings.Contains(err.Error(), "garage init") {
		t.Errorf("error = %q, want it to point to garage init", err)
	}
	if stdout != "" || !strings.Contains(stderr, "Error:") {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestConfigValidateFailsOnInvalidConfiguration(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, configPathIn(dir), "schema_version: 2\n")

	stdout, _, err := runCLI(t, "dev", "config", "validate")
	var problems configProblems
	if !errors.As(err, &problems) {
		t.Fatalf("error = %v, want configuration problems", err)
	}
	if len(problems) != 4 {
		t.Errorf("problems = %v, want the four missing required values", problems)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want it empty on failure", stdout)
	}
}

func TestConfigValidateFailsOnUnparsableConfiguration(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, configPathIn(dir), "schema_version: 2\nunknown: 1\n")

	if _, _, err := runCLI(t, "dev", "config", "validate"); err == nil || !strings.Contains(err.Error(), "cannot parse") {
		t.Fatalf("error = %v, want a parse error", err)
	}
}

func TestConfigValidateRefusesSymlinkedConfiguration(t *testing.T) {
	dir := newProjectDir(t)
	external := filepath.Join(t.TempDir(), "external.yaml")
	writeFile(t, external, validConfigYAML)
	if err := os.Mkdir(filepath.Join(dir, garageDirName), 0o755); err != nil {
		t.Fatalf("create %s: %v", garageDirName, err)
	}
	symlinkOrSkip(t, external, configPathIn(dir))

	if _, _, err := runCLI(t, "dev", "config", "validate"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("error = %v, want the symbolic link to be refused", err)
	}
}

func TestConfigValidateIsReadOnly(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, configPathIn(dir), "schema_version: 1\n")

	if _, _, err := runCLI(t, "dev", "config", "validate"); err == nil {
		t.Fatal("expected a schema version 1 file to be reported as needing an upgrade")
	}
	if got := readFile(t, configPathIn(dir)); got != "schema_version: 1\n" {
		t.Errorf("validate modified the configuration: %q", got)
	}
	if got := entryNames(t, filepath.Join(dir, garageDirName)); len(got) != 1 {
		t.Errorf("validate created files: %v", got)
	}
}

func TestConfigCommandRequiresASubcommand(t *testing.T) {
	newProjectDir(t)
	stdout, _, err := runCLI(t, "dev", "config")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "validate") {
		t.Errorf("config help does not list validate:\n%s", stdout)
	}
}
