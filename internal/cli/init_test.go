package cli

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newProjectDir returns an empty temporary directory and makes it the working
// directory for the duration of the test, so "garage init" never touches a
// real project.
func newProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func configPathIn(projectDir string) string {
	return filepath.Join(projectDir, garageDirName, configFileName)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are not available: %v", err)
	}
}

func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read directory %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// requiredFlags supplies every value that has no default.
var requiredFlags = []string{
	"--discovery", "implementer-judgment",
	"--workflow-owner", "lead maintainer",
	"--change-acceptor", "maintainer",
	"--product-intent-acceptor", "Workflow Owner",
}

func initArgs(extra ...string) []string {
	return append(append([]string{"init"}, requiredFlags...), extra...)
}

// desiredFrom parses init flags the way the command does, for tests that
// call initProject directly.
func desiredFrom(t *testing.T, args ...string) projectConfig {
	t.Helper()
	cmd := newInitCommand()
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("parse flags %v: %v", args, err)
	}
	var f initFlags
	read := func(name string) string { v, _ := cmd.Flags().GetString(name); return v }
	readAll := func(name string) []string { v, _ := cmd.Flags().GetStringArray(name); return v }
	f.IntegrationBranch = read("integration-branch")
	f.PRReviewTriggers = readAll("pr-review-trigger")
	f.AutomatedReviewer = read("automated-reviewer")
	f.AutomatedReviewerRequest = read("automated-reviewer-request")
	f.SmallCorrections = read("small-corrections")
	f.SmallCorrectionsWithoutIssue, _ = cmd.Flags().GetBool("small-corrections-without-issue")
	f.Discovery = read("discovery")
	f.DiscoveryTriggers = readAll("discovery-trigger")
	f.ReleaseConditions = readAll("release-condition")
	f.SuggestionsBacklog = read("suggestions-backlog")
	f.WorkflowOwner = read("workflow-owner")
	f.ChangeAcceptor = read("change-acceptor")
	f.ProductIntentAcceptor = read("product-intent-acceptor")
	f.SmallCorrectionClassifier = read("small-correction-classifier")
	f.InspectionConditions = readAll("inspection-condition")
	f.Manuals = readAll("manual")
	f.ConditionalGuidance = readAll("conditional-guidance")
	f.SkillTargets = readAll("skill-target")
	cfg, err := configFromFlags(f, cmd.Flags().Changed)
	if err != nil {
		t.Fatalf("configFromFlags: %v", err)
	}
	return cfg
}

func mustLoadValid(t *testing.T, projectDir string) projectConfig {
	t.Helper()
	cfg, err := loadValidConfig(projectDir)
	if err != nil {
		t.Fatalf("configuration does not validate: %v\n%s", err, readFile(t, configPathIn(projectDir)))
	}
	return cfg
}

// scriptedPrompter answers prompts from a fixed list.
type scriptedPrompter struct {
	answers   []string
	questions []string
	// onAsk, when set, runs before each question is answered; n counts
	// questions from 1.
	onAsk func(n int)
}

func (s *scriptedPrompter) Ask(question string) (string, error) {
	s.questions = append(s.questions, question)
	if s.onAsk != nil {
		s.onAsk(len(s.questions))
	}
	if len(s.answers) == 0 {
		return "", errors.New("input ended before every required value was supplied")
	}
	answer := s.answers[0]
	s.answers = s.answers[1:]
	return answer, nil
}

func TestInitCreatesValidConfiguration(t *testing.T) {
	dir := newProjectDir(t)

	stdout, stderr, err := runCLI(t, "dev", initArgs()...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := entryNames(t, filepath.Join(dir, garageDirName)); len(got) != 1 || got[0] != configFileName {
		t.Errorf("%s contains %v, want only %s", garageDirName, got, configFileName)
	}
	for _, want := range []string{
		"Initialized Builder Garage",
		"created " + filepath.Join(garageDirName, configFileName),
		"schema version 2",
		"garage config validate",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want it empty", stderr)
	}

	cfg := mustLoadValid(t, dir)
	if cfg.SchemaVersion != configSchemaVersion {
		t.Errorf("schema version = %d, want %d", cfg.SchemaVersion, configSchemaVersion)
	}
	a := cfg.Project.Authorities
	if a.WorkflowOwner.Role != "lead maintainer" || a.ChangeAcceptor.Role != "maintainer" ||
		a.ProductIntentAcceptor.Role != "Workflow Owner" {
		t.Errorf("authorities = %+v %+v %+v", a.WorkflowOwner, a.ChangeAcceptor, a.ProductIntentAcceptor)
	}
	if cfg.Project.IntegrationBranch != nil {
		t.Errorf("integration_branch = %q, want it omitted so the default applies", *cfg.Project.IntegrationBranch)
	}
	if !strings.HasPrefix(readFile(t, configPathIn(dir)), configHeader) {
		t.Error("the new configuration does not start with the explanatory header")
	}
}

func TestInitWithoutRequiredValuesWritesNothing(t *testing.T) {
	dir := newProjectDir(t)

	stdout, stderr, err := runCLI(t, "dev", "init", "--integration-branch", "main")
	if !errors.Is(err, errMissingRequired) {
		t.Fatalf("error = %v, want errMissingRequired", err)
	}
	for _, flag := range []string{"--discovery", "--workflow-owner", "--change-acceptor", "--product-intent-acceptor"} {
		if !strings.Contains(err.Error(), flag) {
			t.Errorf("error does not name %s: %v", flag, err)
		}
	}
	if !strings.Contains(err.Error(), "nothing was written") {
		t.Errorf("error = %q, want it to say nothing was written", err)
	}
	if !strings.Contains(stderr, "Error:") || stdout != "" {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
	if got := entryNames(t, dir); len(got) != 0 {
		t.Errorf("project directory contains %v, want nothing created", got)
	}
}

func TestInitRequiresClassifierWhenSmallCorrectionsEnabled(t *testing.T) {
	dir := newProjectDir(t)

	_, _, err := runCLI(t, "dev", initArgs("--small-corrections", "enabled")...)
	if !errors.Is(err, errMissingRequired) || !strings.Contains(err.Error(), "--small-correction-classifier") {
		t.Fatalf("error = %v, want the classifier to be required", err)
	}
	if got := entryNames(t, dir); len(got) != 0 {
		t.Errorf("project directory contains %v, want nothing created", got)
	}

	if _, _, err := runCLI(t, "dev", initArgs("--small-corrections", "enabled",
		"--small-correction-classifier", "maintainer")...); err != nil {
		t.Fatalf("with the classifier: unexpected error: %v", err)
	}
	mustLoadValid(t, dir)
}

func TestInitRejectsInvalidValuesAndWritesNothing(t *testing.T) {
	cases := map[string][]string{
		"branch name":       {"--integration-branch", "bad branch"},
		"backlog reference": {"--suggestions-backlog", "issue 24"},
		"discovery mode":    {"--discovery", "sometimes"},
		"trigger format":    {"--pr-review-trigger", "no separator"},
		"manual format":     {"--manual", "go"},
		"skill harness":     {"--skill-target", "cursor@1.0@2026-11-12"},
		"skill date":        {"--skill-target", "codex@0.137.0@yesterday"},
		"small corrections": {"--small-corrections", "sometimes"},
		"reviewer request":  {"--automated-reviewer", "codex"},
		"without issue":     {"--small-corrections-without-issue"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			dir := newProjectDir(t)
			args := initArgs()
			if extra[0] == "--discovery" {
				args = append([]string{"init"}, "--workflow-owner", "a", "--change-acceptor", "b", "--product-intent-acceptor", "c")
			}
			if _, _, err := runCLI(t, "dev", append(args, extra...)...); err == nil {
				t.Fatalf("garage %v: expected an error", extra)
			}
			if got := entryNames(t, dir); len(got) != 0 {
				t.Errorf("project directory contains %v, want nothing created", got)
			}
		})
	}
}

func TestInitPromptsForMissingRequiredValues(t *testing.T) {
	dir := t.TempDir()
	ask := &scriptedPrompter{answers: []string{
		"sometimes", // rejected, asked again
		"triggers",
		"", // at least one trigger is required
		"schema-change|the change alters a persisted schema",
		"",
		"lead maintainer:alice",
		"   ", // an empty role is rejected
		"maintainer",
		"Workflow Owner",
	}}

	result, err := initProject(dir, desiredFrom(t, "--integration-branch", "main"), ask)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != initCreated {
		t.Errorf("action = %v, want created", result.Action)
	}
	if len(ask.answers) != 0 {
		t.Errorf("unused answers %q", ask.answers)
	}
	cfg := mustLoadValid(t, dir)
	d := cfg.Project.Discovery
	if d.Mode != discoveryTriggers || len(*d.Triggers) != 1 || (*d.Triggers)[0].Name != "schema-change" {
		t.Errorf("discovery = %+v", d)
	}
	if wo := cfg.Project.Authorities.WorkflowOwner; wo.Role != "lead maintainer" || len(wo.Members) != 1 || wo.Members[0] != "alice" {
		t.Errorf("workflow owner = %+v", wo)
	}
	if *cfg.Project.IntegrationBranch != "main" {
		t.Errorf("integration branch = %q, want the flag value", *cfg.Project.IntegrationBranch)
	}
}

func TestInitPromptsOnlyForValuesFlagsDoNotSupply(t *testing.T) {
	dir := t.TempDir()
	ask := &scriptedPrompter{answers: []string{"maintainer"}}

	_, err := initProject(dir, desiredFrom(t, "--discovery", "implementer-judgment",
		"--workflow-owner", "lead maintainer", "--product-intent-acceptor", "Workflow Owner"), ask)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ask.questions) != 1 || !strings.Contains(ask.questions[0], "Change acceptor") {
		t.Errorf("questions = %q, want only the change acceptor", ask.questions)
	}
}

func TestInitPromptInputEndingWritesNothing(t *testing.T) {
	dir := t.TempDir()
	ask := &scriptedPrompter{answers: []string{"implementer-judgment"}}

	if _, err := initProject(dir, projectConfig{}, ask); err == nil {
		t.Fatal("expected an error when input ends")
	}
	if got := entryNames(t, dir); len(got) != 0 {
		t.Errorf("project directory contains %v, want nothing created", got)
	}
}

func TestLinePrompterReadsTrimmedLines(t *testing.T) {
	var out bytes.Buffer
	p := linePrompter{in: bufio.NewReader(strings.NewReader("  first  \nlast")), out: &out}

	for _, want := range []string{"first", "last"} {
		got, err := p.Ask("Q: ")
		if err != nil || got != want {
			t.Fatalf("Ask = %q, %v; want %q", got, err, want)
		}
	}
	if _, err := p.Ask("Q: "); err == nil {
		t.Error("expected an error once input has ended")
	}
	if out.String() != "Q: Q: Q: " {
		t.Errorf("prompts written = %q", out.String())
	}
}

func TestIsInteractiveRequiresTerminals(t *testing.T) {
	if isInteractive(strings.NewReader(""), &bytes.Buffer{}) {
		t.Error("buffers were treated as a terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	if isInteractive(r, w) {
		t.Error("a pipe was treated as a terminal")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer null.Close()
	if isInteractive(null, null) {
		t.Error("the null device was treated as a terminal")
	}
}

func TestInitRepeatedIsUnchanged(t *testing.T) {
	dir := newProjectDir(t)
	if _, _, err := runCLI(t, "dev", initArgs()...); err != nil {
		t.Fatalf("first init: %v", err)
	}
	first := readFile(t, configPathIn(dir))

	for _, args := range [][]string{{"init"}, initArgs()} {
		stdout, _, err := runCLI(t, "dev", args...)
		if err != nil {
			t.Fatalf("garage %v: unexpected error: %v", args, err)
		}
		if !strings.Contains(stdout, "is up to date") {
			t.Errorf("garage %v: stdout = %q, want an up-to-date report", args, stdout)
		}
		if got := readFile(t, configPathIn(dir)); got != first {
			t.Fatalf("garage %v changed the configuration:\n%s", args, got)
		}
	}
	if got := entryNames(t, filepath.Join(dir, garageDirName)); len(got) != 1 {
		t.Errorf("%s contains %v, want only %s", garageDirName, got, configFileName)
	}
}

func TestInitAddsMissingSettingsAndKeepsEverythingElse(t *testing.T) {
	dir := newProjectDir(t)
	const existing = `# Our configuration. Keep this comment.
schema_version: 2
project:
  # Chosen at kickoff.
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
	writeFile(t, configPathIn(dir), existing)

	stdout, _, err := runCLI(t, "dev", "init", "--integration-branch", "main",
		"--workflow-owner", "lead maintainer", "--skill-target", "codex@0.137.0@2026-11-12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"project.integration_branch", "tooling.skill_targets"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not report %s:\n%s", want, stdout)
		}
	}
	content := readFile(t, configPathIn(dir))
	for _, want := range []string{"# Our configuration. Keep this comment.", "# Chosen at kickoff.", "role: lead maintainer"} {
		if !strings.Contains(content, want) {
			t.Errorf("upgraded configuration lost %q:\n%s", want, content)
		}
	}
	cfg := mustLoadValid(t, dir)
	if *cfg.Project.IntegrationBranch != "main" {
		t.Errorf("integration branch = %q", *cfg.Project.IntegrationBranch)
	}
	if targets := *cfg.Tooling.SkillTargets; len(targets) != 1 || targets[0].Path != supportedSkillTargets["codex"] {
		t.Errorf("skill targets = %+v", targets)
	}
}

func TestInitRefusesToChangeAConfiguredValue(t *testing.T) {
	dir := newProjectDir(t)
	if _, _, err := runCLI(t, "dev", initArgs("--integration-branch", "main")...); err != nil {
		t.Fatalf("first init: %v", err)
	}
	before := readFile(t, configPathIn(dir))

	_, _, err := runCLI(t, "dev", "init", "--integration-branch", "develop", "--change-acceptor", "anyone")
	if err == nil {
		t.Fatal("expected an error for a conflicting value")
	}
	for _, want := range []string{"project.integration_branch", "project.authorities.change_acceptor", "was not changed", "review process"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	}
	if got := readFile(t, configPathIn(dir)); got != before {
		t.Errorf("configuration was modified:\n%s", got)
	}
}

func TestInitUpgradesSchemaVersion1(t *testing.T) {
	dir := newProjectDir(t)
	const v1 = "# Builder Garage project configuration.\n" +
		"# Created by \"garage init\". schema_version identifies the layout of this file.\n" +
		"schema_version: 1\n"
	writeFile(t, configPathIn(dir), v1)

	stdout, _, err := runCLI(t, "dev", initArgs()...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "from schema version 1 to 2") {
		t.Errorf("stdout = %q, want the migration reported", stdout)
	}
	content := readFile(t, configPathIn(dir))
	if !strings.HasPrefix(content, "# Builder Garage project configuration.\n") {
		t.Errorf("the existing header comment was not kept:\n%s", content)
	}
	if cfg := mustLoadValid(t, dir); cfg.SchemaVersion != 2 {
		t.Errorf("schema version = %d, want 2", cfg.SchemaVersion)
	}
}

func TestInitDoesNotMigrateVersion1WithoutRequiredValues(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, configPathIn(dir), "schema_version: 1\n")

	if _, _, err := runCLI(t, "dev", "init"); !errors.Is(err, errMissingRequired) {
		t.Fatalf("error = %v, want errMissingRequired", err)
	}
	if got := readFile(t, configPathIn(dir)); got != "schema_version: 1\n" {
		t.Errorf("configuration was modified: %q", got)
	}
}

func TestInitStopsOnInvalidOrAmbiguousConfiguration(t *testing.T) {
	cases := map[string]string{
		"invalid YAML":         "schema_version: [1\n",
		"empty file":           "",
		"unknown key":          "schema_version: 2\ncustom: keep-me\n",
		"unsupported version":  "schema_version: 3\n",
		"version 1 with extra": "schema_version: 1\nproject:\n  integration_branch: main\n",
		"invalid value":        "schema_version: 2\nproject:\n  integration_branch: \"bad branch\"\n",
		"not a mapping":        "- schema_version\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := newProjectDir(t)
			writeFile(t, configPathIn(dir), content)

			_, _, err := runCLI(t, "dev", initArgs()...)
			if err == nil {
				t.Fatal("expected init to stop")
			}
			if !strings.Contains(err.Error(), "was not changed") {
				t.Errorf("error = %q, want it to say the file was not changed", err)
			}
			if got := readFile(t, configPathIn(dir)); got != content {
				t.Errorf("configuration was modified: %q", got)
			}
		})
	}
}

func TestInitHasNoForceFlag(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, configPathIn(dir), "schema_version: 1\n")

	_, _, err := runCLI(t, "dev", append(initArgs(), "--force")...)
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --force") {
		t.Fatalf("error = %v, want --force to be an unknown flag", err)
	}
	if got := readFile(t, configPathIn(dir)); got != "schema_version: 1\n" {
		t.Errorf("configuration was modified: %q", got)
	}
}

func TestInitInExistingProjectLeavesOtherFilesUntouched(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, filepath.Join(dir, "README.md"), "# existing project\n")
	writeFile(t, filepath.Join(dir, "src", "main.txt"), "source\n")

	if _, _, err := runCLI(t, "dev", initArgs()...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "README.md")); got != "# existing project\n" {
		t.Errorf("README.md was modified: %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "src", "main.txt")); got != "source\n" {
		t.Errorf("src/main.txt was modified: %q", got)
	}
	if got := entryNames(t, dir); len(got) != 3 {
		t.Errorf("project directory contains %v, want .garage, README.md, and src", got)
	}
}

func TestInitUpgradeKeepsOtherFilesInGarageDirectory(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, configPathIn(dir), "schema_version: 1\n")
	writeFile(t, filepath.Join(dir, garageDirName, "notes.txt"), "unrelated\n")

	if _, _, err := runCLI(t, "dev", initArgs()...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, garageDirName, "notes.txt")); got != "unrelated\n" {
		t.Errorf("unrelated file in %s was modified: %q", garageDirName, got)
	}
	if got := entryNames(t, filepath.Join(dir, garageDirName)); len(got) != 2 {
		t.Errorf("%s contains %v, want config.yaml and notes.txt only", garageDirName, got)
	}
}

func TestInitRejectsSymlinkedGarageDirectory(t *testing.T) {
	projectDir := t.TempDir()
	externalDir := t.TempDir()
	externalConfig := filepath.Join(externalDir, configFileName)
	const original = "schema_version: 1\n"
	writeFile(t, externalConfig, original)
	symlinkOrSkip(t, externalDir, filepath.Join(projectDir, garageDirName))

	if _, err := initProject(projectDir, desiredFrom(t, requiredFlags...), nil); err == nil {
		t.Fatal("expected a symlinked .garage directory to be rejected")
	} else if !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("error = %q, want it to identify the symbolic link", err)
	}
	if got := readFile(t, externalConfig); got != original {
		t.Errorf("external configuration was modified: %q", got)
	}
	if got := entryNames(t, externalDir); len(got) != 1 || got[0] != configFileName {
		t.Errorf("external directory contains %v, want only %s", got, configFileName)
	}
}

func TestInitRejectsSymlinkedConfiguration(t *testing.T) {
	projectDir := t.TempDir()
	externalDir := t.TempDir()
	externalConfig := filepath.Join(externalDir, "external.yaml")
	const original = "schema_version: 1\n"
	writeFile(t, externalConfig, original)
	if err := os.Mkdir(filepath.Join(projectDir, garageDirName), 0o755); err != nil {
		t.Fatalf("create %s: %v", garageDirName, err)
	}
	symlinkOrSkip(t, externalConfig, configPathIn(projectDir))

	if _, err := initProject(projectDir, desiredFrom(t, requiredFlags...), nil); err == nil {
		t.Fatal("expected a symlinked configuration to be rejected")
	} else if !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("error = %q, want it to identify the symbolic link", err)
	}
	if got := readFile(t, externalConfig); got != original {
		t.Errorf("external configuration was modified: %q", got)
	}
	info, err := os.Lstat(configPathIn(projectDir))
	if err != nil {
		t.Fatalf("lstat configuration symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("configuration symlink was replaced")
	}
}

func TestInitUpgradeReplacesHardLinkWithoutModifyingPeer(t *testing.T) {
	projectDir := t.TempDir()
	externalConfig := filepath.Join(projectDir, "shared-config.yaml")
	const original = "schema_version: 1\n"
	writeFile(t, externalConfig, original)
	if err := os.Mkdir(filepath.Join(projectDir, garageDirName), 0o755); err != nil {
		t.Fatalf("create %s: %v", garageDirName, err)
	}
	if err := os.Link(externalConfig, configPathIn(projectDir)); err != nil {
		t.Skipf("hard links are not available: %v", err)
	}

	result, err := initProject(projectDir, desiredFrom(t, requiredFlags...), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != initUpgraded {
		t.Errorf("action = %v, want upgraded", result.Action)
	}
	if got := readFile(t, externalConfig); got != original {
		t.Errorf("hard-linked peer was modified: %q", got)
	}
	mustLoadValid(t, projectDir)
	externalInfo, err := os.Stat(externalConfig)
	if err != nil {
		t.Fatalf("stat hard-linked peer: %v", err)
	}
	configInfo, err := os.Stat(configPathIn(projectDir))
	if err != nil {
		t.Fatalf("stat replacement configuration: %v", err)
	}
	if os.SameFile(externalInfo, configInfo) {
		t.Error("replacement configuration still shares the original inode")
	}
}

func TestReplacementPreparationFailurePreservesOriginal(t *testing.T) {
	projectDir := t.TempDir()
	configPath := configPathIn(projectDir)
	const original = "original: keep\n"
	writeFile(t, configPath, original)
	garageDir := filepath.Dir(configPath)
	garageInfo, err := os.Lstat(garageDir)
	if err != nil {
		t.Fatalf("lstat %s: %v", garageDirName, err)
	}
	injectedErr := errors.New("injected temporary write failure")
	prepare := func(dir string, content []byte) (string, error) {
		return prepareTemporaryConfigUsing(
			dir,
			content,
			func(*os.File, []byte) error { return injectedErr },
		)
	}

	err = replaceConfiguration(
		configPath,
		[]byte("replacement\n"),
		garageInfo,
		prepare,
		commitConfigReplacement,
		nil,
	)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("error = %v, want it to wrap the injected write failure", err)
	}
	if got := readFile(t, configPath); got != original {
		t.Errorf("original configuration was modified: %q", got)
	}
	if got := entryNames(t, garageDir); len(got) != 1 || got[0] != configFileName {
		t.Errorf("%s contains %v after failed preparation, want only %s",
			garageDirName, got, configFileName)
	}
}

func TestReplacementCommitFailurePreservesOriginal(t *testing.T) {
	projectDir := t.TempDir()
	configPath := configPathIn(projectDir)
	const original = "original: keep\n"
	writeFile(t, configPath, original)
	garageDir := filepath.Dir(configPath)
	garageInfo, err := os.Lstat(garageDir)
	if err != nil {
		t.Fatalf("lstat %s: %v", garageDirName, err)
	}
	injectedErr := errors.New("injected replacement failure")

	err = replaceConfiguration(
		configPath,
		[]byte("replacement\n"),
		garageInfo,
		prepareTemporaryConfig,
		func(string, string) error { return injectedErr },
		nil,
	)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("error = %v, want it to wrap the injected replacement failure", err)
	}
	if got := readFile(t, configPath); got != original {
		t.Errorf("original configuration was modified: %q", got)
	}
	if got := entryNames(t, garageDir); len(got) != 1 || got[0] != configFileName {
		t.Errorf("%s contains %v after failed replacement, want only %s",
			garageDirName, got, configFileName)
	}
}

// A regular file named .garage makes directory creation fail on every
// platform, without depending on permission bits or the current user.
func TestInitFailsWhenGarageDirCannotBeCreated(t *testing.T) {
	dir := newProjectDir(t)
	blocker := filepath.Join(dir, garageDirName)
	writeFile(t, blocker, "not a directory\n")

	stdout, stderr, err := runCLI(t, "dev", initArgs()...)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), garageDirName) {
		t.Errorf("error = %q, want it to name %s", err, garageDirName)
	}
	if !strings.Contains(stderr, "Error:") || stdout != "" {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
	if got := readFile(t, blocker); got != "not a directory\n" {
		t.Errorf("the existing %s file was modified: %q", garageDirName, got)
	}
}

// A directory at the configuration path cannot be written as a file on every
// platform.
func TestInitFailsWhenConfigurationPathIsADirectory(t *testing.T) {
	dir := newProjectDir(t)
	if err := os.MkdirAll(configPathIn(dir), 0o755); err != nil {
		t.Fatalf("create directory at the configuration path: %v", err)
	}

	stdout, _, err := runCLI(t, "dev", initArgs()...)
	if err == nil {
		t.Fatal("expected an error")
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want no completion message on failure", stdout)
	}
	info, statErr := os.Stat(configPathIn(dir))
	if statErr != nil || !info.IsDir() {
		t.Error("the directory at the configuration path was replaced")
	}
}

func TestInitRejectsArguments(t *testing.T) {
	dir := newProjectDir(t)

	if _, _, err := runCLI(t, "dev", append(initArgs(), "extra")...); err == nil {
		t.Fatal("expected an error for an unexpected argument")
	}
	if got := entryNames(t, dir); len(got) != 0 {
		t.Errorf("project directory contains %v, want nothing created", got)
	}
}

func TestInitProjectUsesTheGivenDirectory(t *testing.T) {
	dir := t.TempDir()

	result, err := initProject(dir, desiredFrom(t, requiredFlags...), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != initCreated {
		t.Errorf("action = %v, want created", result.Action)
	}
	mustLoadValid(t, dir)
}

// builderGarageFlags are the flags that generate Builder Garage's own
// committed configuration.
var builderGarageFlags = []string{
	"--integration-branch", "main",
	"--automated-reviewer", "codex",
	"--automated-reviewer-request", "optional input where available; the chatgpt-codex-connector GitHub integration reviews a pull request when it is opened",
	"--small-corrections", "enabled",
	"--small-corrections-without-issue",
	"--small-correction-classifier", "maintainer",
	"--discovery", "implementer-judgment",
	"--suggestions-backlog", "#24",
	"--workflow-owner", "lead maintainer",
	"--change-acceptor", "maintainer",
	"--product-intent-acceptor", "Workflow Owner",
	"--manual", "go@0.1",
	"--skill-target", "claude-code@2.1.277@2026-11-12",
	"--skill-target", "codex@0.137.0@2026-11-12",
	"--skill-target", "copilot-cli@1.0.93-1@2026-11-12",
}

// withLF normalizes line endings. A Windows checkout may convert the
// repository's files to CRLF; line endings are a checkout artifact, not
// content, so comparisons with repository files ignore them.
func withLF(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// documentedInitFlags returns the flags of the garage init command recorded
// in docs/skill-adoption.md, with shell quoting removed.
func documentedInitFlags(t *testing.T) []string {
	t.Helper()
	doc := withLF(readFile(t, filepath.Join("..", "..", "docs", "skill-adoption.md")))
	start := strings.Index(doc, "```sh\ngarage init \\\n")
	if start < 0 {
		t.Fatal("docs/skill-adoption.md has no garage init command block")
	}
	block := doc[start+len("```sh\n"):]
	block = block[:strings.Index(block, "```")]
	block = strings.ReplaceAll(block, "\\\n", " ")

	var words []string
	var word strings.Builder
	inWord, quoted := false, false
	for _, r := range block {
		switch {
		case r == '"':
			quoted, inWord = !quoted, true
		case (r == ' ' || r == '\n') && !quoted:
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if quoted {
		t.Fatal("unbalanced quotes in the documented garage init command")
	}
	if inWord {
		words = append(words, word.String())
	}
	return words[2:] // drop "garage init"
}

// Builder Garage's committed configuration must be exactly what the
// documented noninteractive command generates, so it is never hand-authored
// and the documentation never drifts from it.
func TestCommittedConfigurationIsGeneratedByTheDocumentedCommand(t *testing.T) {
	documented := documentedInitFlags(t)
	if strings.Join(documented, "\x00") != strings.Join(builderGarageFlags, "\x00") {
		t.Fatalf("documented flags differ from builderGarageFlags:\n%q\n%q", documented, builderGarageFlags)
	}
	committed := withLF(readFile(t, filepath.Join("..", "..", garageDirName, configFileName)))

	dir := t.TempDir()
	if _, err := initProject(dir, desiredFrom(t, documented...), nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if generated := readFile(t, configPathIn(dir)); generated != committed {
		t.Errorf("the committed .garage/config.yaml is not what the documented command generates:\n--- generated\n%s\n--- committed\n%s",
			generated, committed)
	}
	if _, err := loadValidConfig(filepath.Join("..", "..")); err != nil {
		t.Errorf("the committed configuration does not validate: %v", err)
	}
}

func TestBuilderGarageFlagsProduceAValidConfiguration(t *testing.T) {
	dir := newProjectDir(t)
	if _, _, err := runCLI(t, "dev", append([]string{"init"}, builderGarageFlags...)...); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := mustLoadValid(t, dir)
	if !*cfg.Project.SmallCorrections.Enabled || cfg.Project.Authorities.SmallCorrectionClassifier.Role != "maintainer" {
		t.Errorf("small corrections = %+v", cfg.Project.SmallCorrections)
	}
	if len(*cfg.Tooling.SkillTargets) != 3 {
		t.Errorf("skill targets = %+v", *cfg.Tooling.SkillTargets)
	}
}

// concurrentConfig is a valid configuration another writer saves while init
// is running.
const concurrentConfig = `schema_version: 2
project:
  integration_branch: release
  discovery:
    mode: implementer-judgment
  authorities:
    workflow_owner:
      role: alice-team
    change_acceptor:
      role: bob-team
    product_intent_acceptor:
      role: carol-team
`

func TestInitUpgradeStopsWhenConfigurationChangesDuringPrompting(t *testing.T) {
	writeInPlace := func(t *testing.T, path string) {
		if err := os.WriteFile(path, []byte(concurrentConfig), 0o644); err != nil {
			t.Fatalf("concurrent write: %v", err)
		}
	}
	replaceByRename := func(t *testing.T, path string) {
		next := path + ".editor-save"
		if err := os.WriteFile(next, []byte(concurrentConfig), 0o644); err != nil {
			t.Fatalf("concurrent write: %v", err)
		}
		if err := os.Rename(next, path); err != nil {
			t.Fatalf("concurrent rename: %v", err)
		}
	}
	for name, change := range map[string]func(*testing.T, string){
		"written in place":   writeInPlace,
		"replaced by rename": replaceByRename,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := configPathIn(dir)
			writeFile(t, path, "schema_version: 1\n")
			ask := &scriptedPrompter{
				answers: []string{"implementer-judgment", "lead", "maintainer", "owner"},
				onAsk: func(n int) {
					if n == 2 {
						change(t, path)
					}
				},
			}

			_, err := initProject(dir, projectConfig{}, ask)
			if !errors.Is(err, errConfigChanged) {
				t.Fatalf("error = %v, want errConfigChanged", err)
			}
			for _, want := range []string{"was not changed by init", "garage init"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error does not contain %q: %v", want, err)
				}
			}
			if got := readFile(t, path); got != concurrentConfig {
				t.Errorf("the concurrent change was overwritten:\n%s", got)
			}
			if got := entryNames(t, filepath.Join(dir, garageDirName)); len(got) != 1 || got[0] != configFileName {
				t.Errorf("%s contains %v, want only %s", garageDirName, got, configFileName)
			}
		})
	}
}

func TestInitUpgradeStillWritesWhenNothingChangedDuringPrompting(t *testing.T) {
	dir := t.TempDir()
	path := configPathIn(dir)
	writeFile(t, path, "schema_version: 1\n")
	ask := &scriptedPrompter{
		answers: []string{"implementer-judgment", "lead", "maintainer", "owner"},
		// Rewriting identical bytes in place is not a change.
		onAsk: func(n int) {
			if n == 2 {
				if err := os.WriteFile(path, []byte("schema_version: 1\n"), 0o644); err != nil {
					t.Fatalf("rewrite: %v", err)
				}
			}
		},
	}

	result, err := initProject(dir, projectConfig{}, ask)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != initUpgraded {
		t.Errorf("action = %v, want upgraded", result.Action)
	}
	mustLoadValid(t, dir)
}

func TestConfigSnapshotDetectsChanges(t *testing.T) {
	newSnapshot := func(t *testing.T, path string) configSnapshot {
		t.Helper()
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("lstat: %v", err)
		}
		return configSnapshot{path: path, info: info, data: []byte(readFile(t, path))}
	}
	cases := map[string]func(t *testing.T, path string){
		"content changed": func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("schema_version: 2\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"file replaced": func(t *testing.T, path string) {
			writeFile(t, path+".new", "schema_version: 1\n")
			if err := os.Rename(path+".new", path); err != nil {
				t.Fatal(err)
			}
		},
		"file removed": func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			path := configPathIn(t.TempDir())
			writeFile(t, path, "schema_version: 1\n")
			snapshot := newSnapshot(t, path)
			change(t, path)
			if err := snapshot.verifyUnchanged(); !errors.Is(err, errConfigChanged) {
				t.Errorf("verifyUnchanged = %v, want errConfigChanged", err)
			}
		})
	}
	t.Run("unchanged", func(t *testing.T) {
		path := configPathIn(t.TempDir())
		writeFile(t, path, "schema_version: 1\n")
		if err := newSnapshot(t, path).verifyUnchanged(); err != nil {
			t.Errorf("verifyUnchanged = %v, want nil", err)
		}
	})
}

func TestReplacementStopsWhenPreconditionFails(t *testing.T) {
	projectDir := t.TempDir()
	configPath := configPathIn(projectDir)
	const original = "original: keep\n"
	writeFile(t, configPath, original)
	garageDir := filepath.Dir(configPath)
	garageInfo, err := os.Lstat(garageDir)
	if err != nil {
		t.Fatalf("lstat %s: %v", garageDirName, err)
	}
	committed := false

	err = replaceConfiguration(
		configPath,
		[]byte("replacement\n"),
		garageInfo,
		prepareTemporaryConfig,
		func(string, string) error { committed = true; return nil },
		func() error { return errConfigChanged },
	)
	if !errors.Is(err, errConfigChanged) {
		t.Fatalf("error = %v, want errConfigChanged", err)
	}
	if committed {
		t.Error("the replacement was committed despite the failed precondition")
	}
	if got := readFile(t, configPath); got != original {
		t.Errorf("original configuration was modified: %q", got)
	}
	if got := entryNames(t, garageDir); len(got) != 1 || got[0] != configFileName {
		t.Errorf("%s contains %v, want only %s", garageDirName, got, configFileName)
	}
}
