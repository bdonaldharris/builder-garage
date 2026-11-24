package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Transport payloads stay in the GitLab adapter. Shared reports receive
// normalized facts with explicit completeness/capability information.
type glProject struct {
	ID                                        int    `json:"id"`
	Path                                      string `json:"path_with_namespace"`
	DefaultBranch                             string `json:"default_branch"`
	OnlyAllowMergeIfPipelineSucceeds          *bool  `json:"only_allow_merge_if_pipeline_succeeds"`
	OnlyAllowMergeIfAllDiscussionsAreResolved *bool  `json:"only_allow_merge_if_all_discussions_are_resolved"`
}
type glUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
}
type glPipeline struct {
	ProjectID int    `json:"project_id"`
	ID        int    `json:"id"`
	SHA       string `json:"sha"`
	Status    string `json:"status"`
	Ref       string `json:"ref"`
}
type glResource struct {
	ID              int        `json:"id"`
	IID             int        `json:"iid"`
	ProjectID       int        `json:"project_id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	WebURL          string     `json:"web_url"`
	State           string     `json:"state"`
	Confidential    *bool      `json:"confidential"`
	ClosedAt        *time.Time `json:"closed_at"`
	MergedAt        *time.Time `json:"merged_at"`
	SHA             string     `json:"sha"`
	MergeCommitSHA  string     `json:"merge_commit_sha"`
	SourceBranch    string     `json:"source_branch"`
	TargetBranch    string     `json:"target_branch"`
	SourceProjectID int        `json:"source_project_id"`
	TargetProjectID int        `json:"target_project_id"`
	DiffRefs        *struct {
		HeadSHA string `json:"head_sha"`
	} `json:"diff_refs"`
	HeadPipeline        *glPipeline `json:"head_pipeline"`
	DetailedMergeStatus *string     `json:"detailed_merge_status"`
}
type glNote struct {
	ID         int        `json:"id"`
	Body       string     `json:"body"`
	Author     glUser     `json:"author"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  *time.Time `json:"updated_at"`
	System     bool       `json:"system"`
	Resolvable bool       `json:"resolvable"`
	Resolved   *bool      `json:"resolved"`
}
type glDiscussion struct {
	ID    string   `json:"id"`
	Notes []glNote `json:"notes"`
}
type glMembership struct {
	ID          int  `json:"id"`
	AccessLevel *int `json:"access_level"`
}
type glApprovals struct {
	ApprovalsRequired *int `json:"approvals_required"`
	ApprovalsLeft     *int `json:"approvals_left"`
	ApprovedBy        []struct {
		User glUser `json:"user"`
	} `json:"approved_by"`
}

func (o *workflowObservation) fact(name, value string) {
	o.Facts = append(o.Facts, workflowFact{Name: name, Value: value, Available: true})
}
func (o *workflowObservation) unavailable(name string, err error) {
	o.Complete = false
	o.Facts = append(o.Facts, workflowFact{Name: name, Problem: err.Error()})
}

func (o *workflowObservation) governingUnavailable(name string, err error) {
	o.GoverningComplete = false
	o.unavailable(name, err)
}

type gitlabStatusAdapter struct {
	Runner glabRunner
	Forge  forgeConfig
}
type gitlabObservationReader struct {
	client      *gitlabClient
	observation *workflowObservation
	members     map[string]*int
	projects    map[int]glProject
}

func gitlabResourceKind(kind string) string {
	if kind == workflowChangeKind {
		return "merge_requests"
	}
	return "issues"
}
func gitlabURL(f forgeConfig, t workflowTarget) string {
	return effectiveForge(f).Endpoint + "/" + t.Project + "/-/" + gitlabResourceKind(t.Kind) + "/" + strconv.Itoa(t.Number)
}
func glState(s string) string {
	if s == "opened" {
		return "OPEN"
	}
	return strings.ToUpper(s)
}

func (a gitlabStatusAdapter) Read(ctx context.Context, t workflowTarget, cfg *projectConfig) (*workflowItem, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	f := effectiveForge(a.Forge)
	if f.Provider != "gitlab" {
		return nil, 0, errors.New("GitLab adapter requires explicit GitLab configuration")
	}
	if err := validateForge(f); err != nil {
		return nil, 0, err
	}
	if err := validateGitlabTarget(t, f); err != nil {
		return nil, 0, err
	}
	if t.Project == "" {
		t.Project = f.Project
	}
	c := newGitlabClient(a.Runner, f)
	c.projects[t.Project] = true
	o := &workflowObservation{Complete: true, GoverningComplete: true}
	reader := gitlabObservationReader{client: c, observation: o, members: map[string]*int{}, projects: map[int]glProject{}}
	var project glProject
	if _, err := c.get(ctx, "GET", "projects/"+url.PathEscape(t.Project), &project); err != nil {
		return nil, c.reads, err
	}
	if project.ID < 1 || project.Path != t.Project {
		return nil, c.reads, errors.New("unavailable: GitLab project identity did not match the selected project")
	}
	reader.projects[project.ID] = project
	o.PipelineRequired = project.OnlyAllowMergeIfPipelineSucceeds
	o.ResolvedDiscussionsRequired = project.OnlyAllowMergeIfAllDiscussionsAreResolved
	resource, err := reader.resource(ctx, t, project.ID)
	if err != nil {
		return nil, c.reads, err
	}
	item := &workflowItem{Provider: "gitlab", Kind: t.Kind, Number: t.Number, Title: resource.Title, URL: gitlabURL(f, t), State: glState(resource.State), Body: resource.Description,
		ClosedAt: resource.ClosedAt, MergedAt: resource.MergedAt, Merged: resource.State == "merged", RepositoryIdentity: t.Project, DefaultBranch: project.DefaultBranch, Observation: o}
	host, _ := url.Parse(f.Endpoint)
	item.RepositoryHost = host.Host
	if resource.Confidential != nil {
		o.fact("confidential", strconv.FormatBool(*resource.Confidential))
	} else {
		o.unavailable("confidential", errors.New("unavailable: GitLab did not return a confidentiality flag"))
	}
	item.Comments = reader.comments(ctx, t)
	refs, problems := gitlabReferences(resource.Description, forgeForProject(f, t.Project))
	for _, p := range problems {
		o.unavailable("reference", errors.New(p))
	}
	o.References = refs
	for _, ref := range refs {
		o.fact("reference", ref.Project+gitlabReferenceSymbol(ref.Kind)+strconv.Itoa(ref.Number))
	}
	if t.Kind == workflowChangeKind {
		item.HeadRefName, item.BaseRefName, item.HeadRefOid = resource.SourceBranch, resource.TargetBranch, resource.SHA
		if !objectNamePattern.MatchString(resource.SHA) || resource.DiffRefs != nil && resource.DiffRefs.HeadSHA != resource.SHA {
			o.unavailable("head", errors.New("unavailable: GitLab did not establish a consistent MR head SHA"))
			item.HeadRefOid = ""
		}
		if resource.TargetProjectID != project.ID {
			return nil, c.reads, errors.New("unavailable: MR target project identity does not match the requested project")
		}
		item.BaseRepository = &workflowRepository{NameWithOwner: project.Path}
		if source, err := reader.project(ctx, resource.SourceProjectID); err == nil {
			item.HeadRepository = &workflowRepository{NameWithOwner: source.Path}
			o.fact("source project", source.Path)
		} else {
			o.unavailable("source project", err)
		}
		var branch struct {
			Name   string `json:"name"`
			Commit struct {
				ID string `json:"id"`
			} `json:"commit"`
		}
		if resource.TargetBranch == "" {
			o.unavailable("target commit", errors.New("unavailable: MR target branch absent"))
		} else if _, err := c.get(ctx, "GET", c.resource(t.Project, "repository", "branches", url.PathEscape(resource.TargetBranch)), &branch); err != nil {
			o.unavailable("target commit", err)
		} else if branch.Name != resource.TargetBranch || !objectNamePattern.MatchString(branch.Commit.ID) {
			o.unavailable("target commit", errors.New("unavailable: target branch identity is inconsistent"))
		} else {
			item.BaseRefOid = branch.Commit.ID
			o.fact("target commit", branch.Commit.ID)
		}
		if resource.MergeCommitSHA != "" && objectNamePattern.MatchString(resource.MergeCommitSHA) {
			item.MergeCommit = &workflowCommit{Oid: resource.MergeCommitSHA}
		}
		if resource.DetailedMergeStatus != nil {
			o.fact("native merge status", *resource.DetailedMergeStatus)
		} else {
			o.unavailable("native merge status", errors.New("unavailable: merge status not returned"))
		}
		reader.pipeline(ctx, item, t, resource)
		reader.approvals(ctx, t)
		refs = reader.governingReferences(ctx, t, resource.Description)
		o.GoverningTargets = refs
		for _, ref := range refs {
			if ref.Kind == workflowIssueKind {
				if issue, ok := reader.issue(ctx, ref); ok {
					o.Governing = append(o.Governing, issue)
				} else {
					o.governingUnavailable("required governing issue "+ref.Project+"#"+strconv.Itoa(ref.Number), errors.New("unavailable: required issue records could not be read"))
				}
			}
		}
		// A concurrent update invalidates observations tied to the earlier MR
		// snapshot. Do not attach old approvals/pipelines to a new current head.
		latest, e := reader.resource(ctx, t, project.ID)
		if e != nil || latest.SHA != resource.SHA || latest.SourceBranch != resource.SourceBranch || latest.TargetBranch != resource.TargetBranch || latest.SourceProjectID != resource.SourceProjectID || latest.TargetProjectID != resource.TargetProjectID || latest.State != resource.State || latest.Description != resource.Description {
			o.unavailable("MR snapshot", errors.New("unavailable: MR changed during observation or its final snapshot could not be read"))
			item.Checks = nil
			item.BaseRefOid = ""
		}
	} else {
		linked, partial, e := gitlabList[glResource](ctx, c, c.resource(t.Project, "issues", strconv.Itoa(t.Number), "closed_by"))
		if partial || e != nil {
			o.unavailable("closing merge requests", observationProblem(e, "truncated: closing MR list is incomplete"))
		}
		for _, mr := range linked {
			link, valid := reader.linkedTarget(ctx, mr, workflowChangeKind)
			if !valid {
				continue
			}
			item.ClosedByChanges = append(item.ClosedByChanges, workflowLinkedChange{Number: link.Number, URL: gitlabURL(f, link), State: glState(mr.State), MergedAt: mr.MergedAt})
			o.fact("closing merge request", link.Project+"!"+strconv.Itoa(link.Number)+" ("+mr.State+")")
		}
	}
	return item, c.reads, nil
}
func forgeForProject(f forgeConfig, project string) forgeConfig { f.Project = project; return f }
func gitlabReferenceSymbol(kind string) string {
	if kind == workflowChangeKind {
		return "!"
	}
	return "#"
}
func observationProblem(err error, fallback string) error {
	if err != nil {
		return err
	}
	return errors.New(fallback)
}
func gitlabResourceURLMatches(raw string, t workflowTarget, f forgeConfig) bool {
	urlTarget, err := parseGitlabTarget([]string{raw}, f)
	return err == nil && urlTarget.Kind == t.Kind && urlTarget.Number == t.Number && urlTarget.Project == t.Project
}
func (r *gitlabObservationReader) resource(ctx context.Context, t workflowTarget, projectID int) (glResource, error) {
	var resource glResource
	_, err := r.client.get(ctx, "GET", r.client.resource(t.Project, gitlabResourceKind(t.Kind), strconv.Itoa(t.Number)), &resource)
	if err != nil {
		return resource, err
	}
	if resource.IID != t.Number || resource.ProjectID != projectID || resource.ID < 1 || (resource.WebURL != "" && !gitlabResourceURLMatches(resource.WebURL, t, r.client.forge)) {
		return resource, errors.New("unavailable: GitLab resource identity did not match the selected kind/project/IID")
	}
	switch resource.State {
	case "opened", "closed":
	case "merged", "locked":
		if t.Kind != workflowChangeKind {
			return resource, errors.New("unsupported: issue state")
		}
	default:
		return resource, errors.New("unsupported: GitLab resource state")
	}
	return resource, nil
}
func (r *gitlabObservationReader) project(ctx context.Context, id int) (glProject, error) {
	if project, ok := r.projects[id]; ok {
		return project, nil
	}
	if id < 1 {
		return glProject{}, errors.New("unavailable: project identity is absent")
	}
	key := strconv.Itoa(id)
	r.client.projects[key] = true
	var project glProject
	_, err := r.client.get(ctx, "GET", "projects/"+key, &project)
	if err != nil {
		return project, err
	}
	if project.ID != id || !validForgeProject(project.Path, "gitlab") {
		return project, errors.New("unavailable: related project identity is inconsistent")
	}
	r.projects[id] = project
	r.client.projects[project.Path] = true
	return project, nil
}
func (r *gitlabObservationReader) membership(ctx context.Context, project string, user glUser) *int {
	key := project + ":" + strconv.Itoa(user.ID)
	if value, ok := r.members[key]; ok {
		return value
	}
	r.members[key] = nil
	if user.ID < 1 {
		r.observation.unavailable("membership", errors.New("unavailable: note author identity is missing"))
		return nil
	}
	var member glMembership
	_, err := r.client.get(ctx, "GET", r.client.resource(project, "members", "all", strconv.Itoa(user.ID)), &member)
	if err != nil || member.ID != user.ID || member.AccessLevel == nil {
		r.observation.unavailable("membership", observationProblem(err, "unavailable: author membership was not established"))
		return nil
	}
	r.members[key] = member.AccessLevel
	r.observation.fact("membership", fmt.Sprintf("%s in %s: access level %d", user.Username, project, *member.AccessLevel))
	return member.AccessLevel
}
func (r *gitlabObservationReader) comments(ctx context.Context, t workflowTarget) workflowComments {
	base := r.client.resource(t.Project, gitlabResourceKind(t.Kind), strconv.Itoa(t.Number))
	notes, partial, err := gitlabList[glNote](ctx, r.client, base+"/notes")
	if partial || err != nil {
		r.observation.unavailable("notes", observationProblem(err, "truncated: note list incomplete"))
	}
	discussions, dPartial, dErr := gitlabList[glDiscussion](ctx, r.client, base+"/discussions")
	if dPartial || dErr != nil {
		r.observation.unavailable("discussions", observationProblem(dErr, "truncated: discussions incomplete"))
	}
	resolved := true
	resolutionKnown := !dPartial && dErr == nil
	for _, thread := range discussions {
		for _, note := range thread.Notes {
			notes = append(notes, note)
			if note.Resolvable {
				if note.Resolved == nil {
					resolutionKnown = false
				} else if !*note.Resolved {
					resolved = false
				}
			}
		}
	}
	if resolutionKnown && t.Kind == workflowChangeKind {
		r.observation.DiscussionsResolved = &resolved
	}
	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].CreatedAt.Equal(notes[j].CreatedAt) {
			return notes[i].ID < notes[j].ID
		}
		return notes[i].CreatedAt.Before(notes[j].CreatedAt)
	})
	comments := workflowComments{Truncated: partial || dPartial || err != nil || dErr != nil}
	versions := map[int]glNote{}
	conflicting := map[int]bool{}
	for _, note := range notes {
		if old, ok := versions[note.ID]; ok && (old.Body != note.Body || old.Author.ID != note.Author.ID || !old.CreatedAt.Equal(note.CreatedAt)) {
			conflicting[note.ID] = true
		}
		versions[note.ID] = note
	}
	if len(conflicting) > 0 {
		comments.Truncated = true
		r.observation.unavailable("notes", errors.New("unavailable: conflicting note versions observed; affected records are not counted"))
	}
	seen := map[int]bool{}
	for _, note := range notes {
		if note.System || note.ID < 1 || seen[note.ID] {
			continue
		}
		seen[note.ID] = true
		eligible := false
		eligibilityKnown := false
		eligibility := "unavailable: author membership not read"
		// Only workflow records need membership reads. Ordinary notes remain
		// observable without turning every discussion into an N+1 authority query.
		if _, _, record := workflowMarker(note.Body); record {
			if access := r.membership(ctx, t.Project, note.Author); access != nil {
				eligible = *access >= 30
				eligibilityKnown = true
				eligibility = fmt.Sprintf("checked project access level %d (Developer minimum 30)", *access)
			}
		}
		if conflicting[note.ID] {
			eligible = false
			eligibilityKnown = false
			eligibility = "unavailable: conflicting note versions"
		}
		edited := note.UpdatedAt
		if edited != nil && !edited.After(note.CreatedAt) {
			edited = nil
		}
		var eligibilityValue *bool
		if eligibilityKnown {
			eligibilityValue = &eligible
		}
		comments.Nodes = append(comments.Nodes, workflowComment{Provider: "gitlab", Eligible: eligibilityValue, Eligibility: eligibility, URL: gitlabURL(r.client.forge, t) + "#note_" + strconv.Itoa(note.ID), Body: note.Body, CreatedAt: note.CreatedAt, LastEditedAt: edited, Author: &workflowActor{Login: note.Author.Username, Bot: note.Author.Bot}})
	}
	comments.TotalCount = len(comments.Nodes)
	return comments
}
func (r *gitlabObservationReader) linkedTarget(ctx context.Context, resource glResource, kind string) (workflowTarget, bool) {
	t, err := parseGitlabTarget([]string{resource.WebURL}, r.client.forge)
	if err != nil || t.Kind != kind || t.Number != resource.IID {
		r.observation.unavailable("relationship", errors.New("unsupported or inaccessible: related resource is outside enabled project/kind identity"))
		return t, false
	}
	if resource.ProjectID < 1 {
		r.observation.unavailable("relationship", errors.New("unavailable: related project ID absent"))
		return t, false
	}
	for id, known := range r.projects {
		if known.Path == t.Project && id != resource.ProjectID {
			r.observation.unavailable("relationship", errors.New("unavailable: related project ID contradicts its canonical project path"))
			return t, false
		}
	}
	project, err := r.project(ctx, resource.ProjectID)
	if err != nil {
		r.observation.unavailable("relationship", err)
		return t, false
	}
	if project.Path != t.Project {
		r.observation.unavailable("relationship", errors.New("unavailable: contradictory related project identity"))
		return t, false
	}
	return t, true
}
func (r *gitlabObservationReader) governingReferences(ctx context.Context, t workflowTarget, body string) []workflowTarget {
	var targets []workflowTarget
	seen := map[workflowTargetKey]bool{}
	add := func(ref workflowTarget) {
		key := workflowTargetKey{ref.Project, ref.Kind, ref.Number}
		if seen[key] {
			return
		}
		seen[key] = true
		if ref.Kind != workflowIssueKind {
			r.observation.governingUnavailable("governing reference", errors.New("unsupported: a governing issue reference must identify an issue, not an MR"))
			return
		}
		if len(targets) >= 12 {
			r.observation.governingUnavailable("governing references", errors.New("truncated: governing issue read limit reached"))
			return
		}
		targets = append(targets, ref)
	}
	section, _ := bodySection(body, "Governing issue")
	declared, problems := gitlabReferences(section, forgeForProject(r.client.forge, t.Project))
	for _, problem := range problems {
		r.observation.governingUnavailable("governing reference", errors.New(problem))
	}
	for _, ref := range declared {
		add(ref)
	}
	linked, partial, err := gitlabList[glResource](ctx, r.client, r.client.resource(t.Project, "merge_requests", strconv.Itoa(t.Number), "closes_issues"))
	if partial || err != nil {
		r.observation.governingUnavailable("governing relationships", observationProblem(err, "truncated: closing issue list incomplete"))
	}
	for _, resource := range linked {
		if ref, ok := r.linkedTarget(ctx, resource, workflowIssueKind); ok {
			add(ref)
		} else {
			r.observation.GoverningComplete = false
		}
	}
	return targets
}
func (r *gitlabObservationReader) issue(ctx context.Context, t workflowTarget) (workflowIssue, bool) {
	r.client.projects[t.Project] = true
	var project glProject
	for _, known := range r.projects {
		if known.Path == t.Project {
			project = known
			break
		}
	}
	// Canonical related project identity is checked before its issue is read.
	if project.ID == 0 {
		if _, err := r.client.get(ctx, "GET", "projects/"+url.PathEscape(t.Project), &project); err != nil {
			r.observation.governingUnavailable("governing issue", err)
			return workflowIssue{}, false
		}
	}
	if project.ID < 1 || project.Path != t.Project {
		r.observation.governingUnavailable("governing issue", errors.New("unavailable: governing project mismatch"))
		return workflowIssue{}, false
	}
	r.projects[project.ID] = project
	resource, err := r.resource(ctx, t, project.ID)
	if err != nil {
		r.observation.governingUnavailable("governing issue", err)
		return workflowIssue{}, false
	}
	return workflowIssue{Project: t.Project, Number: t.Number, URL: gitlabURL(r.client.forge, t), State: glState(resource.State), Comments: r.comments(ctx, t)}, true
}
func (r *gitlabObservationReader) pipeline(ctx context.Context, item *workflowItem, t workflowTarget, mr glResource) {
	if mr.HeadPipeline == nil || mr.HeadPipeline.ID < 1 {
		r.observation.unavailable("pipeline", errors.New("unavailable: MR head pipeline absent"))
		return
	}
	pipelineProject := t.Project
	pipelineProjectID := mr.HeadPipeline.ProjectID
	if pipelineProjectID != 0 && pipelineProjectID != mr.ProjectID {
		if pipelineProjectID != mr.SourceProjectID {
			r.observation.unavailable("pipeline", errors.New("unavailable: pipeline project is not the MR source or target"))
			return
		}
		source, err := r.project(ctx, pipelineProjectID)
		if err != nil {
			r.observation.unavailable("pipeline", err)
			return
		}
		pipelineProject = source.Path
	}
	var pipeline glPipeline
	if _, err := r.client.get(ctx, "GET", r.client.resource(pipelineProject, "pipelines", strconv.Itoa(mr.HeadPipeline.ID)), &pipeline); err != nil {
		r.observation.unavailable("pipeline", err)
		return
	}
	r.observation.fact("observed pipeline", fmt.Sprintf("%d: %s, SHA %s, ref %s", pipeline.ID, pipeline.Status, pipeline.SHA, pipeline.Ref))
	if pipeline.ProjectID != 0 && pipeline.ProjectID != mr.ProjectID && pipeline.ProjectID != mr.SourceProjectID {
		r.observation.unavailable("pipeline", errors.New("unavailable: returned pipeline project does not belong to the MR"))
		return
	}
	if pipelineProjectID != 0 && pipeline.ProjectID != 0 && pipelineProjectID != pipeline.ProjectID {
		r.observation.unavailable("pipeline", errors.New("unavailable: returned pipeline project does not match the requested project"))
		return
	}
	if pipeline.Status == "" {
		r.observation.unavailable("pipeline", errors.New("unavailable: pipeline status was not returned"))
		return
	}
	if pipeline.ID != mr.HeadPipeline.ID || pipeline.SHA != item.HeadRefOid || item.HeadRefOid == "" {
		r.observation.unavailable("pipeline", errors.New("unavailable: pipeline does not identify the current MR source head"))
		return
	}
	r.observation.fact("pipeline", fmt.Sprintf("%d: %s, SHA %s, ref %s", pipeline.ID, pipeline.Status, pipeline.SHA, pipeline.Ref))
	item.Checks = &workflowChecks{Commit: pipeline.SHA, State: strings.ToUpper(pipeline.Status), Available: true}
}
func (r *gitlabObservationReader) approvals(ctx context.Context, t workflowTarget) {
	var approvals glApprovals
	_, err := r.client.get(ctx, "GET", r.client.resource(t.Project, "merge_requests", strconv.Itoa(t.Number), "approvals"), &approvals)
	if err != nil {
		r.observation.unavailable("native approvals", err)
		return
	}
	if approvals.ApprovalsRequired == nil || approvals.ApprovalsLeft == nil {
		r.observation.unavailable("native approvals", errors.New("unavailable: deployment did not return approval requirements"))
		return
	}
	r.observation.fact("native approvals", fmt.Sprintf("required %d, left %d, approving users %d (not an Agent #2 review or qualified human acceptance)", *approvals.ApprovalsRequired, *approvals.ApprovalsLeft, len(approvals.ApprovedBy)))
	for _, approval := range approvals.ApprovedBy {
		r.observation.fact("native approving user", approval.User.Username)
	}
}
