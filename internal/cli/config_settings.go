package cli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// The setting registry. It is the one definition of every configurable
// setting: "garage init" merges and adds settings by it, guided setup and
// upgrade summaries take their plain-language labels from it, and the
// "garage config" commands read and change settings through it.

// configSetting is one independently configurable setting: an extension
// point, one human responsibility, or a tooling setting. A setting is the
// unit init adds, the unit it refuses to change, and the unit whose node the
// configuration commands add, replace, or remove.
type configSetting struct {
	path []string
	// label is the plain-language name guided setup shows.
	label string
	get   func(*projectConfig) any // nil when the setting is absent
	set   func(dst, src *projectConfig)
	// options are the command-line settings this setting holds, named and
	// formatted as init's flags are. The first is the setting's own name.
	options []configOption
}

// optionKind is how a command-line setting is changed.
type optionKind int

const (
	// optionValue holds one value, changed with set and unset.
	optionValue optionKind = iota
	// optionList holds items, changed with add, remove, and unset.
	optionList
	// optionRetired is no longer configurable; it may only be unset.
	optionRetired
)

// configOption is one command-line setting. Its name is init's flag name and
// its values use that flag's format.
type configOption struct {
	name   string
	kind   optionKind
	format string // the value format, as init's help shows it
	about  string // plain language
	// required options cannot be unset.
	required bool
	// def describes the value that applies when the option is not set.
	def string
	// key describes how remove identifies a list item.
	key string
	// fields are the configuration fields the option covers, as validation
	// names them; refusals name the option instead.
	fields []string

	// get returns the configured value in the flag's format, one entry per
	// list item, or nil when the option is not set.
	get    func(*projectConfig) []string
	set    func(c *projectConfig, value string) error
	unset  func(*projectConfig)
	add    func(c *projectConfig, value string) error
	remove func(c *projectConfig, key string) error
}

func projectOf(c *projectConfig) *projectChoices {
	if c.Project == nil {
		c.Project = &projectChoices{}
	}
	return c.Project
}

func authoritiesOf(c *projectConfig) *authorities {
	pc := projectOf(c)
	if pc.Authorities == nil {
		pc.Authorities = &authorities{}
	}
	return pc.Authorities
}

// proj and auths read a section without creating it.
func proj(c *projectConfig) *projectChoices {
	if c.Project == nil {
		return &projectChoices{}
	}
	return c.Project
}

func auths(c *projectConfig) *authorities {
	if pc := proj(c); pc.Authorities != nil {
		return pc.Authorities
	}
	return &authorities{}
}

func present[T any](v *T) any {
	if v == nil {
		return nil
	}
	return v
}

var configSettings = []configSetting{
	{
		path:  []string{"project", "integration_branch"},
		label: "Integration branch",
		get:   func(c *projectConfig) any { return present(proj(c).IntegrationBranch) },
		set:   func(d, s *projectConfig) { projectOf(d).IntegrationBranch = projectOf(s).IntegrationBranch },
		options: []configOption{{
			name: "integration-branch", format: "BRANCH",
			about:  "the branch implementation pull requests target",
			def:    defaultIntegrationBranch,
			fields: []string{"project.integration_branch"},
			get:    func(c *projectConfig) []string { return stringValue(proj(c).IntegrationBranch) },
			set: func(c *projectConfig, v string) error {
				projectOf(c).IntegrationBranch = &v
				return nil
			},
			unset: func(c *projectConfig) { projectOf(c).IntegrationBranch = nil },
		}},
	},
	{
		path:  []string{"project", "pr_review_triggers"},
		label: "Extra pull-request reviews",
		get:   func(c *projectConfig) any { return present(proj(c).PRReviewTriggers) },
		set:   func(d, s *projectConfig) { projectOf(d).PRReviewTriggers = projectOf(s).PRReviewTriggers },
		options: []configOption{namedConditionOption(
			"pr-review-trigger", "an extra situation that needs a pull-request review",
			"project.pr_review_triggers",
			func(c *projectConfig) *[]namedCondition { return proj(c).PRReviewTriggers },
			func(c *projectConfig, list *[]namedCondition) { projectOf(c).PRReviewTriggers = list },
		)},
	},
	{
		path:  []string{"project", "automated_reviewer"},
		label: "Automated reviewer",
		get:   func(c *projectConfig) any { return present(proj(c).AutomatedReviewer) },
		set:   func(d, s *projectConfig) { projectOf(d).AutomatedReviewer = projectOf(s).AutomatedReviewer },
		options: []configOption{
			{
				name: "automated-reviewer", format: "NAME",
				about:  "an optional automated reviewer, whose output is review input only",
				def:    "none",
				fields: []string{"project.automated_reviewer.name", "project.automated_reviewer"},
				get: func(c *projectConfig) []string {
					if r := proj(c).AutomatedReviewer; r != nil {
						return []string{r.Name}
					}
					return nil
				},
				set: func(c *projectConfig, v string) error {
					r := automatedReviewer{Name: v}
					if old := proj(c).AutomatedReviewer; old != nil {
						r.Request = old.Request
					}
					projectOf(c).AutomatedReviewer = &r
					return nil
				},
				unset: func(c *projectConfig) { projectOf(c).AutomatedReviewer = nil },
			},
			{
				name: "automated-reviewer-request", format: "TEXT",
				about:  "how the automated reviewer's review is requested",
				def:    "none",
				fields: []string{"project.automated_reviewer.request"},
				get: func(c *projectConfig) []string {
					if r := proj(c).AutomatedReviewer; r != nil {
						return []string{r.Request}
					}
					return nil
				},
				set: func(c *projectConfig, v string) error {
					r := automatedReviewer{Request: v}
					if old := proj(c).AutomatedReviewer; old != nil {
						r.Name = old.Name
					}
					projectOf(c).AutomatedReviewer = &r
					return nil
				},
				unset: func(c *projectConfig) {
					if r := proj(c).AutomatedReviewer; r != nil {
						r.Request = ""
					}
				},
			},
		},
	},
	{
		path:  []string{"project", "small_corrections"},
		label: "Small corrections",
		get:   func(c *projectConfig) any { return present(proj(c).SmallCorrections) },
		set:   func(d, s *projectConfig) { projectOf(d).SmallCorrections = projectOf(s).SmallCorrections },
		options: []configOption{
			{
				name: "small-corrections", format: "enabled|disabled",
				about:  "whether a small correction may skip independent review when the classifier decides",
				def:    "disabled",
				fields: []string{"project.small_corrections.enabled", "project.small_corrections"},
				get: func(c *projectConfig) []string {
					sc := proj(c).SmallCorrections
					switch {
					case sc == nil:
						return nil
					case sc.Enabled != nil && *sc.Enabled:
						return []string{"enabled"}
					default:
						return []string{"disabled"}
					}
				},
				set: func(c *projectConfig, v string) error {
					sc := smallCorrections{}
					switch v {
					case "enabled":
						sc.Enabled = boolPtr(true)
						// Whether an issue is needed belongs to enabled
						// small corrections, so it is kept.
						if old := proj(c).SmallCorrections; old != nil {
							sc.WithoutIssue = old.WithoutIssue
						}
					case "disabled":
						// Disabled small corrections have no issue rule.
						sc.Enabled = boolPtr(false)
					default:
						return fmt.Errorf("small-corrections must be enabled or disabled, not %q", v)
					}
					projectOf(c).SmallCorrections = &sc
					return nil
				},
				unset: func(c *projectConfig) { projectOf(c).SmallCorrections = nil },
			},
			{
				name: "small-corrections-without-issue", format: "true|false",
				about:  "whether a small correction may be proposed without an issue",
				def:    "false",
				fields: []string{"project.small_corrections.without_issue"},
				get: func(c *projectConfig) []string {
					if sc := proj(c).SmallCorrections; sc != nil && sc.WithoutIssue != nil {
						return []string{fmt.Sprint(*sc.WithoutIssue)}
					}
					return nil
				},
				set: func(c *projectConfig, v string) error {
					if v != "true" && v != "false" {
						return fmt.Errorf("small-corrections-without-issue must be true or false, not %q", v)
					}
					sc := smallCorrections{Enabled: boolPtr(false)}
					if old := proj(c).SmallCorrections; old != nil {
						sc = *old
					}
					sc.WithoutIssue = boolPtr(v == "true")
					projectOf(c).SmallCorrections = &sc
					return nil
				},
				unset: func(c *projectConfig) {
					if sc := proj(c).SmallCorrections; sc != nil {
						sc.WithoutIssue = nil
					}
				},
			},
		},
	},
	{
		path:  []string{"project", "discovery"},
		label: "Discovery",
		get:   func(c *projectConfig) any { return present(proj(c).Discovery) },
		set:   func(d, s *projectConfig) { projectOf(d).Discovery = projectOf(s).Discovery },
		options: []configOption{
			{
				name: "discovery", format: strings.Join(discoveryModes, "|"),
				about:    "when discovery is needed before implementation",
				required: true,
				fields:   []string{"project.discovery.mode", "project.discovery"},
				get: func(c *projectConfig) []string {
					if d := proj(c).Discovery; d != nil {
						return []string{d.Mode}
					}
					return nil
				},
				set: func(c *projectConfig, v string) error {
					if !slices.Contains(discoveryModes, v) {
						return fmt.Errorf("discovery must be %s, not %q", strings.Join(discoveryModes, " or "), v)
					}
					d := discoveryChoice{Mode: v}
					// Triggers belong to the triggers mode, so they are kept
					// only while it stays selected.
					if old := proj(c).Discovery; old != nil && v == discoveryTriggers {
						d.Triggers = old.Triggers
					}
					projectOf(c).Discovery = &d
					return nil
				},
				unset: func(c *projectConfig) { projectOf(c).Discovery = nil },
			},
			namedConditionOption(
				"discovery-trigger", "a situation in which discovery is required",
				"project.discovery.triggers",
				func(c *projectConfig) *[]namedCondition {
					if d := proj(c).Discovery; d != nil {
						return d.Triggers
					}
					return nil
				},
				func(c *projectConfig, list *[]namedCondition) {
					pc := projectOf(c)
					if pc.Discovery == nil {
						pc.Discovery = &discoveryChoice{}
					}
					pc.Discovery.Triggers = list
				},
			),
		},
	},
	{
		path:  []string{"project", "release_conditions"},
		label: "Release conditions",
		get:   func(c *projectConfig) any { return present(proj(c).ReleaseConditions) },
		set:   func(d, s *projectConfig) { projectOf(d).ReleaseConditions = projectOf(s).ReleaseConditions },
		options: []configOption{listOption(
			"release-condition", "NAME|WHEN|EVIDENCE", "a condition under which release needs particular evidence",
			"NAME", "project.release_conditions",
			listSpec[releaseCond]{
				read: func(c *projectConfig) []releaseCond { return deref(proj(c).ReleaseConditions) },
				write: func(c *projectConfig, list []releaseCond) {
					projectOf(c).ReleaseConditions = listPointer(list)
				},
				format: func(r releaseCond) string { return r.Name + "|" + r.AppliesWhen + "|" + r.RequiredEvidence },
				parse:  parseReleaseCondition,
				key:    func(r releaseCond) string { return r.Name },
			},
		)},
	},
	{
		path:  []string{"project", "suggestions_backlog"},
		label: "Improvement suggestions",
		get:   func(c *projectConfig) any { return present(proj(c).SuggestionsBacklog) },
		set:   func(d, s *projectConfig) { projectOf(d).SuggestionsBacklog = projectOf(s).SuggestionsBacklog },
		options: []configOption{{
			name: "suggestions-backlog", format: "#NUMBER|none",
			about:  "the issue that collects improvement suggestions, or none",
			def:    fmt.Sprintf("the issue named %q", defaultSuggestionsIssue),
			fields: []string{"project.suggestions_backlog"},
			get:    func(c *projectConfig) []string { return stringValue(proj(c).SuggestionsBacklog) },
			set: func(c *projectConfig, v string) error {
				projectOf(c).SuggestionsBacklog = &v
				return nil
			},
			unset: func(c *projectConfig) { projectOf(c).SuggestionsBacklog = nil },
		}},
	},
	authoritySetting("workflow_owner", "Workflow Owner", "workflow-owner",
		"who owns the workflow and decides workflow questions", true,
		func(a *authorities) **authority { return &a.WorkflowOwner }),
	authoritySetting("change_acceptor", "Change acceptor", "change-acceptor",
		"who accepts a change into the project", true,
		func(a *authorities) **authority { return &a.ChangeAcceptor }),
	authoritySetting("product_intent_acceptor", "Product-intent acceptor", "product-intent-acceptor",
		"who decides whether a result is what was intended", true,
		func(a *authorities) **authority { return &a.ProductIntentAcceptor }),
	authoritySetting("small_correction_classifier", "Small-correction classifier", "small-correction-classifier",
		"who decides that a change is a small correction; only when small corrections are enabled", false,
		func(a *authorities) **authority { return &a.SmallCorrectionClassifier }),
	{
		path:  []string{"project", "user_facing_inspection"},
		label: "User-facing inspection",
		get:   func(c *projectConfig) any { return present(proj(c).UserFacingInspection) },
		set:   func(d, s *projectConfig) { projectOf(d).UserFacingInspection = projectOf(s).UserFacingInspection },
		options: []configOption{listOption(
			"inspection-condition", "TEXT", "an added situation in which a person inspects user-facing behavior",
			"the condition's exact text", "project.user_facing_inspection.additional_conditions",
			listSpec[string]{
				read: func(c *projectConfig) []string {
					if ins := proj(c).UserFacingInspection; ins != nil {
						return ins.AdditionalConditions
					}
					return nil
				},
				write: func(c *projectConfig, list []string) {
					if len(list) == 0 {
						projectOf(c).UserFacingInspection = nil
						return
					}
					projectOf(c).UserFacingInspection = &inspectionChoice{AdditionalConditions: list}
				},
				format: func(s string) string { return s },
				parse: func(v string) (string, error) {
					if v = strings.TrimSpace(v); v == "" {
						return "", errors.New("want the condition's text")
					}
					return v, nil
				},
				key: func(s string) string { return s },
			},
		)},
	},
	{
		path:  []string{"project", "guidance"},
		label: "Manuals",
		get:   func(c *projectConfig) any { return present(proj(c).Guidance) },
		set:   func(d, s *projectConfig) { projectOf(d).Guidance = projectOf(s).Guidance },
		options: []configOption{
			listOption(
				"manual", "NAME or NAME@VERSION", "an adopted manual; a name alone adopts the version this garage delivers",
				"NAME", "project.guidance.manuals",
				listSpec[manualChoice]{
					read: func(c *projectConfig) []manualChoice {
						if g := proj(c).Guidance; g != nil {
							return g.Manuals
						}
						return nil
					},
					write: func(c *projectConfig, list []manualChoice) {
						g := guidanceChoice{Manuals: list}
						if old := proj(c).Guidance; old != nil {
							g.Conditional = old.Conditional
						}
						projectOf(c).Guidance = guidanceOrNil(g)
					},
					format: func(m manualChoice) string { return m.Name + "@" + m.Version },
					parse:  parseManualOrName,
					key:    func(m manualChoice) string { return m.Name },
				},
			),
			{
				name: "conditional-guidance", kind: optionRetired,
				about:  "retired: conditional stack or provider guidance is not configurable",
				def:    "none",
				fields: []string{"project.guidance.conditional"},
				get: func(c *projectConfig) []string {
					if g := proj(c).Guidance; g != nil {
						return g.Conditional
					}
					return nil
				},
				unset: func(c *projectConfig) {
					if g := proj(c).Guidance; g != nil {
						projectOf(c).Guidance = guidanceOrNil(guidanceChoice{Manuals: g.Manuals})
					}
				},
			},
		},
	},
	{
		path:  []string{"tooling", "skill_targets"},
		label: "Skills installed for",
		get: func(c *projectConfig) any {
			if c.Tooling == nil {
				return nil
			}
			return present(c.Tooling.SkillTargets)
		},
		set: func(d, s *projectConfig) {
			if d.Tooling == nil {
				d.Tooling = &toolingConfig{}
			}
			d.Tooling.SkillTargets = s.Tooling.SkillTargets
		},
		options: []configOption{listOption(
			"skill-target", "HARNESS or HARNESS@VERSION@YYYY-MM-DD",
			"an agent tool garage skills install manages; a harness alone uses its confirmed version and date ("+
				strings.Join(supportedHarnesses, ", ")+")",
			"HARNESS", "tooling.skill_targets",
			listSpec[skillTarget]{
				read: func(c *projectConfig) []skillTarget {
					if c.Tooling != nil {
						return deref(c.Tooling.SkillTargets)
					}
					return nil
				},
				write: func(c *projectConfig, list []skillTarget) {
					if len(list) == 0 {
						c.Tooling = nil
						return
					}
					c.Tooling = &toolingConfig{SkillTargets: &list}
				},
				format: func(t skillTarget) string { return t.Harness + "@" + t.ConfirmedVersion + "@" + t.ConfirmedOn },
				parse:  parseSkillTargetOrHarness,
				key:    func(t skillTarget) string { return t.Harness },
			},
		)},
	},
}

func settingName(path []string) string { return strings.Join(path, ".") }

func settingByPath(path []string) configSetting {
	for _, s := range configSettings {
		if slices.Equal(s.path, path) {
			return s
		}
	}
	panic("unknown configuration setting " + settingName(path))
}

// optionByName returns the command-line setting called name.
func optionByName(name string) (configOption, bool) {
	for _, s := range configSettings {
		for _, o := range s.options {
			if o.name == name {
				return o, true
			}
		}
	}
	return configOption{}, false
}

// allOptions returns every command-line setting, in registry order.
func allOptions() []configOption {
	var all []configOption
	for _, s := range configSettings {
		all = append(all, s.options...)
	}
	return all
}

// authoritySetting is the setting for one human responsibility.
func authoritySetting(field, label, name, about string, required bool, slot func(*authorities) **authority) configSetting {
	read := func(c *projectConfig) *authority { return *slot(auths(c)) }
	return configSetting{
		path:  []string{"project", "authorities", field},
		label: label,
		get:   func(c *projectConfig) any { return present(read(c)) },
		set:   func(d, s *projectConfig) { *slot(authoritiesOf(d)) = *slot(authoritiesOf(s)) },
		options: []configOption{{
			name: name, format: "ROLE or ROLE:MEMBER,MEMBER",
			about:    about,
			required: required,
			def:      "none",
			fields:   []string{"project.authorities." + field},
			get: func(c *projectConfig) []string {
				if a := read(c); a != nil {
					return []string{formatAuthority(*a)}
				}
				return nil
			},
			set: func(c *projectConfig, v string) error {
				a := parseAuthority(v)
				*slot(authoritiesOf(c)) = &a
				return nil
			},
			unset: func(c *projectConfig) { *slot(authoritiesOf(c)) = nil },
		}},
	}
}

// formatAuthority writes an authority in the format parseAuthority reads.
func formatAuthority(a authority) string {
	if len(a.Members) == 0 {
		return a.Role
	}
	return a.Role + ":" + strings.Join(a.Members, ",")
}

// listSpec describes a list setting's items.
type listSpec[T any] struct {
	read   func(*projectConfig) []T
	write  func(*projectConfig, []T) // an empty list removes the setting
	format func(T) string
	parse  func(string) (T, error)
	key    func(T) string
}

func listOption[T any](name, format, about, key, field string, spec listSpec[T]) configOption {
	return configOption{
		name: name, kind: optionList, format: format, about: about,
		def: "none", key: key, fields: []string{field},
		get: func(c *projectConfig) []string {
			var out []string
			for _, item := range spec.read(c) {
				out = append(out, spec.format(item))
			}
			return out
		},
		add: func(c *projectConfig, v string) error {
			item, err := spec.parse(v)
			if err != nil {
				return fmt.Errorf("%s %q: %v", name, v, err)
			}
			items := spec.read(c)
			for _, existing := range items {
				if spec.key(existing) == spec.key(item) {
					return fmt.Errorf("%s %q is already configured; remove it first to replace it", name, spec.key(item))
				}
			}
			spec.write(c, append(slices.Clone(items), item))
			return nil
		},
		remove: func(c *projectConfig, k string) error {
			items := spec.read(c)
			i := slices.IndexFunc(items, func(item T) bool { return spec.key(item) == k })
			if i < 0 {
				var keys []string
				for _, item := range items {
					keys = append(keys, strconv.Quote(spec.key(item)))
				}
				return fmt.Errorf("no %s %q is configured; configured: %s", name, k, namesOrDefault(keys, "none"))
			}
			spec.write(c, slices.Delete(slices.Clone(items), i, i+1))
			return nil
		},
		unset: func(c *projectConfig) { spec.write(c, nil) },
	}
}

// namedConditionOption is a list of NAME|WHEN triggers.
func namedConditionOption(name, about, field string,
	read func(*projectConfig) *[]namedCondition, write func(*projectConfig, *[]namedCondition)) configOption {
	return listOption(name, "NAME|WHEN", about, "NAME", field, listSpec[namedCondition]{
		read:   func(c *projectConfig) []namedCondition { return deref(read(c)) },
		write:  func(c *projectConfig, list []namedCondition) { write(c, listPointer(list)) },
		format: func(n namedCondition) string { return n.Name + "|" + n.AppliesWhen },
		parse:  parseNamedCondition,
		key:    func(n namedCondition) string { return n.Name },
	})
}

func stringValue(v *string) []string {
	if v == nil {
		return nil
	}
	return []string{*v}
}

func deref[T any](list *[]T) []T {
	if list == nil {
		return nil
	}
	return *list
}

// listPointer returns nil for an empty list, so removing the last item
// removes the setting.
func listPointer[T any](list []T) *[]T {
	if len(list) == 0 {
		return nil
	}
	return &list
}

func guidanceOrNil(g guidanceChoice) *guidanceChoice {
	if len(g.Manuals) == 0 && len(g.Conditional) == 0 {
		return nil
	}
	return &g
}

// parseManualOrName reads NAME@VERSION, or a NAME alone, which adopts the
// version this garage delivers.
func parseManualOrName(v string) (manualChoice, error) {
	if strings.Contains(v, "@") {
		return parseManual(v)
	}
	name := strings.TrimSpace(v)
	catalog, err := embeddedManualCatalog()
	if err != nil {
		return manualChoice{}, fmt.Errorf("cannot read the manuals this garage carries: %v", err)
	}
	m := findManual(catalog, name)
	if m == nil {
		return manualChoice{}, fmt.Errorf("not a manual this garage carries; available: %s",
			namesOrDefault(manualNames(catalog), "none"))
	}
	return manualChoice{Name: m.Name, Version: m.Version}, nil
}

// parseSkillTargetOrHarness reads HARNESS@VERSION@YYYY-MM-DD, or a HARNESS
// alone, which uses the version and date its skill path was confirmed with.
func parseSkillTargetOrHarness(v string) (skillTarget, error) {
	if strings.Contains(v, "@") {
		return parseSkillTarget(v)
	}
	harness := strings.TrimSpace(v)
	confirmed, ok := confirmedSkillTargets[harness]
	if !ok {
		return skillTarget{}, fmt.Errorf("harness must be one of %s", strings.Join(supportedHarnesses, ", "))
	}
	return parseSkillTarget(harness + "@" + confirmed.Version + "@" + confirmed.On)
}
