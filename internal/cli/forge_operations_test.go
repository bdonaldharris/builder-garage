package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

type operationFixture struct {
	t             *testing.T
	calls, writes int
	paths         []string
	input         []byte
	resource      glResource
	posted        operationNote
	notes         []operationNote
	alter         func(string, string, any) (int, any, error)
}

func newOperationFixture(t *testing.T) (*projectConfig, *operationFixture) {
	cfg := mustDecodeGitlabConfig(t)
	r := glResource{ID: 60, IID: 6, ProjectID: 7, Title: "Approved title", Description: "Approved body", WebURL: gitlabURL(gitlabTestForge(), workflowTarget{Project: "group/sub/project", Kind: workflowIssueKind, Number: 6}), Author: glUser{ID: 1, Username: "builder"}}
	return cfg, &operationFixture{t: t, resource: r}
}
func (f *operationFixture) run(ctx context.Context, input []byte, args ...string) ([]byte, []byte, error) {
	f.t.Helper()
	f.calls++
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if len(args) < 7 || args[0] != "api" || args[1] != "--hostname" || args[2] != "gitlab.example" || args[3] != "--method" || args[5] != "--include" {
		f.t.Fatalf("unsafe invocation: %v", args)
	}
	method, path := args[4], args[6]
	f.paths = append(f.paths, method+" "+path)
	if method != "GET" {
		f.writes++
		f.input = append([]byte{}, input...)
		if len(args) != 9 || args[7] != "--input" || args[8] != "-" {
			f.t.Fatal("body was not sent as literal JSON stdin")
		}
	}
	var response any
	status := 200
	switch {
	case path == "projects/group%2Fsub%2Fproject":
		response = glProject{ID: 7, Path: "group/sub/project"}
	case path == "user":
		response = glUser{ID: 1, Username: "builder"}
	case strings.Contains(path, "/members/all/"):
		response = glMembership{ID: 1, AccessLevel: func() *int { n := 30; return &n }()}
	case strings.Contains(path, "/notes?"):
		response = f.notes
		if f.notes == nil {
			response = []operationNote{}
		}
	case strings.HasSuffix(path, "/notes") && method == "POST":
		var body map[string]string
		if err := json.Unmarshal(input, &body); err != nil {
			f.t.Fatal(err)
		}
		f.posted = operationNote{glNote: glNote{ID: 21, Body: body["body"], Author: glUser{ID: 1, Username: "builder"}, CreatedAt: time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC)}, ProjectID: 7, NoteableID: f.resource.ID, NoteableIID: f.resource.IID, NoteableType: "Issue"}
		if strings.Contains(path, "merge_requests") {
			f.posted.NoteableType = "MergeRequest"
		}
		response = f.posted
		status = 201
	case strings.Contains(path, "/notes/21"):
		response = f.posted
	case strings.Contains(path, "?search=") || strings.Contains(path, "&search="):
		response = []glResource{f.resource}
	default:
		if method == "GET" && strings.Contains(path, "/issues/6") && strings.Contains(f.resource.WebURL, "merge_requests") {
			r := f.resource
			r.IID = 6
			r.WebURL = "https://gitlab.example/group/sub/project/-/issues/6"
			response = r
			break
		}
		if method == "POST" || method == "PUT" {
			var fields map[string]any
			if err := json.Unmarshal(input, &fields); err != nil {
				f.t.Fatal(err)
			}
			f.resource.Title = fields["title"].(string)
			f.resource.Description = fields["description"].(string)
			if s, ok := fields["source_branch"].(string); ok {
				f.resource.SourceBranch = s
			}
			if s, ok := fields["target_branch"].(string); ok {
				f.resource.TargetBranch = s
			}
			status = 201
			if method == "PUT" {
				status = 200
			}
		}
		response = f.resource
	}
	if f.alter != nil {
		var err error
		status, response, err = f.alter(method, path, response)
		if err != nil {
			return nil, []byte("private stderr"), err
		}
	}
	data, err := json.Marshal(response)
	if err != nil {
		f.t.Fatal(err)
	}
	return gitlabHTTP(status, "", string(data)), nil, nil
}
func operationFor(action string) forgeOperation {
	return forgeOperation{Action: action, Target: workflowTarget{Project: "group/sub/project", Host: "gitlab.example", Kind: workflowIssueKind, Number: 6}, Title: "Approved title", Body: "literal @file $(touch nope) `code`\n", Actor: "gitlab:gitlab.example/builder", Authorized: true}
}

func TestGitlabOperationsCreateUpdateReadSearch(t *testing.T) {
	for _, action := range []string{"create", "update", "read", "search"} {
		t.Run(action, func(t *testing.T) {
			cfg, f := newOperationFixture(t)
			o := operationFor(action)
			o.Search = "quoted & search"
			result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
			if err != nil || !result.Complete {
				t.Fatalf("result %+v err %v", result, err)
			}
			if action == "create" || action == "update" {
				if !result.Verified || f.writes != 1 || !strings.Contains(string(f.input), "$(touch nope)") {
					t.Fatal("mutation was not literal and verified")
				}
			} else if f.writes != 0 {
				t.Fatal("read/search mutated")
			}
			if action == "search" && !strings.Contains(strings.Join(f.paths, "\n"), url.QueryEscape(o.Search)) {
				t.Fatal("search was not encoded")
			}
		})
	}
	cfg, f := newOperationFixture(t)
	o := operationFor("create")
	o.Target.Kind = workflowChangeKind
	o.Source = "feature/x"
	o.Destination = "staging"
	f.resource.WebURL = strings.Replace(f.resource.WebURL, "issues", "merge_requests", 1)
	if result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run); err != nil || !result.Verified {
		t.Fatalf("MR create: %+v %v", result, err)
	}
}
func TestGitlabMutationRefusalsBeforeWrite(t *testing.T) {
	tests := map[string]func(*forgeOperation){"authorization": func(o *forgeOperation) { o.Authorized = false }, "actor": func(o *forgeOperation) { o.Actor = "builder" }, "quick action": func(o *forgeOperation) { o.Body = "Approved\n /merge\n" }, "NUL": func(o *forgeOperation) { o.Body = "unsafe\x00" }, "missing body": func(o *forgeOperation) { o.Body = "" }, "wrong host": func(o *forgeOperation) { o.Target.Host = "other.example" }, "foreign project": func(o *forgeOperation) { o.Target.Project = "other/project" }, "unknown action": func(o *forgeOperation) { o.Action = "merge" }}
	for name, edit := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, f := newOperationFixture(t)
			o := operationFor("create")
			edit(&o)
			if _, err := executeGitlabOperation(t.Context(), *cfg, o, f.run); err == nil || f.writes != 0 {
				t.Fatalf("unsafe write: %v %d", err, f.writes)
			}
		})
	}
	for _, change := range []string{"actor", "project", "target", "readback", "author", "timeout", "403"} {
		t.Run(change, func(t *testing.T) {
			cfg, f := newOperationFixture(t)
			o := operationFor("create")
			f.alter = func(method, path string, r any) (int, any, error) {
				if change == "actor" && path == "user" {
					return 200, glUser{ID: 2, Username: "different"}, nil
				}
				if change == "project" && path == "projects/group%2Fsub%2Fproject" {
					return 200, glProject{ID: 7, Path: "other/project"}, nil
				}
				if method == "POST" {
					switch change {
					case "target":
						v := r.(glResource)
						v.WebURL = "https://evil.example/wrong"
						return 201, v, nil
					case "timeout":
						return 0, nil, errors.New("private token timeout")
					case "403":
						return 403, map[string]string{"token": "secret"}, nil
					}
				}
				if method == "GET" && strings.HasSuffix(path, "/issues/6") && f.writes > 0 {
					v := r.(glResource)
					if change == "readback" {
						v.Description = "changed"
					}
					if change == "author" {
						v.Author.ID = 2
					}
					return 200, v, nil
				}
				return 200, r, nil
			}
			_, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
			if err == nil || strings.Contains(err.Error(), "private token") || strings.Contains(err.Error(), "secret") || f.writes > 1 {
				t.Fatalf("failed/unsafe write accepted: %v writes %d", err, f.writes)
			}
		})
	}
}
func TestGitlabWorkflowNotesAndAuthority(t *testing.T) {
	var issueNotes []glNote
	if err := json.Unmarshal(gitlabFixture(t, "issue-notes"), &issueNotes); err != nil {
		t.Fatal(err)
	}
	bodies := map[string]string{"implementation": issueNotes[0].Body, "correction": strings.ReplaceAll(strings.ReplaceAll(issueNotes[0].Body, "IMPLEMENTATION", "CORRECTION"), "record: implementation", "record: correction"), "review": issueNotes[1].Body, "discovery": "# Discovery Result\nEvidence", "readiness": "# Implementation Readiness\nREADY", "decision": "[WORKFLOW OWNER — DECISION]\nApproved", "acceptance": "[WORKFLOW OWNER — CHANGE ACCEPTANCE]\nAccepted", "release": "[RELEASE — RECORD]\nEvidence", "promotion": "[PROMOTION — RECORD]\nEvidence"}
	for record, body := range bodies {
		t.Run(record, func(t *testing.T) {
			cfg, f := newOperationFixture(t)
			cfg.Project.Authorities.WorkflowOwner.Members = []string{"gitlab:gitlab.example/builder"}
			cfg.Project.Authorities.ChangeAcceptor.Members = []string{"gitlab:gitlab.example/builder"}
			for _, n := range issueNotes {
				f.notes = append(f.notes, operationNote{glNote: n, ProjectID: 7, NoteableID: 60, NoteableIID: 6, NoteableType: "Issue"})
			}
			o := operationFor("note")
			o.Record = record
			o.Body = body
			result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
			if err != nil || !result.Verified || !strings.HasSuffix(result.URL, "#note_21") || f.writes != 1 {
				t.Fatalf("record: %+v %v writes %d", result, err, f.writes)
			}
		})
	}
	for _, name := range []string{"unqualified", "wrong host", "wrong user", "role only", "Reporter", "missing membership", "wrong location", "wrong issue", "readback", "stale response"} {
		t.Run(name, func(t *testing.T) {
			cfg, f := newOperationFixture(t)
			o := operationFor("note")
			o.Record = "implementation"
			o.Body = bodies["implementation"]
			switch name {
			case "unqualified", "wrong host", "wrong user", "role only":
				o.Record = "decision"
				o.Body = bodies["decision"]
				m := map[string][]string{"unqualified": {"builder"}, "wrong host": {"gitlab:other.example/builder"}, "wrong user": {"gitlab:gitlab.example/other"}, "role only": nil}
				cfg.Project.Authorities.WorkflowOwner.Members = m[name]
			case "wrong location":
				o.Target.Kind = workflowChangeKind
				f.resource.WebURL = strings.Replace(f.resource.WebURL, "issues", "merge_requests", 1)
			case "wrong issue":
				o.Target.Number = 5
				f.resource.IID = 5
				f.resource.WebURL = strings.Replace(f.resource.WebURL, "/6", "/5", 1)
			case "stale response":
				o.Record = "review"
				o.Body = bodies["review"]
				n := issueNotes[0]
				n.ID = 13
				n.CreatedAt = n.CreatedAt.Add(time.Hour)
				f.notes = []operationNote{{glNote: n, ProjectID: 7, NoteableID: 60, NoteableIID: 6, NoteableType: "Issue"}}
			}
			f.alter = func(method, path string, r any) (int, any, error) {
				if strings.Contains(path, "/members/all/") {
					if name == "Reporter" {
						level := 20
						return 200, glMembership{ID: 1, AccessLevel: &level}, nil
					}
					if name == "missing membership" {
						return 403, map[string]string{}, nil
					}
				}
				if method == "GET" && strings.Contains(path, "/notes/21") && name == "readback" {
					n := r.(operationNote)
					n.Body = "changed"
					return 200, n, nil
				}
				return 200, r, nil
			}
			_, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
			if err == nil {
				t.Fatalf("ineligible note accepted: %s", name)
			}
			if name != "readback" && f.writes != 0 {
				t.Fatal("invalid note posted")
			}
		})
	}
}
func TestGitlabSupplementalAndEquivalenceNotes(t *testing.T) {
	var source []glNote
	if err := json.Unmarshal(gitlabFixture(t, "issue-notes"), &source); err != nil {
		t.Fatal(err)
	}
	var mr []glNote
	if err := json.Unmarshal(gitlabFixture(t, "mr-notes"), &mr); err != nil {
		t.Fatal(err)
	}
	for _, record := range []string{"equivalence", "pr-review"} {
		t.Run(record, func(t *testing.T) {
			cfg, f := newOperationFixture(t)
			o := operationFor("note")
			o.Target.Kind = workflowChangeKind
			o.Target.Number = 5
			f.resource.IID = 5
			f.resource.Description = "## Governing issue\nCloses #6\n"
			f.resource.WebURL = "https://gitlab.example/group/sub/project/-/merge_requests/5"
			for _, n := range source {
				f.notes = append(f.notes, operationNote{glNote: n, ProjectID: 7, NoteableID: 60, NoteableIID: 6, NoteableType: "Issue"})
			}
			o.Record = record
			o.Body = mr[0].Body
			if record == "pr-review" {
				o.Body = strings.ReplaceAll(source[1].Body, "record: review", "record: pr-review")
				o.Body = strings.ReplaceAll(o.Body, "responds_to: https://gitlab.example/group/sub/project/-/issues/6#note_11", "responds_to: https://gitlab.example/group/sub/project/-/merge_requests/5#note_21\nsupplements: https://gitlab.example/group/sub/project/-/issues/6#note_12")
			}
			f.alter = func(method, path string, r any) (int, any, error) {
				if strings.Contains(path, "/merge_requests/5/notes?") {
					return 200, []operationNote{{glNote: mr[0], ProjectID: 7, NoteableID: 60, NoteableIID: 5, NoteableType: "MergeRequest"}}, nil
				}
				return 200, r, nil
			}
			result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
			if err != nil || !result.Verified {
				t.Fatalf("%s: %+v %v", record, result, err)
			}
		})
	}
}
func TestQualifiedAuthorityAndGitlabHistory(t *testing.T) {
	a := &authority{Role: "maintainer", Members: []string{"alice", "github:Bob", "gitlab:gitlab.example/builder"}}
	if !authorityMatches(a, "github", "github.com", "ALICE") || !authorityMatches(a, "github", "github.com", "bob") || !authorityMatches(a, "gitlab", "GITLAB.EXAMPLE", "builder") || authorityMatches(a, "gitlab", "elsewhere.example", "builder") || authorityMatches(a, "gitlab", "gitlab.example", "alice") {
		t.Fatal("identity namespace mismatch")
	}
	for _, raw := range []string{"gitlab:host/", "gitlab:host/user/extra", "github:", "gitlab:https://host/user", "other:user"} {
		if _, err := parseAuthorityMember(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if got := parseAuthority("maintainer:github:alice,gitlab:host/builder"); formatAuthority(got) != "maintainer:github:alice,gitlab:host/builder" {
		t.Fatal("qualified config round trip changed")
	}
	dir := newGitRepo(t, "git@gitlab.example:group/sub/project.git")
	writeFile(t, configPathIn(dir), forgeYAML("gitlab", "group/sub/project")+"    endpoint: https://gitlab.example\n")
	cache := t.TempDir()
	old, _ := identifyRepository(t.Context(), dir, runGit)
	repo, err := historyRepository(t.Context(), dir, runGit)
	if err != nil || !repo.GitLab || repo.Key == old.Key {
		t.Fatalf("not isolated: %+v %v", repo, err)
	}
	env := recordEnv{WorkDir: dir, CacheDir: func() (string, error) { return cache, nil }, Git: runGit, Now: time.Now}
	for _, kind := range []string{"issue", "mr"} {
		o := recordOptions{Type: "readiness", Outcome: "ready"}
		if kind == "mr" {
			o.MR = 6
		} else {
			o.Issue = 6
		}
		if _, err := recordEvent(t.Context(), o, env); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := recordEvent(t.Context(), recordOptions{PR: 6, Type: "review", Outcome: "approved"}, env); err == nil {
		t.Fatal("GitLab recorded PR")
	}
	events, _, err := readHistory(historyPath(cache, repo.Key))
	if err != nil || len(events) != 2 || events[0].Kind == events[1].Kind {
		t.Fatalf("history collision: %+v %v", events, err)
	}
	filter, err := parseHistoryFilter(0, 0, 6)
	if err != nil || filter.matches(events[0]) || !filter.matches(events[1]) {
		t.Fatal("MR history filter collided")
	}
	if _, err := parseHistoryFilter(6, 0, 6); err == nil {
		t.Fatal("ambiguous history flags")
	}
	if _, _, err := recordTarget(recordOptions{PR: 6, MR: 6}); err == nil {
		t.Fatal("ambiguous record flags")
	}
	var output strings.Builder
	err = runHistoryList(t.Context(), historyEnv{WorkDir: dir, CacheDir: env.CacheDir, Git: runGit}, historyListOptions{MR: 6, Format: "text"}, &output, &output)
	if err != nil || !strings.Contains(output.String(), "MR !6") {
		t.Fatalf("MR history output: %v %s", err, output.String())
	}
}

func TestGitlabOperationLabelsAndRetargetState(t *testing.T) {
	for _, test := range []string{"existing label", "missing label", "retarget", "close", "invalid state"} {
		t.Run(test, func(t *testing.T) {
			cfg, f := newOperationFixture(t)
			o := operationFor("update")
			if strings.Contains(test, "label") {
				o.Labels = []string{"enhancement"}
			}
			if test == "retarget" {
				o.Target.Kind = workflowChangeKind
				f.resource.WebURL = strings.Replace(f.resource.WebURL, "issues", "merge_requests", 1)
				o.Destination = "upstream/new"
			}
			if test == "close" {
				o.State = "close"
			}
			if test == "invalid state" {
				o.State = "merge"
			}
			f.alter = func(method, path string, r any) (int, any, error) {
				if strings.Contains(path, "/labels?") {
					if test == "missing label" {
						return 200, []map[string]string{}, nil
					}
					return 200, []map[string]string{{"name": "enhancement"}}, nil
				}
				if method == "GET" && f.writes > 0 {
					v := r.(glResource)
					v.Labels = o.Labels
					if test == "close" {
						v.State = "closed"
					}
					return 200, v, nil
				}
				return 200, r, nil
			}
			result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
			if test == "missing label" || test == "invalid state" {
				if err == nil || f.writes != 0 {
					t.Fatal("unsafe metadata change")
				}
				return
			}
			if err != nil || !result.Verified {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}
func TestGitlabOperationsBoundedAndCanceled(t *testing.T) {
	cfg, f := newOperationFixture(t)
	o := operationFor("create")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := executeGitlabOperation(ctx, *cfg, o, f.run); err == nil || f.calls != 0 {
		t.Fatal("canceled mutation attempted")
	}
	c := gitlabOperations{run: f.run, forge: gitlabTestForge(), calls: gitlabReadBudget}
	if _, err := c.request(t.Context(), "GET", "user", nil, &glUser{}); err == nil || f.calls != 0 {
		t.Fatal("exhausted request budget attempted")
	}
	o = operationFor("search")
	o.Search = "full"
	f.alter = func(method, path string, r any) (int, any, error) {
		if strings.Contains(path, "search=") {
			items := make([]glResource, gitlabPageSize)
			for i := range items {
				items[i] = f.resource
			}
			return 200, items, nil
		}
		return 200, r, nil
	}
	result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
	if err == nil || result.Complete || f.calls != gitlabPageLimit+1 || f.writes != 0 {
		t.Fatalf("unbounded/complete search: %+v %v calls %d", result, err, f.calls)
	}
}
func TestGitlabNoteOrderingAndIdentityRefusals(t *testing.T) {
	var source []glNote
	if err := json.Unmarshal(gitlabFixture(t, "issue-notes"), &source); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"wrong note destination", "wrong author", "wrong type", "missing timestamp", "malformed evidence", "foreign response", "wrong issue response", "new completion", "ineligible prior author"} {
		t.Run(scenario, func(t *testing.T) {
			cfg, f := newOperationFixture(t)
			o := operationFor("note")
			o.Record = "review"
			o.Body = source[1].Body
			for _, n := range source {
				f.notes = append(f.notes, operationNote{glNote: n, ProjectID: 7, NoteableID: 60, NoteableIID: 6, NoteableType: "Issue"})
			}
			switch scenario {
			case "malformed evidence":
				o.Body = strings.Replace(o.Body, "garage_evidence: 1", "garage_evidence: 99", 1)
			case "foreign response":
				o.Body = strings.Replace(o.Body, "gitlab.example/group/sub/project/-/issues/6#note_11", "other.example/group/sub/project/-/issues/6#note_11", 1)
			case "wrong issue response":
				o.Body = strings.Replace(o.Body, "issues/6#note_11", "issues/5#note_11", 1)
			case "new completion":
				n := f.notes[0]
				n.ID = 15
				n.CreatedAt = n.CreatedAt.Add(time.Hour)
				f.notes = append(f.notes, n)
			}
			f.alter = func(method, path string, r any) (int, any, error) {
				if scenario == "ineligible prior author" && strings.Contains(path, "/notes?") {
					for i := range f.notes {
						f.notes[i].Author.ID = 2
					}
					return 200, f.notes, nil
				}
				if scenario == "ineligible prior author" && strings.HasSuffix(path, "members/all/2") {
					level := 20
					return 200, glMembership{ID: 2, AccessLevel: &level}, nil
				}
				if method == "GET" && strings.Contains(path, "notes/21") {
					n := r.(operationNote)
					switch scenario {
					case "wrong note destination":
						n.NoteableIID = 7
					case "wrong author":
						n.Author.ID = 2
					case "wrong type":
						n.NoteableType = "MergeRequest"
					case "missing timestamp":
						n.CreatedAt = time.Time{}
					}
					return 200, n, nil
				}
				return 200, r, nil
			}
			result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
			if err == nil || result.Verified || f.writes > 1 {
				t.Fatalf("invalid note verified: %+v %v", result, err)
			}
		})
	}
}

func TestGitlabSupplementalRequestAndInvalidLinks(t *testing.T) {
	cfg, f := newOperationFixture(t)
	o := operationFor("note")
	o.Record = "review-request"
	o.Target.Kind = workflowChangeKind
	f.resource.SHA = strings.Repeat("a", 40)
	f.resource.WebURL = strings.Replace(f.resource.WebURL, "issues", "merge_requests", 1)
	o.Body = markerReviewRequest + "\n\nPR head: " + f.resource.SHA + "\nTrigger: artifact equivalence cannot be shown.\n"
	result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
	if err != nil || !result.Verified {
		t.Fatalf("request: %+v %v", result, err)
	}
	o.Body = strings.ReplaceAll(o.Body, strings.Repeat("a", 40), strings.Repeat("b", 40))
	f.writes = 0
	if _, err := executeGitlabOperation(t.Context(), *cfg, o, f.run); err == nil || f.writes != 0 {
		t.Fatal("stale-head request posted")
	}
	var source []glNote
	if err := json.Unmarshal(gitlabFixture(t, "issue-notes"), &source); err != nil {
		t.Fatal(err)
	}
	o = operationFor("note")
	o.Record = "review"
	o.Body = strings.Replace(source[1].Body, "https://gitlab.example/group/sub/project/-/issues/6#note_11", "none", 1)
	if _, err := executeGitlabOperation(t.Context(), *cfg, o, f.run); err == nil || f.writes != 0 {
		t.Fatal("review without a durable response was posted")
	}
}
func TestGitlabAuthorityConfigEditing(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, configPathIn(dir), validConfigYAML)
	value := "lead maintainer:github:builder,gitlab:gitlab.example/builder"
	_, stderr, err := runCLI(t, "dev", "config", "set", "workflow-owner", value, "--yes")
	if err != nil {
		t.Fatalf("authority editing: %v %s", err, stderr)
	}
	data, err := readConfigFile(configPathIn(dir))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := decodeConfig(data)
	if err != nil || validateConfig(cfg) != nil || !authorityMatches(cfg.Project.Authorities.WorkflowOwner, "gitlab", "gitlab.example", "builder") {
		t.Fatalf("qualified edit did not round-trip: %v", err)
	}
}

func TestOperationPaginationContracts(t *testing.T) {
	for _, scenario := range []string{"short complete", "totals continue", "untrusted link", "changed query", "nonsequential", "conflicting totals", "null"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			c := &gitlabOperations{forge: effectiveForge(gitlabTestForge()), run: func(ctx context.Context, input []byte, args ...string) ([]byte, []byte, error) {
				calls++
				headers := ""
				body := "[]"
				switch scenario {
				case "totals continue":
					headers = "X-Total-Pages: 2\r\n"
					if calls == 1 {
						body = "[{\"id\":1}]"
					}
				case "untrusted link":
					headers = "Link: <https://evil.example/api/v4/projects/group%2Fsub%2Fproject/issues?per_page=100&page=2>; rel=\"next\"\r\n"
				case "changed query":
					headers = "Link: <https://gitlab.example/api/v4/projects/group%2Fsub%2Fproject/issues?per_page=100&page=2&search=other>; rel=\"next\"\r\n"
				case "nonsequential":
					headers = "X-Next-Page: 4\r\n"
				case "conflicting totals":
					headers = "X-Total-Pages: 1\r\nX-Next-Page: 2\r\n"
				case "null":
					body = "null"
				}
				return gitlabHTTP(200, headers, body), nil, nil
			}}
			result, err := operationList[map[string]int](t.Context(), c, "projects/group%2Fsub%2Fproject/issues", nil)
			if scenario == "short complete" {
				if err != nil || calls != 1 {
					t.Fatalf("short page: %v %d", err, calls)
				}
				return
			}
			if scenario == "totals continue" {
				if err != nil || calls != 2 || len(result) != 1 {
					t.Fatalf("totals ignored: %v %d", err, calls)
				}
				return
			}
			if err == nil || calls != 1 {
				t.Fatalf("unsafe pagination: %v calls %d", err, calls)
			}
		})
	}
}
func TestGitlabNotesReadIsCompleteAndReadOnly(t *testing.T) {
	cfg, f := newOperationFixture(t)
	f.notes = []operationNote{{glNote: glNote{ID: 3, Body: "Durable content"}, ProjectID: 7, NoteableID: 60, NoteableIID: 6, NoteableType: "Issue"}}
	o := operationFor("notes")
	o.Authorized = false
	o.Actor = ""
	result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
	if err != nil || !result.Complete || result.Verified || len(result.Notes) != 1 || result.Notes[0].Body != "Durable content" || f.writes != 0 {
		t.Fatalf("read notes: %+v %v", result, err)
	}
}

func TestGitlabCrossProjectGoverningRecords(t *testing.T) {
	var issue []glNote
	if err := json.Unmarshal(gitlabFixture(t, "issue-notes"), &issue); err != nil {
		t.Fatal(err)
	}
	var mr []glNote
	if err := json.Unmarshal(gitlabFixture(t, "mr-notes"), &mr); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"enabled", "disabled", "undeclared"} {
		t.Run(scenario, func(t *testing.T) {
			cfg, f := newOperationFixture(t)
			cfg.Project.Forge.AllowCrossProjectTargets = scenario != "disabled"
			o := operationFor("note")
			o.Record = "equivalence"
			o.Body = strings.ReplaceAll(mr[0].Body, "group/sub/project/-/issues/6", "other/project/-/issues/6")
			o.Target.Kind = workflowChangeKind
			o.Target.Number = 5
			f.resource.IID = 5
			f.resource.WebURL = "https://gitlab.example/group/sub/project/-/merge_requests/5"
			f.resource.Description = "## Governing issue\nCloses other/project#6\n"
			if scenario == "undeclared" {
				f.resource.Description = "## Governing issue\nCloses #6\n"
			}
			run := func(ctx context.Context, input []byte, args ...string) ([]byte, []byte, error) {
				path := args[6]
				if !strings.Contains(path, "projects/other%2Fproject") {
					return f.run(ctx, input, args...)
				}
				var data any
				switch {
				case path == "projects/other%2Fproject":
					data = glProject{ID: 8, Path: "other/project"}
				case strings.Contains(path, "/members/all/1"):
					level := 30
					data = glMembership{ID: 1, AccessLevel: &level}
				case strings.Contains(path, "/notes?"):
					var notes []operationNote
					for _, n := range issue {
						n.Body = strings.ReplaceAll(n.Body, "group/sub/project/-/issues/6", "other/project/-/issues/6")
						notes = append(notes, operationNote{glNote: n, ProjectID: 8, NoteableID: 80, NoteableIID: 6, NoteableType: "Issue"})
					}
					data = notes
				default:
					data = glResource{ID: 80, IID: 6, ProjectID: 8, WebURL: "https://gitlab.example/other/project/-/issues/6"}
				}
				encoded, err := json.Marshal(data)
				if err != nil {
					t.Fatal(err)
				}
				return gitlabHTTP(200, "", string(encoded)), nil, nil
			}
			result, err := executeGitlabOperation(t.Context(), *cfg, o, run)
			if scenario == "enabled" {
				if err != nil || !result.Verified {
					t.Fatalf("qualified governing link rejected: %+v %v", result, err)
				}
			} else if err == nil || f.writes != 0 {
				t.Fatal("disallowed or undeclared project link wrote a record")
			}
		})
	}
}
func TestGitlabHistoryCannotReuseLegacyOrSameIIDIssue(t *testing.T) {
	run := &gitlabFixtureRunner{t: t}
	dir, env := gitlabStatusEnvFixture(t, run)
	legacy, err := identifyRepository(t.Context(), dir, env.Git)
	if err != nil {
		t.Fatal(err)
	}
	isolated, err := historyRepository(t.Context(), dir, env.Git)
	if err != nil {
		t.Fatal(err)
	}
	cache, _ := env.CacheDir()
	for _, row := range []struct{ key, kind, link string }{{legacy.Key, "pr", "https://github.com/owner/repo/pull/5#issuecomment-legacy"}, {isolated.Key, "issue", "https://gitlab.example/group/sub/project/-/issues/5#note_999"}, {isolated.Key, "mr", "https://gitlab.example/group/sub/project/-/merge_requests/5#note_1000"}} {
		event := historyEvent{Version: 1, Time: time.Now().UTC().Format(time.RFC3339), Repo: row.key, Kind: row.kind, Number: 5, Type: "finalization", Outcome: "complete", URL: row.link}
		if _, err := appendHistoryEvent(cache, event); err != nil {
			t.Fatal(err)
		}
	}
	report, err := statusReportForTarget(t.Context(), workflowTarget{Args: []string{"mr", "5"}}, env)
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := report.render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "#note_1000") || strings.Contains(output.String(), "#note_999") || strings.Contains(output.String(), "issuecomment-legacy") {
		t.Fatal("MR history reused another artifact's annotations")
	}
	server := webServer{projectDir: dir, env: webEnv{History: historyEnv{WorkDir: dir, CacheDir: env.CacheDir, Git: env.Git}}}
	activity, err := server.activitySnapshot(t.Context(), webActivityFilter{})
	if err != nil || len(activity.Events) != 2 {
		t.Fatalf("read-only activity namespace mismatch: %+v %v", activity, err)
	}
}

// Review response links bind the reviewed artifact, for both initial and
// corrected completions. An approval and a rejection obey the same rule.
func TestGitlabReviewWritesBindResponseArtifact(t *testing.T) {
	var issue, mr []glNote
	if err := json.Unmarshal(gitlabFixture(t, "issue-notes"), &issue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(gitlabFixture(t, "mr-notes"), &mr); err != nil {
		t.Fatal(err)
	}
	for _, record := range []string{"review", "pr-review"} {
		for _, corrected := range []bool{false, true} {
			for _, verdict := range []string{"approved", "changes-required"} {
				for _, matching := range []bool{false, true} {
					name := fmt.Sprintf("%s/corrected=%t/%s/matching=%t", record, corrected, verdict, matching)
					t.Run(name, func(t *testing.T) {
						cfg, f := newOperationFixture(t)
						o := operationFor("note")
						o.Record = record
						completion := issue[0]
						if corrected {
							completion.Body = strings.ReplaceAll(completion.Body, "[AGENT #1 — IMPLEMENTATION]", "[AGENT #1 — CORRECTION]")
							completion.Body = strings.ReplaceAll(completion.Body, "record: implementation", "record: correction")
						}
						f.notes = []operationNote{{glNote: completion, ProjectID: 7, NoteableID: 60, NoteableIID: 6, NoteableType: "Issue"}}
						o.Body = issue[1].Body
						if record == "pr-review" {
							// The authoritative Issue approval is valid independently of the MR
							// response artifact, and remains required by the supplements check.
							f.notes = append(f.notes, operationNote{glNote: issue[1], ProjectID: 7, NoteableID: 60, NoteableIID: 6, NoteableType: "Issue"})
							o.Target.Kind = workflowChangeKind
							o.Target.Number = 5
							f.resource.IID = 5
							f.resource.Description = "## Governing issue\nCloses #6\n"
							f.resource.WebURL = "https://gitlab.example/group/sub/project/-/merge_requests/5"
							o.Body = mr[1].Body
							response := mr[0]
							if corrected {
								response.Body = strings.ReplaceAll(response.Body, "[AGENT #1 — IMPLEMENTATION]", "[AGENT #1 — CORRECTION]")
								response.Body = strings.ReplaceAll(response.Body, "record: equivalence", "record: correction")
								response.Body = strings.ReplaceAll(response.Body, "equivalent_to: https://gitlab.example/group/sub/project/-/issues/6#note_12\n", "")
							}
							f.alter = func(method, path string, r any) (int, any, error) {
								if strings.Contains(path, "/merge_requests/5/notes?") {
									return 200, []operationNote{{glNote: response, ProjectID: 7, NoteableID: 60, NoteableIID: 5, NoteableType: "MergeRequest"}}, nil
								}
								return 200, r, nil
							}
						}
						o.Body = strings.ReplaceAll(o.Body, "verdict: approved", "verdict: "+verdict)
						if !matching {
							o.Body = strings.ReplaceAll(o.Body, strings.Repeat("d", 64), strings.Repeat("e", 64))
						}
						result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
						if matching {
							if err != nil || !result.Verified || f.writes != 1 {
								t.Fatalf("matching disposition refused: %+v %v, writes=%d", result, err, f.writes)
							}
						} else {
							if err == nil || !strings.Contains(err.Error(), "review artifact does not match") || result.Verified || f.writes != 0 {
								t.Fatalf("mismatched disposition attempted: %+v %v, writes=%d", result, err, f.writes)
							}
						}
					})
				}
			}
		}
	}
}

// A supplemental review can legitimately review a changed MR artifact while
// supplementing the original Issue approval. Only its responds_to record
// identifies that current MR artifact; supplements must not be conflated.
func TestGitlabSupplementalReviewCanDifferFromIssueApproval(t *testing.T) {
	var issue, mr []glNote
	if err := json.Unmarshal(gitlabFixture(t, "issue-notes"), &issue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(gitlabFixture(t, "mr-notes"), &mr); err != nil {
		t.Fatal(err)
	}
	cfg, f := newOperationFixture(t)
	o := operationFor("note")
	o.Record = "pr-review"
	o.Target.Kind = workflowChangeKind
	o.Target.Number = 5
	f.resource.IID = 5
	f.resource.Description = "## Governing issue\nCloses #6\n"
	f.resource.WebURL = "https://gitlab.example/group/sub/project/-/merge_requests/5"
	for _, n := range issue {
		f.notes = append(f.notes, operationNote{glNote: n, ProjectID: 7, NoteableID: 60, NoteableIID: 6, NoteableType: "Issue"})
	}
	response := mr[0]
	response.Body = strings.ReplaceAll(response.Body, strings.Repeat("d", 64), strings.Repeat("e", 64))
	response.Body = strings.ReplaceAll(response.Body, "equivalent_to: https://gitlab.example/group/sub/project/-/issues/6#note_12", "equivalent_to: none")
	o.Body = strings.ReplaceAll(mr[1].Body, strings.Repeat("d", 64), strings.Repeat("e", 64))
	f.alter = func(method, path string, r any) (int, any, error) {
		if strings.Contains(path, "/merge_requests/5/notes?") {
			return 200, []operationNote{{glNote: response, ProjectID: 7, NoteableID: 60, NoteableIID: 5, NoteableType: "MergeRequest"}}, nil
		}
		return 200, r, nil
	}
	result, err := executeGitlabOperation(t.Context(), *cfg, o, f.run)
	if err != nil || !result.Verified || f.writes != 1 {
		t.Fatalf("valid changed-artifact supplemental review refused: %+v %v writes=%d", result, err, f.writes)
	}
}
