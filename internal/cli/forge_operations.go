package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Mutations are a separate explicit operation, never part of statusProvider.
// Authorization is the invoking workflow's human assertion, not granted by a
// command flag. The expected actor is independently checked against GET user.
type forgeOperation struct {
	Labels                                   []string
	Action                                   string
	Target                                   workflowTarget
	Search, Title, Body, Source, Destination string
	Record, Actor                            string
	State                                    string
	Authorized                               bool
}
type forgeOperationResult struct {
	URL      string          `json:"url,omitempty"`
	Verified bool            `json:"verified"`
	Complete bool            `json:"complete"`
	Items    []glResource    `json:"items,omitempty"`
	Notes    []operationNote `json:"notes,omitempty"`
}
type glabInputRunner func(context.Context, []byte, ...string) ([]byte, []byte, error)
type gitlabOperations struct {
	run       glabInputRunner
	forge     forgeConfig
	calls     int
	linkedAt  time.Time
	governing map[workflowTargetKey]bool
}

func (c *gitlabOperations) request(ctx context.Context, method, path string, body any, result any) (http.Header, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.New("GitLab operation canceled or timed out; any attempted mutation has an uncertain result, inspect before retrying")
	}
	if c.calls >= gitlabReadBudget || c.run == nil {
		return nil, errors.New("GitLab operation unavailable: request budget or client unavailable")
	}
	c.calls++
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{"api", "--hostname", forgeHost(c.forge), "--method", method, "--include", path}
	var input []byte
	if body != nil {
		var err error
		input, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		args = append(args, "--input", "-")
	}
	data, _, runErr := c.run(ctx, input, args...)
	if ctx.Err() != nil || runErr != nil {
		return nil, errors.New("GitLab operation failed; any attempted mutation has an uncertain result, inspect before retrying")
	}
	if len(data) > glabResponseLimit {
		return nil, errors.New("GitLab operation response exceeds size limit; verification incomplete")
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), &http.Request{Method: method})
	if err != nil {
		return nil, errors.New("GitLab operation returned no valid HTTP response; verification incomplete")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.Header, &gitlabReadError{Status: response.StatusCode, Problem: gitlabHTTPProblem(response.StatusCode)}
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, glabResponseLimit+1))
	if decoder.Decode(result) != nil {
		return nil, errors.New("GitLab operation returned invalid JSON; verification incomplete")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("GitLab operation returned trailing data; verification incomplete")
	}
	return response.Header, nil
}
func (c *gitlabOperations) resource(t workflowTarget) string {
	return "projects/" + url.PathEscape(t.Project) + "/" + gitlabResourceKind(t.Kind)
}
func (c *gitlabOperations) item(ctx context.Context, t workflowTarget, projectID int) (glResource, error) {
	var r glResource
	_, err := c.request(ctx, "GET", c.resource(t)+"/"+strconv.Itoa(t.Number), nil, &r)
	if err == nil && !validOperationItem(r, t, c.forge, projectID) {
		err = errors.New("GitLab resource identity does not match the requested target")
	}
	return r, err
}
func validOperationItem(r glResource, t workflowTarget, f forgeConfig, projectID int) bool {
	return r.ID > 0 && r.IID == t.Number && r.ProjectID == projectID && r.WebURL == gitlabURL(f, t)
}

var quickActionLine = regexp.MustCompile(`(?m)^[\t ]*/[A-Za-z]`)

func validateOperation(o forgeOperation) error {
	switch o.Action {
	case "search", "read", "notes":
		return nil
	case "create", "update", "note":
	default:
		return errors.New("unsupported forge operation")
	}
	if !o.Authorized {
		return errors.New("mutation requires the invoking workflow's explicit authorization and --authorized assertion")
	}
	if _, err := parseAuthorityMember(o.Actor); err != nil || !strings.HasPrefix(o.Actor, "gitlab:") {
		return errors.New("mutation requires --actor gitlab:HOST/USER")
	}
	if !utf8.ValidString(o.Body) || len(o.Body) > 1_000_000 || strings.TrimSpace(o.Body) == "" || strings.ContainsRune(o.Body, 0) || quickActionLine.MatchString(o.Body) || quickActionLine.MatchString(o.Title) {
		return errors.New("body must be nonempty, bounded text without GitLab quick-action lines")
	}
	if o.Action != "note" && (strings.TrimSpace(o.Title) == "" || strings.ContainsAny(o.Title, "\r\n\x00")) {
		return errors.New("create/update requires a nonempty single-line title")
	}
	if o.State != "" && (o.Action != "update" || !contains([]string{"close", "reopen"}, o.State)) {
		return errors.New("state-event is only close or reopen on an explicitly authorized update")
	}
	if o.Target.Kind == workflowIssueKind && (o.Source != "" || o.Destination != "") {
		return errors.New("branch fields apply only to merge requests")
	}
	for _, branch := range []string{o.Source, o.Destination} {
		if branch != "" {
			if err := checkBranchName(branch); err != nil {
				return errors.New("source/target branch is not a valid branch name")
			}
		}
	}
	if o.Action == "create" && o.Target.Kind == workflowChangeKind && (o.Source == "" || o.Destination == "" || strings.ContainsAny(o.Source+o.Destination, "\r\n\x00")) {
		return errors.New("MR creation requires source and target branches")
	}
	return nil
}

func executeGitlabOperation(ctx context.Context, cfg projectConfig, o forgeOperation, run glabInputRunner) (forgeOperationResult, error) {
	result := forgeOperationResult{}
	if problems := validateConfig(cfg); problems != nil {
		return result, problems
	}
	if proj(&cfg).Forge == nil || cfg.Project.Forge.Provider != "gitlab" {
		return result, errors.New("forge operations require explicit GitLab configuration; GitHub retains its existing tools")
	}
	f := effectiveForge(*cfg.Project.Forge)
	if o.Target.Project == "" {
		o.Target.Project = f.Project
	}
	if o.Target.Host == "" {
		o.Target.Host = forgeHost(f)
	}
	t := o.Target
	check := t
	if o.Action == "search" || o.Action == "create" {
		check.Number = 1
	}
	if err := validateGitlabTarget(check, f); err != nil {
		return result, err
	}
	if err := validateOperation(o); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	c := &gitlabOperations{run: run, forge: f}
	var project glProject
	if _, err := c.request(ctx, "GET", "projects/"+url.PathEscape(t.Project), nil, &project); err != nil {
		return result, err
	}
	if project.ID < 1 || project.Path != t.Project {
		return result, errors.New("GitLab project identity does not match configured target")
	}
	if o.Action == "search" {
		return c.search(ctx, t, project.ID, o.Search)
	}
	if o.Action == "notes" {
		resource, err := c.item(ctx, t, project.ID)
		if err != nil {
			return result, err
		}
		notes, err := operationList[operationNote](ctx, c, c.resource(t)+"/"+strconv.Itoa(t.Number)+"/notes", nil)
		if err != nil {
			return result, err
		}
		kind := "Issue"
		if t.Kind == workflowChangeKind {
			kind = "MergeRequest"
		}
		for _, note := range notes {
			if note.ID < 1 || note.ProjectID != project.ID || note.NoteableID != resource.ID || note.NoteableIID != t.Number || note.NoteableType != kind {
				return result, errors.New("GitLab note collection identity mismatch")
			}
		}
		return forgeOperationResult{URL: resource.WebURL, Complete: true, Notes: notes}, nil
	}
	if o.Action == "read" {
		r, err := c.item(ctx, t, project.ID)
		if err != nil {
			return result, err
		}
		return forgeOperationResult{URL: r.WebURL, Complete: true, Items: []glResource{r}}, nil
	}
	actor, err := c.checkActor(ctx, o.Actor)
	if err != nil {
		return result, err
	}
	if o.Action == "note" {
		return c.note(ctx, cfg, o, t, project.ID, actor)
	}
	for _, label := range o.Labels {
		if label == "" || strings.ContainsAny(label, ",\r\n\x00") {
			return result, errors.New("approved labels must be nonempty single names without commas")
		}
		var existing []struct {
			Name string `json:"name"`
		}
		q := url.Values{"search": {label}, "per_page": {"100"}, "page": {"1"}}
		if _, err := c.request(ctx, "GET", "projects/"+url.PathEscape(t.Project)+"/labels?"+q.Encode(), nil, &existing); err != nil {
			return result, err
		}
		found := false
		for _, item := range existing {
			if item.Name == label {
				found = true
			}
		}
		if !found {
			return result, errors.New("approved label does not exist or could not be verified; labels are never created automatically")
		}
	}
	var resource glResource
	endpoint := c.resource(t)
	method := "POST"
	fields := map[string]any{"title": o.Title, "description": o.Body}
	if o.State != "" {
		fields["state_event"] = o.State
	}
	if o.Action == "update" && t.Kind == workflowChangeKind && o.Destination != "" {
		fields["target_branch"] = o.Destination
	}
	if len(o.Labels) > 0 {
		fields["labels"] = strings.Join(o.Labels, ",")
	}
	if o.Action == "update" {
		if _, err := c.item(ctx, t, project.ID); err != nil {
			return result, err
		}
		endpoint += "/" + strconv.Itoa(t.Number)
		method = "PUT"
	}
	if o.Action == "create" && t.Kind == workflowChangeKind {
		fields["source_branch"] = o.Source
		fields["target_branch"] = o.Destination
		fields["remove_source_branch"] = false
	}
	if _, err := c.request(ctx, method, endpoint, fields, &resource); err != nil {
		return result, err
	}
	if o.Action == "create" {
		t.Number = resource.IID
	}
	if !validOperationItem(resource, t, f, project.ID) {
		return result, errors.New("mutation returned a mismatched GitLab target; inspect before retrying")
	}
	read, err := c.item(ctx, t, project.ID)
	if err != nil {
		return result, err
	}
	labelsMatch := true
	if len(o.Labels) > 0 {
		expected, actual := slices.Clone(o.Labels), slices.Clone(read.Labels)
		slices.Sort(expected)
		slices.Sort(actual)
		labelsMatch = slices.Equal(expected, actual)
	}
	stateMatch := o.State == "" || o.State == "close" && read.State == "closed" || o.State == "reopen" && read.State == "opened"
	targetMatch := o.Destination == "" || t.Kind != workflowChangeKind || read.TargetBranch == o.Destination
	if !stateMatch || !targetMatch || !labelsMatch || read.ID != resource.ID || read.Title != o.Title || read.Description != o.Body || (o.Action == "create" && read.Author.ID != actor.ID) || (o.Action == "create" && t.Kind == workflowChangeKind && (read.SourceBranch != o.Source || read.TargetBranch != o.Destination)) {
		return result, errors.New("GitLab mutation read-back mismatch; not verified, inspect before retrying")
	}
	return forgeOperationResult{URL: read.WebURL, Verified: true, Complete: true}, nil
}

func (c *gitlabOperations) search(ctx context.Context, t workflowTarget, projectID int, search string) (forgeOperationResult, error) {
	result := forgeOperationResult{}
	if strings.TrimSpace(search) == "" || len(search) > 512 {
		return result, errors.New("search requires nonempty bounded text")
	}
	items, err := operationList[glResource](ctx, c, c.resource(t), url.Values{"search": {search}, "scope": {"all"}})
	if err != nil {
		return result, err
	}
	for _, item := range items {
		selected := t
		selected.Number = item.IID
		if !validOperationItem(item, selected, c.forge, projectID) {
			return result, errors.New("GitLab search returned a mismatched target")
		}
	}
	return forgeOperationResult{Items: items, Complete: true}, nil
}

// Pagination is reconstructed under the same host/path/query policy. Missing
// totals never imply a full page is complete; contradictory/foreign metadata
// cannot establish complete search or current-record ordering.
func operationList[T any](ctx context.Context, c *gitlabOperations, path string, query url.Values) ([]T, error) {
	if query == nil {
		query = url.Values{}
	}
	var result []T
	for page := 1; page <= gitlabPageLimit; page++ {
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", strconv.Itoa(gitlabPageSize))
		var items []T
		h, err := c.request(ctx, "GET", path+"?"+query.Encode(), nil, &items)
		if err != nil {
			return nil, err
		}
		if items == nil {
			return nil, errors.New("GitLab collection unavailable: null response")
		}
		result = append(result, items...)
		next := h.Get("X-Next-Page")
		for _, link := range strings.Split(h.Get("Link"), ",") {
			fields := strings.Split(link, ";")
			if len(fields) < 2 || !strings.Contains(strings.Join(fields[1:], ";"), `rel="next"`) {
				continue
			}
			u, e := url.Parse(strings.Trim(strings.TrimSpace(fields[0]), "<>"))
			base, _ := url.Parse(c.forge.APIBaseURL)
			if e != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || !strings.EqualFold(canonicalHTTPSHost(u.Host), canonicalHTTPSHost(base.Host)) || u.EscapedPath() != base.EscapedPath()+"/"+path {
				return nil, errors.New("GitLab pagination continuation is untrusted")
			}
			values, e := url.ParseQuery(u.RawQuery)
			expected := url.Values{}
			for k, v := range query {
				expected[k] = slices.Clone(v)
			}
			expected.Set("page", strconv.Itoa(page+1))
			if e != nil || values.Encode() != expected.Encode() {
				return nil, errors.New("GitLab pagination changed the query")
			}
			if next != "" && next != values.Get("page") {
				return nil, errors.New("GitLab pagination is contradictory")
			}
			next = values.Get("page")
		}
		if next != "" && next != strconv.Itoa(page+1) {
			return nil, errors.New("GitLab pagination is nonsequential")
		}
		if total := h.Get("X-Total-Pages"); total != "" {
			n, e := strconv.Atoi(total)
			if e != nil || n < page && !(n == 0 && page == 1 && len(items) == 0) || n <= page && next != "" {
				return nil, errors.New("GitLab pagination totals are inconsistent")
			}
			if n <= page {
				return result, nil
			}
			continue
		}
		if next == "" && len(items) < gitlabPageSize {
			return result, nil
		}
	}
	return nil, errors.New("GitLab collection truncated at page limit; search/ordering is incomplete")
}

type operationNote struct {
	glNote
	ProjectID    int    `json:"project_id"`
	NoteableID   int    `json:"noteable_id"`
	NoteableIID  int    `json:"noteable_iid"`
	NoteableType string `json:"noteable_type"`
}

func (c *gitlabOperations) note(ctx context.Context, cfg projectConfig, o forgeOperation, t workflowTarget, projectID int, actor glUser) (forgeOperationResult, error) {
	result := forgeOperationResult{}
	resource, err := c.item(ctx, t, projectID)
	if err != nil {
		return result, err
	}
	if t.Kind == workflowChangeKind {
		records := parseWorkflowRecords([]workflowComment{{Body: o.Body}}, "")
		if len(records) == 1 && records[0].Block != nil {
			section, _ := bodySection(resource.Description, "Governing issue")
			refs, problems := gitlabReferences(section, forgeForProject(c.forge, t.Project))
			found := false
			c.governing = map[workflowTargetKey]bool{}
			for _, ref := range refs {
				if ref.Kind == workflowIssueKind {
					c.governing[workflowTargetKey{ref.Project, ref.Kind, ref.Number}] = true
				}
				if ref.Kind == workflowIssueKind && ref.Number == records[0].Block.Issue {
					found = true
				}
			}
			if !found || len(problems) > 0 {
				return result, errors.New("MR record issue is not a declared governing issue of this MR")
			}
		}
	}
	if o.Record == "review-request" {
		records := parseWorkflowRecords([]workflowComment{{Body: o.Body}}, "")
		if len(records) != 1 || records[0].RequestHead != resource.SHA {
			return result, errors.New("supplemental review request must identify the current MR head")
		}
	}
	if err := validateGitlabNote(o, t); err != nil {
		return result, err
	}
	role := noteAuthority(o.Record)
	if role != "" {
		var a *authority
		if cfg.Project.Authorities != nil {
			switch role {
			case "workflow_owner":
				a = cfg.Project.Authorities.WorkflowOwner
			case "change_acceptor":
				a = cfg.Project.Authorities.ChangeAcceptor
			}
		}
		if !authorityMatches(a, "gitlab", forgeHost(c.forge), actor.Username) {
			return result, errors.New("GitLab actor is not the E8-qualified authority for this record")
		}
	} else {
		var membership glMembership
		if _, err := c.request(ctx, "GET", "projects/"+url.PathEscape(t.Project)+"/members/all/"+strconv.Itoa(actor.ID), nil, &membership); err != nil {
			return result, err
		}
		if membership.ID != actor.ID || membership.AccessLevel == nil || *membership.AccessLevel < 30 {
			return result, errors.New("GitLab workflow records require checked Developer-or-higher project access")
		}
	}
	if err := c.checkNoteLinks(ctx, o, t, projectID); err != nil {
		return result, err
	}
	endpoint := c.resource(t) + "/" + strconv.Itoa(t.Number) + "/notes"
	var posted operationNote
	if _, err := c.request(ctx, "POST", endpoint, map[string]string{"body": o.Body}, &posted); err != nil {
		return result, err
	}
	if posted.ID < 1 {
		return result, errors.New("GitLab note creation returned no note ID; inspect before retrying")
	}
	var read operationNote
	if _, err := c.request(ctx, "GET", endpoint+"/"+strconv.Itoa(posted.ID), nil, &read); err != nil {
		return result, err
	}
	kind := "Issue"
	if t.Kind == workflowChangeKind {
		kind = "MergeRequest"
	}
	if read.ID != posted.ID || read.ProjectID != projectID || read.NoteableID != resource.ID || read.NoteableIID != t.Number || read.NoteableType != kind || read.Body != o.Body || read.Author.ID != actor.ID || read.Author.Username != actor.Username || read.System || read.Resolvable || read.CreatedAt.IsZero() || read.CreatedAt.Before(c.linkedAt) || !read.CreatedAt.Equal(posted.CreatedAt) {
		return result, errors.New("GitLab note read-back mismatch; not verified, inspect before retrying")
	}
	return forgeOperationResult{URL: gitlabURL(c.forge, t) + "#note_" + strconv.Itoa(read.ID), Verified: true, Complete: true}, nil
}
func noteAuthority(record string) string {
	switch record {
	case "decision":
		return "workflow_owner"
	case "acceptance":
		return "change_acceptor"
	}
	return ""
}
func validateGitlabNote(o forgeOperation, t workflowTarget) error {
	if o.Record == "review-request" {
		records := parseWorkflowRecords([]workflowComment{{Body: o.Body}}, "")
		if t.Kind != workflowChangeKind || len(records) != 1 || records[0].Kind != recordRequest || !objectNamePattern.MatchString(records[0].RequestHead) {
			return errors.New("supplemental review request requires the existing request marker and exact MR head on the MR")
		}
		return nil
	}
	if contains([]string{"implementation", "correction", "review", "pr-review", "equivalence"}, o.Record) {
		records := parseWorkflowRecords([]workflowComment{{Body: o.Body}}, "")
		if len(records) != 1 || records[0].Kind != recordEvidence || records[0].Block.Record != o.Record {
			return errors.New("workflow note must carry the existing marker and valid evidence block for its record type")
		}
		b := records[0].Block
		if (o.Record == "implementation" || o.Record == "review") && t.Kind != workflowIssueKind || (o.Record == "pr-review" || o.Record == "equivalence") && t.Kind != workflowChangeKind {
			return errors.New("workflow record is on the wrong governing artifact kind")
		}
		if t.Kind == workflowIssueKind && b.Issue != t.Number {
			return errors.New("workflow evidence issue does not match governing issue")
		}
		if o.Record == "review" && b.Supplements != "" {
			return errors.New("issue implementation review cannot be supplemental")
		}
		return nil
	}
	prefix, known := map[string]string{"decision": "[WORKFLOW OWNER —", "discovery": "# Discovery", "readiness": "# Implementation Readiness", "release": "[RELEASE", "promotion": "[PROMOTION", "acceptance": "[WORKFLOW OWNER — CHANGE ACCEPTANCE]"}[o.Record]
	if !known || !strings.HasPrefix(strings.TrimSpace(o.Body), prefix) {
		return errors.New("note requires a supported record type and its documented heading/marker")
	}
	if (o.Record == "discovery" || o.Record == "readiness") && t.Kind != workflowIssueKind {
		return errors.New("discovery/readiness must be recorded on the governing issue")
	}
	return nil
}

// Linked records retain their exact host, project, kind and note identity.
// Fetching the note also establishes that response links are durable, not just
// syntactically plausible. Semantic review/approval remains the evaluator's job.
func (c *gitlabOperations) checkNoteLinks(ctx context.Context, o forgeOperation, t workflowTarget, projectID int) error {
	records := parseWorkflowRecords([]workflowComment{{Body: o.Body}}, "")
	if len(records) != 1 || records[0].Block == nil {
		return nil
	}
	b := records[0].Block
	links := []struct{ value, kind string }{{b.RespondsTo, ""}, {b.Supplements, workflowIssueKind}, {b.EquivalentTo, ""}}
	for _, link := range links {
		if link.value == "" || link.value == "none" && b.Record == "equivalence" && link.value == b.EquivalentTo {
			continue
		}
		u, err := url.Parse(link.value)
		if err != nil || !strings.HasPrefix(u.Fragment, "note_") {
			return errors.New("record link must identify an exact GitLab note")
		}
		noteID := strings.TrimPrefix(u.Fragment, "note_")
		if !positiveID(noteID) {
			return errors.New("record link note ID is invalid")
		}
		u.Fragment = ""
		linked, err := parseGitlabTarget([]string{u.String()}, c.forge)
		if err != nil || link.kind != "" && linked.Kind != link.kind || linked.Kind == workflowIssueKind && linked.Number != b.Issue {
			return errors.New("record link does not match the governing project/issue")
		}
		if linked.Kind == workflowChangeKind && (t.Kind != workflowChangeKind || linked.Project != t.Project || linked.Number != t.Number) {
			return errors.New("MR response/approval link belongs to another MR")
		}
		if b.Record == "review" && (linked.Kind != workflowIssueKind || linked.Number != t.Number) || b.Record == "pr-review" && link.value == b.RespondsTo && (linked.Kind != workflowChangeKind || linked.Number != t.Number) {
			return errors.New("review response link is on the wrong artifact")
		}
		linkedProjectID := projectID
		if linked.Kind == workflowIssueKind {
			if t.Kind == workflowIssueKind && linked.Project != t.Project || t.Kind == workflowChangeKind && !c.governing[workflowTargetKey{linked.Project, linked.Kind, linked.Number}] {
				return errors.New("response/approval link does not identify a declared governing issue")
			}
		}
		if linked.Project != t.Project {
			var project glProject
			if _, err := c.request(ctx, "GET", "projects/"+url.PathEscape(linked.Project), nil, &project); err != nil {
				return err
			}
			if project.Path != linked.Project || project.ID < 1 {
				return errors.New("linked governing project identity is unavailable")
			}
			linkedProjectID = project.ID
		}
		records, err := c.workflowNotes(ctx, linked, linkedProjectID)
		if err != nil {
			return err
		}
		var referenced *workflowRecord
		for i := range records {
			if records[i].Comment.URL == link.value {
				referenced = &records[i]
			}
		}
		if referenced == nil || !referenced.Counted || referenced.Kind != recordEvidence || referenced.Block.Issue != b.Issue {
			return errors.New("linked workflow record is missing, ineligible or malformed")
		}
		if linked.Kind == workflowIssueKind {
			current := currentGitlabIssueReview(workflowIssue{Project: linked.Project, Number: linked.Number, Comments: workflowComments{Nodes: recordComments(records)}})
			if link.value == b.RespondsTo {
				if current.Completion == nil || current.Completion.Comment.URL != link.value {
					return errors.New("review must respond to the current issue completion")
				}
			} else if !current.approved() || current.Review.Comment.URL != link.value {
				return errors.New("record must link the current approving issue disposition")
			}
		} else {
			var latest *workflowRecord
			for i := range records {
				r := &records[i]
				want := (r.Marker == markerImplementation || r.Marker == markerCorrection) && (r.Block == nil || r.Block.Record == "equivalence" || r.Block.Record == "correction")
				if link.value != b.RespondsTo {
					want = r.Marker == markerReview && (r.Block == nil || r.Block.Record == "pr-review")
				}
				if r.Counted && want {
					latest = r
				}
			}
			if latest == nil || latest.Comment.URL != link.value || link.value != b.RespondsTo && latest.Block.verdict() != "approved" {
				return errors.New("record link does not identify the current applicable MR record")
			}
		}
		if referenced.Comment.CreatedAt.After(c.linkedAt) {
			c.linkedAt = referenced.Comment.CreatedAt
		}
		// A disposition reviews the artifact of the completion/equivalence it
		// answers. Its Issue supplements link has a different responsibility:
		// that approval may cover an older artifact that triggered MR review.
		if (b.Record == "review" || b.Record == "pr-review") && link.value == b.RespondsTo && referenced.Block.artifact().Changeset != b.artifact().Changeset {
			return errors.New("review artifact does not match the completion or equivalence it answers")
		}
		if b.Record == "equivalence" && referenced.Block.artifact().Changeset != b.artifact().Changeset {
			return errors.New("equivalence does not match its approving artifact")
		}

	}
	return nil
}

func recordComments(records []workflowRecord) []workflowComment {
	var comments []workflowComment
	for _, r := range records {
		comments = append(comments, r.Comment)
	}
	return comments
}

// Read all bounded top-level notes before accepting a current response link.
// Partial or inaccessible collections never establish ordering/completeness.
func (c *gitlabOperations) workflowNotes(ctx context.Context, t workflowTarget, projectID int) ([]workflowRecord, error) {
	resource, err := c.item(ctx, t, projectID)
	if err != nil {
		return nil, err
	}
	notes, err := operationList[operationNote](ctx, c, c.resource(t)+"/"+strconv.Itoa(t.Number)+"/notes", nil)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].CreatedAt.Equal(notes[j].CreatedAt) {
			return notes[i].ID < notes[j].ID
		}
		return notes[i].CreatedAt.Before(notes[j].CreatedAt)
	})
	var comments []workflowComment
	members := map[int]bool{}
	seen := map[int]operationNote{}
	expectedKind := "Issue"
	if t.Kind == workflowChangeKind {
		expectedKind = "MergeRequest"
	}
	for _, note := range notes {
		if note.System {
			continue
		}
		if note.ID < 1 || note.ProjectID != projectID || note.NoteableID != resource.ID || note.NoteableIID != t.Number || note.NoteableType != expectedKind || note.CreatedAt.IsZero() {
			return nil, errors.New("workflow note identity unavailable")
		}
		if previous, ok := seen[note.ID]; ok {
			if previous.Body != note.Body || previous.Author.ID != note.Author.ID || previous.Author.Username != note.Author.Username || !previous.CreatedAt.Equal(note.CreatedAt) {
				return nil, errors.New("conflicting workflow notes")
			}
			continue
		}
		seen[note.ID] = note
		if _, _, ok := workflowMarker(note.Body); !ok {
			continue
		}
		eligible, known := members[note.Author.ID]
		if !known {
			var member glMembership
			if _, err := c.request(ctx, "GET", "projects/"+url.PathEscape(t.Project)+"/members/all/"+strconv.Itoa(note.Author.ID), nil, &member); err != nil {
				return nil, err
			}
			if member.ID != note.Author.ID || member.AccessLevel == nil {
				return nil, errors.New("workflow author membership unavailable")
			}
			eligible = *member.AccessLevel >= 30
			members[note.Author.ID] = eligible
		}
		value := eligible
		comments = append(comments, workflowComment{Provider: "gitlab", Eligible: &value, Body: note.Body, URL: gitlabURL(c.forge, t) + "#note_" + strconv.Itoa(note.ID), CreatedAt: note.CreatedAt, Author: &workflowActor{Login: note.Author.Username}})
	}
	return parseWorkflowRecords(comments, gitlabRecordLocation(t.Project, t.Kind, t.Number)), nil
}
