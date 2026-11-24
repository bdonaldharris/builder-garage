package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func gitlabStatusReportFor(ctx context.Context, t workflowTarget, cfg *projectConfig, env statusEnv, repo repoIdentity) (*statusReport, error) {
	f := effectiveForge(*proj(cfg).Forge)
	if err := validateGitlabTarget(t, f); err != nil {
		return nil, err
	}
	host, _ := url.Parse(f.Endpoint)
	t.Host = host.Host
	if t.Project == "" {
		t.Project = f.Project
	}
	var provider statusProvider = gitlabStatusAdapter{Runner: env.GLab, Forge: f}
	item, reads, err := provider.Read(ctx, t, cfg)
	if err != nil {
		r := &statusReport{provider: "gitlab", target: t, header: []string{"GitLab status unavailable", "", "GITLAB WAS NOT CONSULTED SUCCESSFULLY: " + err.Error() + ".", "No workflow state or eligible action is established; no GitHub fallback or local history is used."}}
		addConfigSection(r, statusInputs{Item: &workflowItem{}, Config: cfg})
		return r, err
	}
	now := time.Now()
	if env.Now != nil {
		now = env.Now()
	}
	in := statusInputs{Item: item, Config: cfg, Reads: reads, ReadAt: now}
	// Foreign-project commits must not be recomputed in the origin checkout.
	if t.Project == f.Project {
		in.Recompute = func(ref, commit string) (string, string) {
			return recomputeCommitChangeset(ctx, env.Git, repo.TopLevel, ref, commit)
		}
	}
	report := buildGitlabStatusReport(in)
	if t.Project == f.Project {
		if historyRepo, err := historyRepository(ctx, repo.TopLevel, env.Git); err == nil {
			kind := "issue"
			if t.Kind == workflowChangeKind {
				kind = "mr"
			}
			addLocalHistory(report, env, historyRepo, kind, t.Number, false)
		}
	}
	return report, nil
}
func buildGitlabStatusReport(in statusInputs) *statusReport {
	it := in.Item
	o := it.Observation
	name, symbol := "issue", "#"
	if it.isPullRequest() {
		name, symbol = "merge request", "!"
	}
	r := &statusReport{provider: "gitlab", target: workflowTarget{Kind: it.Kind, Number: it.Number, Project: it.RepositoryIdentity, Host: it.RepositoryHost}, header: []string{fmt.Sprintf("Status of %s %s%d: %s", name, symbol, it.Number, oneLine(it.Title)), it.URL, "", fmt.Sprintf("GitLab records are authoritative. GitLab was read %d time(s), at %s.", in.Reads, in.ReadAt.UTC().Format(time.RFC3339)), "Observation writes nothing; no workflow or agent is advanced.", `"Asserted" values are recorded claims, not platform certification.`}}
	state := r.section("State")
	state.add(labelChecked, "%s is %s in %s", name, strings.ToLower(it.State), it.RepositoryIdentity)
	if it.isPullRequest() {
		if kind := declaredKind(it.Body); kind != "" {
			state.add(labelChecked, "declared workflow kind: %s; the declaration grants no destination exception", kind)
		}
		state.add(labelChecked, "source branch %s, head %s; actual target branch %s, target commit %s", it.HeadRefName, it.HeadRefOid, it.BaseRefName, it.BaseRefOid)
	}
	facts := r.section("GitLab observation and limitations")
	for _, fact := range o.Facts {
		if fact.Available {
			facts.add(labelChecked, "%s: %s", fact.Name, oneLine(fact.Value))
		} else {
			facts.add(labelChecked, "%s: %s", fact.Name, oneLine(fact.Problem))
		}
	}
	if o.DiscussionsResolved != nil {
		facts.add(labelChecked, "all resolvable discussions resolved: %t", *o.DiscussionsResolved)
	}
	if o.PipelineRequired != nil {
		facts.add(labelChecked, "project requires successful pipeline: %t", *o.PipelineRequired)
	} else {
		facts.add(labelChecked, "project pipeline policy: unavailable")
	}
	if o.ResolvedDiscussionsRequired != nil {
		facts.add(labelChecked, "project requires resolved discussions: %t", *o.ResolvedDiscussionsRequired)
	} else {
		facts.add(labelChecked, "project discussion policy: unavailable")
	}
	var records []workflowRecord
	var issueState gitlabIssueReview
	if it.isPullRequest() {
		records = gitlabMRRecords(it)
	} else {
		issueState = currentGitlabIssueReview(workflowIssue{Project: it.RepositoryIdentity, Number: it.Number, URL: it.URL, State: it.State, Comments: it.Comments})
		records = issueState.Records
	}
	addRecordsSection(r, "Workflow records on "+gitlabRecordLocation(it.RepositoryIdentity, it.Kind, it.Number), it.Comments, records)
	if !it.isPullRequest() {
		g := r.section("Implementation review (authoritative issue record)")
		next := "required current issue review is not established; inspect the current completion/disposition"
		if issueState.Problem == "" {
			next = evaluateImplementationReview(g, records)
		} else {
			g.add(labelChecked, "current issue review: %s; approval is not established", issueState.Problem)
			if !gitlabRecordsComplete(it.Comments, records) {
				g.add(labelChecked, "review evaluation: unavailable because workflow notes or author membership are incomplete")
			}
			if issueState.Completion == nil && gitlabRecordsComplete(it.Comments, records) {
				next = "Agent #1 implementation (application-implementation-workflow)"
			}
			if issueState.Completion != nil && issueState.Completion.Block != nil && issueState.Completion.Block.status() == "material-blocker" {
				next = "resolve the material blocker recorded at " + issueState.Completion.Comment.URL
			}
		}
		n := r.section("Next eligible action")
		switch {
		case it.State != "OPEN":
			n.add(labelInfo, "none: issue is %s", strings.ToLower(it.State))
		case !o.Complete:
			n.add(labelInfo, "required observation is incomplete; unavailable evidence grants no eligibility")
		case len(it.ClosedByChanges) > 0:
			n.add(labelInfo, "inspect the linked MR using a typed mr IID or canonical URL")
		default:
			n.add(labelInfo, "%s", next)
		}
	} else {
		states, complete := gitlabGoverningReviews(o)
		for _, state := range states {
			addRecordsSection(r, "Workflow records on governing "+gitlabRecordLocation(state.Issue.Project, workflowIssueKind, state.Issue.Number), state.Issue.Comments, state.Records)
		}
		gates := r.section("Gates for the current MR artifact")
		evaluateGitlabMRReviews(gates, in, records, states, complete)
		if it.Checks != nil && it.Checks.Commit == it.HeadRefOid && it.Checks.Available {
			gates.add(labelChecked, "observed current-source-head pipeline: %s; this is not certification of every required CI/merge policy", strings.ToLower(it.Checks.State))
		} else {
			gates.add(labelChecked, "current-source-head pipeline: unavailable")
		}
		if in.Config.Project.PRReviewTriggers != nil {
			for _, trigger := range *in.Config.Project.PRReviewTriggers {
				gates.add(labelConfig, "configured PR-review trigger %q: not evaluated (%s)", trigger.Name, oneLine(trigger.AppliesWhen))
			}
		}
		gates.add(labelChecked, "automated findings: unavailable as a complete native review feed; discussion notes are observed, never assumed absent findings")
		gates.add(labelChecked, "human acceptance: unavailable until qualified GitLab change-acceptor identity AND current-artifact native approval policy are established; native approval counts alone do not satisfy this gate")
		if !o.Complete {
			gates.add(labelChecked, "observation is incomplete; unavailable required evidence cannot pass a gate")
		}
		if !strings.Contains(it.Body, "Environment / Deployment Requirements") {
			gates.add(labelChecked, "MR description has no Environment / Deployment Requirements section")
		}
		next := r.section("Next eligible action")
		next.add(labelInfo, "no merge eligibility is established: complete any missing review/evidence, pipeline/policy and qualified human acceptance requirements; observation advances no workflow")
	}
	config := r.section("Project configuration")
	config.add(labelConfig, "valid; GitLab project %s; integration branch %s", it.RepositoryIdentity, effectiveIntegrationBranch(proj(in.Config)))
	return r
}
func gitlabRecordLocation(project, kind string, number int) string {
	return project + gitlabReferenceSymbol(kind) + fmt.Sprint(number)
}

func gitlabRecordsComplete(comments workflowComments, records []workflowRecord) bool {
	if comments.Truncated {
		return false
	}
	for _, record := range records {
		if record.Counted && record.Comment.CreatedAt.IsZero() {
			return false
		}
		if record.Comment.Provider == "gitlab" && record.Comment.Eligible == nil {
			return false
		}
	}
	return true
}
