package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func gitlabTestForge() forgeConfig {
	return effectiveForge(forgeConfig{Provider: "gitlab", Endpoint: "https://gitlab.example", Project: "group/sub/project"})
}
func gitlabFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "gitlab", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func gitlabHTTP(status int, headers, body string) []byte {
	return []byte(fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Type: application/json\r\n%s\r\n%s", status, http.StatusText(status), headers, body))
}

type gitlabFixtureRunner struct {
	t        *testing.T
	calls    []string
	override func(string, int) ([]byte, bool)
}

func (f *gitlabFixtureRunner) run(ctx context.Context, args ...string) ([]byte, []byte, error) {
	f.t.Helper()
	if _, ok := ctx.Deadline(); !ok {
		f.t.Error("GitLab request has no timeout")
	}
	if len(args) != 7 || !reflect.DeepEqual(args[:6], []string{"api", "--hostname", "gitlab.example", "--method", "GET", "--include"}) {
		f.t.Fatalf("unexpected CLI arguments: %q", args)
	}
	endpoint := args[6]
	f.calls = append(f.calls, endpoint)
	if f.override != nil {
		if data, ok := f.override(endpoint, len(f.calls)); ok {
			return data, nil, nil
		}
	}
	path := strings.SplitN(endpoint, "?", 2)[0]
	prefix := "projects/group%2Fsub%2Fproject"
	name := ""
	switch path {
	case prefix:
		name = "project"
	case prefix + "/issues/5":
		name = "issue5"
	case prefix + "/issues/6":
		name = "issue6"
	case prefix + "/merge_requests/5":
		name = "mr5"
	case prefix + "/issues/6/notes":
		name = "issue-notes"
	case prefix + "/merge_requests/5/notes":
		name = "mr-notes"
	case prefix + "/merge_requests/5/discussions":
		name = "discussions"
	case prefix + "/repository/branches/stack%2Fbase":
		name = "branch"
	case prefix + "/pipelines/30":
		name = "pipeline"
	case prefix + "/members/all/1":
		name = "membership"
	case prefix + "/merge_requests/5/approvals":
		name = "approvals"
	default:
		if strings.HasSuffix(path, "/notes") || strings.HasSuffix(path, "/discussions") || strings.HasSuffix(path, "/closed_by") || strings.HasSuffix(path, "/closes_issues") {
			return gitlabHTTP(200, "", "[]"), nil, nil
		}
		f.t.Fatalf("unexpected endpoint %q", endpoint)
	}
	return gitlabHTTP(200, "", string(gitlabFixture(f.t, name))), nil, nil
}
func gitlabReadFixture(t *testing.T, kind string, run *gitlabFixtureRunner) *workflowItem {
	t.Helper()
	item, reads, err := (gitlabStatusAdapter{Runner: run.run, Forge: gitlabTestForge()}).Read(context.Background(), workflowTarget{Kind: kind, Number: 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reads != len(run.calls) || reads > gitlabReadBudget {
		t.Fatalf("read count %d / calls %d", reads, len(run.calls))
	}
	return item
}
func TestGitlabTypedTargetAndReferenceGrammar(t *testing.T) {
	f := gitlabTestForge()
	for _, test := range []struct {
		args []string
		kind string
	}{{[]string{"issue", "5"}, workflowIssueKind}, {[]string{"mr", "5"}, workflowChangeKind}, {[]string{f.Endpoint + "/" + f.Project + "/-/issues/5"}, workflowIssueKind}, {[]string{f.Endpoint + "/" + f.Project + "/-/merge_requests/5"}, workflowChangeKind}} {
		target, err := parseGitlabTarget(test.args, f)
		if err != nil || target.Kind != test.kind || target.Number != 5 || target.Project != f.Project || target.Host != "gitlab.example" {
			t.Fatalf("%v: %+v, %v", test.args, target, err)
		}
	}
	for _, args := range [][]string{{"5"}, {"#5"}, {"pr", "5"}, {"mr", "#5"}, {"mr", "05"}, {"issue", "0"}, {f.Endpoint + "/g/p/-/issues/5"}, {"http://gitlab.example/group/sub/project/-/issues/5"}, {f.Endpoint + "/group/sub/project/-/issues/5?token=secret"}, {"https://other.example/group/sub/project/-/issues/5"}, {f.Endpoint + "/group/sub/project/-/issues/5/extra"}, {f.Endpoint + "/group/sub/project/-/issues/5#note_1"}, {"https://user:secret@gitlab.example/group/sub/project/-/issues/5"}} {
		if _, err := parseGitlabTarget(args, f); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	refs, problems := gitlabReferences("Refs #5 !5 #5 group/sub/other#5 group/sub/other!5", f)
	if len(refs) != 2 || len(problems) != 2 || refs[0].Kind == refs[1].Kind {
		t.Fatalf("local and denied references: %+v, %v", refs, problems)
	}
	f.AllowCrossProjectTargets = true
	refs, problems = gitlabReferences("#5 !5 group/sub/other#5 group/sub/other!5", f)
	if len(refs) != 4 || len(problems) != 0 {
		t.Fatalf("qualified references: %+v %v", refs, problems)
	}
	if _, err := parseGitlabTarget([]string{f.Endpoint + "/group/sub/other/-/merge_requests/5"}, f); err != nil {
		t.Fatal(err)
	}
}
func TestGitlabReadOnlyResourceGuard(t *testing.T) {
	c := newGitlabClient(func(context.Context, ...string) ([]byte, []byte, error) {
		t.Fatal("refused request reached glab")
		return nil, nil, nil
	}, gitlabTestForge())
	for _, test := range []struct{ method, path string }{
		{"POST", "projects/group%2Fsub%2Fproject/issues/5/notes"}, {"DELETE", "projects/group%2Fsub%2Fproject/issues/5"}, {"GET", "https://attacker.example/api/v4/projects/group%2Fsub%2Fproject/issues/5"}, {"GET", "projects/other%2Fproject/issues/5"}, {"GET", "projects/group%2Fsub%2Fproject/issues/5?sudo=root"}, {"GET", "projects/group%2Fsub%2Fproject/issues/5/notes?per_page=100&page=2&token=secret"}, {"GET", "projects/group%2Fsub%2Fproject/../../users"}, {"GET", "projects/group%2Fsub%2Fproject/releases"}, {"GET", "projects/group%2Fsub%2Fproject/issues/5/notes?per_page=100&page=0"}, {"GET", "projects/group%2Fsub%2Fproject/issues/5/notes?page=1&page=2"},
	} {
		if _, err := c.get(context.Background(), test.method, test.path, &map[string]any{}); !errors.Is(err, errGLabNotReadOnly) {
			t.Errorf("%+v: %v", test, err)
		}
	}
	if c.reads != 0 {
		t.Fatal("refused requests consumed read budget")
	}
}
func TestGitlabSameIIDNormalizationAndEvidence(t *testing.T) {
	issueRun := &gitlabFixtureRunner{t: t}
	issue := gitlabReadFixture(t, workflowIssueKind, issueRun)
	mrRun := &gitlabFixtureRunner{t: t}
	mr := gitlabReadFixture(t, workflowChangeKind, mrRun)
	if issue.Title != "Issue five" || mr.Title != "MR five" || issue.Number != mr.Number || issue.Kind == mr.Kind {
		t.Fatal("same-IID target identity lost")
	}
	if mr.BaseRefName != "stack/base" || mr.BaseRefOid != strings.Repeat("b", 40) || mr.HeadRefOid != strings.Repeat("a", 40) || mr.Checks == nil || mr.Checks.Commit != mr.HeadRefOid || mr.Checks.State != "SUCCESS" {
		t.Fatalf("actual branch/head/pipeline: %+v", mr)
	}
	if len(mr.Observation.Governing) != 1 || mr.Observation.Governing[0].Number != 6 || mr.Observation.Governing[0].Project != "group/sub/project" {
		t.Fatal("governing issue identity lost")
	}
	if mr.Observation.DiscussionsResolved == nil || !*mr.Observation.DiscussionsResolved {
		t.Fatal("resolved discussions not normalized")
	}
	records := parseWorkflowRecords(mr.Observation.Governing[0].Comments.Nodes, "issue")
	if len(records) != 2 || !records[0].Counted || !records[1].Counted || records[1].Block == nil || records[1].Block.verdict() != "approved" {
		t.Fatalf("membership eligible records not normalized: %+v", records)
	}
	for _, n := range mr.Comments.Nodes {
		if n.AuthorAssociation != "" {
			t.Fatal("GitLab membership impersonated a GitHub association")
		}
	}
	var output bytes.Buffer
	if err := buildGitlabStatusReport(statusInputs{Item: mr, Config: mustDecodeGitlabConfig(t), ReadAt: time.Now(), Reads: len(mrRun.calls)}).render(&output); err != nil {
		t.Fatal(err)
	}
	assertContains(t, output.String(), "Status of merge request !5", "GitLab, checked", "actual target branch stack/base", "required 1, left 0", "approving users 1", "human acceptance: unavailable", "no merge eligibility is established", "changeset equals the latest approval")
	if strings.Contains(output.String(), "GitHub, checked") || strings.Contains(output.String(), "approving GitHub review") {
		t.Fatal("GitHub provenance leaked into GitLab report")
	}
}
func mustDecodeGitlabConfig(t *testing.T) *projectConfig {
	t.Helper()
	cfg, err := decodeConfig([]byte(forgeYAML("gitlab", "group/sub/project") + "    endpoint: https://gitlab.example\n"))
	if err != nil {
		t.Fatal(err)
	}
	return &cfg
}
func TestGitlabMembershipAndCapabilityFailuresNeverPass(t *testing.T) {
	for _, test := range []struct {
		name, path string
		status     int
		body       string
		needle     string
	}{
		{"membership inaccessible", "/members/all/1", 403, "{}", "membership: inaccessible"},
		{"reporter ineligible", "/members/all/1", 200, `{"id":1,"access_level":20}`, "access level 20"},
		{"approval unsupported", "/approvals", 501, "{}", "unsupported"},
		{"approval insufficient access", "/approvals", 403, "{}", "native approvals: inaccessible"},
		{"approval details missing", "/approvals", 200, `{}`, "approval requirements"},
		{"pipeline wrong artifact", "/pipelines/30", 200, `{"id":30,"sha":"ffffffffffffffffffffffffffffffffffffffff","status":"success"}`, "current MR source head"},
		{"notes inaccessible", "/issues/6/notes", 403, "{}", "notes: inaccessible"},
		{"discussion unavailable", "/discussions", 404, "{}", "discussions: inaccessible"},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := &gitlabFixtureRunner{t: t}
			run.override = func(endpoint string, _ int) ([]byte, bool) {
				if strings.HasSuffix(strings.SplitN(endpoint, "?", 2)[0], test.path) {
					return gitlabHTTP(test.status, "", test.body), true
				}
				return nil, false
			}
			item := gitlabReadFixture(t, workflowChangeKind, run)
			var output bytes.Buffer
			_ = buildGitlabStatusReport(statusInputs{Item: item, Config: mustDecodeGitlabConfig(t), ReadAt: time.Now()}).render(&output)
			assertContains(t, output.String(), test.needle, "no merge eligibility is established")
			if test.name == "membership inaccessible" || test.name == "notes inaccessible" || test.name == "discussion unavailable" {
				assertContains(t, output.String(), "review evaluation: unavailable")
				if strings.Contains(output.String(), "changeset equals the latest approval") {
					t.Fatal("incomplete required records passed review/equivalence evaluation")
				}
			}

			if test.name == "reporter ineligible" {
				for _, comment := range item.Observation.Governing[0].Comments.Nodes {
					if workflowCommentEligible(comment) {
						t.Fatal("reporter record counted")
					}
				}
			}
			if strings.HasPrefix(test.name, "pipeline") && item.Checks != nil {
				t.Fatal("wrong-head successful pipeline passed")
			}
		})
	}
}
func TestGitlabChangingMRHeadFailsClosed(t *testing.T) {
	run := &gitlabFixtureRunner{t: t}
	reads := 0
	run.override = func(endpoint string, _ int) ([]byte, bool) {
		if endpoint == "projects/group%2Fsub%2Fproject/merge_requests/5" {
			reads++
			if reads > 1 {
				var doc map[string]any
				_ = json.Unmarshal(gitlabFixture(t, "mr5"), &doc)
				doc["sha"] = strings.Repeat("e", 40)
				b, _ := json.Marshal(doc)
				return gitlabHTTP(200, "", string(b)), true
			}
		}
		return nil, false
	}
	item := gitlabReadFixture(t, workflowChangeKind, run)
	if item.Checks != nil || item.BaseRefOid != "" || item.Observation.Complete {
		t.Fatal("concurrent MR update retained passing artifact facts")
	}
}
func TestGitlabBoundedPaginationAndContinuationRefusal(t *testing.T) {
	for _, scenario := range []string{"two pages", "missing totals", "page limit", "host escape", "path escape", "query escape", "nonsequential", "contradictory"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			runner := func(_ context.Context, args ...string) ([]byte, []byte, error) {
				calls++
				header := ""
				body := "[1]"
				switch scenario {
				case "two pages":
					if calls == 1 {
						header = "X-Next-Page: 2\r\n"
					} else {
						body = "[2]"
					}
				case "missing totals":
					if calls == 1 {
						body = "[" + strings.TrimSuffix(strings.Repeat("1,", gitlabPageSize), ",") + "]"
					} else {
						body = "[]"
					}
				case "page limit":
					body = "[" + strings.TrimSuffix(strings.Repeat("1,", gitlabPageSize), ",") + "]"
				case "host escape":
					header = `Link: <https://attacker.example/api/v4/projects/group%2Fsub%2Fproject/issues/5/notes?per_page=100&page=2>; rel="next"` + "\r\n"
				case "path escape":
					header = `Link: <https://gitlab.example/api/v4/projects/other%2Fproject/issues/5/notes?per_page=100&page=2>; rel="next"` + "\r\n"
				case "query escape":
					header = `Link: <https://gitlab.example/api/v4/projects/group%2Fsub%2Fproject/issues/5/notes?per_page=100&page=2&sudo=root>; rel="next"` + "\r\n"
				case "nonsequential":
					header = "X-Next-Page: 9\r\n"
				case "contradictory":
					header = "X-Next-Page: 3\r\n" + `Link: <https://gitlab.example/api/v4/projects/group%2Fsub%2Fproject/issues/5/notes?per_page=100&page=2>; rel="next"` + "\r\n"
				}
				return gitlabHTTP(200, header, body), nil, nil
			}
			client := newGitlabClient(runner, gitlabTestForge())
			items, partial, err := gitlabList[int](context.Background(), client, "projects/group%2Fsub%2Fproject/issues/5/notes")
			if scenario == "two pages" || scenario == "missing totals" {
				if partial || err != nil || calls != 2 {
					t.Fatalf("pagination incomplete: %d %d %t %v", len(items), calls, partial, err)
				}
			} else {
				if !partial || err == nil || calls > gitlabPageLimit {
					t.Fatalf("unsafe/unbounded pagination accepted: %d %t %v", calls, partial, err)
				}
			}
		})
	}
}
func TestGitlabTransportFailureAndBudget(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 500, 501} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			c := newGitlabClient(func(context.Context, ...string) ([]byte, []byte, error) {
				return gitlabHTTP(status, "", `{"token":"do-not-show"}`), []byte("secret stderr"), errors.New("secret runtime error")
			}, gitlabTestForge())
			_, err := c.get(context.Background(), "GET", "projects/group%2Fsub%2Fproject", &glProject{})
			if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "do-not-show") {
				t.Fatalf("failed response exposed or accepted: %v", err)
			}
		})
	}
	c := newGitlabClient(func(context.Context, ...string) ([]byte, []byte, error) {
		t.Fatal("exhausted budget called runner")
		return nil, nil, nil
	}, gitlabTestForge())
	c.reads = gitlabReadBudget
	if _, err := c.get(context.Background(), "GET", "projects/group%2Fsub%2Fproject", &glProject{}); err == nil {
		t.Fatal("read budget ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.reads = 0
	if _, err := c.get(ctx, "GET", "projects/group%2Fsub%2Fproject", &glProject{}); err == nil || c.reads != 0 {
		t.Fatal("cancellation ignored")
	}
}
func gitlabStatusEnvFixture(t *testing.T, run *gitlabFixtureRunner) (string, statusEnv) {
	t.Helper()
	dir := t.TempDir()
	initGitRemote(t, dir, "git@gitlab.example:group/sub/project.git")
	writeFile(t, configPathIn(dir), forgeYAML("gitlab", "group/sub/project")+"    endpoint: https://gitlab.example\n")
	env := statusTestEnv(t, dir, func(context.Context, ...string) ([]byte, []byte, error) {
		t.Fatal("GitLab dispatched to GitHub")
		return nil, nil, nil
	})
	env.GLab = run.run
	env.CacheDir = func() (string, error) { t.Fatal("GitLab reused GitHub history"); return "", nil }
	return dir, env
}
func TestGitlabStatusDispatchAndReadOnlyWeb(t *testing.T) {
	run := &gitlabFixtureRunner{t: t}
	dir, env := gitlabStatusEnvFixture(t, run)
	for _, args := range [][]string{{"issue", "5"}, {"mr", "5"}} {
		report, err := statusReportForTarget(context.Background(), workflowTarget{Args: args}, env)
		if err != nil || report == nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	before := len(run.calls)
	if _, err := statusReportForTarget(context.Background(), workflowTarget{Args: []string{"5"}}, env); err == nil || len(run.calls) != before {
		t.Fatal("ambiguous GitLab CLI consulted provider")
	}
	server := &webServer{projectDir: dir, env: webEnv{Status: env}}
	for _, kind := range []string{"issue", "mr"} {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/status?target="+url.QueryEscape(kind+" 5"), nil)
		rr := httptest.NewRecorder()
		server.status(rr, req)
		var value webWorkflowStatus
		if err := json.Unmarshal(rr.Body.Bytes(), &value); err != nil || value.Problem != "" {
			t.Fatalf("web %s: %s", kind, rr.Body.String())
		}
		if value.Number != 5 {
			t.Fatal("web lost the normalized IID")
		}
		if !strings.Contains(value.URL, "/-/"+gitlabResourceKind(map[string]string{"issue": workflowIssueKind, "mr": workflowChangeKind}[kind])+"/5") {
			t.Fatal("web lost typed kind")
		}
	}
	if !strings.Contains(webPage, "workflow-kind") || !strings.Contains(webPage, "Open MR") {
		t.Fatal("typed read-only web lookup unavailable")
	}
}

func TestGitlabCrossProjectObservationAndForkPipeline(t *testing.T) {
	t.Run("opt-in target stays isolated", func(t *testing.T) {
		run := &gitlabFixtureRunner{t: t}
		f := gitlabTestForge()
		f.AllowCrossProjectTargets = true
		run.override = func(endpoint string, _ int) ([]byte, bool) {
			path := strings.SplitN(endpoint, "?", 2)[0]
			switch path {
			case "projects/group%2Fother":
				return gitlabHTTP(200, "", `{"id":8,"path_with_namespace":"group/other","default_branch":"main"}`), true
			case "projects/group%2Fother/issues/5":
				var issue map[string]any
				_ = json.Unmarshal(gitlabFixture(t, "issue5"), &issue)
				issue["project_id"] = 8
				issue["web_url"] = "https://gitlab.example/group/other/-/issues/5"
				issue["description"] = ""
				b, _ := json.Marshal(issue)
				return gitlabHTTP(200, "", string(b)), true
			}
			return nil, false
		}
		item, _, err := (gitlabStatusAdapter{Runner: run.run, Forge: f}).Read(context.Background(), workflowTarget{Kind: workflowIssueKind, Number: 5, Project: "group/other"}, nil)
		if err != nil || item.RepositoryIdentity != "group/other" || !strings.Contains(item.URL, "group/other/-/issues/5") {
			t.Fatalf("foreign identity %+v: %v", item, err)
		}
		f.AllowCrossProjectTargets = false
		before := len(run.calls)
		if _, _, err := (gitlabStatusAdapter{Runner: run.run, Forge: f}).Read(context.Background(), workflowTarget{Kind: workflowIssueKind, Number: 5, Project: "group/other"}, nil); err == nil || len(run.calls) != before {
			t.Fatal("disabled cross-project target was read")
		}
	})
	t.Run("fork source and pipeline retain native ownership", func(t *testing.T) {
		run := &gitlabFixtureRunner{t: t}
		run.override = func(endpoint string, _ int) ([]byte, bool) {
			switch endpoint {
			case "projects/group%2Fsub%2Fproject/merge_requests/5":
				var mr map[string]any
				_ = json.Unmarshal(gitlabFixture(t, "mr5"), &mr)
				mr["source_project_id"] = 8
				mr["head_pipeline"].(map[string]any)["project_id"] = 8
				b, _ := json.Marshal(mr)
				return gitlabHTTP(200, "", string(b)), true
			case "projects/8":
				return gitlabHTTP(200, "", `{"id":8,"path_with_namespace":"fork/project"}`), true
			case "projects/fork%2Fproject/pipelines/30":
				return gitlabHTTP(200, "", `{"id":30,"project_id":8,"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","status":"success","ref":"feature/topic"}`), true
			}
			return nil, false
		}
		item := gitlabReadFixture(t, workflowChangeKind, run)
		if item.HeadRepository == nil || item.HeadRepository.NameWithOwner != "fork/project" || item.BaseRepository.NameWithOwner != "group/sub/project" || item.Checks == nil || item.Checks.Commit != item.HeadRefOid {
			t.Fatal("fork ownership/pipeline normalization lost")
		}
	})
}
func TestGitlabMissingFieldsAndContradictoryIdentity(t *testing.T) {
	for _, field := range []string{"project_id", "iid", "sha", "confidential", "head_pipeline"} {
		t.Run(field, func(t *testing.T) {
			run := &gitlabFixtureRunner{t: t}
			run.override = func(endpoint string, _ int) ([]byte, bool) {
				if endpoint == "projects/group%2Fsub%2Fproject/merge_requests/5" {
					var mr map[string]any
					_ = json.Unmarshal(gitlabFixture(t, "mr5"), &mr)
					delete(mr, field)
					b, _ := json.Marshal(mr)
					return gitlabHTTP(200, "", string(b)), true
				}
				return nil, false
			}
			item, _, err := (gitlabStatusAdapter{Runner: run.run, Forge: gitlabTestForge()}).Read(context.Background(), workflowTarget{Kind: workflowChangeKind, Number: 5}, nil)
			if field == "project_id" || field == "iid" {
				if err == nil {
					t.Fatal("missing selected resource identity accepted")
				}
			} else if err != nil || item.Observation.Complete {
				t.Fatalf("missing %s not explicit: %v %+v", field, err, item)
			}
		})
	}
	for _, url := range []string{"https://gitlab.example/group/other/-/merge_requests/5", "https://attacker.example/group/sub/project/-/merge_requests/5", "https://gitlab.example/group/sub/project/-/issues/5"} {
		if gitlabResourceURLMatches(url, workflowTarget{Kind: workflowChangeKind, Project: "group/sub/project", Number: 5}, gitlabTestForge()) {
			t.Fatalf("accepted mismatched resource URL %s", url)
		}
	}
}
func TestGitlabTransportMalformedAndOversizedResponses(t *testing.T) {
	for _, data := range [][]byte{[]byte(`{"not":"HTTP headers"}`), gitlabHTTP(200, "", "{} {}"), gitlabHTTP(200, "", "invalid JSON"), bytes.Repeat([]byte("x"), glabResponseLimit+1)} {
		c := newGitlabClient(func(context.Context, ...string) ([]byte, []byte, error) { return data, nil, nil }, gitlabTestForge())
		if _, err := c.get(context.Background(), "GET", "projects/group%2Fsub%2Fproject", &glProject{}); err == nil {
			t.Fatal("unbounded/malformed response accepted")
		}
	}
	b := &boundedGLabOutput{}
	if _, err := b.Write(bytes.Repeat([]byte("x"), glabResponseLimit+1)); err == nil || b.Len() != 0 {
		t.Fatal("subprocess output limit not enforced")
	}
}
func TestGitlabCanonicalHostIdentity(t *testing.T) {
	f := gitlabTestForge()
	f.Endpoint = "https://GITLAB.example:443"
	f.APIBaseURL = "https://GITLAB.example:443/api/v4"
	if !gitlabResourceURLMatches("https://gitlab.example/group/sub/project/-/issues/5", workflowTarget{Kind: workflowIssueKind, Project: f.Project, Number: 5}, f) {
		t.Fatal("equivalent HTTPS host identity rejected")
	}
}

func TestGitlabNullAndContradictoryPageDataStayIncomplete(t *testing.T) {
	for _, test := range []struct{ headers, body string }{{"", "null"}, {"X-Total-Pages: invalid\r\n", "[]"}, {"X-Next-Page: 7\r\n", "[]"}} {
		c := newGitlabClient(func(context.Context, ...string) ([]byte, []byte, error) {
			return gitlabHTTP(200, test.headers, test.body), nil, nil
		}, gitlabTestForge())
		if _, partial, err := gitlabList[int](context.Background(), c, "projects/group%2Fsub%2Fproject/issues/5/notes"); !partial || err == nil {
			t.Fatal("invalid pagination appeared complete")
		}
	}
	calls := 0
	c := newGitlabClient(func(context.Context, ...string) ([]byte, []byte, error) {
		calls++
		return gitlabHTTP(200, "X-Total-Pages: 2\r\n", "[]"), nil, nil
	}, gitlabTestForge())
	if _, partial, err := gitlabList[int](context.Background(), c, "projects/group%2Fsub%2Fproject/issues/5/notes"); err != nil || partial || calls != 2 {
		t.Fatalf("reliable totals ignored: %d %t %v", calls, partial, err)
	}
}
func TestGitlabConflictingNoteVersionsAreNotEvidence(t *testing.T) {
	run := &gitlabFixtureRunner{t: t}
	run.override = func(endpoint string, _ int) ([]byte, bool) {
		if strings.HasPrefix(endpoint, "projects/group%2Fsub%2Fproject/issues/6/discussions?") {
			var notes []glNote
			_ = json.Unmarshal(gitlabFixture(t, "issue-notes"), &notes)
			notes[1].Body = strings.Replace(notes[1].Body, "verdict: approved", "verdict: changes-required", 1)
			data, _ := json.Marshal([]glDiscussion{{ID: "changed-review", Notes: []glNote{notes[1]}}})
			return gitlabHTTP(200, "", string(data)), true
		}
		return nil, false
	}
	item := gitlabReadFixture(t, workflowChangeKind, run)
	if item.Observation.Complete || !item.Observation.Governing[0].Comments.Truncated {
		t.Fatal("conflicting note versions appeared complete")
	}
	records := parseWorkflowRecords(item.Observation.Governing[0].Comments.Nodes, "governing issue")
	for _, record := range records {
		if record.Comment.URL == "https://gitlab.example/group/sub/project/-/issues/6#note_12" && record.Counted {
			t.Fatal("stale review counted despite conflict")
		}
	}
}
func TestGitlabInFlightCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newGitlabClient(func(ctx context.Context, _ ...string) ([]byte, []byte, error) {
		cancel()
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}, gitlabTestForge())
	_, err := c.get(ctx, "GET", "projects/group%2Fsub%2Fproject", &glProject{})
	if err == nil || !strings.Contains(err.Error(), "canceled or timed out") {
		t.Fatal("in-flight cancellation was misreported")
	}
}

func TestGitlabEmptyCollectionZeroTotalPages(t *testing.T) {
	c := newGitlabClient(func(context.Context, ...string) ([]byte, []byte, error) {
		return gitlabHTTP(200, "X-Total-Pages: 0\r\nX-Total: 0\r\n", "[]"), nil, nil
	}, gitlabTestForge())
	if items, partial, err := gitlabList[int](context.Background(), c, "projects/group%2Fsub%2Fproject/issues/5/notes"); len(items) != 0 || partial || err != nil {
		t.Fatalf("empty collection: %v %t %v", items, partial, err)
	}
}

func TestGitlabNativeGoverningAndClosingRelationships(t *testing.T) {
	t.Run("native closing issue without body declaration", func(t *testing.T) {
		run := &gitlabFixtureRunner{t: t}
		run.override = func(endpoint string, _ int) ([]byte, bool) {
			if endpoint == "projects/group%2Fsub%2Fproject/merge_requests/5" {
				var doc map[string]any
				_ = json.Unmarshal(gitlabFixture(t, "mr5"), &doc)
				doc["description"] = ""
				data, _ := json.Marshal(doc)
				return gitlabHTTP(200, "", string(data)), true
			}
			if strings.Contains(endpoint, "/closes_issues?") {
				return gitlabHTTP(200, "", "["+string(gitlabFixture(t, "issue6"))+"]"), true
			}
			return nil, false
		}
		item := gitlabReadFixture(t, workflowChangeKind, run)
		if len(item.Observation.Governing) != 1 || item.Observation.Governing[0].Number != 6 {
			t.Fatal("native governing relationship was not read")
		}
	})
	t.Run("native issue closer keeps MR identity", func(t *testing.T) {
		run := &gitlabFixtureRunner{t: t}
		run.override = func(endpoint string, _ int) ([]byte, bool) {
			if strings.Contains(endpoint, "/closed_by?") {
				return gitlabHTTP(200, "", "["+string(gitlabFixture(t, "mr5"))+"]"), true
			}
			return nil, false
		}
		item := gitlabReadFixture(t, workflowIssueKind, run)
		if len(item.ClosedByChanges) != 1 || item.ClosedByChanges[0].Number != 5 || !strings.Contains(item.ClosedByChanges[0].URL, "/merge_requests/5") {
			t.Fatal("native issue closer confused same-IID issue and MR")
		}
	})
}

func TestGitlabContradictoryRelationshipDoesNotSelectProject(t *testing.T) {
	run := &gitlabFixtureRunner{t: t}
	run.override = func(endpoint string, _ int) ([]byte, bool) {
		if strings.Contains(endpoint, "/closed_by?") {
			var mr map[string]any
			_ = json.Unmarshal(gitlabFixture(t, "mr5"), &mr)
			mr["project_id"] = 999
			data, _ := json.Marshal(mr)
			return gitlabHTTP(200, "", "["+string(data)+"]"), true
		}
		return nil, false
	}
	item := gitlabReadFixture(t, workflowIssueKind, run)
	if item.Observation.Complete || len(item.ClosedByChanges) != 0 {
		t.Fatal("contradictory closing relationship accepted")
	}
	for _, call := range run.calls {
		if strings.HasPrefix(call, "projects/999") {
			t.Fatal("contradictory native ID selected another project")
		}
	}
}

func TestGitlabExactFullFinalPageUsesKnownTotal(t *testing.T) {
	calls := 0
	c := newGitlabClient(func(context.Context, ...string) ([]byte, []byte, error) {
		calls++
		return gitlabHTTP(200, "X-Total-Pages: 1\r\nX-Total: 100\r\n", "["+strings.TrimSuffix(strings.Repeat("1,", 100), ",")+"]"), nil, nil
	}, gitlabTestForge())
	items, partial, err := gitlabList[int](context.Background(), c, "projects/group%2Fsub%2Fproject/issues/5/notes")
	if err != nil || partial || len(items) != 100 || calls != 1 {
		t.Fatalf("final page was over-read or truncated: %d %d %t %v", len(items), calls, partial, err)
	}
}
