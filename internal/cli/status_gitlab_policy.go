package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	policyPassed      = "passed"
	policyFailed      = "failed"
	policyUnavailable = "unavailable"
	policyNotRequired = "not-required"
)

type glAccess struct {
	AccessLevel *int `json:"access_level"`
	UserID      *int `json:"user_id"`
	GroupID     *int `json:"group_id"`
	DeployKeyID *int `json:"deploy_key_id"`
}
type glProtectedRef struct {
	Name      string     `json:"name"`
	ForcePush *bool      `json:"allow_force_push"`
	Push      []glAccess `json:"push_access_levels"`
	Merge     []glAccess `json:"merge_access_levels"`
	Create    []glAccess `json:"create_access_levels"`
}
type glJob struct {
	ID           int         `json:"id"`
	Name         string      `json:"name"`
	Status       string      `json:"status"`
	AllowFailure *bool       `json:"allow_failure"`
	Pipeline     *glPipeline `json:"pipeline"`
}
type glDiffVersion struct {
	ID        int       `json:"id"`
	MRID      int       `json:"merge_request_id"`
	Head      string    `json:"head_commit_sha"`
	Target    string    `json:"start_commit_sha"`
	CreatedAt time.Time `json:"created_at"`
	State     string    `json:"state"`
	Patch     string    `json:"patch_id_sha"`
}
type glApprovalState struct {
	Rules []struct {
		ID       int      `json:"id"`
		Required *int     `json:"approvals_required"`
		Approved *bool    `json:"approved"`
		Eligible []glUser `json:"eligible_approvers"`
		By       []glUser `json:"approved_by"`
	} `json:"rules"`
}
type glApprovalConfig struct {
	Reset             *bool `json:"reset_approvals_on_push"`
	Author            *bool `json:"merge_requests_author_approval"`
	DisableCommitters *bool `json:"merge_requests_disable_committers_approval"`
}

func (r *gitlabObservationReader) policyGate(name, state string, required bool, detail, path string) {
	r.observation.PolicyGates = append(r.observation.PolicyGates, workflowPolicyGate{Name: name, State: state, Required: required, Detail: detail, Source: r.client.forge.APIBaseURL + "/" + path, Sources: []string{r.client.forge.APIBaseURL + "/" + path}})
}
func invalidateGitlabPolicy(o *workflowObservation, problem string) {
	for i := range o.PolicyGates {
		g := &o.PolicyGates[i]
		if g.Name == "protected tags" {
			continue
		} // advisory inventory is not head-bound
		g.State = policyUnavailable
		g.Detail = problem
	}
}
func (r *gitlabObservationReader) policy(ctx context.Context, item *workflowItem, t workflowTarget, mr glResource, project glProject, cfg *projectConfig) {
	projectPath := "projects/" + url.PathEscape(t.Project)
	mrPath := r.client.resource(t.Project, "merge_requests", strconv.Itoa(t.Number))
	projectURL := projectPath
	if project.OnlyAllowMergeIfPipelineSucceeds == nil || project.AllowMergeOnSkippedPipeline == nil {
		r.policyGate("pipeline enforcement", policyUnavailable, true, "required pipeline/skipped-pipeline settings were not returned", projectURL)
	} else if !*project.OnlyAllowMergeIfPipelineSucceeds || *project.AllowMergeOnSkippedPipeline {
		r.policyGate("pipeline enforcement", policyFailed, true, "require successful pipelines and disallow skipped-pipeline merges", projectURL)
	} else {
		r.policyGate("pipeline enforcement", policyPassed, true, "native project settings require pipeline success without a skipped-pipeline bypass", projectURL)
	}
	r.policyPipeline(ctx, item, t, mr)
	r.policyProtectedRefs(ctx, t, mr)
	if project.OnlyAllowMergeIfAllDiscussionsAreResolved == nil {
		r.policyGate("discussion resolution", policyUnavailable, true, "whether resolution is required could not be established", projectURL)
	} else if !*project.OnlyAllowMergeIfAllDiscussionsAreResolved {
		r.policyGate("discussion resolution", policyNotRequired, false, "native project policy does not require resolved discussions; no resolution pass is inferred", projectURL)
	} else if r.observation.DiscussionsResolved == nil {
		r.policyGate("discussion resolution", policyUnavailable, true, "required discussion collection is incomplete", mrPath+"/discussions")
	} else if !*r.observation.DiscussionsResolved {
		r.policyGate("discussion resolution", policyFailed, true, "required resolvable discussions remain unresolved", mrPath+"/discussions")
	} else {
		r.policyGate("discussion resolution", policyPassed, true, "all observed required resolvable discussions are resolved", mrPath+"/discussions")
	}
	r.policyApprovals(ctx, item, t, mr, cfg)
	status := ""
	if mr.DetailedMergeStatus != nil {
		status = *mr.DetailedMergeStatus
	}
	state := policyFailed
	if status == "mergeable" && mr.State == "opened" && mr.Draft != nil && !*mr.Draft {
		state = policyPassed
	}
	if status == "" || status == "checking" || status == "unchecked" || status == "approvals_syncing" || mr.Draft == nil {
		state = policyUnavailable
	}
	r.policyGate("native merge status", state, true, "observed native status "+status+"; scheduling auto-merge is not evidence that gates passed", mrPath)
	// Re-read the actual target and project settings. A stable source SHA alone
	// cannot validate a retarget, a moving target commit, or changed native policy.
	target, err := r.policyTarget(ctx, t, mr.TargetBranch)
	var latest glProject
	_, projectErr := r.client.get(ctx, "GET", projectPath, &latest)
	if err != nil || projectErr != nil || target != item.BaseRefOid || item.BaseRefOid == "" || !reflect.DeepEqual(project, latest) {
		invalidateGitlabPolicy(r.observation, "target commit or native project settings changed/unavailable during observation")
		item.Checks = nil
		item.BaseRefOid = ""
	}
}
func (r *gitlabObservationReader) policyTarget(ctx context.Context, t workflowTarget, branch string) (string, error) {
	if checkBranchName(branch) != nil {
		return "", errors.New("target branch is unavailable")
	}
	var result struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	_, err := r.client.get(ctx, "GET", r.client.resource(t.Project, "repository", "branches", url.PathEscape(branch)), &result)
	if err != nil {
		return "", err
	}
	if result.Name != branch || !objectNamePattern.MatchString(result.Commit.ID) {
		return "", errors.New("target commit identity unavailable")
	}
	return result.Commit.ID, nil
}
func (r *gitlabObservationReader) policyPipeline(ctx context.Context, item *workflowItem, t workflowTarget, mr glResource) {
	source := r.client.resource(t.Project, "merge_requests", strconv.Itoa(t.Number), "pipelines")
	fail := func(detail string) { r.policyGate("current pipeline", policyUnavailable, true, detail, source) }
	if mr.HeadPipeline == nil || mr.HeadPipeline.ID < 1 || item.HeadRefOid == "" {
		fail("MR has no current identified pipeline/source head")
		return
	}
	pipelines, partial, err := gitlabList[glPipeline](ctx, r.client, source)
	if err != nil || partial {
		fail("MR pipeline relationship collection is unavailable or truncated")
		return
	}
	pipelineProject := t.Project
	expectedProject := mr.ProjectID
	if mr.HeadPipeline.ProjectID != 0 && mr.HeadPipeline.ProjectID != mr.ProjectID {
		if mr.HeadPipeline.ProjectID != mr.SourceProjectID {
			fail("pipeline project is unrelated to the MR")
			return
		}
		project, err := r.project(ctx, mr.SourceProjectID)
		if err != nil {
			fail("source project identity unavailable")
			return
		}
		pipelineProject = project.Path
		expectedProject = project.ID
	}
	endpoint := r.client.resource(pipelineProject, "pipelines", strconv.Itoa(mr.HeadPipeline.ID))
	var pipeline glPipeline
	if _, err := r.client.get(ctx, "GET", endpoint, &pipeline); err != nil {
		fail("pipeline detail unavailable")
		return
	}
	if pipeline.ProjectID != expectedProject || pipeline.ID != mr.HeadPipeline.ID || pipeline.SHA != item.HeadRefOid {
		fail("pipeline project/ID/SHA does not identify the current source-head artifact; synthetic SHAs are not equated with it")
		return
	}
	related := false
	for _, candidate := range pipelines {
		if candidate.ID == pipeline.ID && candidate.SHA == pipeline.SHA && candidate.ProjectID == pipeline.ProjectID {
			related = true
		}
	}
	// Branch pipelines can be native MR head pipelines. Their source project,
	// branch and SHA must all identify this MR's actual source, never another ref.
	branchBound := pipeline.Source == "push" && pipeline.Ref == mr.SourceBranch && pipeline.ProjectID == mr.SourceProjectID
	if !(pipeline.Source == "merge_request_event" && related) && !branchBound {
		fail("pipeline is not bound to this MR or its exact source branch")
		return
	}
	source = endpoint
	jobs, partial, err := gitlabList[glJob](ctx, r.client, endpoint+"/jobs")
	if err != nil || partial || len(jobs) == 0 {
		fail("required job evidence unavailable, empty or truncated")
		return
	}
	state := policyPassed
	required := 0
	for _, job := range jobs {
		if job.ID < 1 || job.AllowFailure == nil || job.Pipeline == nil || job.Pipeline.ID != pipeline.ID || job.Pipeline.SHA != pipeline.SHA || job.Pipeline.ProjectID != pipeline.ProjectID {
			fail("job ownership or required/advisory status unavailable")
			return
		}
		if *job.AllowFailure {
			continue
		}
		required++
		if job.Status != "success" {
			state = policyFailed
		}
	}
	if required == 0 {
		fail("no non-advisory validation job was established")
		return
	}
	if pipeline.Status != "success" {
		state = policyFailed
	}
	var latest glPipeline
	if _, err := r.client.get(ctx, "GET", endpoint, &latest); err != nil || !reflect.DeepEqual(pipeline, latest) {
		fail("pipeline changed or became unavailable during observation")
		return
	}
	r.policyGate("current pipeline", state, true, fmt.Sprintf("pipeline %d, project %d, source %s, SHA %s, status %s; %d non-advisory jobs checked", pipeline.ID, pipeline.ProjectID, pipeline.Source, pipeline.SHA, pipeline.Status, required), source)
	r.policySources("current pipeline", source+"/jobs", r.client.resource(t.Project, "merge_requests", strconv.Itoa(t.Number), "pipelines"))
}
func nativeRefMatches(pattern, ref string) bool {
	if pattern == "" || len(pattern) > 1024 || strings.ContainsAny(pattern, "\r\n\\") {
		return false
	}
	re := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
	ok, _ := regexp.MatchString(re, ref)
	return ok
}
func nativeAccess(level glAccess) (bool, bool) {
	for _, id := range []*int{level.UserID, level.GroupID, level.DeployKeyID} {
		if id != nil && *id < 0 {
			return false, false
		}
	}
	for _, id := range []*int{level.UserID, level.GroupID, level.DeployKeyID} {
		if id != nil && *id > 0 {
			return true, true
		}
	}
	if level.AccessLevel == nil {
		return false, false
	}
	switch *level.AccessLevel {
	case 0:
		return false, true
	case 30, 40, 50, 60:
		return true, true
	}
	return false, false
}

func (r *gitlabObservationReader) policyProtectedRefs(ctx context.Context, t workflowTarget, mr glResource) {
	endpoint := "projects/" + url.PathEscape(t.Project) + "/protected_branches"
	refs, partial, err := gitlabList[glProtectedRef](ctx, r.client, endpoint)
	state, detail := policyPassed, "all applicable target protections disallow direct pushes and force pushes, and permit PR/MR merging"
	if err != nil || partial {
		state, detail = policyUnavailable, "protected-branch collection unavailable or truncated"
	} else {
		matched, mergeAllowed := 0, false
		for _, ref := range refs {
			if !nativeRefMatches(ref.Name, mr.TargetBranch) {
				continue
			}
			matched++
			if ref.ForcePush == nil || ref.Push == nil || ref.Merge == nil {
				state, detail = policyUnavailable, "an applicable branch rule has missing controls"
				break
			}
			if *ref.ForcePush {
				state, detail = policyFailed, "an applicable branch rule permits force pushes"
				break
			}
			for _, level := range ref.Push {
				allowed, known := nativeAccess(level)
				if !known {
					state, detail = policyUnavailable, "push eligibility is unknown"
					break
				}
				if allowed {
					state, detail = policyFailed, "an applicable branch rule permits direct pushes"
					break
				}
			}
			if state != policyPassed {
				break
			}
			for _, level := range ref.Merge {
				allowed, known := nativeAccess(level)
				if !known {
					state, detail = policyUnavailable, "merge eligibility is unknown"
					break
				}
				mergeAllowed = mergeAllowed || allowed
			}
			if state != policyPassed {
				break
			}
		}
		if state == policyPassed && (matched == 0 || !mergeAllowed) {
			state, detail = policyFailed, "target is unprotected or no permitted MR merger is configured"
		}
	}
	r.policyGate("protected target", state, true, detail, endpoint)
	endpoint = "projects/" + url.PathEscape(t.Project) + "/protected_tags"
	tags, partial, err := gitlabList[glProtectedRef](ctx, r.client, endpoint)
	if err != nil || partial {
		r.policyGate("protected tags", policyUnavailable, false, "advisory release-tag inventory unavailable; this is not an MR gate or a release certification", endpoint)
		return
	}
	names := []string{}
	for _, tag := range tags {
		if tag.Name == "" || tag.Create == nil {
			r.policyGate("protected tags", policyUnavailable, false, "tag pattern/create-access detail missing; release protection not certified", endpoint)
			return
		}
		grants := []string{}
		for _, level := range tag.Create {
			allowed, known := nativeAccess(level)
			if !known {
				grants = append(grants, "unknown")
				continue
			}
			if !allowed {
				grants = append(grants, "no one")
				continue
			}
			if level.UserID != nil && *level.UserID > 0 {
				grants = append(grants, fmt.Sprintf("user %d", *level.UserID))
			} else if level.GroupID != nil && *level.GroupID > 0 {
				grants = append(grants, fmt.Sprintf("group %d", *level.GroupID))
			} else if level.AccessLevel != nil {
				grants = append(grants, fmt.Sprintf("access %d", *level.AccessLevel))
			}
		}
		names = append(names, oneLine(tag.Name)+" (create: "+strings.Join(grants, "; ")+")")
	}
	r.policyGate("protected tags", policyNotRequired, false, "advisory native patterns: "+strings.Join(names, ", ")+"; release applicability is evaluated by its release workflow", endpoint)
}
func (r *gitlabObservationReader) diffVersion(ctx context.Context, t workflowTarget, mr glResource, item *workflowItem) (glDiffVersion, error) {
	versions, partial, err := gitlabList[glDiffVersion](ctx, r.client, r.client.resource(t.Project, "merge_requests", strconv.Itoa(t.Number), "versions"))
	if err != nil || partial || len(versions) == 0 {
		return glDiffVersion{}, errors.New("current diff versions unavailable or truncated")
	}
	latest := versions[0]
	for _, v := range versions {
		if v.ID > latest.ID {
			latest = v
		}
	}
	if latest.ID < 1 || latest.MRID != mr.ID || latest.Head != item.HeadRefOid || latest.Target != item.BaseRefOid || latest.State != "collected" || latest.CreatedAt.IsZero() || !objectNamePattern.MatchString(latest.Patch) {
		return glDiffVersion{}, errors.New("latest collected diff does not bind the current source and target artifact")
	}
	return latest, nil
}
func (r *gitlabObservationReader) policyApprovals(ctx context.Context, item *workflowItem, t workflowTarget, mr glResource, cfg *projectConfig) {
	endpoint := r.client.resource(t.Project, "merge_requests", strconv.Itoa(t.Number))
	var approvals glApprovals
	_, err := r.client.get(ctx, "GET", endpoint+"/approvals", &approvals)
	if err != nil {
		r.policyGate("native approvals", policyUnavailable, true, "native approval state unavailable", endpoint+"/approvals")
		r.policyGate("human acceptance", policyUnavailable, true, "qualified current-artifact approval unavailable", endpoint+"/approvals")
		return
	}
	var rules glApprovalState
	_, ruleErr := r.client.get(ctx, "GET", endpoint+"/approval_state", &rules)
	version, versionErr := r.diffVersion(ctx, t, mr, item)
	pending := mr.DetailedMergeStatus == nil || *mr.DetailedMergeStatus == "checking" || *mr.DetailedMergeStatus == "unchecked" || *mr.DetailedMergeStatus == "approvals_syncing"
	state, detail := policyUnavailable, "applicable rule requirements unavailable; aggregate counts are not policy proof"
	ruleRequired := approvals.HasApprovalRules == nil || *approvals.HasApprovalRules
	if approvals.HasApprovalRules != nil && !*approvals.HasApprovalRules {
		state, detail = policyNotRequired, "native response explicitly reports no approval rules; human acceptance remains a separate required gate"
	} else if ruleErr == nil && rules.Rules != nil && len(rules.Rules) > 0 {
		state, detail = policyPassed, "all applicable native approval rules are satisfied"
		for _, rule := range rules.Rules {
			if rule.ID < 1 || rule.Required == nil || *rule.Required < 0 || rule.Approved == nil {
				state, detail = policyUnavailable, "an applicable approval rule has missing requirements/state"
				break
			}
			if *rule.Required > 0 && !*rule.Approved {
				state, detail = policyFailed, "an applicable approval rule still requires approvals"
			}
		}
		if state == policyPassed && (approvals.Approved == nil || !*approvals.Approved) {
			state, detail = policyUnavailable, "native aggregate/rule approval states are unavailable or contradictory"
		}
	}
	if versionErr != nil || pending {
		state, detail = policyUnavailable, "current artifact approval binding unavailable or approvals are still syncing"
	}
	r.policyGate("native approvals", state, ruleRequired, detail, endpoint+"/approval_state")
	if ruleErr != nil && !ruleRequired {
		r.policyGate("approval-rule detail", policyUnavailable, false, "tier-dependent rule detail unavailable; no rules are explicitly required", endpoint+"/approval_state")
	}
	var settings glApprovalConfig
	_, settingsErr := r.client.get(ctx, "GET", r.client.resource(t.Project, "approvals"), &settings)
	var acceptor *authority
	if cfg != nil && cfg.Project.Authorities != nil {
		acceptor = cfg.Project.Authorities.ChangeAcceptor
	}
	configured := false
	if acceptor != nil {
		for _, raw := range acceptor.Members {
			member, e := parseAuthorityMember(raw)
			if e == nil && member.Provider == "gitlab" && strings.EqualFold(member.Host, canonicalHTTPSHost(forgeHost(r.client.forge))) {
				configured = true
			}
		}
	}
	humanState, humanDetail := policyUnavailable, "current qualified human approval has not been established; configure an explicit host-qualified change acceptor"
	if versionErr == nil && !pending && configured {
		humanState, humanDetail = policyFailed, "no current native approval by the configured change acceptor"
		for _, by := range approvals.ApprovedBy {
			if by.User.ID < 1 || by.User.Bot || !authorityMatches(acceptor, "gitlab", forgeHost(r.client.forge), by.User.Username) {
				continue
			}
			if by.ApprovedAt == nil || by.ApprovedAt.Before(version.CreatedAt) || by.ApprovedAt.After(r.policyNow()) {
				humanState, humanDetail = policyUnavailable, "qualified approval timestamp does not prove current-diff acceptance"
				continue
			}
			eligible := false
			if ruleErr == nil {
				for _, rule := range rules.Rules {
					for _, user := range rule.Eligible {
						if user.ID == by.User.ID && user.Username == by.User.Username {
							eligible = true
						}
					}
				}
			}
			if settingsErr != nil || settings.Author == nil || settings.DisableCommitters == nil {
				humanState, humanDetail = policyUnavailable, "native author/committer approval eligibility unavailable"
				continue
			}
			if mr.Author.ID == 0 {
				humanState, humanDetail = policyUnavailable, "MR author identity unavailable"
				continue
			}
			if by.User.ID == mr.Author.ID && !*settings.Author {
				humanState, humanDetail = policyFailed, "native policy disallows MR author approval"
				continue
			}
			if *settings.DisableCommitters && !eligible {
				humanState, humanDetail = policyUnavailable, "committer eligibility cannot be proven from applicable native rules"
				continue
			}
			var member glMembership
			if _, e := r.client.get(ctx, "GET", r.client.resource(t.Project, "members", "all", strconv.Itoa(by.User.ID)), &member); e != nil || member.ID != by.User.ID || member.AccessLevel == nil || *member.AccessLevel < 30 {
				humanState, humanDetail = policyUnavailable, "qualified approver project eligibility unavailable"
				continue
			}
			humanState, humanDetail = policyPassed, "current-diff native approval by E8-qualified "+by.User.Username+"; separate from Agent #2 and native rule satisfaction"
			break
		}
	}
	r.policyGate("human acceptance", humanState, true, humanDetail, endpoint+"/approvals")
	r.policySources("native approvals", endpoint+"/approvals", endpoint+"/versions")
	r.policySources("human acceptance", endpoint+"/versions", r.client.resource(t.Project, "approvals"), endpoint+"/approval_state")
	// Approval data is head/diff-bound; changes during the bounded read make it
	// unavailable, never a reason to keep polling until it turns green.
	var lastSettings glApprovalConfig
	_, lastSettingsErr := r.client.get(ctx, "GET", r.client.resource(t.Project, "approvals"), &lastSettings)
	var last glApprovals
	_, lastErr := r.client.get(ctx, "GET", endpoint+"/approvals", &last)
	latest, latestErr := r.diffVersion(ctx, t, mr, item)
	var lastRules glApprovalState
	_, lastRuleErr := r.client.get(ctx, "GET", endpoint+"/approval_state", &lastRules)
	if settingsErr == nil && (lastSettingsErr != nil || !reflect.DeepEqual(settings, lastSettings)) || lastErr != nil || !reflect.DeepEqual(approvals, last) || versionErr == nil && (latestErr != nil || !reflect.DeepEqual(version, latest)) || ruleErr == nil && (lastRuleErr != nil || !reflect.DeepEqual(rules, lastRules)) {
		for i := range r.observation.PolicyGates {
			g := &r.observation.PolicyGates[i]
			if g.Name == "native approvals" || g.Name == "human acceptance" {
				g.State = policyUnavailable
				g.Detail = "approval/diff evidence changed or became unavailable during observation"
			}
		}
	}
}

func (r *gitlabObservationReader) policyNow() time.Time {
	if r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func (r *gitlabObservationReader) policySources(name string, paths ...string) {
	for i := range r.observation.PolicyGates {
		g := &r.observation.PolicyGates[i]
		if g.Name == name {
			for _, path := range paths {
				g.Sources = append(g.Sources, r.client.forge.APIBaseURL+"/"+path)
			}
		}
	}
}
