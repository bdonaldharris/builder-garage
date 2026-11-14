package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// The project configuration schema. Version 1 recorded only the schema
// version. Version 2 adds the "project" section, which holds a project's
// choices at the extension points declared in docs/extension-points.md, and
// the "tooling" section, documented in docs/tooling-configuration.md.

// projectConfig is the content of .garage/config.yaml. Optional settings are
// pointers so that an omitted setting, to which the canonical default applies,
// is distinguishable from one the project set explicitly.
type projectConfig struct {
	SchemaVersion int             `yaml:"schema_version"`
	Project       *projectChoices `yaml:"project,omitempty"`
	Tooling       *toolingConfig  `yaml:"tooling,omitempty"`
}

// projectChoices holds the extension points E1 to E10.
type projectChoices struct {
	IntegrationBranch    *string            `yaml:"integration_branch,omitempty"`     // E1
	PRReviewTriggers     *[]namedCondition  `yaml:"pr_review_triggers,omitempty"`     // E2
	AutomatedReviewer    *automatedReviewer `yaml:"automated_reviewer,omitempty"`     // E3
	SmallCorrections     *smallCorrections  `yaml:"small_corrections,omitempty"`      // E4
	Discovery            *discoveryChoice   `yaml:"discovery,omitempty"`              // E5
	ReleaseConditions    *[]releaseCond     `yaml:"release_conditions,omitempty"`     // E6
	SuggestionsBacklog   *string            `yaml:"suggestions_backlog,omitempty"`    // E7
	Authorities          *authorities       `yaml:"authorities,omitempty"`            // E8
	UserFacingInspection *inspectionChoice  `yaml:"user_facing_inspection,omitempty"` // E9
	Guidance             *guidanceChoice    `yaml:"guidance,omitempty"`               // E10
}

// namedCondition is a named trigger with a description of when it applies.
type namedCondition struct {
	Name        string `yaml:"name"`
	AppliesWhen string `yaml:"applies_when"`
}

type automatedReviewer struct {
	Name    string `yaml:"name"`
	Request string `yaml:"request"`
}

type smallCorrections struct {
	Enabled      *bool `yaml:"enabled"`
	WithoutIssue *bool `yaml:"without_issue,omitempty"`
}

type discoveryChoice struct {
	Mode     string            `yaml:"mode"`
	Triggers *[]namedCondition `yaml:"triggers,omitempty"`
}

type releaseCond struct {
	Name             string `yaml:"name"`
	AppliesWhen      string `yaml:"applies_when"`
	RequiredEvidence string `yaml:"required_evidence"`
}

// authorities holds the human responsibilities of E8. Garage checks only
// that each required responsibility is declared and well formed. Whether the
// declared authority is human is a governance responsibility; Garage does not
// certify it.
type authorities struct {
	WorkflowOwner             *authority `yaml:"workflow_owner,omitempty"`
	ChangeAcceptor            *authority `yaml:"change_acceptor,omitempty"`
	ProductIntentAcceptor     *authority `yaml:"product_intent_acceptor,omitempty"`
	SmallCorrectionClassifier *authority `yaml:"small_correction_classifier,omitempty"`
}

type authority struct {
	Role    string   `yaml:"role"`
	Members []string `yaml:"members,omitempty"`
}

type inspectionChoice struct {
	AdditionalConditions []string `yaml:"additional_conditions,omitempty"`
}

type guidanceChoice struct {
	Manuals []manualChoice `yaml:"manuals,omitempty"`
	// Conditional is retired (#59): conditional stack or provider guidance
	// is not configurable. It is decoded only so that validateConfig can
	// refuse a value with a clear correction.
	Conditional []string `yaml:"conditional,omitempty"`
}

type manualChoice struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// toolingConfig holds Garage tooling settings. Tooling settings are outside
// the responsibility extension-point registry and grant no workflow authority.
type toolingConfig struct {
	SkillTargets *[]skillTarget `yaml:"skill_targets,omitempty"`
}

type skillTarget struct {
	Harness          string        `yaml:"harness"`
	Scope            string        `yaml:"scope"`
	Path             skillPathRule `yaml:"path"`
	ConfirmedVersion string        `yaml:"confirmed_version"`
	ConfirmedOn      string        `yaml:"confirmed_on"`
}

// skillPathRule resolves a target directory: the override, when its
// environment variable is set, before the default.
type skillPathRule struct {
	Override string `yaml:"override"`
	Default  string `yaml:"default"`
}

const (
	discoveryJudgment = "implementer-judgment"
	discoveryTriggers = "triggers"
	backlogNone       = "none"
	scopeUser         = "user"

	defaultIntegrationBranch = "staging"
	defaultSuggestionsIssue  = "Application Improvement Suggestions"
)

var discoveryModes = []string{discoveryJudgment, discoveryTriggers}

// supportedSkillTargets are the harness targets Garage may manage, with the
// path rule confirmed for each (#37 decision). Only user scope is supported.
var supportedSkillTargets = map[string]skillPathRule{
	"claude-code": {Override: "$CLAUDE_CONFIG_DIR/skills", Default: "~/.claude/skills"},
	"codex":       {Override: "$CODEX_HOME/skills", Default: "~/.codex/skills"},
	"copilot-cli": {Override: "$COPILOT_HOME/skills", Default: "~/.copilot/skills"},
}

var supportedHarnesses = []string{"claude-code", "codex", "copilot-cli"}

// confirmedSkillTargets are the harness versions whose skill paths the #37
// load test confirmed, and the date it did. Guided setup declares a chosen
// target with these values, so an adopter never types a version.
var confirmedSkillTargets = map[string]struct{ Version, On string }{
	"claude-code": {"2.1.277", "2026-11-12"},
	"codex":       {"0.137.0", "2026-11-12"},
	"copilot-cli": {"1.0.93-1", "2026-11-12"},
}

// errNoConfig reports that the project has no configuration file.
var errNoConfig = errors.New("no configuration")

// configProblems is the complete list of reasons a configuration is invalid.
type configProblems []string

func (p configProblems) Error() string {
	return "invalid configuration:\n  - " + strings.Join(p, "\n  - ")
}

// readConfigFile reads the configuration file without following a symbolic
// link and without opening anything other than a regular file.
func readConfigFile(configPath string) ([]byte, error) {
	info, err := os.Lstat(configPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w at %s: run \"garage init\" to create one", errNoConfig, configPath)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot inspect %s: %w", configPath, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to read %s: it is a symbolic link", configPath)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing to read %s: it is not a regular file (%s)", configPath, info.Mode().Type())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", configPath, err)
	}
	return data, nil
}

// decodeConfig decodes one YAML document strictly: unknown keys, wrong types,
// and additional documents are errors. It does not validate values.
func decodeConfig(data []byte) (projectConfig, error) {
	var cfg projectConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return cfg, errors.New("the file is empty")
		}
		return cfg, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return cfg, errors.New("the file must contain exactly one YAML document")
	}
	return cfg, nil
}

// validateConfig checks every declared, mechanically checkable value and
// returns all problems found, or nil.
func validateConfig(cfg projectConfig) configProblems {
	var p configProblems
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }

	switch cfg.SchemaVersion {
	case configSchemaVersion:
	case 1:
		add("schema_version is 1, which records no project choices; run \"garage init\" to upgrade it to %d", configSchemaVersion)
		return p
	default:
		add("schema_version must be %d, not %d", configSchemaVersion, cfg.SchemaVersion)
		return p
	}

	pc := cfg.Project
	if pc == nil {
		pc = &projectChoices{}
	}

	if pc.IntegrationBranch != nil {
		if err := checkBranchName(*pc.IntegrationBranch); err != nil {
			add("project.integration_branch (E1): %v", err)
		}
	}
	if pc.PRReviewTriggers != nil {
		checkNamedConditions(&p, "project.pr_review_triggers (E2)", *pc.PRReviewTriggers)
	}
	if r := pc.AutomatedReviewer; r != nil {
		checkReviewerName(&p, r.Name)
		checkText(&p, "project.automated_reviewer.request (E3)", r.Request)
	}

	smallEnabled := false
	if sc := pc.SmallCorrections; sc != nil {
		if sc.Enabled == nil {
			add("project.small_corrections.enabled (E4) is required when small_corrections is present")
		} else {
			smallEnabled = *sc.Enabled
		}
		if sc.WithoutIssue != nil && !smallEnabled {
			add("project.small_corrections.without_issue (E4) applies only when small corrections are enabled")
		}
	}

	if d := pc.Discovery; d == nil {
		add("project.discovery (E5) is required: choose %s", strings.Join(discoveryModes, " or "))
	} else {
		switch d.Mode {
		case discoveryJudgment:
			if d.Triggers != nil && len(*d.Triggers) > 0 {
				add("project.discovery.triggers (E5) apply only when mode is %q", discoveryTriggers)
			}
		case discoveryTriggers:
			if d.Triggers == nil || len(*d.Triggers) == 0 {
				add("project.discovery.triggers (E5) must list at least one trigger when mode is %q", discoveryTriggers)
			} else {
				checkNamedConditions(&p, "project.discovery.triggers (E5)", *d.Triggers)
			}
		default:
			add("project.discovery.mode (E5) must be %s, not %q", strings.Join(discoveryModes, " or "), d.Mode)
		}
	}

	if pc.ReleaseConditions != nil {
		seen := map[string]bool{}
		for i, c := range *pc.ReleaseConditions {
			field := fmt.Sprintf("project.release_conditions[%d] (E6)", i)
			checkLine(&p, field+".name", c.Name)
			checkText(&p, field+".applies_when", c.AppliesWhen)
			checkText(&p, field+".required_evidence", c.RequiredEvidence)
			if c.Name != "" && seen[c.Name] {
				add("%s: duplicate name %q", field, c.Name)
			}
			seen[c.Name] = true
		}
	}

	if b := pc.SuggestionsBacklog; b != nil && *b != backlogNone && !issueRefPattern.MatchString(*b) {
		add("project.suggestions_backlog (E7) must be an issue reference such as \"#24\", or %q, not %q", backlogNone, *b)
	}

	a := pc.Authorities
	if a == nil {
		a = &authorities{}
	}
	required := []struct {
		field string
		value *authority
	}{
		{"workflow_owner", a.WorkflowOwner},
		{"change_acceptor", a.ChangeAcceptor},
		{"product_intent_acceptor", a.ProductIntentAcceptor},
	}
	for _, r := range required {
		if r.value == nil {
			add("project.authorities.%s (E8) is required", r.field)
		} else {
			checkAuthority(&p, "project.authorities."+r.field+" (E8)", *r.value)
		}
	}
	switch {
	case smallEnabled && a.SmallCorrectionClassifier == nil:
		add("project.authorities.small_correction_classifier (E8) is required when small corrections are enabled")
	case !smallEnabled && a.SmallCorrectionClassifier != nil:
		add("project.authorities.small_correction_classifier (E8) applies only when small corrections are enabled")
	case a.SmallCorrectionClassifier != nil:
		checkAuthority(&p, "project.authorities.small_correction_classifier (E8)", *a.SmallCorrectionClassifier)
	}

	if ins := pc.UserFacingInspection; ins != nil {
		checkUniqueTexts(&p, "project.user_facing_inspection.additional_conditions (E9)", ins.AdditionalConditions)
	}

	if g := pc.Guidance; g != nil {
		seen := map[string]bool{}
		// A configured manual must be one this garage carries (#60 decision
		// 4). A version other than the one it delivers is still valid; it
		// is reported, not refused.
		catalog, catalogErr := embeddedManualCatalog()
		if catalogErr != nil && len(g.Manuals) > 0 {
			add("project.guidance.manuals (E10): cannot read the manuals this garage carries: %v", catalogErr)
		}
		for i, m := range g.Manuals {
			field := fmt.Sprintf("project.guidance.manuals[%d] (E10)", i)
			checkLine(&p, field+".name", m.Name)
			checkToken(&p, field+".version", m.Version)
			if m.Name != "" && seen[m.Name] {
				add("%s: manual %q is listed more than once", field, m.Name)
			}
			seen[m.Name] = true
			if catalogErr == nil && m.Name != "" && findManual(catalog, m.Name) == nil {
				add("%s.name: %q is not a manual this garage carries; available: %s",
					field, m.Name, namesOrDefault(manualNames(catalog), "none"))
			}
		}
		if len(g.Conditional) > 0 {
			add("project.guidance.conditional is not configurable: conditional stack or provider guidance in the canonical skills applies whenever its own stated condition holds, and no project selects or disables it; remove project.guidance.conditional")
		}
	}

	if t := cfg.Tooling; t != nil && t.SkillTargets != nil {
		seen := map[string]bool{}
		for i, target := range *t.SkillTargets {
			field := fmt.Sprintf("tooling.skill_targets[%d]", i)
			rule, known := supportedSkillTargets[target.Harness]
			if !known {
				add("%s.harness must be one of %s, not %q", field, strings.Join(supportedHarnesses, ", "), target.Harness)
			} else {
				if seen[target.Harness] {
					add("%s: harness %q is listed more than once", field, target.Harness)
				}
				seen[target.Harness] = true
				if target.Path != rule {
					add("%s.path must be the confirmed rule for %s: override %q, default %q",
						field, target.Harness, rule.Override, rule.Default)
				}
			}
			if target.Scope != scopeUser {
				add("%s.scope must be %q, not %q", field, scopeUser, target.Scope)
			}
			checkToken(&p, field+".confirmed_version", target.ConfirmedVersion)
			if _, err := time.Parse(time.DateOnly, target.ConfirmedOn); err != nil {
				add("%s.confirmed_on must be a date written YYYY-MM-DD, not %q", field, target.ConfirmedOn)
			}
		}
	}

	if len(p) == 0 {
		return nil
	}
	return p
}

var issueRefPattern = regexp.MustCompile(`^#[1-9][0-9]*$`)

func checkLine(p *configProblems, field, value string) {
	if strings.TrimSpace(value) == "" {
		*p = append(*p, field+" must not be empty")
	} else if strings.ContainsAny(value, "\r\n") {
		*p = append(*p, field+" must be a single line")
	}
}

// checkReviewerName validates the automated reviewer's name: one line, and a
// real name rather than a placeholder. Guided setup already refuses placeholder
// answers, through the same checkNotPlaceholder rule.
func checkReviewerName(p *configProblems, value string) {
	const field = "project.automated_reviewer.name (E3)"
	checkLine(p, field, value)
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
		return
	}
	if err := checkNotPlaceholder(value); err != nil {
		*p = append(*p, fmt.Sprintf("%s: %v; to have no automated reviewer, omit the setting or run \"garage config unset automated-reviewer\"", field, err))
	}
}

func checkText(p *configProblems, field, value string) {
	if strings.TrimSpace(value) == "" {
		*p = append(*p, field+" must not be empty")
	}
}

func checkToken(p *configProblems, field, value string) {
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r <= ' ' || r == 0x7f }) >= 0 {
		*p = append(*p, fmt.Sprintf("%s must be a single word without spaces, not %q", field, value))
	}
}

func checkNamedConditions(p *configProblems, field string, list []namedCondition) {
	seen := map[string]bool{}
	for i, c := range list {
		item := fmt.Sprintf("%s[%d]", field, i)
		checkLine(p, item+".name", c.Name)
		checkText(p, item+".applies_when", c.AppliesWhen)
		if c.Name != "" && seen[c.Name] {
			*p = append(*p, fmt.Sprintf("%s: duplicate name %q", item, c.Name))
		}
		seen[c.Name] = true
	}
}

func checkAuthority(p *configProblems, field string, a authority) {
	checkLine(p, field+".role", a.Role)
	checkUniqueLines(p, field+".members", a.Members)
}

func checkUniqueLines(p *configProblems, field string, values []string) {
	seen := map[string]bool{}
	for i, v := range values {
		item := field + "[" + strconv.Itoa(i) + "]"
		checkLine(p, item, v)
		if v != "" && seen[v] {
			*p = append(*p, fmt.Sprintf("%s: %q is listed more than once", item, v))
		}
		seen[v] = true
	}
}

func checkUniqueTexts(p *configProblems, field string, values []string) {
	seen := map[string]bool{}
	for i, v := range values {
		item := field + "[" + strconv.Itoa(i) + "]"
		checkText(p, item, v)
		if v != "" && seen[v] {
			*p = append(*p, fmt.Sprintf("%s: %q is listed more than once", item, v))
		}
		seen[v] = true
	}
}

// checkBranchName applies the subset of git's branch-name rules that can be
// checked without running git.
func checkBranchName(name string) error {
	switch {
	case name == "":
		return errors.New("must not be empty")
	case name == "@", strings.HasPrefix(name, "-"), strings.HasPrefix(name, "/"),
		strings.HasSuffix(name, "/"), strings.HasSuffix(name, "."), strings.HasSuffix(name, ".lock"),
		strings.Contains(name, ".."), strings.Contains(name, "//"), strings.Contains(name, "@{"),
		strings.Contains(name, "/."), strings.HasPrefix(name, "."):
		return fmt.Errorf("%q is not a valid branch name", name)
	}
	if strings.IndexFunc(name, func(r rune) bool {
		return r <= ' ' || r == 0x7f || strings.ContainsRune(`~^:?*[\`, r)
	}) >= 0 {
		return fmt.Errorf("%q is not a valid branch name", name)
	}
	return nil
}
