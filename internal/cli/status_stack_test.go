package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func stackTestRegistry(repository string) *stackRegistry {
	entries := make([]stackRegistryEntry, stackEntryCount)
	for i := range entries {
		issue := 301 + i
		prerequisitePR := 700 + i
		prerequisite := stackPrerequisite{Issue: stackPrerequisiteChangeIssue, PullRequest: &prerequisitePR, Branch: "staging"}
		if i > 0 {
			prerequisite = stackPrerequisite{
				Issue: 300 + i, PullRequest: &prerequisitePR, Branch: fmt.Sprintf("gitlab-stack/issue-%d", 300+i),
			}
		}
		entries[i] = stackRegistryEntry{
			Issue: issue, Sequence: i + 1, Branch: fmt.Sprintf("gitlab-stack/issue-%d", issue),
			DirectPrerequisite: prerequisite,
		}
	}
	return &stackRegistry{
		GarageStack: 1, ProgramIssue: stackProgramIssue, Repository: repository,
		PrerequisiteChangePR: 700, Entries: entries,
	}
}

func stackRegistryComment(registry *stackRegistry) workflowComment {
	var body strings.Builder
	fmt.Fprintf(&body, "%s\n\n```yaml\ngarage_stack: %d\nprogram_issue: %d\nrepository: %q\nprerequisite_change_pr: %d\nentries:\n",
		stackRegistryMarker, registry.GarageStack, registry.ProgramIssue, registry.Repository, registry.PrerequisiteChangePR)
	for _, entry := range registry.Entries {
		fmt.Fprintf(&body, "  - issue: %d\n    sequence: %d\n    branch: %q\n    direct_prerequisite:\n      issue: %d\n      branch: %q\n",
			entry.Issue, entry.Sequence, entry.Branch, entry.DirectPrerequisite.Issue, entry.DirectPrerequisite.Branch)
		if entry.DirectPrerequisite.PullRequest != nil {
			fmt.Fprintf(&body, "      pull_request: %d\n", entry.DirectPrerequisite.pullRequestNumber())
		}
	}
	body.WriteString("```\n")
	return workflowComment{URL: "https://github.com/o/r/issues/192#issuecomment/1",
		Body: body.String(), AuthorAssociation: "OWNER"}
}

func stackDependencyBody(issue int, entry stackRegistryEntry) string {
	return fmt.Sprintf("## Governing issue\n\nRefs #%d\n\n## Pull request kind\n\nimplementation\n\n## Stack dependency\n\n```yaml\ngarage_stack: 1\nprogram_issue: 192\nsequence: %d\ndirect_prerequisite_issue: %d\ndirect_prerequisite_pr: %d\ntarget_branch: %s\n```\n",
		issue, entry.Sequence, entry.DirectPrerequisite.Issue, entry.DirectPrerequisite.pullRequestNumber(), entry.DirectPrerequisite.Branch)
}

func stackFeaturePRBody(entry stackRegistryEntry, target string) string {
	body := stackDependencyBody(entry.Issue, entry)
	return strings.Replace(body, "target_branch: "+entry.DirectPrerequisite.Branch, "target_branch: "+target, 1)
}

func stackFeatureWithoutDeclarationBody(issue int) string {
	return fmt.Sprintf("## Governing issue\n\nRefs #%d\n\n## Pull request kind\n\nimplementation\n", issue)
}

func stackTestMergedPR(number int, base, head, body, mergedAt string) workflowChange {
	at, err := time.Parse(time.RFC3339, mergedAt)
	if err != nil {
		panic(err)
	}
	return workflowChange{
		Number: number, State: "MERGED", Merged: true, MergedAt: &at,
		BaseRefName: base, HeadRefName: head, Body: body,
		BaseRepository: stackTestRepository, HeadRepository: stackTestRepository,
	}
}

const stackTestRepository = "bdonaldharris/builder-garage"

func stackTestRepositoryName() *workflowRepository {
	return &workflowRepository{NameWithOwner: stackTestRepository}
}

func stackTestOpenPR(number int, base, head, body string) workflowChange {
	return workflowChange{Number: number, State: "OPEN", BaseRefName: base, HeadRefName: head, Body: body,
		BaseRepository: stackTestRepository, HeadRepository: stackTestRepository}
}

func stackTestRepositoryOrNil(name string) *workflowRepository {
	if name == "" {
		return nil
	}
	return &workflowRepository{NameWithOwner: name}
}

// stackTestSubject builds the status subject from the same facts a
// predecessor read would return for that pull request.
func stackTestSubject(pr workflowChange, others map[int]workflowChange, registry *stackRegistry) *workflowItem {
	prs := make(map[int]workflowChange, len(others))
	for number, other := range others {
		if number != pr.Number {
			prs[number] = other
		}
	}
	return &workflowItem{
		Kind: workflowChangeKind, Number: pr.Number, State: pr.State, Merged: pr.Merged, MergedAt: pr.MergedAt,
		Body: pr.Body, BaseRefName: pr.BaseRefName, BaseRefOid: strings.Repeat("a", 40), HeadRefName: pr.HeadRefName,
		BaseRepository: stackTestRepositoryOrNil(pr.BaseRepository),
		HeadRepository: stackTestRepositoryOrNil(pr.HeadRepository),
		RepositoryHost: "github.com", RepositoryIdentity: "bdonaldharris/builder-garage",
		StackRegistryFetched:  true,
		StackRegistryComments: []workflowComment{stackRegistryComment(registry)},
		StatusPRs:             prs,
	}
}

func stackTestItem(body string, entry stackRegistryEntry) *workflowItem {
	return &workflowItem{
		Kind: workflowChangeKind, State: "OPEN", Body: body, BaseRefName: entry.DirectPrerequisite.Branch,
		BaseRefOid: strings.Repeat("a", 40), HeadRefName: entry.Branch,
		BaseRepository: stackTestRepositoryName(), HeadRepository: stackTestRepositoryName(),
		RepositoryHost: "github.com", RepositoryIdentity: "bdonaldharris/builder-garage",
		StackRegistryFetched:  true,
		StackRegistryComments: []workflowComment{stackRegistryComment(stackTestRegistry("bdonaldharris/builder-garage"))},
		StatusPRs: map[int]workflowChange{
			700: stackTestMergedPR(700, "staging", "feature/196",
				"## Governing issue\n\nRefs #196\n\n## Pull request kind\n\nimplementation\n",
				"2026-01-01T09:00:00Z"),
			701: stackTestOpenPR(701, "staging", "gitlab-stack/issue-301",
				stackFeaturePRBody(stackTestRegistry("bdonaldharris/builder-garage").Entries[0], "staging")),
		},
	}
}

func TestValidateStackRegistry(t *testing.T) {
	valid := stackTestRegistry("bdonaldharris/builder-garage")
	if err := validateStackRegistry(valid, "github.com", "bdonaldharris/builder-garage", "staging", "main"); err != nil {
		t.Fatalf("valid registry: %v", err)
	}
	if err := validateStackRegistry(valid, "gitlab.example", "bdonaldharris/builder-garage", "staging", "main"); err == nil {
		t.Fatal("registry was accepted on an unauthorized host")
	}
	cases := map[string]func(*stackRegistry){
		"wrong repository": func(registry *stackRegistry) { registry.Repository = "other/repo" },
		"wrong program":    func(registry *stackRegistry) { registry.ProgramIssue = 193 },
		"wrong sequence":   func(registry *stackRegistry) { registry.Entries[2].Sequence = 4 },
		"wrong predecessor": func(registry *stackRegistry) {
			registry.Entries[2].DirectPrerequisite.Issue++
		},
		"duplicate branch": func(registry *stackRegistry) {
			registry.Entries[2].Branch = registry.Entries[1].Branch
		},
		"reserved target": func(registry *stackRegistry) {
			registry.Entries[0].Branch = "main"
		},
		"missing entry": func(registry *stackRegistry) {
			registry.Entries = registry.Entries[:5]
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			registry := stackTestRegistry("bdonaldharris/builder-garage")
			change(registry)
			if err := validateStackRegistry(registry, "github.com", "bdonaldharris/builder-garage", "staging", "main"); err == nil {
				t.Fatal("invalid registry was accepted")
			}
		})
	}
}

func TestLatestStackRegistryIsFailClosed(t *testing.T) {
	valid := stackRegistryComment(stackTestRegistry("o/r"))
	if got := latestStackRegistry([]workflowComment{valid}); got.Registry == nil || got.URL != valid.URL {
		t.Fatalf("latestStackRegistry = %+v, want valid record", got)
	}
	invalid := workflowComment{URL: "https://github.com/o/r/issues/192#issuecomment/2",
		Body: stackRegistryMarker + "\n\nno YAML block\n", AuthorAssociation: "OWNER"}
	got := latestStackRegistry([]workflowComment{valid, invalid})
	if got.Registry != nil || got.URL != invalid.URL || !strings.Contains(got.Problem, "no fenced YAML") {
		t.Fatalf("latest malformed record did not govern: %+v", got)
	}
	untrusted := invalid
	untrusted.AuthorAssociation = "NONE"
	if got := latestStackRegistry([]workflowComment{untrusted}); got.Registry != nil || !strings.Contains(got.Problem, "no counted") {
		t.Fatalf("unassociated record counted: %+v", got)
	}
}

func TestFetchStatusItemReadsRegistryOnlyForAuthorizedRepository(t *testing.T) {
	registry := stackRegistryComment(stackTestRegistry("bdonaldharris/builder-garage"))
	response, err := json.Marshal(map[string]any{
		"data": map[string]any{"repository": map[string]any{
			"defaultBranchRef": map[string]any{"name": "main"},
			"stackRegistry": map[string]any{"__typename": "Issue", "comments": map[string]any{
				"nodes": []any{map[string]any{
					"url": registry.URL, "body": registry.Body, "authorAssociation": "OWNER",
				}},
			}},
			"issueOrPullRequest": map[string]any{
				"__typename": "PullRequest", "number": 900, "state": "OPEN",
				"baseRefName": "staging", "headRefName": "feature/900",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var query string
	item, err := fetchStatusItem(context.Background(), func(_ context.Context, args ...string) ([]byte, []byte, error) {
		query = strings.Join(args, " ")
		return response, nil, nil
	}, ghRepository{Host: "github.com", Owner: "bdonaldharris", Name: "builder-garage"}, 900)
	if err != nil {
		t.Fatalf("fetch authorized status item: %v", err)
	}
	if !item.StackRegistryFetched || len(item.StackRegistryComments) != 1 ||
		!strings.Contains(query, "includeStackRegistry=true") ||
		!strings.Contains(query, "stackRegistry: issueOrPullRequest(number: 192)") {
		t.Fatalf("authorized registry read = fetched %v, comments %d, query %q",
			item.StackRegistryFetched, len(item.StackRegistryComments), query)
	}

	unreadable, err := fetchStatusItem(context.Background(), func(_ context.Context, args ...string) ([]byte, []byte, error) {
		return []byte(`{"data":{"repository":{"issueOrPullRequest":{"__typename":"PullRequest","number":900,"state":"OPEN","baseRefName":"staging","headRefName":"feature/900"}}},"errors":[{"type":"NOT_FOUND","message":"Could not resolve to an issue or pull request with number 192","path":["repository","stackRegistry"]}]}`), nil, nil
	}, ghRepository{Host: "github.com", Owner: "bdonaldharris", Name: "builder-garage"}, 900)
	if err != nil {
		t.Fatalf("registry-only read failure blocked ordinary status: %v", err)
	}
	if unreadable.StackRegistryFetched {
		t.Fatal("registry-only read failure was reported as a successful read")
	}

	var foreignQuery string
	foreign, err := fetchStatusItem(context.Background(), func(_ context.Context, args ...string) ([]byte, []byte, error) {
		foreignQuery = strings.Join(args, " ")
		return []byte(`{"data":{"repository":{"issueOrPullRequest":{"__typename":"PullRequest","number":900,"state":"OPEN","baseRefName":"staging","headRefName":"feature/900"}}}}`), nil, nil
	}, ghRepository{Host: "github.com", Owner: "other", Name: "repo"}, 900)
	if err != nil {
		t.Fatalf("fetch unrelated status item: %v", err)
	}
	if foreign.StackRegistryFetched || strings.Contains(foreignQuery, "includeStackRegistry=true") {
		t.Fatalf("unrelated repository queried the #192 registry: %+v, query %q", foreign, foreignQuery)
	}
}

func TestEmptyStackTemplateSectionsAreAbsent(t *testing.T) {
	body := "## Stack dependency\n\n<!-- Optional: registered stack metadata. -->\n\n## Stack maintenance\n\n<!-- Optional: maintenance metadata. -->\n"
	if _, present, err := parseStackDependency(body); err != nil || present {
		t.Fatalf("empty Stack dependency template section = present %v, error %v", present, err)
	}
	if _, present, err := parseStackMaintenance(body); err != nil || present {
		t.Fatalf("empty Stack maintenance template section = present %v, error %v", present, err)
	}
	body = "## Stack dependency\n\ninvalid declaration\n"
	if _, present, err := parseStackDependency(body); err == nil || !present {
		t.Fatalf("non-empty malformed Stack dependency section = present %v, error %v", present, err)
	}
}

func TestValidateStackDependency(t *testing.T) {
	registry := stackTestRegistry("o/r")
	entry := registry.Entries[1]
	item := stackTestItem(stackDependencyBody(entry.Issue, entry), entry)
	got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
	if !got.Valid || !got.Present || got.Maintenance || got.Entry.Issue != entry.Issue {
		t.Fatalf("valid stack dependency = %+v", got)
	}
	retargeted := stackTestItem(
		strings.Replace(stackDependencyBody(entry.Issue, entry), "target_branch: "+entry.DirectPrerequisite.Branch, "target_branch: staging", 1),
		entry)
	retargeted.BaseRefName = "staging"
	retargeted.StatusPRs[701] = stackTestMergedPR(701, "staging", entry.DirectPrerequisite.Branch,
		stackFeaturePRBody(stackTestRegistry("bdonaldharris/builder-garage").Entries[0], "staging"),
		"2026-01-02T09:00:00Z")
	if got := validateStackDeclaration(retargeted, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil); !got.Valid {
		t.Fatalf("retargeting after the direct prerequisite merged was rejected: %+v", got)
	}

	cases := map[string]func(*workflowItem){
		"registered issue cannot omit declaration for E1 target": func(item *workflowItem) {
			item.Body = stackFeatureWithoutDeclarationBody(entry.Issue)
			item.BaseRefName = "staging"
			item.HeadRefName = entry.Branch
		},
		"registered issue cannot omit declaration after retarget": func(item *workflowItem) {
			item.Body = stackFeatureWithoutDeclarationBody(entry.Issue)
			item.BaseRefName = "staging"
		},
		"unregistered governing issue": func(item *workflowItem) {
			item.Body = stackDependencyBody(999, entry)
		},
		"missing prerequisite-change PR": func(item *workflowItem) {
			delete(item.StatusPRs, 700)
		},
		"unmerged prerequisite-change PR": func(item *workflowItem) {
			pr := item.StatusPRs[700]
			pr.Merged = false
			pr.State = "OPEN"
			item.StatusPRs[700] = pr
		},
		"wrong registered head": func(item *workflowItem) { item.HeadRefName = "gitlab-stack/wrong" },
		"wrong target":          func(item *workflowItem) { item.BaseRefName = "staging" },
		"wrong declaration target": func(item *workflowItem) {
			item.Body = strings.Replace(item.Body, "target_branch: gitlab-stack/issue-301", "target_branch: staging", 1)
		},
		"wrong prerequisite PR": func(item *workflowItem) {
			item.StatusPRs[701] = stackTestOpenPR(701, "staging", "wrong", "")
		},
		"wrong predecessor base": func(item *workflowItem) {
			item.StatusPRs[701] = stackTestOpenPR(701, "main", "gitlab-stack/issue-301",
				stackFeaturePRBody(stackTestRegistry("bdonaldharris/builder-garage").Entries[0], "staging"))
		},
		"wrong repository": func(item *workflowItem) {
			item.StackRegistryComments = []workflowComment{stackRegistryComment(stackTestRegistry("other/repo"))}
		},
		"missing registry": func(item *workflowItem) { item.StackRegistryFetched = false },
		"not declared implementation": func(item *workflowItem) {
			item.Body = strings.Replace(item.Body, "implementation", "hotfix", 1)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := stackTestItem(stackDependencyBody(entry.Issue, entry), entry)
			change(candidate)
			result := validateStackDeclaration(candidate, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
			if result.Valid || result.Problem == "" {
				t.Fatalf("invalid stack dependency accepted: %+v", result)
			}
		})
	}

	unreadable := stackTestItem(stackFeatureWithoutDeclarationBody(entry.Issue), entry)
	unreadable.StackRegistryFetched = false
	got = validateStackDeclaration(unreadable, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
	if got.Present || !got.Unknown || got.Problem == "" {
		t.Fatalf("undeclared E1 PR with unreadable registry was not reported as unknown: %+v", got)
	}
	malformed := stackTestItem(stackFeatureWithoutDeclarationBody(entry.Issue), entry)
	malformed.StackRegistryComments = []workflowComment{{
		Body: stackRegistryMarker + "\n\nmalformed registry\n", AuthorAssociation: "OWNER",
	}}
	got = validateStackDeclaration(malformed, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
	if got.Present || !got.Unknown || got.Problem == "" {
		t.Fatalf("undeclared E1 PR with malformed registry was not reported as unknown: %+v", got)
	}

	unrelatedRepo := stackTestItem(stackFeatureWithoutDeclarationBody(entry.Issue), entry)
	unrelatedRepo.RepositoryHost = "github.com"
	unrelatedRepo.RepositoryIdentity = "other/repo"
	unrelatedRepo.HeadRefName = entry.Branch
	unrelatedRepo.StackRegistryComments = []workflowComment{stackRegistryComment(stackTestRegistry("other/repo"))}
	got = validateStackDeclaration(unrelatedRepo, "github.com", "other/repo", "staging", "main", nil)
	if got.Present || got.Problem != "" {
		t.Fatalf("another repository's registry changed ordinary PR eligibility: %+v", got)
	}
}

func TestStackMembershipBoundaries(t *testing.T) {
	registry := stackTestRegistry("bdonaldharris/builder-garage")
	entry := registry.Entries[1]

	t.Run("registered issue requires declaration on E1 with arbitrary branch", func(t *testing.T) {
		registry.Entries[1].Branch = "feature/custom-stack-302"
		registry.Entries[2].DirectPrerequisite.Branch = registry.Entries[1].Branch
		item := stackTestItem(stackFeatureWithoutDeclarationBody(entry.Issue), entry)
		item.StackRegistryComments = []workflowComment{stackRegistryComment(registry)}
		item.HeadRefName = registry.Entries[1].Branch
		item.BaseRefName = "staging"
		got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
		if !got.Present || got.Valid || got.Problem == "" {
			t.Fatalf("registered E1 PR without a declaration = %+v, want ineligible", got)
		}
	})

	t.Run("registered arbitrary branch with matching declaration is accepted", func(t *testing.T) {
		registry := stackTestRegistry("bdonaldharris/builder-garage")
		registry.Entries[1].Branch = "feature/custom-stack-302"
		registry.Entries[2].DirectPrerequisite.Branch = registry.Entries[1].Branch
		entry := registry.Entries[1]
		item := stackTestItem(stackDependencyBody(entry.Issue, entry), entry)
		item.StackRegistryComments = []workflowComment{stackRegistryComment(registry)}
		item.HeadRefName = entry.Branch
		item.BaseRefName = entry.DirectPrerequisite.Branch
		item.StatusPRs[701] = stackTestOpenPR(701, "staging", registry.Entries[0].Branch,
			stackFeaturePRBody(registry.Entries[0], "staging"))
		if got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil); !got.Valid {
			t.Fatalf("registered arbitrary branch was rejected: %+v", got)
		}
	})

	t.Run("valid registry omission remains ordinary despite conventional branch", func(t *testing.T) {
		item := stackTestItem(stackFeatureWithoutDeclarationBody(999), entry)
		item.HeadRefName = "gitlab-stack/issue-999"
		item.BaseRefName = "staging"
		got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
		if got.Present || got.Problem != "" {
			t.Fatalf("known non-member was treated as enrolled: %+v", got)
		}
	})

	t.Run("unknown registry is reported without blocking ordinary E1", func(t *testing.T) {
		for _, state := range []string{"missing", "unreadable", "malformed"} {
			t.Run(state, func(t *testing.T) {
				item := stackTestItem(stackFeatureWithoutDeclarationBody(entry.Issue), entry)
				item.BaseRefName = "staging"
				item.HeadRefName = "feature/custom-stack-302"
				switch state {
				case "missing":
					item.StackRegistryComments = nil
				case "unreadable":
					item.StackRegistryFetched = false
				case "malformed":
					item.StackRegistryComments = []workflowComment{{
						Body: stackRegistryMarker + "\n\nmalformed registry\n", AuthorAssociation: "OWNER",
					}}
				}
				report := buildStatusReport(statusInputs{
					Item: item, Config: stagingConfig(), ReadAt: time.Now(), Reads: 1,
				})
				var output strings.Builder
				if err := report.render(&output); err != nil {
					t.Fatal(err)
				}
				assertContains(t, output.String(), "stack membership is unknown", "ordinary E1 gates are evaluated",
					"NO approving issue-recorded implementation review was found", "the check state of the head was not available")
				assertNotContains(t, output.String(), "stack declaration is not eligible")
			})
		}
	})

	t.Run("explicit declaration with unknown registry is not authorized", func(t *testing.T) {
		item := stackTestItem(stackDependencyBody(entry.Issue, entry), entry)
		item.StackRegistryFetched = false
		got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
		if !got.Present || got.Valid || got.Problem == "" {
			t.Fatalf("explicit declaration without registry evidence = %+v, want ineligible", got)
		}
	})

	t.Run("unknown registry grants no non-E1 destination", func(t *testing.T) {
		item := stackTestItem(stackFeatureWithoutDeclarationBody(entry.Issue), entry)
		item.StackRegistryFetched = false
		item.BaseRefName = entry.Branch
		report := buildStatusReport(statusInputs{
			Item: item, Config: stagingConfig(), ReadAt: time.Now(), Reads: 1,
		})
		var output strings.Builder
		if err := report.render(&output); err != nil {
			t.Fatal(err)
		}
		assertContains(t, output.String(), "stack membership is unknown", "no stack exception is granted", "ordinary target rules remain in force")
		assertNotContains(t, output.String(), "targets registered stack branch")
	})
}

func TestStackPredecessorChainValidatesRelevantMergeEvidence(t *testing.T) {
	registry := stackTestRegistry("bdonaldharris/builder-garage")
	entry := registry.Entries[2]
	newItem := func() *workflowItem {
		item := stackTestItem(stackDependencyBody(entry.Issue, entry), entry)
		item.StackRegistryComments = []workflowComment{stackRegistryComment(registry)}
		item.StatusPRs[701] = stackTestOpenPR(701, "staging", registry.Entries[0].Branch,
			stackFeaturePRBody(registry.Entries[0], "staging"))
		item.StatusPRs[702] = stackTestOpenPR(702, registry.Entries[0].Branch, registry.Entries[1].Branch,
			stackFeaturePRBody(registry.Entries[1], registry.Entries[0].Branch))
		item.BaseRefName = registry.Entries[1].Branch
		return item
	}

	t.Run("open chain permits direct predecessor branch", func(t *testing.T) {
		item := newItem()
		if got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil); !got.Valid {
			t.Fatalf("valid open predecessor chain was rejected: %+v", got)
		}
	})

	t.Run("merged chain permits actual merge destination", func(t *testing.T) {
		item := newItem()
		item.StatusPRs[701] = stackTestMergedPR(701, "staging", registry.Entries[0].Branch,
			stackFeaturePRBody(registry.Entries[0], "staging"), "2026-01-02T09:00:00Z")
		item.StatusPRs[702] = stackTestMergedPR(702, "staging", registry.Entries[1].Branch,
			stackFeaturePRBody(registry.Entries[1], "staging"), "2026-01-03T09:00:00Z")
		item.BaseRefName = "staging"
		item.Body = strings.Replace(item.Body, "target_branch: "+entry.DirectPrerequisite.Branch,
			"target_branch: staging", 1)
		if got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil); !got.Valid {
			t.Fatalf("valid merged predecessor chain was rejected: %+v", got)
		}
	})

	t.Run("merged predecessor cannot skip its open prerequisite", func(t *testing.T) {
		item := newItem()
		item.StatusPRs[702] = stackTestMergedPR(702, "staging", registry.Entries[1].Branch,
			stackFeaturePRBody(registry.Entries[1], "staging"), "2026-01-02T10:00:00Z")
		item.BaseRefName = "staging"
		item.Body = strings.Replace(item.Body, "target_branch: "+entry.DirectPrerequisite.Branch,
			"target_branch: staging", 1)
		if got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil); got.Valid {
			t.Fatalf("skipped predecessor transition was accepted: %+v", got)
		}
	})

	t.Run("predecessor declaration is required", func(t *testing.T) {
		item := newItem()
		pr := item.StatusPRs[701]
		pr.Body = stackFeatureWithoutDeclarationBody(registry.Entries[0].Issue)
		item.StatusPRs[701] = pr
		if got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil); got.Valid {
			t.Fatalf("predecessor without its required declaration was accepted: %+v", got)
		}
	})
}

// stackHistory is the state of the first two feature PRs of the registry.
// Every history is run through each path that consumes those PRs: status on
// the PR itself, status on a dependent feature PR, and maintenance.
type stackHistory struct {
	name     string
	pr1, pr2 workflowChange
	// pr1Valid and pr2Valid are each PR's own feature-rule result.
	pr1Valid, pr2Valid bool
	// pr3Target is the target permitted now for the third feature PR.
	pr3Target string
}

func TestStackFeatureHistoriesAcrossConsumptionPaths(t *testing.T) {
	const (
		host, repository = "github.com", "bdonaldharris/builder-garage"
		jan2, jan3       = "2026-01-02T09:00:00Z", "2026-01-03T09:00:00Z"
	)
	registry := stackTestRegistry(repository)
	e1, e2, e3 := registry.Entries[0], registry.Entries[1], registry.Entries[2]
	b1, b2 := e1.Branch, e2.Branch
	body1 := stackFeaturePRBody(e1, "staging")
	body2 := func(target string) string { return stackFeaturePRBody(e2, target) }
	open1 := stackTestOpenPR(701, "staging", b1, body1)
	merged1 := func(at string) workflowChange { return stackTestMergedPR(701, "staging", b1, body1, at) }
	open2 := func(base, declared string) workflowChange { return stackTestOpenPR(702, base, b2, body2(declared)) }
	merged2 := func(base, declared, at string) workflowChange {
		return stackTestMergedPR(702, base, b2, body2(declared), at)
	}

	histories := []stackHistory{
		// Supported workflow histories.
		{"all open", open1, open2(b1, b1), true, true, b2},
		{"predecessor merged, dependent retargeted", merged1(jan2), open2("staging", "staging"), true, true, b2},
		{"in-order merges into E1", merged1(jan2), merged2("staging", "staging", jan3), true, true, "staging"},
		{"nested merge, predecessor still open", open1, merged2(b1, b1, jan2), true, true, b1},
		{"nested merge, predecessor merged later", merged1(jan3), merged2(b1, b1, jan2), true, true, "staging"},
		// Workflow histories the contract rejects.
		{"merge into E1 skipped an open predecessor", open1, merged2("staging", "staging", jan2), true, false, ""},
		{"skipped merge, predecessor merged later", merged1(jan3), merged2("staging", "staging", jan2), true, false, ""},
		{"dependent not retargeted after predecessor merged", merged1(jan2), open2(b1, b1), true, false, ""},
		{"merge into predecessor branch after it merged", merged1(jan2), merged2(b1, b1, jan3), true, false, ""},
		{"merged PR declares an unrelated target", open1, merged2(b1, "main", jan2), true, false, ""},
		{"merged PR declares its pre-retarget target", merged1(jan2), merged2("staging", b1, jan3), true, false, ""},
		{"open PR declares another target", open1, open2(b1, "staging"), true, false, ""},
		{"first feature targets an unpermitted branch", stackTestOpenPR(701, "main", b1, body1), open2(b1, b1), false, true, ""},
		{"first feature merged before the prerequisite change", merged1("2025-12-31T09:00:00Z"),
			open2("staging", "staging"), false, true, ""},
	}
	prerequisite := stackTestItem("", e1).StatusPRs[700]
	validate := func(item *workflowItem) stackValidation {
		return validateStackDeclaration(item, host, repository, "staging", "main", nil)
	}
	for _, history := range histories {
		t.Run(history.name, func(t *testing.T) {
			prs := map[int]workflowChange{700: prerequisite, 701: history.pr1, 702: history.pr2}
			chainValid := history.pr1Valid && history.pr2Valid

			// One rule: the same facts get the same feature result whether they
			// arrive as a predecessor read or as the status subject.
			asPredecessor := newStackChain(prs, registry, "staging")
			for sequence, pr := range map[int]workflowChange{1: history.pr1, 2: history.pr2} {
				asSubject := newStackChain(prs, registry, "staging")
				asSubject.features[sequence] = stackFactsFromItem(stackTestSubject(pr, prs, registry))
				if got, want := asSubject.featureProblem(sequence), asPredecessor.featureProblem(sequence); got != want {
					t.Errorf("entry %d: subject result %q differs from predecessor result %q", sequence, got, want)
				}
			}
			if got := asPredecessor.featureProblem(1) == ""; got != history.pr1Valid {
				t.Errorf("first feature rule valid = %v, want %v", got, history.pr1Valid)
			}
			if got := asPredecessor.featureProblem(2) == ""; got != history.pr2Valid {
				t.Errorf("second feature rule valid = %v, want %v", got, history.pr2Valid)
			}

			// Subject path.
			if got := validate(stackTestSubject(history.pr1, prs, registry)); got.Valid != history.pr1Valid {
				t.Errorf("status on the first feature PR: valid = %v, want %v: %s", got.Valid, history.pr1Valid, got.Problem)
			}
			if got := validate(stackTestSubject(history.pr2, prs, registry)); got.Valid != chainValid {
				t.Errorf("status on the second feature PR: valid = %v, want %v: %s", got.Valid, chainValid, got.Problem)
			}

			// Predecessor path: an open third feature PR.
			for _, target := range []string{"staging", b1, b2} {
				third := stackTestSubject(stackTestOpenPR(703, target, e3.Branch, stackFeaturePRBody(e3, target)), prs, registry)
				want := chainValid && target == history.pr3Target
				if got := validate(third); got.Valid != want {
					t.Errorf("third feature PR targeting %s: valid = %v, want %v: %s", target, got.Valid, want, got.Problem)
				}
			}

			// Maintenance path: both synchronization sources into the third entry.
			for name, body := range map[string]string{
				"E1 synchronization": stackMaintenanceBody(e3.Issue, "synchronize", "staging", 0, e3.Branch, ""),
				"propagation":        stackMaintenanceBody(e3.Issue, "synchronize", b2, 702, e3.Branch, ""),
			} {
				item := stackTestSubject(stackTestOpenPR(710, e3.Branch, "maintenance/stack-192/303-sync", body), prs, registry)
				if got := validate(item); got.Valid != chainValid {
					t.Errorf("%s: valid = %v, want %v: %s", name, got.Valid, chainValid, got.Problem)
				}
			}
		})
	}
}

// A registered branch is a branch of the registry's repository. These cases
// change only which repository owns a ref, on a history that is otherwise
// valid on every path.
func TestStackBranchIdentityRequiresRegistryRepository(t *testing.T) {
	const fork = "someone/builder-garage"
	registry := stackTestRegistry(stackTestRepository)
	e1, e2, e3 := registry.Entries[0], registry.Entries[1], registry.Entries[2]
	b1, b2 := e1.Branch, e2.Branch
	validate := func(item *workflowItem) stackValidation {
		return validateStackDeclaration(item, "github.com", stackTestRepository, "staging", "main", nil)
	}
	newPRs := func() map[int]workflowChange {
		return map[int]workflowChange{
			700: stackTestItem("", e1).StatusPRs[700],
			701: stackTestOpenPR(701, "staging", b1, stackFeaturePRBody(e1, "staging")),
			702: stackTestOpenPR(702, b1, b2, stackFeaturePRBody(e2, b1)),
		}
	}
	third := func(prs map[int]workflowChange, target string) *workflowItem {
		return stackTestSubject(stackTestOpenPR(703, target, e3.Branch, stackFeaturePRBody(e3, target)), prs, registry)
	}
	maintenance := func(prs map[int]workflowChange) map[string]*workflowItem {
		items := make(map[string]*workflowItem)
		for name, body := range map[string]string{
			"E1 synchronization": stackMaintenanceBody(e3.Issue, "synchronize", "staging", 0, e3.Branch, ""),
			"propagation":        stackMaintenanceBody(e3.Issue, "synchronize", b2, 702, e3.Branch, ""),
		} {
			items[name] = stackTestSubject(stackTestOpenPR(710, e3.Branch, "maintenance/stack-192/303-sync", body), prs, registry)
		}
		return items
	}

	t.Run("registered repository and branch are accepted on every path", func(t *testing.T) {
		prs := newPRs()
		if got := validate(stackTestSubject(prs[702], prs, registry)); !got.Valid {
			t.Errorf("subject: %+v", got)
		}
		if got := validate(third(prs, b2)); !got.Valid {
			t.Errorf("dependent of registered predecessors: %+v", got)
		}
		for name, item := range maintenance(prs) {
			if got := validate(item); !got.Valid {
				t.Errorf("%s: %+v", name, got)
			}
		}
	})

	// owner is the repository reported for the ref: a fork holding a branch
	// with the registered name, or none at all.
	for ownerName, owner := range map[string]string{"same-named fork branch": fork, "repository not returned": ""} {
		t.Run("subject head: "+ownerName, func(t *testing.T) {
			prs := newPRs()
			subject := prs[702]
			subject.HeadRepository = owner
			if got := validate(stackTestSubject(subject, prs, registry)); got.Valid || got.Problem == "" {
				t.Errorf("a head outside the registry's repository was accepted: %+v", got)
			}
		})
		t.Run("subject base: "+ownerName, func(t *testing.T) {
			prs := newPRs()
			subject := prs[702]
			subject.BaseRepository = owner
			if got := validate(stackTestSubject(subject, prs, registry)); got.Valid || got.Problem == "" {
				t.Errorf("a base outside the registry's repository was accepted: %+v", got)
			}
		})
		for _, number := range []int{701, 702} {
			t.Run(fmt.Sprintf("predecessor #%d head: %s", number, ownerName), func(t *testing.T) {
				prs := newPRs()
				predecessor := prs[number]
				predecessor.HeadRepository = owner
				prs[number] = predecessor
				for _, target := range []string{"staging", b1, b2} {
					if got := validate(third(prs, target)); got.Valid || got.Problem == "" {
						t.Errorf("dependent targeting %s was accepted: %+v", target, got)
					}
				}
				for name, item := range maintenance(prs) {
					if got := validate(item); got.Valid || got.Problem == "" {
						t.Errorf("%s was accepted: %+v", name, got)
					}
				}
				if number == 701 {
					if got := validate(stackTestSubject(prs[702], prs, registry)); got.Valid {
						t.Errorf("subject whose predecessor is not the registered branch was accepted: %+v", got)
					}
				}
			})
		}
		t.Run("merged predecessor head: "+ownerName, func(t *testing.T) {
			// The merge of a PR that is not the registered branch must not move
			// its dependent's permitted target.
			prs := newPRs()
			merged := stackTestMergedPR(702, b1, b2, stackFeaturePRBody(e2, b1), "2026-01-02T09:00:00Z")
			merged.HeadRepository = owner
			prs[702] = merged
			for _, target := range []string{"staging", b1, b2} {
				if got := validate(third(prs, target)); got.Valid || got.Problem == "" {
					t.Errorf("dependent targeting %s was accepted: %+v", target, got)
				}
			}
		})
		t.Run("prerequisite-change base: "+ownerName, func(t *testing.T) {
			prs := newPRs()
			prerequisite := prs[700]
			prerequisite.BaseRepository = owner
			prs[700] = prerequisite
			if got := validate(stackTestSubject(prs[701], prs, registry)); got.Valid || got.Problem == "" {
				t.Errorf("a prerequisite change with an unestablished E1 was accepted: %+v", got)
			}
		})
		t.Run("maintenance target: "+ownerName, func(t *testing.T) {
			prs := newPRs()
			for name, item := range maintenance(prs) {
				item.BaseRepository = stackTestRepositoryOrNil(owner)
				if got := validate(item); got.Valid || got.Problem == "" {
					t.Errorf("%s into a target outside the registry's repository was accepted: %+v", name, got)
				}
			}
		})
	}
}

// A pull request outside the stack exception is not judged by where its head
// lives. A fork head, or a deleted fork, changes nothing in its report.
func TestOrdinaryForkPullRequestsAreUnchangedByStackIdentity(t *testing.T) {
	entry := stackTestRegistry(stackTestRepository).Entries[1]
	newItem := func(head *workflowRepository) *workflowItem {
		item := stackTestItem(stackFeatureWithoutDeclarationBody(999), entry)
		item.BaseRefName = "staging"
		item.HeadRefName = "feature/ordinary"
		item.HeadRepository = head
		item.BodyIssues = []workflowIssue{{Number: 999}}
		return item
	}
	want := renderItem(t, newItem(stackTestRepositoryName()), stagingConfig(), 2)
	for name, head := range map[string]*workflowRepository{
		"fork":         {NameWithOwner: "someone/builder-garage"},
		"deleted fork": nil,
	} {
		t.Run(name, func(t *testing.T) {
			item := newItem(head)
			got := validateStackDeclaration(item, "github.com", stackTestRepository, "staging", "main", nil)
			if got.Present || got.Unknown || got.Problem != "" {
				t.Fatalf("ordinary fork PR was drawn into stack validation: %+v", got)
			}
			if text := renderItem(t, item, stagingConfig(), 2); text != want {
				t.Errorf("ordinary fork PR report differs from the same PR with a head in the repository:\n%s", text)
			}
		})
	}
	t.Run("unknown registry", func(t *testing.T) {
		item := newItem(&workflowRepository{NameWithOwner: "someone/builder-garage"})
		item.StackRegistryFetched = false
		sameRepository := newItem(stackTestRepositoryName())
		sameRepository.StackRegistryFetched = false
		if got, want := renderItem(t, item, stagingConfig(), 2), renderItem(t, sameRepository, stagingConfig(), 2); got != want {
			t.Errorf("unknown-membership report differs for a fork head:\n%s", got)
		}
	})
	t.Run("unrelated repository", func(t *testing.T) {
		item := newItem(&workflowRepository{NameWithOwner: "someone/other"})
		got := validateStackDeclaration(item, "github.com", "other/repo", "staging", "main", nil)
		if got.Present || got.Unknown || got.Problem != "" {
			t.Fatalf("fork PR in an unrelated repository was drawn into stack validation: %+v", got)
		}
	})
}

// Both query paths must ask GitHub which repository owns each ref.
func TestStatusQueriesReadRefOwners(t *testing.T) {
	for name, query := range map[string]string{"primary": statusQuery, "follow-up": governingIssuesQuery(1)} {
		for _, want := range []string{"headRepository { nameWithOwner }", "baseRepository { nameWithOwner }"} {
			if !strings.Contains(query, want) {
				t.Errorf("%s query does not select %q", name, want)
			}
		}
	}
	var item ghItem
	if err := json.Unmarshal([]byte(`{"__typename":"PullRequest","headRefName":"b","headRepository":{"nameWithOwner":"someone/fork"},"baseRepository":{"nameWithOwner":"o/r"}}`), &item); err != nil {
		t.Fatal(err)
	}
	facts := stackFactsFromItem(normalizeGitHubItem(&item))
	if facts.HeadRepository != "someone/fork" || facts.BaseRepository != "o/r" {
		t.Errorf("subject ref owners were not decoded: %+v", facts)
	}
	if err := json.Unmarshal([]byte(`{"__typename":"PullRequest","headRefName":"b","headRepository":null}`), &item); err != nil {
		t.Fatal(err)
	}
	if facts := stackFactsFromItem(normalizeGitHubItem(&item)); facts.HeadRepository != "" {
		t.Errorf("a deleted fork decoded as %q, want no owner", facts.HeadRepository)
	}
}

// Provider evidence that cannot establish a merge or its order grants
// nothing. These are malformed or unavailable reads, not workflow histories.
func TestStackFeatureEvidenceThatCannotBeEstablished(t *testing.T) {
	const jan2 = "2026-01-02T09:00:00Z"
	registry := stackTestRegistry("bdonaldharris/builder-garage")
	e1, e2, e3 := registry.Entries[0], registry.Entries[1], registry.Entries[2]
	prerequisite := stackTestItem("", e1).StatusPRs[700]
	valid1 := stackTestMergedPR(701, "staging", e1.Branch, stackFeaturePRBody(e1, "staging"), jan2)
	cases := map[string]func(prs map[int]workflowChange){
		"predecessor not read": func(prs map[int]workflowChange) { delete(prs, 702) },
		"predecessor closed without merging": func(prs map[int]workflowChange) {
			pr := prs[702]
			pr.State = "CLOSED"
			prs[702] = pr
		},
		"merged predecessor without a merge time": func(prs map[int]workflowChange) {
			pr := stackTestMergedPR(702, "staging", e2.Branch, stackFeaturePRBody(e2, "staging"), jan2)
			pr.MergedAt = nil
			prs[702] = pr
		},
		"equal merge times": func(prs map[int]workflowChange) {
			prs[702] = stackTestMergedPR(702, "staging", e2.Branch, stackFeaturePRBody(e2, "staging"), jan2)
		},
		"merge time equal to the prerequisite change": func(prs map[int]workflowChange) {
			at := *prerequisite.MergedAt
			pr := prs[701]
			pr.MergedAt = &at
			prs[701] = pr
		},
		"prerequisite change not read": func(prs map[int]workflowChange) { delete(prs, 700) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			prs := map[int]workflowChange{700: prerequisite, 701: valid1,
				702: stackTestOpenPR(702, "staging", e2.Branch, stackFeaturePRBody(e2, "staging"))}
			change(prs)
			for _, target := range []string{"staging", e1.Branch, e2.Branch} {
				third := stackTestSubject(stackTestOpenPR(703, target, e3.Branch, stackFeaturePRBody(e3, target)), prs, registry)
				got := validateStackDeclaration(third, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
				if got.Valid || got.Problem == "" {
					t.Errorf("target %s was authorized without established evidence: %+v", target, got)
				}
			}
		})
	}
}

func stackMaintenanceBody(issue int, purpose, upstream string, upstreamPR int, target string, evidence string) string {
	return fmt.Sprintf("## Governing issue\n\nRefs #%d\n\n## Pull request kind\n\nimplementation\n\n## Stack maintenance\n\n```yaml\ngarage_stack: 1\nprogram_issue: 192\nentry_issue: %d\npurpose: %s\nupstream: %s\nupstream_pr: %d\ntarget_branch: %s\n%s```\n",
		issue, issue, purpose, upstream, upstreamPR, target, evidence)
}

func TestValidateStackMaintenance(t *testing.T) {
	registry := stackTestRegistry("o/r")
	entry := registry.Entries[1]
	item := stackTestItem(stackMaintenanceBody(entry.Issue, "synchronize", "staging", 0, entry.Branch, ""), entry)
	item.BaseRefName = entry.Branch
	item.HeadRefName = "maintenance/stack-192/302-sync"
	item.StatusPRs[701] = stackTestOpenPR(701, "staging", "gitlab-stack/issue-301",
		stackFeaturePRBody(stackTestRegistry("bdonaldharris/builder-garage").Entries[0], "staging"))
	got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
	if !got.Valid || !got.Maintenance {
		t.Fatalf("valid staging synchronization = %+v", got)
	}

	propagation := stackMaintenanceBody(entry.Issue, "synchronize", entry.DirectPrerequisite.Branch,
		entry.DirectPrerequisite.pullRequestNumber(), entry.Branch, "")
	item.Body = propagation
	if got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil); !got.Valid {
		t.Fatalf("valid dependent propagation = %+v", got)
	}
	item.StatusPRs[701] = stackTestOpenPR(701, "main", entry.DirectPrerequisite.Branch,
		stackFeaturePRBody(stackTestRegistry("bdonaldharris/builder-garage").Entries[0], "staging"))
	if got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil); got.Valid {
		t.Fatalf("propagation from a predecessor PR with the wrong base was accepted: %+v", got)
	}

	for name, body := range map[string]string{
		"wrong target":       stackMaintenanceBody(entry.Issue, "synchronize", "staging", 0, "other", ""),
		"wrong upstream":     stackMaintenanceBody(entry.Issue, "synchronize", "unregistered", 0, entry.Branch, ""),
		"non-temporary head": stackMaintenanceBody(entry.Issue, "synchronize", "staging", 0, entry.Branch, ""),
	} {
		t.Run(name, func(t *testing.T) {
			candidate := stackTestItem(body, entry)
			candidate.HeadRefName = "maintenance/stack-192/302-sync"
			if name == "non-temporary head" {
				candidate.HeadRefName = "feature/302"
			}
			result := validateStackDeclaration(candidate, "github.com", "bdonaldharris/builder-garage", "staging", "main", nil)
			if result.Valid {
				t.Fatalf("invalid maintenance accepted: %+v", result)
			}
		})
	}
}

func TestValidateStackCorrectionRequiresIssueCorrectionAndApproval(t *testing.T) {
	const correctionURL = "https://github.com/o/r/issues/302#issuecomment/10"
	const reviewURL = "https://github.com/o/r/issues/302#issuecomment/11"
	artifact := "method: garage-artifact-v1\n    object_format: sha1\n    stage: working-tree\n    integration_ref: " +
		strings.Repeat("a", 40) + "\n    base: " + strings.Repeat("b", 40) + "\n    tree: " +
		strings.Repeat("c", 40) + "\n    changeset: sha256:" + strings.Repeat("d", 64) + "\n    paths: 1\n"
	correction := workflowComment{URL: correctionURL, AuthorAssociation: "COLLABORATOR",
		Body: markerCorrection + "\n\n```yaml\ngarage_evidence: 1\nrecord: correction\nissue: 302\ncheckable:\n  artifact:\n    " + artifact + "asserted:\n  status: ready-for-re-review\n```\n"}
	review := workflowComment{URL: reviewURL, AuthorAssociation: "MEMBER",
		Body: markerReview + "\n\n```yaml\ngarage_evidence: 1\nrecord: review\nissue: 302\nresponds_to: \"" + correctionURL + "\"\ncheckable:\n  artifact:\n    " + artifact + "  matches_responds_to: true\nasserted:\n  answers: {acceptance_criteria_met: true, new_regression: false, works: true}\n  verdict: approved\n```\n"}
	issue := workflowIssue{Number: 302, Comments: workflowComments{Nodes: []workflowComment{correction, review}}}
	if problem := validateStackCorrection([]workflowIssue{issue}, 302, correctionURL, reviewURL); problem != "" {
		t.Fatalf("valid correction evidence rejected: %s", problem)
	}
	if problem := validateStackCorrection([]workflowIssue{issue}, 302, correctionURL, "https://github.com/o/r/issues/302#issuecomment/12"); problem == "" {
		t.Fatal("missing disposition accepted")
	}
	if problem := validateStackCorrection([]workflowIssue{issue}, 301, correctionURL, reviewURL); problem == "" {
		t.Fatal("correction evidence from a different stack entry was accepted")
	}
	mismatchedIssue := issue
	mismatchedIssue.Comments.Nodes = append([]workflowComment(nil), issue.Comments.Nodes...)
	mismatchedIssue.Comments.Nodes[1].Body = strings.Replace(
		mismatchedIssue.Comments.Nodes[1].Body,
		"changeset: sha256:"+strings.Repeat("d", 64),
		"changeset: sha256:"+strings.Repeat("e", 64), 1)
	if problem := validateStackCorrection([]workflowIssue{mismatchedIssue}, 302, correctionURL, reviewURL); problem == "" {
		t.Fatal("a review of a different correction artifact was accepted")
	}

	entry := stackTestRegistry("o/r").Entries[1]
	item := stackTestItem(stackMaintenanceBody(entry.Issue, "correction", entry.Branch, 0, entry.Branch,
		"correction_record: \""+correctionURL+"\"\nagent2_disposition: \""+reviewURL+"\"\n"), entry)
	item.BaseRefName = entry.Branch
	item.HeadRefName = "maintenance/stack-192/302-correction"
	if got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", []workflowIssue{issue}); !got.Valid {
		t.Fatalf("valid correction maintenance was rejected: %+v", got)
	}
	item.Body = stackMaintenanceBody(entry.Issue, "correction", entry.Branch, 0, entry.Branch,
		"correction_record: \""+correctionURL+"\"\nagent2_disposition: \"https://github.com/o/r/issues/302#issuecomment-12\"\n")
	if got := validateStackDeclaration(item, "github.com", "bdonaldharris/builder-garage", "staging", "main", []workflowIssue{issue}); got.Valid {
		t.Fatalf("correction maintenance without applicable approval was accepted: %+v", got)
	}
}

func TestStackDeclarationReadsOnlyRequiredReferences(t *testing.T) {
	entry := stackTestRegistry("o/r").Entries[1]
	item := stackTestItem(stackDependencyBody(entry.Issue, entry), entry)
	item.DefaultBranch = "main"
	got := statusReferencesToFetch(item, "github.com", "bdonaldharris/builder-garage", "staging", "main")
	for _, want := range []int{entry.Issue, 700, 701} {
		if !containsInt(got, want) {
			t.Errorf("status references %v do not include #%d", got, want)
		}
	}
	for _, excluded := range []int{stackProgramIssue, stackPrerequisiteChangeIssue} {
		if containsInt(got, excluded) {
			t.Errorf("status references %v unexpectedly include issue #%d", got, excluded)
		}
	}
	thirdEntry := stackTestRegistry("bdonaldharris/builder-garage").Entries[2]
	third := stackTestItem(stackDependencyBody(thirdEntry.Issue, thirdEntry), thirdEntry)
	got = statusReferencesToFetch(third, "github.com", "bdonaldharris/builder-garage", "staging", "main")
	for _, want := range []int{700, 701, 702} {
		if !containsInt(got, want) {
			t.Errorf("full predecessor chain %v does not include PR #%d", got, want)
		}
	}
	ordinary := &workflowItem{Kind: workflowChangeKind, Body: stackFeatureWithoutDeclarationBody(entry.Issue)}
	ordinary.StackRegistryFetched = true
	ordinary.StackRegistryComments = []workflowComment{stackRegistryComment(stackTestRegistry("bdonaldharris/builder-garage"))}
	got = statusReferencesToFetch(ordinary, "github.com", "bdonaldharris/builder-garage", "staging", "main")
	for _, want := range []int{entry.Issue, 700, 701} {
		if !containsInt(got, want) {
			t.Errorf("undeclared candidate references %v do not include #%d", got, want)
		}
	}
	otherRepository := statusReferencesToFetch(ordinary, "github.com", "other/repo", "staging", "main")
	for _, excluded := range []int{stackProgramIssue, stackPrerequisiteChangeIssue, 700, 701} {
		if containsInt(otherRepository, excluded) {
			t.Errorf("unrelated repository stack references %v unexpectedly include #%d", otherRepository, excluded)
		}
	}
}

// The follow-up query is only ever sent to a mocked gh in tests, so its
// structure is checked here: an unbalanced selection is rejected by GitHub
// and silently drops every governing-issue record.
func TestStatusQueriesHaveBalancedSelections(t *testing.T) {
	queries := map[string]string{"primary": statusQuery}
	for _, count := range []int{1, 3, maxGoverningIssues + stackEntryCount} {
		queries[fmt.Sprintf("follow-up for %d references", count)] = governingIssuesQuery(count)
	}
	for name, query := range queries {
		depth := 0
		for _, r := range query {
			switch r {
			case '{':
				depth++
			case '}':
				depth--
			}
			if depth < 0 {
				break
			}
		}
		if depth != 0 {
			t.Errorf("%s query has unbalanced braces (depth %d at end)", name, depth)
		}
	}
	// Each alias must hold both type conditions as siblings.
	follow := governingIssuesQuery(1)
	issue := strings.Index(follow, "... on Issue {")
	comments := strings.Index(follow, "comments(last: 100) {")
	timeline := strings.Index(follow, "timelineItems(")
	if issue < 0 || comments < issue || timeline < comments {
		t.Fatalf("follow-up query does not select issue comments and timeline: %s", follow)
	}
	if depth := strings.Count(follow[comments:timeline], "{") - strings.Count(follow[comments:timeline], "}"); depth != 0 {
		t.Errorf("the issue timeline is nested inside the comments selection (depth %d)", depth)
	}
}

func TestFetchStatusReferencesReadsRegistryAndPredecessorPR(t *testing.T) {
	registry := stackRegistryComment(stackTestRegistry("o/r"))
	response, err := json.Marshal(map[string]any{
		"data": map[string]any{"repository": map[string]any{
			"i0": map[string]any{"__typename": "Issue", "number": 302, "url": "https://github.com/o/r/issues/302",
				"state": "OPEN", "comments": map[string]any{"nodes": []any{}}},
			"i1": map[string]any{"__typename": "Issue", "number": 192, "url": "https://github.com/o/r/issues/192",
				"state": "OPEN", "comments": map[string]any{"nodes": []any{map[string]any{
					"url": registry.URL, "body": registry.Body, "authorAssociation": "OWNER",
				}}}},
			"i2": map[string]any{"__typename": "PullRequest", "number": 701, "url": "https://github.com/o/r/pull/701",
				"state": "MERGED", "merged": true, "mergedAt": "2026-01-02T09:00:00Z",
				"baseRefName": "staging", "headRefName": "gitlab-stack/issue-301",
				"headRepository": map[string]any{"nameWithOwner": "someone/fork"},
				"baseRepository": map[string]any{"nameWithOwner": "o/r"}},
			"i3": map[string]any{"__typename": "PullRequest", "number": 900, "url": "https://github.com/o/r/pull/900",
				"state": "OPEN", "baseRefName": "staging", "headRefName": "other"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	issues, prs, notes, err := fetchStatusReferences(context.Background(), func(_ context.Context, args ...string) ([]byte, []byte, error) {
		called = true
		query := strings.Join(args, " ")
		for _, want := range []string{
			"n0=302", "n1=192", "n2=701", "n3=900", "baseRefName", "headRefName", "mergedAt",
		} {
			if !strings.Contains(query, want) {
				t.Errorf("follow-up query %q does not contain %q", query, want)
			}
		}
		return response, nil, nil
	}, ghRepository{Host: "github.com", Owner: "o", Name: "r"}, []int{302, 192, 701, 900}, map[int]bool{701: true})
	if err != nil {
		t.Fatalf("fetchStatusReferences: %v", err)
	}
	if !called || len(issues) != 2 || len(prs) != 2 || prs[701].HeadRefName != "gitlab-stack/issue-301" ||
		len(notes) != 1 || notes[0] != "#900 is a pull request, not an issue" ||
		!strings.Contains(issues[1].Comments.Nodes[0].Body, stackRegistryMarker) {
		t.Fatalf("references = issues %d, prs %+v, notes %v", len(issues), prs, notes)
	}
	if prs[701].MergedAt == nil || !prs[701].Merged || prs[701].BaseRefName != "staging" ||
		prs[701].HeadRepository != "someone/fork" || prs[701].BaseRepository != "o/r" || prs[900].HeadRepository != "" {
		t.Fatalf("merged predecessor facts were not decoded: %+v", prs[701])
	}
}

func TestStatusValidStackKeepsImplementationGatesAndRetargetingInvalidatesEquivalence(t *testing.T) {
	registry := stackTestRegistry("o/r")
	entry := registry.Entries[1]
	item := stackTestItem(stackDependencyBody(entry.Issue, entry), entry)
	item.BodyIssues = []workflowIssue{{Number: entry.Issue}}
	text := renderItem(t, item, stagingConfig(), 2)
	assertContains(t, text,
		"stack dependency declaration matches registry entry",
		"trigger 1 (equivalence not shown) APPLIES",
		"NO approving issue-recorded implementation review",
		"Environment / Deployment Requirements")
	assertNotContains(t, text, "implementation-review gates do not apply")

	head := strings.Repeat("c", 40)
	item.BaseRefOid = strings.Repeat("b", 40)
	record := workflowRecord{
		Comment: workflowComment{URL: "https://github.com/o/r/pull/900#issuecomment/1"},
		Kind:    recordEvidence,
		Counted: true,
		Block: &evidenceBlock{
			Record: "equivalence",
			Checkable: evidenceCheckable{Artifact: &evidenceArtifact{
				Method: "garage-artifact-v1", ObjectFormat: "sha1", Stage: "commit",
				IntegrationRef: strings.Repeat("a", 40), Base: strings.Repeat("a", 40),
				Tree: strings.Repeat("b", 40), Changeset: "sha256:" + strings.Repeat("d", 64),
				Paths: intPointer(1), Commit: head,
			}},
		},
	}
	in := statusInputs{Item: item, Recompute: notLocal, kind: &prKind{stack: true}}
	gates := &statusSection{}
	if got := evaluateHeadIdentity(gates, in, []workflowRecord{record}, head); got != "" {
		t.Fatalf("stale stack target returned changeset %q", got)
	}
	if evaluateEquivalence(gates, []workflowRecord{record}, nil, nil, "", head) != true {
		t.Fatal("retargeted artifact did not require a separate PR review")
	}
	item.BaseRefOid = strings.Repeat("a", 40)
	approval := workflowRecord{
		Comment: workflowComment{URL: "https://github.com/o/r/issues/302#issuecomment/2"},
		Kind:    recordEvidence,
		Counted: true,
		Block: &evidenceBlock{
			Record: "review",
			Checkable: evidenceCheckable{Artifact: &evidenceArtifact{
				Method: "garage-artifact-v1", ObjectFormat: "sha1", Stage: "working-tree",
				IntegrationRef: item.BaseRefOid, Base: strings.Repeat("a", 40),
				Tree: strings.Repeat("b", 40), Changeset: "sha256:" + strings.Repeat("e", 64),
				Paths: intPointer(1),
			}},
		},
	}
	in.Item = item
	headChangeset := evaluateHeadIdentity(gates, in, []workflowRecord{record}, head)
	if !evaluateEquivalence(gates, []workflowRecord{record},
		[]approvalRecord{{rec: &approval, what: "issue review"}},
		&approvalRecord{rec: &approval, what: "issue review"}, headChangeset, head) {
		t.Fatal("changed stack artifact did not require a separate PR review")
	}
	var report strings.Builder
	if err := (&statusReport{sections: []*statusSection{gates}}).render(&report); err != nil {
		t.Fatal(err)
	}
	assertContains(t, report.String(),
		"the recorded integration_ref does not match the current target branch commit",
		"trigger 1 (equivalence not shown) APPLIES",
		"differs from the latest approval's")
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func intPointer(value int) *int { return &value }

// A complete branch plan must bootstrap without inventing future PRs. Each
// dependent gains topology eligibility only after its real predecessor is bound.
func TestStackRegistryIncrementalBootstrap(t *testing.T) {
	registry := stackTestRegistry(stackTestRepository)
	for i := 1; i < len(registry.Entries); i++ {
		registry.Entries[i].DirectPrerequisite.PullRequest = nil
	}
	prs := stackTestItem("", registry.Entries[0]).StatusPRs
	delete(prs, 701)
	validate := func(item *workflowItem) stackValidation {
		return validateStackDeclaration(item, "github.com", stackTestRepository, "staging", "main", nil)
	}
	for i := range registry.Entries {
		entry := registry.Entries[i]
		if err := validateStackRegistry(registry, "github.com", stackTestRepository, "staging", "main"); err != nil {
			t.Fatalf("partial plan at sequence %d: %v", i+1, err)
		}
		if i > 0 {
			// A declaration cannot supply a missing registry binding, even when
			// GitHub already returned the claimed PR and matching branch.
			claimed := entry
			predecessorNumber := 700 + i
			claimed.DirectPrerequisite.PullRequest = &predecessorNumber
			candidate := stackTestSubject(stackTestOpenPR(701+i, entry.DirectPrerequisite.Branch,
				entry.Branch, stackDependencyBody(entry.Issue, claimed)), prs, registry)
			if got := validate(candidate); got.Valid || !strings.Contains(got.Problem, "no direct-prerequisite PR") {
				t.Fatalf("unbound sequence %d = %+v", i+1, got)
			}
			for _, number := range stackPRReferencesToFetch(candidate, "github.com", stackTestRepository, "staging", "main") {
				if number < 1 || number >= predecessorNumber {
					t.Fatalf("unbound entry requested unregistered prerequisite %d", number)
				}
			}
			undeclared := *candidate
			undeclared.Body = stackFeatureWithoutDeclarationBody(entry.Issue)
			undeclared.BaseRefName = "staging"
			if got := validate(&undeclared); got.Valid || got.Unknown || !got.Present {
				t.Fatalf("planned entry escaped enrollment through ordinary E1 path: %+v", got)
			}
			for _, purpose := range []string{"correction", "synchronize"} {
				maintenance := stackTestSubject(stackTestOpenPR(800+i, entry.Branch, "maintenance/stack-192/bootstrap",
					stackMaintenanceBody(entry.Issue, purpose, "staging", 0, entry.Branch, "")), prs, registry)
				if got := validate(maintenance); got.Valid || !strings.Contains(got.Problem, "no direct-prerequisite PR") {
					t.Fatalf("unbound %s sequence %d = %+v", purpose, i+1, got)
				}
			}
			registry.Entries[i].DirectPrerequisite.PullRequest = &predecessorNumber
			entry = registry.Entries[i]
		}
		pr := stackTestOpenPR(701+i, entry.DirectPrerequisite.Branch, entry.Branch, stackDependencyBody(entry.Issue, entry))
		item := stackTestSubject(pr, prs, registry)
		if got := validate(item); !got.Valid {
			t.Fatalf("bound sequence %d = %+v", i+1, got)
		}
		maintenance := stackTestSubject(stackTestOpenPR(800+i, entry.Branch, "maintenance/stack-192/bootstrap",
			stackMaintenanceBody(entry.Issue, "synchronize", "staging", 0, entry.Branch, "")), prs, registry)
		if got := validate(maintenance); !got.Valid {
			t.Fatalf("bound maintenance sequence %d = %+v", i+1, got)
		}
		for _, number := range stackPRReferencesToFetch(item, "github.com", stackTestRepository, "staging", "main") {
			if number < 1 || number > 700+i {
				t.Fatalf("sequence %d fetched nonexistent/future prerequisite %d", i+1, number)
			}
		}
		if i > 0 {
			predecessor := prs[700+i]
			bad := predecessor
			bad.HeadRefName = "unregistered/branch"
			prs[700+i] = bad
			if got := validate(stackTestSubject(pr, prs, registry)); got.Valid {
				t.Fatalf("sequence %d accepted mismatched real predecessor", i+1)
			}
			delete(prs, 700+i)
			if got := validate(stackTestSubject(pr, prs, registry)); got.Valid {
				t.Fatalf("sequence %d accepted unread predecessor", i+1)
			}
			prs[700+i] = predecessor
		}
		prs[pr.Number] = pr
	}
}

func TestStackRegistryOptionalPRContract(t *testing.T) {
	for _, test := range []struct {
		name  string
		index int
		value *int
	}{
		{name: "first prerequisite is mandatory", index: 0},
		{name: "zero is not a PR", index: 2, value: intPointer(0)},
		{name: "negative is not a PR", index: 2, value: intPointer(-1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := stackTestRegistry(stackTestRepository)
			registry.Entries[test.index].DirectPrerequisite.PullRequest = test.value
			record := latestStackRegistry([]workflowComment{stackRegistryComment(registry)})
			if record.Registry == nil {
				t.Fatalf("parse: %s", record.Problem)
			}
			if err := validateStackRegistry(record.Registry, "github.com", stackTestRepository, "staging", "main"); err == nil {
				t.Fatal("invalid prerequisite accepted")
			}
		})
	}
	registry := stackTestRegistry(stackTestRepository)
	registry.Entries[2].DirectPrerequisite.PullRequest = nil
	comment := stackRegistryComment(registry)
	for _, suffix := range []string{"", "      pull_request: null\n"} {
		candidate := comment
		candidate.Body = strings.Replace(candidate.Body, "      branch: \"gitlab-stack/issue-302\"\n", "      branch: \"gitlab-stack/issue-302\"\n"+suffix, 1)
		record := latestStackRegistry([]workflowComment{candidate})
		if record.Registry == nil {
			t.Fatalf("optional reference parse: %s", record.Problem)
		}
		if err := validateStackRegistry(record.Registry, "github.com", stackTestRepository, "staging", "main"); err != nil {
			t.Fatalf("optional reference: %v", err)
		}
		if record.Registry.Entries[2].DirectPrerequisite.PullRequest != nil {
			t.Fatal("missing reference became a PR")
		}
	}
}
