package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGoverningIssueNumbers(t *testing.T) {
	for name, c := range map[string]struct {
		body string
		want []int
	}{
		"every accepted form": {"## Governing issue\n\n#1, Closes #2, fixes #3\nResolves #4 and Refs #5 (also #6)\n", []int{1, 2, 3, 4, 5, 6}},
		"duplicates once":     {"## Governing issue\n\nCloses #7\nRefs #7\n", []int{7}},
		"only inside the section": {
			"Closes #1\n\n## Governing issue\n\nCloses #2\n\n## Summary\n\nCloses #3\n", []int{2}},
		"another repository": {"## Governing issue\n\nCloses other/repo#9 and #10\n", []int{10}},
		"malformed":          {"## Governing issue\n\n#abc #0 #12x Closes # and a#13\n", nil},
		"HTML comments":      {"## Governing issue\n\n<!--\nCloses #4\n-->\nCloses #5\n", []int{5}},
		"small correction":   {"## Governing issue\n\nNone - small correction\n", nil},
		"no section":         {"## Summary\n\nCloses #1\n", nil},
		"a subsection is part of it": {
			"## Governing issue\n\n### Primary\n\nRefs #8\n\n## Summary\n", []int{8}},
		"the PR template": {"## Governing issue\n\nCloses #\n\n<!--\nSubstantive changes require an issue. Closes #99\n-->\n", nil},
	} {
		t.Run(name, func(t *testing.T) {
			if got := governingIssueNumbers(c.body); !slices.Equal(got, c.want) {
				t.Errorf("governingIssueNumbers = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDeclaredKind(t *testing.T) {
	for body, want := range map[string]string{
		"## Pull request kind\n\npromotion\n":                      "promotion",
		"## Pull request kind\n\n- `Clean  Synchronization`\n":     "clean synchronization",
		"## Pull request kind\n\n<!-- one of ... -->\nhotfix\n":    "hotfix",
		"## Pull request kind\n\n\n## Summary\n\nimplementation\n": "",
		"## Summary\n\npromotion\n":                                "",
		"## Pull request kind\n\nsomething else\n":                 "something else",
	} {
		if got := declaredKind(body); got != want {
			t.Errorf("declaredKind(%q) = %q, want %q", body, got, want)
		}
	}
}

func designationComment(issue int, designation, release, association string, at int) workflowComment {
	return workflowComment{
		URL: fmt.Sprintf("https://example.invalid/c%d", at),
		Body: fmt.Sprintf("%s\n\n```yaml\ngarage_designation: 1\ndesignation: %s\nissue: %d\nrelease_branch: %s\nasserted:\n  authority: workflow_owner\n```\n\nReason: a released defect\n",
			markerHotfixDesignation, designation, issue, release),
		CreatedAt:         time.Date(2026, 11, 18, 0, at, 0, 0, time.UTC),
		AuthorAssociation: association,
	}
}

func TestHotfixDesignation(t *testing.T) {
	for name, c := range map[string]struct {
		comments []workflowComment
		want     bool
	}{
		"designated":             {[]workflowComment{designationComment(5, "hotfix", "main", "OWNER", 1)}, true},
		"withdrawn later":        {[]workflowComment{designationComment(5, "hotfix", "main", "OWNER", 1), designationComment(5, "withdrawn", "main", "OWNER", 2)}, false},
		"designated again":       {[]workflowComment{designationComment(5, "withdrawn", "main", "OWNER", 1), designationComment(5, "hotfix", "main", "OWNER", 2)}, true},
		"another issue":          {[]workflowComment{designationComment(6, "hotfix", "main", "OWNER", 1)}, false},
		"another release branch": {[]workflowComment{designationComment(5, "hotfix", "production", "OWNER", 1)}, false},
		"unassociated author":    {[]workflowComment{designationComment(5, "hotfix", "main", "NONE", 1)}, false},
		"no record":              {[]workflowComment{{Body: "hotfix please", AuthorAssociation: "OWNER"}}, false},
		"malformed block": {[]workflowComment{{Body: markerHotfixDesignation + "\n\n```yaml\ngarage_designation: 1\nunknown: x\n```\n",
			AuthorAssociation: "OWNER"}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got, _ := hotfixDesignation(c.comments, 5, "main"); got != c.want {
				t.Errorf("designated = %v, want %v", got, c.want)
			}
		})
	}
}

func TestClassifyPullRequest(t *testing.T) {
	designated := []workflowIssue{{Number: 5, Comments: workflowComments{Nodes: []workflowComment{designationComment(5, "hotfix", "main", "OWNER", 1)}}}}
	undesignated := []workflowIssue{{Number: 5}}
	pr := func(base, head, kind string) *workflowItem {
		body := "## Summary\n\nx\n"
		if kind != "" {
			body = "## Pull request kind\n\n" + kind + "\n\n" + body
		}
		return &workflowItem{Kind: workflowChangeKind, BaseRefName: base, HeadRefName: head, Body: body}
	}
	type want struct {
		kind              string
		declared, valid   bool
		relaxed           bool
		problemContaining string
	}
	for name, c := range map[string]struct {
		item      *workflowItem
		release   string
		governing []workflowIssue
		want      want
	}{
		// Declared and valid.
		"implementation":             {pr("staging", "feat/1-x", "implementation"), "main", nil, want{kindImplementation, true, true, false, ""}},
		"promotion":                  {pr("main", "release/v1.0.0-rc.1", "promotion"), "main", nil, want{kindPromotion, true, true, true, ""}},
		"hotfix":                     {pr("main", "fix/5-x", "hotfix"), "main", designated, want{kindHotfix, true, true, false, ""}},
		"clean synchronization":      {pr("staging", "main", "clean synchronization"), "main", nil, want{kindCleanSync, true, true, true, ""}},
		"conflicted synchronization": {pr("staging", "backmerge/v1.0.1", "conflicted synchronization"), "main", nil, want{kindConflictedSync, true, true, false, ""}},
		// Declared but mismatched.
		"implementation into the release branch":          {pr("main", "feat/1-x", "implementation"), "main", nil, want{kindImplementation, true, false, false, "the base is main"}},
		"implementation from a backmerge branch":          {pr("staging", "backmerge/v1", "implementation"), "main", nil, want{kindImplementation, true, false, false, "the head is backmerge/v1"}},
		"promotion into staging":                          {pr("staging", "release/v1.0.0-rc.1", "promotion"), "main", nil, want{kindPromotion, true, false, false, "the base is staging"}},
		"promotion from a feature branch":                 {pr("main", "feat/1-x", "promotion"), "main", nil, want{kindPromotion, true, false, false, "release/vX.Y.Z-rc.N"}},
		"promotion from release/notes":                    {pr("main", "release/notes", "promotion"), "main", nil, want{kindPromotion, true, false, false, "the head is release/notes"}},
		"promotion from a two-part version":               {pr("main", "release/v1.0", "promotion"), "main", nil, want{kindPromotion, true, false, false, "the head is release/v1.0"}},
		"promotion without an rc number":                  {pr("main", "release/v1.0.0", "promotion"), "main", nil, want{kindPromotion, true, false, false, "the head is release/v1.0.0"}},
		"promotion from rc.0":                             {pr("main", "release/v1.0.0-rc.0", "promotion"), "main", nil, want{kindPromotion, true, false, false, "release/vX.Y.Z-rc.N"}},
		"promotion with a leading zero":                   {pr("main", "release/v1.01.0-rc.1", "promotion"), "main", nil, want{kindPromotion, true, false, false, "release/vX.Y.Z-rc.N"}},
		"promotion with a suffix":                         {pr("main", "release/v1.0.0-rc.1x", "promotion"), "main", nil, want{kindPromotion, true, false, false, "release/vX.Y.Z-rc.N"}},
		"promotion from rc.12":                            {pr("main", "release/v10.2.0-rc.12", "promotion"), "main", nil, want{kindPromotion, true, true, true, ""}},
		"promotion without a release branch":              {pr("main", "release/v1.0.0-rc.1", "promotion"), "", nil, want{kindPromotion, true, false, false, "none is configured"}},
		"hotfix into staging":                             {pr("staging", "fix/5-x", "hotfix"), "main", designated, want{kindHotfix, true, false, false, "the base is staging"}},
		"hotfix without a release branch":                 {pr("main", "fix/5-x", "hotfix"), "", designated, want{kindHotfix, true, false, false, "none is configured"}},
		"hotfix without a designation":                    {pr("main", "fix/5-x", "hotfix"), "main", undesignated, want{kindHotfix, true, false, false, "no hotfix designation"}},
		"hotfix without a governing issue":                {pr("main", "fix/5-x", "hotfix"), "main", nil, want{kindHotfix, true, false, false, "none was read"}},
		"hotfix from a release candidate":                 {pr("main", "release/v1.0.0-rc.1", "hotfix"), "main", designated, want{kindHotfix, true, false, false, "release candidate"}},
		"clean synchronization from a feature branch":     {pr("staging", "feat/1", "clean synchronization"), "main", nil, want{kindCleanSync, true, false, false, "the head is feat/1"}},
		"clean synchronization into main":                 {pr("main", "main", "clean synchronization"), "main", nil, want{kindCleanSync, true, false, false, "the base is main"}},
		"conflicted synchronization from main":            {pr("staging", "main", "conflicted synchronization"), "main", nil, want{kindConflictedSync, true, false, false, "backmerge/vX.Y.Z"}},
		"conflicted synchronization from backmerge/notes": {pr("staging", "backmerge/notes", "conflicted synchronization"), "main", nil, want{kindConflictedSync, true, false, false, "the head is backmerge/notes"}},
		"conflicted synchronization without a patch":      {pr("staging", "backmerge/v1.0", "conflicted synchronization"), "main", nil, want{kindConflictedSync, true, false, false, "the head is backmerge/v1.0"}},
		"an unrecognized kind":                            {pr("main", "release/v1", "release"), "main", nil, want{"", true, false, false, "not a pull-request kind"}},
		// Undeclared.
		"undeclared from the release branch":     {pr("staging", "main", ""), "main", nil, want{kindCleanSync, false, true, false, ""}},
		"undeclared from a backmerge branch":     {pr("staging", "backmerge/v1", ""), "main", nil, want{kindConflictedSync, false, true, false, ""}},
		"undeclared from any backmerge/ branch":  {pr("staging", "backmerge/notes", ""), "main", nil, want{kindConflictedSync, false, true, false, ""}},
		"undeclared from an issue branch":        {pr("staging", "feat/1", ""), "main", nil, want{kindImplementation, false, true, false, ""}},
		"undeclared into main is not a hotfix":   {pr("main", "fix/5-x", ""), "main", designated, want{"", false, false, false, ""}},
		"undeclared from a release candidate":    {pr("main", "release/v1.0.0-rc.1", ""), "main", nil, want{"", false, false, false, ""}},
		"undeclared from main, no release":       {pr("staging", "main", ""), "", nil, want{kindImplementation, false, true, false, ""}},
		"undeclared clean synchronization relax": {pr("staging", "main", ""), "main", nil, want{kindCleanSync, false, true, false, ""}},
	} {
		t.Run(name, func(t *testing.T) {
			got := classifyPullRequest(c.item, "staging", c.release, c.governing)
			if got.kind != c.want.kind || got.declared != c.want.declared || got.valid != c.want.valid || got.relaxed() != c.want.relaxed {
				t.Errorf("got %+v (relaxed %v), want %+v", got, got.relaxed(), c.want)
			}
			if !strings.Contains(got.problem, c.want.problemContaining) || (c.want.problemContaining == "" && got.problem != "") {
				t.Errorf("problem = %q, want one containing %q", got.problem, c.want.problemContaining)
			}
		})
	}
}

// stagingConfig is Builder Garage's configuration after the cutover:
// integration branch staging, release branch main.
func stagingConfig() *projectConfig {
	cfg := testConfig()
	staging, main := "staging", "main"
	cfg.Project.IntegrationBranch = &staging
	cfg.Project.ReleaseBranch = &main
	return cfg
}

func renderItem(t *testing.T, item *workflowItem, cfg *projectConfig, reads int) string {
	t.Helper()
	var out bytes.Buffer
	if err := buildStatusReport(statusInputs{Item: item, Config: cfg, Recompute: notLocal,
		ReadAt: time.Date(2026, 11, 13, 13, 0, 0, 0, time.UTC), Reads: reads}).render(&out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func section(text, heading string) string {
	_, rest, ok := strings.Cut(text, "\n"+heading)
	if !ok {
		return ""
	}
	body, _, _ := strings.Cut(rest, "\n\n")
	return body
}

// An implementation pull request into staging that names its issue only in
// the Governing issue section evaluates the same gates as one into the
// default branch that closes it.
func TestStatusGovernsFromTheGoverningIssueSection(t *testing.T) {
	closing := loadFixtureItem(t, fixtureBytes(t, "pr46"))
	want := section(renderItem(t, closing, testConfig(), 1), "Gates for the current head")

	body := loadFixtureItem(t, fixtureBytes(t, "pr46"))
	body.DefaultBranch = "main"
	body.BaseRefName = "staging"
	body.HeadRefName = "feat/33-config-aware"
	body.BodyIssues = body.ClosingIssues.Nodes
	body.ClosingIssues = nil
	text := renderItem(t, body, stagingConfig(), 2)
	if got := section(text, "Gates for the current head"); got != want {
		t.Errorf("gates differ.\nfrom the Governing issue section:\n%s\nfrom a closing reference:\n%s", got, want)
	}
	assertContains(t, text,
		"GitHub was read twice",
		"governing issue #33 is named in the Governing issue section; its records were read by a follow-up read",
		"Workflow records on governing issue #33",
		"pull-request kind: implementation, inferred",
		"the pull request targets the configured integration branch")
	assertNotContains(t, text, "closes no issue")
}

func TestStatusNamesTheKindWithoutChangingTodaysReport(t *testing.T) {
	text := reportText(t, fixtureBytes(t, "pr46"), testConfig())
	assertContains(t, text, "GitHub was read once",
		"pull-request kind: implementation, inferred, because no Pull request kind section declares one",
		"the pull request targets the configured integration branch")
}

func TestStatusRelaxesOnlyADeclaredValidPromotion(t *testing.T) {
	item := loadFixtureItem(t, fixtureBytes(t, "pr46"))
	item.BaseRefName, item.HeadRefName = "main", "release/v1.0.0-rc.1"
	item.Body = "## Pull request kind\n\npromotion\n\n" + item.Body
	text := renderItem(t, item, stagingConfig(), 1)
	assertContains(t, text,
		"pull-request kind: promotion, declared, with a base and head a promotion permits",
		"implementation-review gates do not apply to a promotion",
		"governing issues: #33",
		"checks on the head: success",
		"the pull request targets main, the configured release branch, as a declared promotion")
	assertNotContains(t, text, "NO approving", "trigger 1", "NOT the configured integration branch")

	// A declared promotion from a head that is not a release candidate: a
	// mismatch, evaluated with every implementation gate.
	item.HeadRefName = "release/notes"
	text = renderItem(t, item, stagingConfig(), 1)
	assertContains(t, text, "pull-request kind: declared promotion, MISMATCH: a promotion comes from a release candidate branch, release/vX.Y.Z-rc.N, but the head is release/notes",
		"trigger 1 (equivalence not shown)", "approving issue-recorded implementation review",
		"the pull request targets main, NOT the configured integration branch staging")
	assertNotContains(t, text, "do not apply to a")

	// The same pull request, declared as a hotfix into staging: a mismatch,
	// evaluated with every implementation gate.
	item.BaseRefName, item.HeadRefName = "staging", "fix/33"
	item.Body = strings.Replace(item.Body, "promotion", "hotfix", 1)
	text = renderItem(t, item, stagingConfig(), 1)
	assertContains(t, text, "pull-request kind: declared hotfix, MISMATCH: a hotfix targets the release branch main, but the base is staging",
		"trigger 1 (equivalence not shown)", "approving issue-recorded implementation review")
	assertNotContains(t, text, "do not apply to a")

	// Undeclared into main: never a hotfix or a promotion.
	item.BaseRefName, item.HeadRefName = "main", "fix/33"
	item.Body = strings.Replace(item.Body, "## Pull request kind\n\nhotfix\n\n", "", 1)
	text = renderItem(t, item, stagingConfig(), 1)
	assertContains(t, text, "pull-request kind: none", "the pull request targets main, NOT the configured integration branch staging",
		"trigger 1 (equivalence not shown)")
}

func TestStatusReportsTruncatedClosingReferences(t *testing.T) {
	item := loadFixtureItem(t, fixtureBytes(t, "pr46"))
	item.ClosingIssues.TotalCount = 7
	text := renderItem(t, item, testConfig(), 1)
	assertContains(t, text, "the pull request closes 7 issues; only the first 1 were read")
}

func TestStatusIssueFindsPullRequestsNamingItInTheirGoverningSection(t *testing.T) {
	item := loadFixtureItem(t, fixtureBytes(t, "is33"))
	item.State = "OPEN"
	item.ClosedByChanges = nil
	ref := func(number int, cross bool, body string) workflowReference {
		r := workflowReference{IsCrossRepository: cross}
		r.Source = &workflowReferenceSource{Kind: workflowChangeKind, Number: number,
			URL: fmt.Sprintf("https://example.invalid/pull/%d", number), State: "OPEN", Body: body}
		return r
	}
	item.References = &workflowReferences{TotalCount: 60, Nodes: []workflowReference{
		ref(70, false, "## Summary\n\nSee #33\n"),          // a mention outside the section
		ref(71, true, "## Governing issue\n\nRefs #33\n"),  // another repository
		ref(72, false, "## Governing issue\n\nRefs #33\n"), // this one
		ref(73, false, "## Governing issue\n\nRefs #34\n"), // another issue
		ref(72, false, "## Governing issue\n\nRefs #33\n"), // listed once
	}}
	text := renderItem(t, item, stagingConfig(), 1)
	assertContains(t, text,
		"pull request #72 names this issue in its Governing issue section and is open",
		"only the latest 5 of 60 cross-references to this issue were read",
		"the pull-request sequence governs from here; run \"garage status 72\"")
	assertNotContains(t, text, "#70 names", "#71 names", "#73 names")
	if strings.Count(text, "#72 names") != 1 {
		t.Error("pull request #72 is listed more than once")
	}
}

func TestBodyIssuesToFetch(t *testing.T) {
	closes := func(numbers ...int) *workflowIssues {
		c := &workflowIssues{TotalCount: len(numbers)}
		for _, n := range numbers {
			c.Nodes = append(c.Nodes, workflowIssue{Number: n})
		}
		return c
	}
	body := "## Governing issue\n\nCloses #1 Refs #2\n"
	for name, c := range map[string]struct {
		item *workflowItem
		want []int
	}{
		"into the default branch, closing":         {&workflowItem{Kind: workflowChangeKind, Body: body, BaseRefName: "main", DefaultBranch: "main", ClosingIssues: closes(1)}, nil},
		"into the default branch, closing nothing": {&workflowItem{Kind: workflowChangeKind, Body: body, BaseRefName: "main", DefaultBranch: "main"}, []int{1, 2}},
		"into staging":                      {&workflowItem{Kind: workflowChangeKind, Body: body, BaseRefName: "staging", DefaultBranch: "main"}, []int{1, 2}},
		"into staging, one already closing": {&workflowItem{Kind: workflowChangeKind, Body: body, BaseRefName: "staging", DefaultBranch: "main", ClosingIssues: closes(1)}, []int{2}},
		"an issue":                          {&workflowItem{Kind: workflowIssueKind, Body: body}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			if got := bodyIssuesToFetch(c.item); !slices.Equal(got, c.want) {
				t.Errorf("bodyIssuesToFetch = %v, want %v", got, c.want)
			}
		})
	}
	many := &workflowItem{Kind: workflowChangeKind, BaseRefName: "staging", DefaultBranch: "main",
		Body: "## Governing issue\n\n#1 #2 #3 #4 #5 #6 #7\n"}
	if got := bodyIssuesToFetch(many); len(got) != maxGoverningIssues || len(many.BodyIssueNotes) != 1 ||
		!strings.Contains(many.BodyIssueNotes[0], "#6 and any later ones were not read") {
		t.Errorf("bodyIssuesToFetch = %v, notes %q", got, many.BodyIssueNotes)
	}
}

// runStatus uses one bounded follow-up read for governing issues; stack
// registry reads are limited to the authorized repository.
func TestRunStatusMakesOneFollowUpReadForGoverningAndStackReferences(t *testing.T) {
	pr46 := fixtureBytes(t, "pr46") // before newGitRepo changes the working directory
	dir := newGitRepo(t, "https://github.com/o/r.git")
	isolateUserCache(t)

	var doc map[string]any
	if err := json.Unmarshal(pr46, &doc); err != nil {
		t.Fatal(err)
	}
	repo := doc["data"].(map[string]any)["repository"].(map[string]any)
	repo["defaultBranchRef"] = map[string]any{"name": "main"}
	pr := repo["issueOrPullRequest"].(map[string]any)
	issue33 := pr["closingIssuesReferences"].(map[string]any)["nodes"].([]any)[0].(map[string]any)
	pr["closingIssuesReferences"] = map[string]any{"totalCount": 0, "nodes": []any{}}
	pr["baseRefName"] = "staging"
	pr["body"] = "## Governing issue\n\nRefs #33 and #900\n\n" + pr["body"].(string)
	first, _ := json.Marshal(doc)
	issue33["__typename"] = "Issue"
	second, _ := json.Marshal(map[string]any{
		"data":   map[string]any{"repository": map[string]any{"i0": issue33, "i1": nil}},
		"errors": []any{map[string]any{"type": "NOT_FOUND", "message": "Could not resolve to an issue or pull request with the number of 900."}},
	})

	var calls [][]string
	gh := func(_ context.Context, args ...string) ([]byte, []byte, error) {
		if err := checkReadOnlyGHArgs(args); err != nil {
			t.Errorf("a request that is not read-only reached gh: %v", args)
		}
		calls = append(calls, args)
		if len(calls) == 1 {
			return first, nil, nil
		}
		return second, nil, nil
	}
	var out bytes.Buffer
	if err := runStatus(context.Background(), 46, statusTestEnv(t, dir, gh), &out); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("gh was called %d times, want 2", len(calls))
	}
	if second := strings.Join(calls[1], " "); !strings.Contains(second, "n0=33") || !strings.Contains(second, "n1=900") {
		t.Errorf("the follow-up read does not ask for #33 and #900: %v", calls[1])
	}
	assertContains(t, out.String(), "GitHub was read twice",
		"governing issue #33 is named in the Governing issue section",
		"#900 names no issue in this repository",
		"approving issue-recorded implementation review: https://github.com/bdonaldharris/builder-garage/issues/33#issuecomment-6852033450")

	// An unrelated repository does not incur a stack-registry read.
	calls = nil
	out.Reset()
	if err := runStatus(context.Background(), 46, statusTestEnv(t, dir, fixtureGH(t, pr46)), &out); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
	assertContains(t, out.String(), "GitHub was read once")
}

func TestGoverningIssuesQueryIsReadOnly(t *testing.T) {
	args := []string{"api", "graphql", "-f", "query=" + governingIssuesQuery(3), "-F", "n0=1"}
	if err := checkReadOnlyGHArgs(args); err != nil {
		t.Errorf("the follow-up query is refused: %v", err)
	}
	if q := governingIssuesQuery(2); !strings.Contains(q, "$n1: Int!") || strings.Contains(q, "$n2") {
		t.Errorf("governingIssuesQuery(2) =\n%s", q)
	}
}
