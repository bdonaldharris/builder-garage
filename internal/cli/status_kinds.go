package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Pull-request kinds and governing issues for "garage status" (#137).
//
// A pull request declares its governing issues in a "## Governing issue"
// section and its kind in a "## Pull request kind" section. Status reads the
// declared kind first and then checks it against the base and head the kind
// permits. A mismatch, or an unrecognized kind, never relaxes a gate.

const (
	kindImplementation = "implementation"
	kindPromotion      = "promotion"
	kindHotfix         = "hotfix"
	kindCleanSync      = "clean synchronization"
	kindConflictedSync = "conflicted synchronization"

	markerHotfixDesignation = "[WORKFLOW OWNER — HOTFIX DESIGNATION]"
)

var pullRequestKinds = []string{kindImplementation, kindPromotion, kindHotfix, kindCleanSync, kindConflictedSync}

var (
	bodyHeading   = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	htmlComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	governingRef  = regexp.MustCompile(`(?i)(?:^|[\s(\[])(?:(?:closes|closed|close|fixes|fixed|fix|resolves|resolved|resolve|refs)\s+)?#([0-9]+)\b`)
	repoQualified = regexp.MustCompile(`[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+#[0-9]+`)

	// The heads a declared promotion and a declared conflicted
	// synchronization permit: release/vX.Y.Z-rc.N and backmerge/vX.Y.Z, with
	// X.Y.Z written as a release tag is (no leading zeros), and N from 1.
	releaseCandidateHead = regexp.MustCompile(`^release/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-rc\.[1-9][0-9]*$`)
	backmergeHead        = regexp.MustCompile(`^backmerge/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

// bodySection returns the text of the "## <name>" section of a pull request
// body, up to the next heading of the same or a higher level, with HTML
// comments removed. It returns false when the body has no such section.
func bodySection(body, name string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	start := -1
	for i, line := range lines {
		m := bodyHeading.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		level := len(strings.TrimSpace(line)) - len(strings.TrimLeft(strings.TrimSpace(line), "#"))
		if start >= 0 && level <= 2 {
			return htmlComment.ReplaceAllString(strings.Join(lines[start:i], "\n"), ""), true
		}
		if start < 0 && level == 2 && strings.EqualFold(m[1], name) {
			start = i + 1
		}
	}
	if start < 0 {
		return "", false
	}
	return htmlComment.ReplaceAllString(strings.Join(lines[start:], "\n"), ""), true
}

// governingIssueNumbers returns the issues a pull request body names in its
// "## Governing issue" section, as #N, Closes #N, Fixes #N, Resolves #N, or
// Refs #N, in order and without duplicates. A reference qualified with
// another repository, such as owner/repo#N, is not a governing issue here.
func governingIssueNumbers(body string) []int {
	section, ok := bodySection(body, "Governing issue")
	if !ok {
		return nil
	}
	section = repoQualified.ReplaceAllString(section, "")
	var numbers []int
	seen := map[int]bool{}
	for _, m := range governingRef.FindAllStringSubmatch(section, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 || n > maxTargetNumber || seen[n] {
			continue
		}
		seen[n] = true
		numbers = append(numbers, n)
	}
	return numbers
}

// declaredKind returns the kind a pull request body declares in its
// "## Pull request kind" section: the first non-empty line, without list
// markers or backticks, lower-cased. It returns "" when there is no such
// section or it is empty.
func declaredKind(body string) string {
	section, ok := bodySection(body, "Pull request kind")
	if !ok {
		return ""
	}
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "-*+"))
		line = strings.Trim(line, "`")
		if line != "" {
			return strings.ToLower(strings.Join(strings.Fields(line), " "))
		}
	}
	return ""
}

// designationBlock is a hotfix-designation record's YAML block, as the
// extension-point registry defines it (E11).
type designationBlock struct {
	GarageDesignation int    `yaml:"garage_designation"`
	Designation       string `yaml:"designation"`
	Issue             int    `yaml:"issue"`
	ReleaseBranch     string `yaml:"release_branch"`
	Asserted          *struct {
		Authority string `yaml:"authority"`
	} `yaml:"asserted"`
}

// hotfixDesignation reports whether issue is hotfix-designated for
// releaseBranch by the latest valid designation record among its comments,
// with the URL of the record that governs, if any. A record counts only when
// its author's association counts, its block is well formed, and its issue
// and release branch match. No record, or a latest "withdrawn", means the
// issue is not designated.
func hotfixDesignation(comments []ghComment, issue int, releaseBranch string) (bool, string) {
	designated, url := false, ""
	for _, c := range comments {
		if !contains(countedAssociations, c.AuthorAssociation) {
			continue
		}
		body := strings.ReplaceAll(c.Body, "\r\n", "\n")
		first, rest, _ := strings.Cut(strings.TrimLeft(body, " \t\n"), "\n")
		if strings.TrimSpace(first) != markerHotfixDesignation {
			continue
		}
		raw, ok := firstYAMLBlock(rest)
		if !ok {
			continue
		}
		block, err := decodeDesignationBlock(raw)
		if err != nil || block.GarageDesignation != 1 || block.Issue != issue || block.ReleaseBranch != releaseBranch {
			continue
		}
		switch block.Designation {
		case "hotfix":
			designated, url = true, c.URL
		case "withdrawn":
			designated, url = false, c.URL
		}
	}
	return designated, url
}

func decodeDesignationBlock(raw string) (*designationBlock, error) {
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.KnownFields(true)
	var block designationBlock
	if err := decoder.Decode(&block); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("more than one YAML document")
	}
	return &block, nil
}

// prKind is how status classifies a pull request.
type prKind struct {
	kind        string // one of pullRequestKinds, or "" when none applies
	declared    bool   // the body declares it
	valid       bool   // the base and head are ones the kind permits
	stack       bool   // a registry-validated #192 stack or maintenance implementation
	maintenance bool
	problem     string // why a declared kind is not accepted
}

// relaxed reports whether implementation-review gates do not apply: only to
// a declared, topology-valid promotion or clean synchronization.
func (k prKind) relaxed() bool {
	return k.declared && k.valid && (k.kind == kindPromotion || k.kind == kindCleanSync)
}

// classifyPullRequest determines a pull request's kind from its declaration
// and its base and head. integration is the configured E1; release is the
// configured release branch (E11), or "" when none is configured. governing
// holds the governing issues that were read, for the hotfix designation.
func classifyPullRequest(it *ghItem, integration, release string, governing []ghClosingIssue) prKind {
	base, head := it.BaseRefName, it.HeadRefName
	declared := declaredKind(it.Body)
	if declared != "" {
		k := prKind{kind: declared, declared: true}
		if !contains(pullRequestKinds, declared) {
			k.kind = ""
			k.problem = fmt.Sprintf("declares %q, which is not a pull-request kind (%s)", declared, strings.Join(pullRequestKinds, ", "))
			return k
		}
		k.problem = topologyProblem(declared, base, head, integration, release)
		if k.problem == "" && declared == kindHotfix {
			k.problem = designationProblem(governing, release)
		}
		k.valid = k.problem == ""
		return k
	}
	if base != integration {
		return prKind{}
	}
	switch {
	case release != "" && head == release:
		return prKind{kind: kindCleanSync, valid: true}
	case strings.HasPrefix(head, "backmerge/"):
		return prKind{kind: kindConflictedSync, valid: true}
	default:
		return prKind{kind: kindImplementation, valid: true}
	}
}

// topologyProblem returns why base and head are not ones kind permits, or "".
func topologyProblem(kind, base, head, integration, release string) string {
	needRelease := func() string {
		if release == "" {
			return "a " + kind + " targets the release branch, and none is configured"
		}
		if base != release {
			return fmt.Sprintf("a %s targets the release branch %s, but the base is %s", kind, release, base)
		}
		return ""
	}
	switch kind {
	case kindImplementation:
		switch {
		case base != integration:
			return fmt.Sprintf("an implementation targets the integration branch %s, but the base is %s", integration, base)
		case release != "" && head == release, strings.HasPrefix(head, "backmerge/"), strings.HasPrefix(head, "release/"):
			return fmt.Sprintf("an implementation comes from an issue branch, but the head is %s", head)
		}
	case kindPromotion:
		if p := needRelease(); p != "" {
			return p
		}
		if !releaseCandidateHead.MatchString(head) {
			return fmt.Sprintf("a promotion comes from a release candidate branch, release/vX.Y.Z-rc.N, but the head is %s", head)
		}
	case kindHotfix:
		if p := needRelease(); p != "" {
			return p
		}
		if strings.HasPrefix(head, "release/") {
			return fmt.Sprintf("a hotfix comes from a hotfix branch, but the head is %s, a release candidate", head)
		}
	case kindCleanSync:
		switch {
		case base != integration:
			return fmt.Sprintf("a synchronization targets the integration branch %s, but the base is %s", integration, base)
		case release == "":
			return "a clean synchronization comes from the release branch, and none is configured"
		case head != release:
			return fmt.Sprintf("a clean synchronization comes from the release branch %s, but the head is %s", release, head)
		}
	case kindConflictedSync:
		switch {
		case base != integration:
			return fmt.Sprintf("a synchronization targets the integration branch %s, but the base is %s", integration, base)
		case !backmergeHead.MatchString(head):
			return fmt.Sprintf("a conflicted synchronization comes from a backmerge/vX.Y.Z branch, but the head is %s", head)
		}
	}
	return ""
}

// designationProblem returns why a hotfix's governing issues are not all
// hotfix-designated, or "".
func designationProblem(governing []ghClosingIssue, release string) string {
	if len(governing) == 0 {
		return "a hotfix needs a hotfix-designated governing issue, and none was read"
	}
	for _, issue := range governing {
		if ok, _ := hotfixDesignation(issue.Comments.Nodes, issue.Number, release); !ok {
			return fmt.Sprintf("governing issue #%d carries no hotfix designation for the release branch %s", issue.Number, release)
		}
	}
	return ""
}

// bodyIssuesToFetch returns the governing issues a pull request's
// "## Governing issue" section names that its closing references do not
// supply, for the follow-up read. The section is used when the base is not
// the default branch, where closing keywords create no link, or when the
// pull request closes no issue. Numbers beyond maxGoverningIssues are noted
// on the item and not fetched.
func bodyIssuesToFetch(it *ghItem) []int {
	if !it.isPullRequest() {
		return nil
	}
	closing := map[int]bool{}
	if it.ClosingIssuesReferences != nil {
		for _, issue := range it.ClosingIssuesReferences.Nodes {
			closing[issue.Number] = true
		}
	}
	onDefault := it.DefaultBranch != "" && it.BaseRefName == it.DefaultBranch
	if len(closing) > 0 && (onDefault || it.DefaultBranch == "") {
		return nil
	}
	var numbers []int
	for _, n := range governingIssueNumbers(it.Body) {
		if closing[n] {
			continue
		}
		if len(numbers) == maxGoverningIssues {
			it.BodyIssueNotes = append(it.BodyIssueNotes,
				fmt.Sprintf("the Governing issue section names more than %d issues; #%d and any later ones were not read", maxGoverningIssues, n))
			break
		}
		numbers = append(numbers, n)
	}
	return numbers
}
