package cli

import "context"

// githubStatusAdapter owns the query budget, transport and normalization.
type githubStatusAdapter struct {
	Runner     ghRunner
	Repository ghRepository
}

func (a githubStatusAdapter) Read(ctx context.Context, target workflowTarget, cfg *projectConfig) (*workflowItem, int, error) {
	item, err := fetchStatusItem(ctx, a.Runner, a.Repository, target.Number)
	reads := 1
	if err != nil {
		return nil, reads, err
	}
	integration, release := "", ""
	if cfg != nil {
		integration, release, _ = configuredBranches(statusInputs{Config: cfg})
	}
	repository := a.Repository.Owner + "/" + a.Repository.Name
	stackPRs := stackPRReferencesToFetch(item, a.Repository.Host, repository, integration, release)
	if numbers := statusReferencesToFetch(item, a.Repository.Host, repository, integration, release); len(numbers) > 0 {
		reads++
		expectedPRs := make(map[int]bool)
		for _, number := range stackPRs {
			expectedPRs[number] = true
		}
		issues, prs, notes, err := fetchStatusReferences(ctx, a.Runner, a.Repository, numbers, expectedPRs)
		if err != nil {
			item.BodyIssueNotes = append(item.BodyIssueNotes, "the related issues and pull requests needed for status could not be read: "+err.Error())
		}
		item.StatusPRs = prs
		governing := make(map[int]bool)
		for _, n := range governingIssueNumbers(item.Body) {
			governing[n] = true
		}
		item.BodyIssueNotes = append(item.BodyIssueNotes, notes...)
		mergeStatusReferenceIssues(item, issues, governing)
	}
	item.RepositoryIdentity, item.RepositoryHost = repository, a.Repository.Host
	return item, reads, nil
}

func githubKind(kind string) string {
	switch kind {
	case "Issue":
		return workflowIssueKind
	case "PullRequest":
		return workflowChangeKind
	}
	return ""
}
func normalizeGitHubActor(a *ghActor) *workflowActor {
	if a == nil {
		return nil
	}
	return &workflowActor{Login: a.Login, Bot: a.Typename == "Bot"}
}
func normalizeGitHubComments(c ghComments) workflowComments {
	out := workflowComments{TotalCount: c.TotalCount, Truncated: c.PageInfo.HasPreviousPage}
	for _, n := range c.Nodes {
		out.Nodes = append(out.Nodes, workflowComment{URL: n.URL, Body: n.Body, CreatedAt: n.CreatedAt, LastEditedAt: n.LastEditedAt, AuthorAssociation: n.AuthorAssociation, Author: normalizeGitHubActor(n.Author)})
	}
	return out
}
func normalizeGitHubRepository(r *ghRepositoryName) *workflowRepository {
	if r == nil {
		return nil
	}
	return &workflowRepository{NameWithOwner: r.NameWithOwner}
}
func normalizeGitHubReferences(t *struct {
	TotalCount int                `json:"totalCount"`
	Nodes      []ghCrossReference `json:"nodes"`
}) *workflowReferences {
	if t == nil {
		return nil
	}
	out := &workflowReferences{TotalCount: t.TotalCount}
	for _, r := range t.Nodes {
		n := workflowReference{IsCrossRepository: r.IsCrossRepository}
		if s := r.Source; s != nil {
			n.Source = &workflowReferenceSource{Kind: githubKind(s.Typename), Number: s.Number, URL: s.URL, State: s.State, Merged: s.Merged, MergedAt: s.MergedAt, Body: s.Body, BaseRefName: s.BaseRefName, HeadRefName: s.HeadRefName, BaseRepository: normalizeGitHubRepository(s.BaseRepository), HeadRepository: normalizeGitHubRepository(s.HeadRepository)}
		}
		out.Nodes = append(out.Nodes, n)
	}
	return out
}
func normalizeGitHubItem(g *ghItem) *workflowItem {
	w := &workflowItem{Kind: githubKind(g.Typename), Number: g.Number, Title: g.Title, URL: g.URL, State: g.State, ClosedAt: g.ClosedAt,
		Comments: normalizeGitHubComments(g.Comments), References: normalizeGitHubReferences(g.TimelineItems), Body: g.Body, Merged: g.Merged, MergedAt: g.MergedAt,
		BaseRefName: g.BaseRefName, BaseRefOid: g.BaseRefOid, HeadRefName: g.HeadRefName, HeadRefOid: g.HeadRefOid,
		BaseRepository: normalizeGitHubRepository(g.BaseRepository), HeadRepository: normalizeGitHubRepository(g.HeadRepository),
		DefaultBranch: g.DefaultBranch, StackRegistryFetched: g.StackRegistryFetched}
	for _, c := range g.StackRegistryComments {
		w.StackRegistryComments = append(w.StackRegistryComments, normalizeGitHubComments(ghComments{Nodes: []ghComment{c}}).Nodes[0])
	}
	if g.MergeCommit != nil {
		w.MergeCommit = &workflowCommit{Oid: g.MergeCommit.Oid}
	}
	if g.ClosedByPullRequestsReferences != nil {
		for _, c := range g.ClosedByPullRequestsReferences.Nodes {
			w.ClosedByChanges = append(w.ClosedByChanges, workflowLinkedChange{Number: c.Number, URL: c.URL, State: c.State, MergedAt: c.MergedAt})
		}
	}
	if g.ClosingIssuesReferences != nil {
		w.ClosingIssues = &workflowIssues{TotalCount: g.ClosingIssuesReferences.TotalCount}
		for _, i := range g.ClosingIssuesReferences.Nodes {
			w.ClosingIssues.Nodes = append(w.ClosingIssues.Nodes, workflowIssue{Number: i.Number, URL: i.URL, State: i.State, Comments: normalizeGitHubComments(i.Comments), References: normalizeGitHubReferences(i.TimelineItems)})
		}
	}
	if g.Reviews != nil {
		for _, r := range g.Reviews.Nodes {
			n := workflowReview{State: r.State, SubmittedAt: r.SubmittedAt, URL: r.URL, AuthorAssociation: r.AuthorAssociation, Author: normalizeGitHubActor(r.Author), CommentCount: r.Comments.TotalCount}
			if r.Commit != nil {
				n.Commit = &workflowCommit{Oid: r.Commit.Oid}
			}
			w.Reviews = append(w.Reviews, n)
		}
	}
	if g.Commits != nil && len(g.Commits.Nodes) > 0 {
		c := g.Commits.Nodes[0].Commit
		w.Checks = &workflowChecks{Commit: c.Oid}
		if c.StatusCheckRollup != nil {
			w.Checks.Available = true
			w.Checks.State = c.StatusCheckRollup.State
		}
	}
	return w
}
