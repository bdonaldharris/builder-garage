package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	stackProgramIssue            = 192
	stackPrerequisiteChangeIssue = 196
	stackRegistryMarker          = "[WORKFLOW OWNER — GITLAB STACK REGISTRY]"
	stackEntryCount              = 6
	authorizedStackHost          = "github.com"
	authorizedStackRepository    = "bdonaldharris/builder-garage"
)

var stackBranchPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
var stackMaintenanceBranchPattern = regexp.MustCompile(`^maintenance/stack-192/[A-Za-z0-9._/-]+$`)

type stackPrerequisite struct {
	Issue       int    `yaml:"issue"`
	PullRequest int    `yaml:"pull_request"`
	Branch      string `yaml:"branch"`
}

type stackRegistryEntry struct {
	Issue              int               `yaml:"issue"`
	Sequence           int               `yaml:"sequence"`
	Branch             string            `yaml:"branch"`
	DirectPrerequisite stackPrerequisite `yaml:"direct_prerequisite"`
}

type stackRegistry struct {
	GarageStack          int                  `yaml:"garage_stack"`
	ProgramIssue         int                  `yaml:"program_issue"`
	Repository           string               `yaml:"repository"`
	PrerequisiteChangePR int                  `yaml:"prerequisite_change_pr"`
	Entries              []stackRegistryEntry `yaml:"entries"`
}

type stackDependency struct {
	GarageStack             int    `yaml:"garage_stack"`
	ProgramIssue            int    `yaml:"program_issue"`
	Sequence                int    `yaml:"sequence"`
	DirectPrerequisiteIssue int    `yaml:"direct_prerequisite_issue"`
	DirectPrerequisitePR    int    `yaml:"direct_prerequisite_pr"`
	TargetBranch            string `yaml:"target_branch"`
}

type stackMaintenance struct {
	GarageStack       int    `yaml:"garage_stack"`
	ProgramIssue      int    `yaml:"program_issue"`
	EntryIssue        int    `yaml:"entry_issue"`
	Purpose           string `yaml:"purpose"`
	Upstream          string `yaml:"upstream"`
	UpstreamPR        int    `yaml:"upstream_pr"`
	TargetBranch      string `yaml:"target_branch"`
	CorrectionRecord  string `yaml:"correction_record,omitempty"`
	Agent2Disposition string `yaml:"agent2_disposition,omitempty"`
}

type stackRegistryRecord struct {
	Registry *stackRegistry
	URL      string
	Problem  string
}

type stackValidation struct {
	Present     bool
	Valid       bool
	Unknown     bool
	Maintenance bool
	RegistryURL string
	Entry       *stackRegistryEntry
	Problem     string
}

func parseStackDependency(body string) (*stackDependency, bool, error) {
	raw, present, err := stackSectionYAML(body, "Stack dependency")
	if err != nil || !present {
		return nil, present, err
	}
	var declaration stackDependency
	if err := decodeStrictYAML(raw, &declaration); err != nil {
		return nil, true, err
	}
	return &declaration, true, nil
}

func parseStackMaintenance(body string) (*stackMaintenance, bool, error) {
	raw, present, err := stackSectionYAML(body, "Stack maintenance")
	if err != nil || !present {
		return nil, present, err
	}
	var declaration stackMaintenance
	if err := decodeStrictYAML(raw, &declaration); err != nil {
		return nil, true, err
	}
	return &declaration, true, nil
}

func stackSectionYAML(body, name string) (string, bool, error) {
	section, present := bodySection(body, name)
	if !present {
		return "", false, nil
	}
	if strings.TrimSpace(section) == "" {
		return "", false, nil
	}
	raw, ok := firstYAMLBlock(section)
	if !ok {
		return "", true, fmt.Errorf("the Stack %s section has no fenced YAML block", strings.ToLower(name))
	}
	return raw, true, nil
}

func decodeStrictYAML(raw string, value any) error {
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.KnownFields(true)
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("more than one YAML document")
	}
	return nil
}

func latestStackRegistry(comments []ghComment) stackRegistryRecord {
	var latest *ghComment
	for i := range comments {
		comment := &comments[i]
		if !contains(countedAssociations, comment.AuthorAssociation) {
			continue
		}
		body := strings.ReplaceAll(comment.Body, "\r\n", "\n")
		first, _, _ := strings.Cut(strings.TrimLeft(body, " \t\n"), "\n")
		if strings.TrimSpace(first) == stackRegistryMarker {
			latest = comment
		}
	}
	if latest == nil {
		return stackRegistryRecord{Problem: "issue #192 has no counted Workflow Owner GitLab Stack Registry record"}
	}
	body := strings.ReplaceAll(latest.Body, "\r\n", "\n")
	_, rest, _ := strings.Cut(strings.TrimLeft(body, " \t\n"), "\n")
	raw, ok := firstYAMLBlock(rest)
	if !ok {
		return stackRegistryRecord{URL: latest.URL, Problem: "the latest GitLab Stack Registry record has no fenced YAML block"}
	}
	var registry stackRegistry
	if err := decodeStrictYAML(raw, &registry); err != nil {
		return stackRegistryRecord{URL: latest.URL, Problem: "the latest GitLab Stack Registry record is malformed: " + err.Error()}
	}
	return stackRegistryRecord{Registry: &registry, URL: latest.URL}
}

func validateStackRegistry(registry *stackRegistry, host, repository, integration, release string) error {
	if registry.GarageStack != 1 || registry.ProgramIssue != stackProgramIssue {
		return errors.New("the registry must declare garage_stack: 1 and program_issue: 192")
	}
	if host != authorizedStackHost {
		return fmt.Errorf("the #192 stack exception is not authorized on host %q", host)
	}
	if repository != authorizedStackRepository {
		return fmt.Errorf("the #192 stack exception is not authorized for repository %q", repository)
	}
	if registry.Repository == "" || registry.Repository != repository {
		return fmt.Errorf("the registry repository %q does not match %s", registry.Repository, repository)
	}
	if registry.PrerequisiteChangePR < 1 {
		return errors.New("the registry does not identify the merged prerequisite-change pull request")
	}
	if len(registry.Entries) != stackEntryCount {
		return fmt.Errorf("the registry must contain exactly %d feature entries", stackEntryCount)
	}
	byIssue := make(map[int]stackRegistryEntry, len(registry.Entries))
	branches := make(map[string]bool, len(registry.Entries))
	for i, entry := range registry.Entries {
		if entry.Issue < 1 || entry.Sequence != i+1 {
			return fmt.Errorf("registry entry %d must have a positive issue and sequence %d", i+1, i+1)
		}
		if _, exists := byIssue[entry.Issue]; exists {
			return fmt.Errorf("issue #%d appears more than once in the registry", entry.Issue)
		}
		if !validStackBranch(entry.Branch) || entry.Branch == integration || entry.Branch == release {
			return fmt.Errorf("registry entry #%d has an invalid or reserved branch %q", entry.Issue, entry.Branch)
		}
		if branches[entry.Branch] {
			return fmt.Errorf("branch %q appears more than once in the registry", entry.Branch)
		}
		if entry.DirectPrerequisite.Issue < 1 || entry.DirectPrerequisite.PullRequest < 1 ||
			!validStackBranch(entry.DirectPrerequisite.Branch) {
			return fmt.Errorf("registry entry #%d has an incomplete direct prerequisite", entry.Issue)
		}
		if i == 0 {
			if entry.DirectPrerequisite.Issue != stackPrerequisiteChangeIssue ||
				entry.DirectPrerequisite.PullRequest != registry.PrerequisiteChangePR ||
				entry.DirectPrerequisite.Branch != integration {
				return errors.New("the first entry must depend on issue #196's merged prerequisite-change PR into E1")
			}
		} else {
			previous := registry.Entries[i-1]
			if entry.DirectPrerequisite.Issue != previous.Issue || entry.DirectPrerequisite.Branch != previous.Branch {
				return fmt.Errorf("registry entry #%d must depend directly on entry #%d and its registered branch",
					entry.Issue, previous.Issue)
			}
		}
		branches[entry.Branch] = true
		byIssue[entry.Issue] = entry
	}
	return nil
}

func validStackBranch(branch string) bool {
	return branch != "" && stackBranchPattern.MatchString(branch) &&
		!strings.HasPrefix(branch, "/") && !strings.HasSuffix(branch, "/") &&
		!strings.Contains(branch, "//") && !strings.Contains(branch, "..") &&
		!strings.Contains(branch, "@{")
}

func statusReferencesToFetch(it *ghItem, host, repository, integration, release string) []int {
	numbers := append([]int(nil), bodyIssuesToFetch(it)...)
	numbers = append(numbers, stackPRReferencesToFetch(it, host, repository, integration, release)...)
	seen := make(map[int]bool, len(numbers))
	result := numbers[:0]
	for _, number := range numbers {
		if number < 1 || seen[number] {
			continue
		}
		seen[number] = true
		result = append(result, number)
	}
	return result
}

func stackPRReferencesToFetch(it *ghItem, host, repository, integration, release string) []int {
	if !it.isPullRequest() || host != authorizedStackHost || repository != authorizedStackRepository {
		return nil
	}
	registry, _, problem := stackRegistryForStatus(it, host, repository, integration, release)
	if problem != "" {
		return nil
	}
	entry, ok := stackEntryForStatus(it, registry)
	if !ok {
		return nil
	}
	numbers := []int{registry.PrerequisiteChangePR}
	for i := 0; i < entry.Sequence; i++ {
		numbers = append(numbers, registry.Entries[i].DirectPrerequisite.PullRequest)
	}
	return numbers
}

func validateStackDeclaration(it *ghItem, host, repository, integration, release string, governing []ghClosingIssue) stackValidation {
	_, dependencyPresent, dependencyErr := parseStackDependency(it.Body)
	maintenance, maintenancePresent, maintenanceErr := parseStackMaintenance(it.Body)
	result := stackValidation{Present: dependencyPresent || maintenancePresent, Maintenance: maintenancePresent}
	fail := func(problem string) stackValidation {
		result.Problem = problem
		return result
	}
	if dependencyPresent && maintenancePresent {
		return fail("a pull request cannot declare both Stack dependency and Stack maintenance")
	}
	if dependencyErr != nil {
		return fail(dependencyErr.Error())
	}
	if maintenanceErr != nil {
		return fail(maintenanceErr.Error())
	}
	authorizedRepository := host == authorizedStackHost && repository == authorizedStackRepository
	if result.Present && !authorizedRepository {
		return fail("the #192 stack exception is not authorized for this GitHub host and repository")
	}
	if !authorizedRepository {
		return result
	}
	if !it.StackRegistryFetched {
		if result.Present {
			return fail("the Workflow Owner registry on issue #192 was not read")
		}
		result.Unknown = true
		result.Problem = "the Workflow Owner registry on issue #192 could not be read"
		return result
	}
	record := latestStackRegistry(it.StackRegistryComments)
	result.RegistryURL = record.URL
	if record.Registry == nil {
		if result.Present {
			return fail(record.Problem)
		}
		result.Unknown = true
		result.Problem = record.Problem
		return result
	}
	registry := record.Registry
	if err := validateStackRegistry(registry, host, repository, integration, release); err != nil {
		if result.Present {
			return fail(err.Error())
		}
		result.Unknown = true
		result.Problem = err.Error()
		return result
	}
	entries := stackRegistryEntries(registry)
	entry, enrolled := stackEntryForStatus(it, registry)
	candidate := result.Present || enrolled
	if !candidate {
		return result
	}
	result.Present = true
	if !dependencyPresent && !maintenancePresent {
		return fail("a registered #192 feature or maintenance pull request must include its required stack declaration")
	}
	if it.isPullRequest() == false || declaredKind(it.Body) != kindImplementation {
		return fail("a stack feature or maintenance pull request must declare kind implementation")
	}
	if !objectNamePattern.MatchString(it.BaseRefOid) {
		return fail("the current target branch commit was not available from GitHub")
	}
	chain := newStackChain(it.StatusPRs, registry, integration)
	if dependencyPresent {
		issues := governingIssueNumbers(it.Body)
		if len(issues) != 1 {
			return fail("a stack feature pull request must name exactly one governing feature issue")
		}
		registeredEntry, ok := entries[issues[0]]
		if !ok {
			return fail(fmt.Sprintf("governing issue #%d is not enrolled in the #192 registry", issues[0]))
		}
		entry = registeredEntry
		// The subject is judged by the same feature rule as its predecessors.
		chain.features[entry.Sequence] = stackFactsFromItem(it)
		if problem := chain.problemThrough(entry.Sequence); problem != "" {
			return fail(problem)
		}
	} else {
		if maintenance.GarageStack != 1 || maintenance.ProgramIssue != stackProgramIssue {
			return fail("Stack maintenance must declare garage_stack: 1 and program_issue: 192")
		}
		var ok bool
		entry, ok = entries[maintenance.EntryIssue]
		if !ok {
			return fail(fmt.Sprintf("maintenance entry issue #%d is not enrolled in the #192 registry", maintenance.EntryIssue))
		}
		issues := governingIssueNumbers(it.Body)
		if len(issues) != 1 || issues[0] != entry.Issue {
			return fail(fmt.Sprintf("maintenance must govern only its registered issue #%d", entry.Issue))
		}
		if maintenance.TargetBranch != entry.Branch || it.BaseRefName != entry.Branch {
			return fail(fmt.Sprintf("maintenance target must be registered branch %s", entry.Branch))
		}
		if problem := chain.refOwnerProblem("the maintenance PR", "base", it.BaseRefName, it.BaseRepository.name()); problem != "" {
			return fail(problem)
		}
		if !validStackBranch(it.HeadRefName) || !stackMaintenanceBranchPattern.MatchString(it.HeadRefName) ||
			containsStackBranch(registry.Entries, it.HeadRefName) {
			return fail("maintenance must use a temporary maintenance/stack-192 branch")
		}
		propagation := false
		switch maintenance.Purpose {
		case "correction":
			if maintenance.Upstream != entry.Branch || maintenance.UpstreamPR != 0 {
				return fail("a correction maintenance PR must use its registered target as upstream and no upstream PR")
			}
			if problem := validateStackCorrection(governing, entry.Issue, maintenance.CorrectionRecord, maintenance.Agent2Disposition); problem != "" {
				return fail(problem)
			}
		case "synchronize":
			switch maintenance.Upstream {
			case integration:
				if maintenance.UpstreamPR != 0 {
					return fail("a staging synchronization must not name an upstream PR")
				}
			case entry.DirectPrerequisite.Branch:
				if maintenance.UpstreamPR != entry.DirectPrerequisite.PullRequest {
					return fail("dependent propagation must name the registered direct-prerequisite PR")
				}
				propagation = true
			default:
				return fail("synchronization upstream must be staging or the direct-prerequisite branch")
			}
			if maintenance.CorrectionRecord != "" || maintenance.Agent2Disposition != "" {
				return fail("a synchronization must not include correction evidence links")
			}
		default:
			return fail("Stack maintenance purpose must be correction or synchronize")
		}
		// Maintenance reuses feature-chain validation for every predecessor of
		// its entry, in addition to its own target and upstream requirements.
		if problem := chain.problemThrough(entry.Sequence - 1); problem != "" {
			return fail(problem)
		}
		if propagation {
			upstream := chain.features[entry.Sequence-1]
			if !upstream.Read || upstream.Number != maintenance.UpstreamPR || upstream.HeadRefName != maintenance.Upstream {
				return fail("the propagation upstream PR is not the registered direct prerequisite")
			}
			if problem := chain.refOwnerProblem("the propagation upstream PR", "head",
				upstream.HeadRefName, upstream.HeadRepository); problem != "" {
				return fail(problem)
			}
		}
	}
	result.Valid = true
	result.Entry = &entry
	return result
}

func stackRegistryEntries(registry *stackRegistry) map[int]stackRegistryEntry {
	entries := make(map[int]stackRegistryEntry, len(registry.Entries))
	for _, entry := range registry.Entries {
		entries[entry.Issue] = entry
	}
	return entries
}

func stackRegistryForStatus(it *ghItem, host, repository, integration, release string) (*stackRegistry, string, string) {
	if !it.StackRegistryFetched {
		return nil, "", "the Workflow Owner registry on issue #192 was not read"
	}
	record := latestStackRegistry(it.StackRegistryComments)
	if record.Registry == nil {
		return nil, record.URL, record.Problem
	}
	if err := validateStackRegistry(record.Registry, host, repository, integration, release); err != nil {
		return nil, record.URL, err.Error()
	}
	return record.Registry, record.URL, ""
}

func stackEntryForStatus(it *ghItem, registry *stackRegistry) (stackRegistryEntry, bool) {
	entries := stackRegistryEntries(registry)
	if maintenance, present, err := parseStackMaintenance(it.Body); present && err == nil && maintenance != nil {
		entry, ok := entries[maintenance.EntryIssue]
		if ok && hasOnlyGoverningIssue(it.Body, entry.Issue) {
			return entry, true
		}
	}
	for _, issue := range governingIssueNumbers(it.Body) {
		if entry, ok := entries[issue]; ok {
			return entry, true
		}
	}
	return stackRegistryEntry{}, false
}

// stackFeatureFacts is the single representation of a registered feature pull
// request that stack validation reads. The status subject and every
// predecessor are converted to it, so one rule judges both.
type stackFeatureFacts struct {
	Read        bool
	Number      int
	State       string
	Merged      bool
	MergedAt    *time.Time
	Body        string
	BaseRefName string
	HeadRefName string
	// The repositories that own the base and head refs. A branch name is
	// only the registered branch when the registry's repository owns it.
	BaseRepository string
	HeadRepository string
}

func stackFactsFromItem(it *ghItem) stackFeatureFacts {
	return stackFeatureFacts{
		Read: true, Number: it.Number, State: it.State, Merged: it.Merged, MergedAt: it.MergedAt,
		Body: it.Body, BaseRefName: it.BaseRefName, HeadRefName: it.HeadRefName,
		BaseRepository: it.BaseRepository.name(), HeadRepository: it.HeadRepository.name(),
	}
}

func stackFactsFromStatusPR(prs map[int]ghStatusPR, number int) stackFeatureFacts {
	pr, ok := prs[number]
	if !ok || pr.Number != number {
		return stackFeatureFacts{Number: number}
	}
	return stackFeatureFacts{
		Read: true, Number: pr.Number, State: pr.State, Merged: pr.Merged, MergedAt: pr.MergedAt,
		Body: pr.Body, BaseRefName: pr.BaseRefName, HeadRefName: pr.HeadRefName,
		BaseRepository: pr.BaseRepository, HeadRepository: pr.HeadRepository,
	}
}

// refOwnerProblem reports why a pull request's base or head ref is not
// established as a branch of the registry's repository. An owner GitHub did
// not return establishes nothing.
func (c stackChain) refOwnerProblem(label, role, branch, owner string) string {
	switch {
	case owner == "":
		return fmt.Sprintf("%s: GitHub did not return the repository that owns its %s branch %s", label, role, branch)
	case !strings.EqualFold(owner, c.registry.Repository):
		return fmt.Sprintf("%s: its %s branch %s belongs to %s, not %s", label, role, branch, owner, c.registry.Repository)
	}
	return ""
}

// mergeState reports whether the pull request is merged or open. Any other
// state, or a merged state without a merge time, is a problem: neither the
// current-state rule nor the merge-time rule can be applied to it.
func (f stackFeatureFacts) mergeState() (merged bool, problem string) {
	switch {
	case !f.Read:
		return false, "was not read from GitHub"
	case f.State == "OPEN" && !f.Merged && f.MergedAt == nil:
		return false, ""
	case f.State == "MERGED" && f.Merged && f.MergedAt != nil:
		return true, ""
	case f.State == "MERGED" || f.Merged:
		return false, "is merged, but GitHub returned no consistent merge time"
	default:
		return false, "is neither open nor merged"
	}
}

// stackChain holds the registry and the facts of the prerequisite-change PR
// and of each entry's feature PR, keyed by sequence.
//
// GitHub does not allow the base of a merged pull request to change, so a
// merged PR's base is its merge destination. Merge times order the merges.
type stackChain struct {
	registry     *stackRegistry
	integration  string
	prerequisite stackFeatureFacts
	features     map[int]stackFeatureFacts
}

// newStackChain reads the registered predecessor PRs. The registry names the
// feature PR of every entry except the last, as its dependent's prerequisite.
func newStackChain(prs map[int]ghStatusPR, registry *stackRegistry, integration string) stackChain {
	chain := stackChain{
		registry: registry, integration: integration,
		prerequisite: stackFactsFromStatusPR(prs, registry.PrerequisiteChangePR),
		features:     make(map[int]stackFeatureFacts, len(registry.Entries)),
	}
	for i := 1; i < len(registry.Entries); i++ {
		chain.features[i] = stackFactsFromStatusPR(prs, registry.Entries[i].DirectPrerequisite.PullRequest)
	}
	return chain
}

func (c stackChain) label(sequence int) string {
	entry := c.registry.Entries[sequence-1]
	if number := c.features[sequence].Number; number > 0 {
		return fmt.Sprintf("feature PR #%d for registered issue #%d", number, entry.Issue)
	}
	return fmt.Sprintf("the feature PR for registered issue #%d", entry.Issue)
}

// prerequisiteProblem checks the #196 prerequisite-change PR: merged into E1,
// governing only issue #196.
func (c stackChain) prerequisiteProblem() string {
	number := c.registry.PrerequisiteChangePR
	merged, problem := c.prerequisite.mergeState()
	switch {
	case problem != "":
		return fmt.Sprintf("prerequisite-change PR #%d %s", number, problem)
	case !merged:
		return fmt.Sprintf("prerequisite-change PR #%d is not merged", number)
	case c.prerequisite.BaseRefName != c.integration:
		return fmt.Sprintf("prerequisite-change PR #%d was merged into %s, not E1", number, c.prerequisite.BaseRefName)
	case !hasOnlyGoverningIssue(c.prerequisite.Body, stackPrerequisiteChangeIssue):
		return fmt.Sprintf("prerequisite-change PR #%d must govern only issue #196", number)
	}
	return c.refOwnerProblem(fmt.Sprintf("prerequisite-change PR #%d", number), "base",
		c.prerequisite.BaseRefName, c.prerequisite.BaseRepository)
}

// permittedTarget returns the target the registry permits for the feature PR
// of sequence: now when at is nil, otherwise at that merge time. It is where
// the direct prerequisite's change is at that moment: the prerequisite's own
// branch while its PR is unmerged, and afterward the branch its change was
// merged into, followed through any later merge of that branch. It does not
// judge whether the chain is valid; problemThrough does.
func (c stackChain) permittedTarget(sequence int, at *time.Time) (string, string) {
	if problem := c.prerequisiteProblem(); problem != "" {
		return "", problem
	}
	if at != nil && !c.prerequisite.MergedAt.Before(*at) {
		return "", fmt.Sprintf("%s was not merged after prerequisite-change PR #%d, or their merge times cannot be ordered",
			c.label(sequence), c.registry.PrerequisiteChangePR)
	}
	for holder := sequence - 1; holder >= 1; {
		facts := c.features[holder]
		merged, problem := facts.mergeState()
		switch {
		case problem != "":
			return "", fmt.Sprintf("%s %s", c.label(holder), problem)
		case !merged || (at != nil && at.Before(*facts.MergedAt)):
			return c.registry.Entries[holder-1].Branch, ""
		case at != nil && at.Equal(*facts.MergedAt):
			return "", fmt.Sprintf("the merge times of %s and %s are equal, so their order cannot be established",
				c.label(holder), c.label(sequence))
		}
		// The holder's change had merged by then. Follow it into an earlier
		// registered branch; any other destination is returned as it is.
		next := 0
		for i := 0; i < holder-1; i++ {
			if c.registry.Entries[i].Branch == facts.BaseRefName {
				next = i + 1
			}
		}
		if next == 0 {
			return facts.BaseRefName, ""
		}
		holder = next
	}
	return c.integration, ""
}

// featureProblem is the one feature-validation rule. It applies to the
// feature PR of sequence wherever that PR is consumed: as the status subject,
// as a predecessor, or as a maintenance upstream.
//
// An open PR is judged by current state: its base and declared target must be
// the target permitted now. A merged PR is judged by merge-time state: its
// merge destination and declared target must be the target permitted when it
// merged. The declaration read is the PR's current body; GitHub does not show
// what the body said at merge time.
func (c stackChain) featureProblem(sequence int) string {
	entry := c.registry.Entries[sequence-1]
	facts := c.features[sequence]
	label := c.label(sequence)
	merged, problem := facts.mergeState()
	if problem != "" {
		return fmt.Sprintf("%s %s", label, problem)
	}
	if facts.HeadRefName != entry.Branch {
		return fmt.Sprintf("%s must have registered head branch %s", label, entry.Branch)
	}
	// The registered branch is that name in the registry's repository. The
	// same name in a fork is a different branch.
	if problem := c.refOwnerProblem(label, "head", facts.HeadRefName, facts.HeadRepository); problem != "" {
		return problem
	}
	if problem := c.refOwnerProblem(label, "base", facts.BaseRefName, facts.BaseRepository); problem != "" {
		return problem
	}
	if !hasOnlyGoverningIssue(facts.Body, entry.Issue) {
		return fmt.Sprintf("%s must name exactly its registered governing issue", label)
	}
	if declaredKind(facts.Body) != kindImplementation {
		return fmt.Sprintf("%s must declare kind implementation", label)
	}
	dependency, present, err := parseStackDependency(facts.Body)
	_, maintenancePresent, maintenanceErr := parseStackMaintenance(facts.Body)
	if err != nil || maintenanceErr != nil || !present || maintenancePresent || dependency == nil {
		return fmt.Sprintf("%s must include exactly its required Stack dependency declaration", label)
	}
	if dependency.GarageStack != 1 || dependency.ProgramIssue != stackProgramIssue {
		return fmt.Sprintf("%s: Stack dependency must declare garage_stack: 1 and program_issue: 192", label)
	}
	if dependency.Sequence != entry.Sequence ||
		dependency.DirectPrerequisiteIssue != entry.DirectPrerequisite.Issue ||
		dependency.DirectPrerequisitePR != entry.DirectPrerequisite.PullRequest {
		return fmt.Sprintf("%s: Stack dependency does not match the governing registry entry", label)
	}
	var at *time.Time
	if merged {
		at = facts.MergedAt
	}
	permitted, problem := c.permittedTarget(sequence, at)
	if problem != "" {
		return problem
	}
	if facts.BaseRefName != permitted {
		if merged {
			return fmt.Sprintf("%s merged into %s; the target permitted at its merge time was %s",
				label, facts.BaseRefName, permitted)
		}
		return fmt.Sprintf("%s targets %s; the current permitted target is %s", label, facts.BaseRefName, permitted)
	}
	if dependency.TargetBranch != permitted {
		return fmt.Sprintf("%s declares target_branch %s; its validated target is %s",
			label, dependency.TargetBranch, permitted)
	}
	return ""
}

// problemThrough reports whether the chain is valid from the prerequisite-
// change PR through the feature PR of sequence. Sequence 0 checks only the
// prerequisite-change PR.
func (c stackChain) problemThrough(sequence int) string {
	if problem := c.prerequisiteProblem(); problem != "" {
		return problem
	}
	for i := 1; i <= sequence; i++ {
		if problem := c.featureProblem(i); problem != "" {
			return problem
		}
	}
	return ""
}

func containsStackBranch(entries []stackRegistryEntry, branch string) bool {
	for _, entry := range entries {
		if entry.Branch == branch {
			return true
		}
	}
	return false
}

func hasOnlyGoverningIssue(body string, issue int) bool {
	numbers := governingIssueNumbers(body)
	return len(numbers) == 1 && numbers[0] == issue
}

func validateStackCorrection(governing []ghClosingIssue, issueNumber int, correctionURL, dispositionURL string) string {
	if correctionURL == "" || dispositionURL == "" {
		return "a correction maintenance PR must link its issue correction record and Agent #2 disposition"
	}
	for _, issue := range governing {
		if issue.Number != issueNumber {
			continue
		}
		records := parseWorkflowRecords(issue.Comments.Nodes, fmt.Sprintf("issue #%d", issue.Number))
		var correction *workflowRecord
		var disposition *workflowRecord
		for i := range records {
			record := &records[i]
			if !record.Counted {
				continue
			}
			if record.Comment.URL == correctionURL && record.isEvidence("correction") {
				correction = record
			}
			if record.Comment.URL == dispositionURL && record.isEvidence("review") &&
				record.Block.verdict() == "approved" && record.Block.RespondsTo == correctionURL {
				disposition = record
			}
		}
		if correction != nil && disposition != nil &&
			correction.Block.Issue == issueNumber && disposition.Block.Issue == issueNumber &&
			disposition.Block.artifact().Changeset == correction.Block.artifact().Changeset {
			latest := latestEvidence(records, "correction")
			if latest == nil || latest.Comment.URL != correctionURL {
				return "the correction link is not the latest issue correction record"
			}
			latestReview := latestEvidence(records, "review")
			if latestReview == nil || latestReview.Comment.URL != dispositionURL {
				return "the linked Agent #2 disposition is not the latest issue review"
			}
			return ""
		}
	}
	return "the linked correction record and approving Agent #2 disposition were not found on the governing issue"
}
