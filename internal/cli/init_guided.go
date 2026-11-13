package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The guided setup asks plain-language questions in an interactive terminal.
// It never shows extension-point identifiers or flag syntax, and it builds
// exactly the values the equivalent flags would. Every question goes through
// prompter.Ask, so the flow can be driven without a terminal.

// choice is one numbered option in a menu.
type choice struct {
	label  string
	detail string
}

// guide asks the questions of the guided setup.
type guide struct {
	p prompter
}

// choose presents a numbered menu and returns the index of the chosen
// option. An empty answer selects def.
func (g guide) choose(intro string, options []choice, def int) (int, error) {
	var b strings.Builder
	b.WriteString("\n" + intro + "\n")
	for i, o := range options {
		fmt.Fprintf(&b, "  %d) %s\n", i+1, o.label)
		if o.detail != "" {
			fmt.Fprintf(&b, "     %s\n", o.detail)
		}
	}
	question := fmt.Sprintf("%sChoose 1-%d [%d]: ", b.String(), len(options), def+1)
	for {
		answer, err := g.p.Ask(question)
		if err != nil {
			return 0, err
		}
		if answer == "" {
			return def, nil
		}
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		question = fmt.Sprintf("Please enter a number from 1 to %d [%d]: ", len(options), def+1)
	}
}

// chooseMany presents a numbered list and returns the chosen indexes in list
// order. An empty answer chooses none.
func (g guide) chooseMany(intro string, options []choice) ([]int, error) {
	var b strings.Builder
	b.WriteString("\n" + intro + "\n")
	for i, o := range options {
		fmt.Fprintf(&b, "  %d) %s\n", i+1, o.label)
	}
	question := b.String() + "Enter numbers separated by commas, or press Enter for none: "
	for {
		answer, err := g.p.Ask(question)
		if err != nil {
			return nil, err
		}
		chosen, ok := parseNumberList(answer, len(options))
		if ok {
			return chosen, nil
		}
		question = fmt.Sprintf("Please enter numbers from 1 to %d separated by commas, or press Enter for none: ", len(options))
	}
}

func parseNumberList(answer string, n int) ([]int, bool) {
	picked := make([]bool, n)
	for _, part := range strings.Split(answer, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.Atoi(part)
		if err != nil || v < 1 || v > n {
			return nil, false
		}
		picked[v-1] = true
	}
	var chosen []int
	for i, p := range picked {
		if p {
			chosen = append(chosen, i)
		}
	}
	return chosen, true
}

// text asks for a free-text answer. An empty answer selects def when def is
// not empty, and is asked again otherwise. check, when given, rejects an
// answer and explains why.
func (g guide) text(question, def string, check func(string) error) (string, error) {
	prompt := question
	if def != "" {
		prompt = fmt.Sprintf("%s[%s]: ", question, def)
	}
	for {
		answer, err := g.p.Ask(prompt)
		if err != nil {
			return "", err
		}
		if answer == "" {
			if def != "" {
				return def, nil
			}
			prompt = "An answer is required. " + question
			continue
		}
		if check != nil {
			if err := check(answer); err != nil {
				prompt = fmt.Sprintf("%s. %s", err, question)
				continue
			}
		}
		return answer, nil
	}
}

// yesNo asks a yes-or-no question. An empty answer selects def.
func (g guide) yesNo(question string, def bool) (bool, error) {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	prompt := fmt.Sprintf("%s %s: ", question, hint)
	for {
		answer, err := g.p.Ask(prompt)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		prompt = fmt.Sprintf("Please answer y or n. %s %s: ", question, hint)
	}
}

// placeholderAnswers are answers that name the kind of value requested
// rather than an actual one. Interactive setup refuses them.
var placeholderAnswers = map[string]bool{
	"role": true, "roles": true, "member": true, "members": true,
	"role:member": true, "role:members": true, "name": true,
	"tbd": true, "tba": true, "todo": true, "n/a": true, "na": true,
	"none": true, "placeholder": true, "xxx": true, "...": true, "…": true, "?": true,
}

// checkNotPlaceholder rejects an answer that is a placeholder rather than a
// real value.
func checkNotPlaceholder(v string) error {
	s := strings.ToLower(strings.TrimSpace(v))
	bracketed := len(s) >= 2 && ((s[0] == '<' && s[len(s)-1] == '>') ||
		(s[0] == '[' && s[len(s)-1] == ']') || (s[0] == '{' && s[len(s)-1] == '}'))
	if placeholderAnswers[s] || bracketed {
		return fmt.Errorf("%q looks like a placeholder, not an actual name", v)
	}
	return nil
}

// checkSingleLine rejects an answer that cannot be stored as a single line.
func checkSingleLine(v string) error {
	if strings.ContainsAny(v, "\r\n") {
		return errors.New("the answer must be a single line")
	}
	return nil
}

// --- Responsibilities ---

const authoritiesIntro = `Some workflow decisions always belong to a person, never to an agent or a bot.
The next questions record who holds each of them. You can name a role, a team,
or a person, and optionally the GitHub usernames or teams who fill it.`

// authorityQuestion describes one human responsibility.
type authorityQuestion struct {
	title   string
	explain string
}

var (
	workflowOwnerQuestion = authorityQuestion{
		title: "Workflow Owner",
		explain: "The Workflow Owner makes the workflow decisions that belong to a person:\n" +
			"answering clarifying questions, requiring a discovery investigation when one\n" +
			"is needed, and choosing which improvement suggestions become planned work.",
	}
	changeAcceptorQuestion = authorityQuestion{
		title: "Change acceptor",
		explain: "The change acceptor decides whether the project accepts a change. This is the\n" +
			"human approval a pull request needs before it merges. It is separate from, and\n" +
			"never replaced by, the independent review of the change.",
	}
	productIntentQuestion = authorityQuestion{
		title: "Product-intent acceptor",
		explain: "The product-intent acceptor decides whether a result is what was intended for\n" +
			"its users, and inspects a change that users see or use before accepting it,\n" +
			"when that is practical.",
	}
	classifierQuestion = authorityQuestion{
		title: "Small-correction classifier",
		explain: "The small-correction classifier decides when a change is a small correction\n" +
			"that may skip independent review. Only this person or role may make that call;\n" +
			"an agent never does.",
	}
)

// chosenAuthority is an authority already selected, offered for reuse.
type chosenAuthority struct {
	title string
	value authority
}

// askAuthorityGuided asks who holds one responsibility. Authorities already
// chosen are offered for reuse, so they need not be typed again.
func (g guide) askAuthorityGuided(q authorityQuestion, reuse []chosenAuthority) (*authority, error) {
	intro := q.title + "\n" + q.explain
	var options []choice
	var values []authority
	for _, r := range reuse {
		duplicate := false
		for _, v := range values {
			if authoritiesEqual(v, r.value) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		options = append(options, choice{label: fmt.Sprintf("Same as the %s: %s", r.title, describeAuthority(r.value))})
		values = append(values, r.value)
	}
	if len(options) > 0 {
		options = append(options, choice{label: "Someone else"})
		n, err := g.choose(intro+"\nWho holds this responsibility?", options, 0)
		if err != nil {
			return nil, err
		}
		if n < len(values) {
			a := values[n]
			a.Members = append([]string(nil), a.Members...)
			return &a, nil
		}
		intro = ""
	}
	if intro != "" {
		if err := g.p.Tell("\n" + intro + "\n"); err != nil {
			return nil, err
		}
	}

	role, err := g.text("Role, team, or person who holds it (for example: maintainer, lead maintainer): ", "",
		func(v string) error {
			if err := checkSingleLine(v); err != nil {
				return err
			}
			if strings.ContainsAny(v, ":,") {
				return errors.New("a name cannot contain a colon or a comma")
			}
			return checkNotPlaceholder(v)
		})
	if err != nil {
		return nil, err
	}
	a := authority{Role: strings.TrimSpace(role)}
	for {
		answer, err := g.p.Ask("GitHub usernames or teams who fill it, separated by commas (optional; press Enter to skip): ")
		if err != nil {
			return nil, err
		}
		members, problem := parseMembers(answer)
		if problem == nil {
			a.Members = members
			return &a, nil
		}
		if err := g.p.Tell(problem.Error() + ".\n"); err != nil {
			return nil, err
		}
	}
}

// parseMembers splits a comma-separated list of members, rejecting
// placeholders and repeats.
func parseMembers(answer string) ([]string, error) {
	var members []string
	seen := map[string]bool{}
	for _, m := range strings.Split(answer, ",") {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if err := checkNotPlaceholder(m); err != nil {
			return nil, err
		}
		if strings.Contains(m, ":") {
			return nil, fmt.Errorf("%q cannot contain a colon", m)
		}
		if seen[m] {
			return nil, fmt.Errorf("%q is listed more than once", m)
		}
		seen[m] = true
		members = append(members, m)
	}
	return members, nil
}

func authoritiesEqual(a, b authority) bool {
	return a.Role == b.Role && slicesEqual(a.Members, b.Members)
}

func describeAuthority(a authority) string {
	if len(a.Members) == 0 {
		return a.Role
	}
	return fmt.Sprintf("%s (%s)", a.Role, strings.Join(a.Members, ", "))
}

// --- Discovery ---

const discoveryIntro = `Discovery
Discovery is an investigation of how the existing system works, done before a
change is implemented and kept separate from it. How should the project decide
when a change needs discovery?`

func (g guide) askDiscoveryGuided() (*discoveryChoice, error) {
	n, err := g.choose(discoveryIntro, []choice{
		{label: "The implementer decides, and the Workflow Owner can require it"},
		{label: "The project lists specific situations that always require discovery"},
	}, 0)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return &discoveryChoice{Mode: discoveryJudgment}, nil
	}
	list, err := g.askNamedConditions("situation that requires discovery",
		"schema-change", "the change alters a persisted schema", true)
	if err != nil {
		return nil, err
	}
	return &discoveryChoice{Mode: discoveryTriggers, Triggers: &list}, nil
}

// askNamedConditions asks for one or more named situations. With atLeastOne,
// the first is asked without an option to stop.
func (g guide) askNamedConditions(what, exampleName, exampleWhen string, atLeastOne bool) ([]namedCondition, error) {
	var list []namedCondition
	for {
		c, err := g.askNamedCondition(what, exampleName, exampleWhen, list)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
		more, err := g.yesNo("Add another "+what+"?", false)
		if err != nil {
			return nil, err
		}
		if !more {
			return list, nil
		}
	}
}

func (g guide) askNamedCondition(what, exampleName, exampleWhen string, existing []namedCondition) (namedCondition, error) {
	name, err := g.text(fmt.Sprintf("Short name for the %s (for example: %s): ", what, exampleName), "",
		func(v string) error {
			if err := checkSingleLine(v); err != nil {
				return err
			}
			if strings.Contains(v, "|") {
				return errors.New("a name cannot contain |")
			}
			for _, c := range existing {
				if c.Name == strings.TrimSpace(v) {
					return fmt.Errorf("%q is already listed", v)
				}
			}
			return checkNotPlaceholder(v)
		})
	if err != nil {
		return namedCondition{}, err
	}
	when, err := g.text(fmt.Sprintf("When does it apply? (for example: %s): ", exampleWhen), "",
		func(v string) error {
			if strings.Contains(v, "|") {
				return errors.New("the description cannot contain |")
			}
			return nil
		})
	if err != nil {
		return namedCondition{}, err
	}
	return namedCondition{Name: strings.TrimSpace(name), AppliesWhen: strings.TrimSpace(when)}, nil
}

// --- The full walkthrough for a new configuration ---

// detectDefaultBranch returns the repository's default branch, as recorded by
// origin/HEAD, or "" when it cannot be determined. It is a variable so tests
// can replace it.
var detectDefaultBranch = func(projectDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := runGit(ctx, projectDir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	branch := strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/")
	if checkBranchName(branch) != nil {
		return ""
	}
	return branch
}

// harnessNames are the display names of the supported skill-install targets.
var harnessNames = map[string]string{
	"claude-code": "Claude Code",
	"codex":       "Codex",
	"copilot-cli": "GitHub Copilot CLI",
}

// guideNewConfiguration walks through every project choice that cfg does not
// already hold, as a flag may have supplied some. A choice left at its
// default is not written, so the canonical default applies.
func guideNewConfiguration(cfg *projectConfig, p prompter, projectDir string) error {
	g := guide{p: p}
	if err := p.Tell("This sets up Builder Garage for this project. Each question shows its\n" +
		"default in brackets; press Enter to accept it. Nothing is written until you\n" +
		"confirm the summary at the end.\n"); err != nil {
		return err
	}
	pc := projectOf(cfg)

	if pc.IntegrationBranch == nil {
		if err := g.askIntegrationBranch(pc, projectDir); err != nil {
			return err
		}
	}
	if pc.PRReviewTriggers == nil {
		n, err := g.choose("Extra pull-request reviews\n"+
			"A pull request always gets another independent review when it no longer matches\n"+
			"what was approved, or when an automated reviewer reports findings on it. Does\n"+
			"the project add other situations that require one?", []choice{
			{label: "No, the two standard situations are enough"},
			{label: "Yes, add project-specific situations"},
		}, 0)
		if err != nil {
			return err
		}
		if n == 1 {
			list, err := g.askNamedConditions("situation that requires a pull-request review",
				"security-sensitive", "the change touches authentication or payments", true)
			if err != nil {
				return err
			}
			pc.PRReviewTriggers = &list
		}
	}
	if pc.AutomatedReviewer == nil {
		n, err := g.choose("Automated reviewer\n"+
			"Does the project use an automated code reviewer, such as a bot that comments on\n"+
			"pull requests? Its comments are review input only; they never approve a change.", []choice{
			{label: "No automated reviewer"},
			{label: "Yes, the project uses one"},
		}, 0)
		if err != nil {
			return err
		}
		if n == 1 {
			name, err := g.text("Name of the automated reviewer: ", "", func(v string) error {
				if err := checkSingleLine(v); err != nil {
					return err
				}
				return checkNotPlaceholder(v)
			})
			if err != nil {
				return err
			}
			request, err := g.text("How is its review requested? (for example: it reviews each pull request when it is opened): ", "", nil)
			if err != nil {
				return err
			}
			pc.AutomatedReviewer = &automatedReviewer{Name: strings.TrimSpace(name), Request: strings.TrimSpace(request)}
		}
	}
	if pc.SmallCorrections == nil {
		n, err := g.choose("Small corrections\n"+
			"May a person you name let trivial fixes, such as typos, broken links, or\n"+
			"formatting, skip independent review? Human approval is still required.", []choice{
			{label: "No, every change gets independent review"},
			{label: "Yes, a named person or role may classify small corrections"},
		}, 0)
		if err != nil {
			return err
		}
		if n == 1 {
			withoutIssue, err := g.yesNo("May a small correction be proposed without opening an issue first?", false)
			if err != nil {
				return err
			}
			sc := &smallCorrections{Enabled: boolPtr(true)}
			if withoutIssue {
				sc.WithoutIssue = boolPtr(true)
			}
			pc.SmallCorrections = sc
		}
	}
	if pc.Discovery == nil {
		d, err := g.askDiscoveryGuided()
		if err != nil {
			return err
		}
		pc.Discovery = d
	}
	if pc.ReleaseConditions == nil {
		n, err := g.choose("Release conditions\n"+
			"Some projects require extra evidence before certain changes are released, for\n"+
			"example a security sign-off for payment changes. Does this project?", []choice{
			{label: "No extra release conditions"},
			{label: "Yes, add release conditions"},
		}, 0)
		if err != nil {
			return err
		}
		if n == 1 {
			list, err := g.askReleaseConditions()
			if err != nil {
				return err
			}
			pc.ReleaseConditions = &list
		}
	}
	if pc.SuggestionsBacklog == nil {
		if err := g.askSuggestionsBacklog(pc); err != nil {
			return err
		}
	}
	if err := g.askAuthorities(cfg); err != nil {
		return err
	}
	if pc.UserFacingInspection == nil {
		n, err := g.choose("Inspecting user-facing changes\n"+
			"Before accepting a change that users see or use, the product-intent acceptor\n"+
			"inspects the result when that is practical. Should other situations also call\n"+
			"for that inspection?", []choice{
			{label: "No, the standard rule is enough"},
			{label: "Yes, add situations"},
		}, 0)
		if err != nil {
			return err
		}
		if n == 1 {
			var conditions []string
			for {
				c, err := g.text("Describe a situation that also calls for inspection: ", "", func(v string) error {
					for _, existing := range conditions {
						if existing == strings.TrimSpace(v) {
							return fmt.Errorf("%q is already listed", v)
						}
					}
					return nil
				})
				if err != nil {
					return err
				}
				conditions = append(conditions, strings.TrimSpace(c))
				more, err := g.yesNo("Add another situation?", false)
				if err != nil {
					return err
				}
				if !more {
					break
				}
			}
			pc.UserFacingInspection = &inspectionChoice{AdditionalConditions: conditions}
		}
	}
	if pc.Guidance == nil {
		if err := g.askGuidance(pc); err != nil {
			return err
		}
	}
	if cfg.Tooling == nil || cfg.Tooling.SkillTargets == nil {
		if err := g.askSkillTargets(cfg); err != nil {
			return err
		}
	}
	return nil
}

func (g guide) askIntegrationBranch(pc *projectChoices, projectDir string) error {
	intro := "Integration branch\nWhich branch do implementation pull requests target?"
	var options []choice
	var branches []string
	if detected := detectDefaultBranch(projectDir); detected != "" && detected != defaultIntegrationBranch {
		options = append(options, choice{label: detected, detail: "this repository's default branch"})
		branches = append(branches, detected)
	}
	options = append(options, choice{label: defaultIntegrationBranch, detail: "the Builder Garage default"})
	branches = append(branches, defaultIntegrationBranch)
	options = append(options, choice{label: "Another branch"})
	n, err := g.choose(intro, options, 0)
	if err != nil {
		return err
	}
	branch := ""
	if n < len(branches) {
		branch = branches[n]
	} else {
		branch, err = g.text("Branch name: ", "", checkBranchName)
		if err != nil {
			return err
		}
	}
	if branch != defaultIntegrationBranch {
		pc.IntegrationBranch = &branch
	}
	return nil
}

func (g guide) askReleaseConditions() ([]releaseCond, error) {
	var list []releaseCond
	for {
		var named []namedCondition
		for _, c := range list {
			named = append(named, namedCondition{Name: c.Name, AppliesWhen: c.AppliesWhen})
		}
		c, err := g.askNamedCondition("release condition", "payments",
			"the change touches payment processing", named)
		if err != nil {
			return nil, err
		}
		evidence, err := g.text("What evidence does it require? (for example: a security sign-off recorded on the pull request): ", "",
			func(v string) error {
				if strings.Contains(v, "|") {
					return errors.New("the description cannot contain |")
				}
				return nil
			})
		if err != nil {
			return nil, err
		}
		list = append(list, releaseCond{Name: c.Name, AppliesWhen: c.AppliesWhen, RequiredEvidence: strings.TrimSpace(evidence)})
		more, err := g.yesNo("Add another release condition?", false)
		if err != nil {
			return nil, err
		}
		if !more {
			return list, nil
		}
	}
}

func (g guide) askSuggestionsBacklog(pc *projectChoices) error {
	n, err := g.choose("Improvement suggestions\n"+
		"Reviewers sometimes notice improvements that are not needed for the current\n"+
		"change. Where should those suggestions be kept?", []choice{
		{label: fmt.Sprintf("In this repository's issue named %q", defaultSuggestionsIssue)},
		{label: "In a specific issue"},
		{label: "In the review record that raised them; no separate issue"},
	}, 0)
	if err != nil {
		return err
	}
	switch n {
	case 1:
		ref, err := g.text("Issue number (for example: 24): ", "", func(v string) error {
			if !issueRefPattern.MatchString("#" + strings.TrimPrefix(strings.TrimSpace(v), "#")) {
				return fmt.Errorf("%q is not an issue number", v)
			}
			return nil
		})
		if err != nil {
			return err
		}
		ref = "#" + strings.TrimPrefix(strings.TrimSpace(ref), "#")
		pc.SuggestionsBacklog = &ref
	case 2:
		none := backlogNone
		pc.SuggestionsBacklog = &none
	}
	return nil
}

// askAuthorities asks for each responsibility cfg does not already hold.
func (g guide) askAuthorities(cfg *projectConfig) error {
	a := authoritiesOf(cfg)
	if a.WorkflowOwner != nil && a.ChangeAcceptor != nil && a.ProductIntentAcceptor != nil &&
		(!smallCorrectionsEnabled(cfg) || a.SmallCorrectionClassifier != nil) {
		return nil
	}
	if err := g.p.Tell("\nWho decides\n" + authoritiesIntro + "\n"); err != nil {
		return err
	}
	for _, step := range authoritySteps(cfg) {
		if *step.slot != nil {
			continue
		}
		v, err := g.askAuthorityGuided(step.question, chosenAuthorities(a))
		if err != nil {
			return err
		}
		*step.slot = v
	}
	return nil
}

type authorityStep struct {
	question authorityQuestion
	slot     **authority
}

func authoritySteps(cfg *projectConfig) []authorityStep {
	a := authoritiesOf(cfg)
	steps := []authorityStep{
		{workflowOwnerQuestion, &a.WorkflowOwner},
		{changeAcceptorQuestion, &a.ChangeAcceptor},
		{productIntentQuestion, &a.ProductIntentAcceptor},
	}
	if smallCorrectionsEnabled(cfg) {
		steps = append(steps, authorityStep{classifierQuestion, &a.SmallCorrectionClassifier})
	}
	return steps
}

// chosenAuthorities lists the authorities already held, for reuse.
func chosenAuthorities(a *authorities) []chosenAuthority {
	var out []chosenAuthority
	for _, c := range []struct {
		title string
		value *authority
	}{
		{"Workflow Owner", a.WorkflowOwner},
		{"change acceptor", a.ChangeAcceptor},
		{"product-intent acceptor", a.ProductIntentAcceptor},
		{"small-correction classifier", a.SmallCorrectionClassifier},
	} {
		if c.value != nil {
			out = append(out, chosenAuthority{title: c.title, value: *c.value})
		}
	}
	return out
}

// askGuidance offers the manuals this garage carries as a pick list, each
// with its version, status, and description; choosing none is the default.
// A chosen manual is written with the version this garage delivers (#60).
// Conditional stack or provider guidance is not configurable, so it is not
// offered (#59).
func (g guide) askGuidance(pc *projectChoices) error {
	catalog, err := embeddedManualCatalog()
	if err != nil {
		return err
	}
	if len(catalog) == 0 {
		return nil
	}
	options := make([]choice, len(catalog))
	for i, m := range catalog {
		options[i] = choice{label: describeManual(m)}
	}
	chosen, err := g.chooseMany("Manuals\n"+
		"A project may adopt Builder Garage manuals as guidance for its engineering\n"+
		"choices. A manual never relaxes a review or approval gate. Which manuals does\n"+
		"this project adopt?", options)
	if err != nil || len(chosen) == 0 {
		return err
	}
	var gc guidanceChoice
	for _, i := range chosen {
		gc.Manuals = append(gc.Manuals, manualChoice{Name: catalog[i].Name, Version: catalog[i].Version})
	}
	pc.Guidance = &gc
	return nil
}

func (g guide) askSkillTargets(cfg *projectConfig) error {
	var options []choice
	for _, h := range supportedHarnesses {
		options = append(options, choice{label: harnessNames[h]})
	}
	chosen, err := g.chooseMany("Agent tools\n"+
		"Garage can install the workflow skills for these agent tools. Which does this\n"+
		"project use?", options)
	if err != nil || len(chosen) == 0 {
		return err
	}
	var targets []skillTarget
	for _, i := range chosen {
		h := supportedHarnesses[i]
		c := confirmedSkillTargets[h]
		t, err := parseSkillTarget(h + "@" + c.Version + "@" + c.On)
		if err != nil {
			return err
		}
		targets = append(targets, t)
	}
	if cfg.Tooling == nil {
		cfg.Tooling = &toolingConfig{}
	}
	cfg.Tooling.SkillTargets = &targets
	return nil
}

// --- Summary and confirmation ---

// summaryLine is one plain-language line of the configuration summary.
type summaryLine struct {
	label, value string
	// name is the setting's command-line name.
	name string
}

// describeConfiguration summarizes cfg in plain language, showing the
// default that applies wherever cfg leaves a choice unset.
func describeConfiguration(cfg projectConfig) []summaryLine {
	pc := proj(&cfg)
	a := auths(&cfg)
	def := func(v string) string { return v + " (default)" }
	names := func(list []namedCondition) string {
		var n []string
		for _, c := range list {
			n = append(n, c.Name)
		}
		return strings.Join(n, ", ")
	}
	var lines []summaryLine
	add := func(field, value string) {
		s := settingByPath(strings.Split(field, "."))
		lines = append(lines, summaryLine{label: s.label, value: value, name: s.options[0].name})
	}

	if pc.IntegrationBranch != nil {
		add("project.integration_branch", *pc.IntegrationBranch)
	} else {
		add("project.integration_branch", def(defaultIntegrationBranch))
	}
	if pc.PRReviewTriggers != nil && len(*pc.PRReviewTriggers) > 0 {
		add("project.pr_review_triggers", "also when: "+names(*pc.PRReviewTriggers))
	} else {
		add("project.pr_review_triggers", def("only the two standard situations"))
	}
	if r := pc.AutomatedReviewer; r != nil {
		add("project.automated_reviewer", r.Name+"; review input only")
	} else {
		add("project.automated_reviewer", def("none"))
	}
	if smallCorrectionsEnabled(&cfg) {
		v := "allowed when the small-correction classifier decides; an issue is required"
		if sc := pc.SmallCorrections; sc.WithoutIssue != nil && *sc.WithoutIssue {
			v = "allowed when the small-correction classifier decides; no issue needed"
		}
		add("project.small_corrections", v)
	} else if pc.SmallCorrections != nil {
		add("project.small_corrections", "not allowed; every change gets independent review")
	} else {
		add("project.small_corrections", def("not allowed; every change gets independent review"))
	}
	if d := pc.Discovery; d != nil {
		if d.Mode == discoveryTriggers && d.Triggers != nil {
			add("project.discovery", "required when: "+names(*d.Triggers))
		} else {
			add("project.discovery", "the implementer decides; the Workflow Owner can require it")
		}
	}
	if pc.ReleaseConditions != nil && len(*pc.ReleaseConditions) > 0 {
		var n []string
		for _, c := range *pc.ReleaseConditions {
			n = append(n, c.Name)
		}
		add("project.release_conditions", strings.Join(n, ", "))
	} else {
		add("project.release_conditions", def("none"))
	}
	switch {
	case pc.SuggestionsBacklog == nil:
		add("project.suggestions_backlog", def(fmt.Sprintf("the issue named %q", defaultSuggestionsIssue)))
	case *pc.SuggestionsBacklog == backlogNone:
		add("project.suggestions_backlog", "kept in the review record that raised them")
	default:
		add("project.suggestions_backlog", "issue "+*pc.SuggestionsBacklog)
	}
	for _, c := range []struct {
		field string
		value *authority
	}{
		{"workflow_owner", a.WorkflowOwner},
		{"change_acceptor", a.ChangeAcceptor},
		{"product_intent_acceptor", a.ProductIntentAcceptor},
		{"small_correction_classifier", a.SmallCorrectionClassifier},
	} {
		if c.value != nil {
			add("project.authorities."+c.field, describeAuthority(*c.value))
		}
	}
	if ui := pc.UserFacingInspection; ui != nil && len(ui.AdditionalConditions) > 0 {
		add("project.user_facing_inspection", "the standard rule, plus: "+strings.Join(ui.AdditionalConditions, "; "))
	} else {
		add("project.user_facing_inspection", def("the standard rule"))
	}
	if gd := pc.Guidance; gd != nil && len(gd.Manuals) > 0 {
		var m []string
		for _, man := range gd.Manuals {
			m = append(m, man.Name+" "+man.Version)
		}
		add("project.guidance", strings.Join(m, ", "))
	} else {
		add("project.guidance", def("none"))
	}
	if cfg.Tooling != nil && cfg.Tooling.SkillTargets != nil && len(*cfg.Tooling.SkillTargets) > 0 {
		var t []string
		for _, target := range *cfg.Tooling.SkillTargets {
			t = append(t, harnessNames[target.Harness])
		}
		add("tooling.skill_targets", strings.Join(t, ", "))
	} else {
		add("tooling.skill_targets", "no agent tools")
	}
	return lines
}

func formatSummary(lines []summaryLine) string {
	width := 0
	for _, l := range lines {
		if len(l.label) > width {
			width = len(l.label)
		}
	}
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, l.label, l.value)
	}
	return b.String()
}

// describeAdded names the added settings in plain language, by their
// registry labels.
func describeAdded(paths [][]string) string {
	var names []string
	for _, p := range paths {
		names = append(names, labelInSentence(settingByPath(p).label))
	}
	return strings.Join(names, ", ")
}

// labelInSentence lowercases a label's first letter for use inside a
// sentence, except in "Workflow Owner", which is a proper name.
func labelInSentence(label string) string {
	if strings.HasPrefix(label, "Workflow Owner") {
		return label
	}
	return strings.ToLower(label[:1]) + label[1:]
}

// confirmWrite shows what will be written and asks for explicit confirmation.
// Only an explicit yes confirms; an empty answer declines.
func confirmWrite(p prompter, heading string, cfg projectConfig) (bool, error) {
	if err := p.Tell("\n" + heading + "\n\n" + formatSummary(describeConfiguration(cfg)) + "\n"); err != nil {
		return false, err
	}
	return guide{p: p}.yesNo("Write this configuration?", false)
}
