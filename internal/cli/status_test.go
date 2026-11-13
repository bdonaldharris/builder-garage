package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fixtures in testdata/status are the responses to statusQuery for real
// Builder Garage records, captured read-only with gh:
//
//	is33/pr46  a clean flow: issue review, equivalence, merge
//	is47/pr48  equivalence not shown after a post-approval commit, a review
//	           request with no recorded disposition, then a merge
//	is35/pr45  a PR review requiring changes at the merged head, and an
//	           approved correction that was never pushed
//
// No test calls the network.

func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "status", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// fixtureGH returns a gh runner that answers every request with data. It
// fails the test if a request reaches it that is not read-only.
func fixtureGH(t *testing.T, data []byte) ghRunner {
	return func(_ context.Context, args ...string) ([]byte, []byte, error) {
		if err := checkReadOnlyGHArgs(args); err != nil {
			t.Errorf("a request that is not read-only reached gh: %v", args)
		}
		return data, nil, nil
	}
}

func loadFixtureItem(t *testing.T, data []byte) *ghItem {
	t.Helper()
	item, err := fetchStatusItem(context.Background(), fixtureGH(t, data),
		ghRepository{Host: "github.com", Owner: "o", Name: "r"}, 1)
	if err != nil {
		t.Fatalf("fetchStatusItem: %v", err)
	}
	return item
}

// editFixture decodes a fixture, lets edit change it, and re-encodes it.
func editFixture(t *testing.T, name string, edit func(item map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(fixtureBytes(t, name), &doc); err != nil {
		t.Fatal(err)
	}
	item := doc["data"].(map[string]any)["repository"].(map[string]any)["issueOrPullRequest"].(map[string]any)
	edit(item)
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func commentNodes(item map[string]any) []any {
	return item["comments"].(map[string]any)["nodes"].([]any)
}

func testConfig() *projectConfig {
	branch := "main"
	return &projectConfig{SchemaVersion: 2, Project: &projectChoices{
		IntegrationBranch: &branch,
		Authorities: &authorities{
			WorkflowOwner:         &authority{Role: "lead maintainer"},
			ChangeAcceptor:        &authority{Role: "maintainer"},
			ProductIntentAcceptor: &authority{Role: "Workflow Owner"},
		},
	}}
}

func notLocal(string, string) (string, string) { return "", "commit abc is not present locally" }

func reportText(t *testing.T, data []byte, cfg *projectConfig) string {
	t.Helper()
	in := statusInputs{
		Item:      loadFixtureItem(t, data),
		Config:    cfg,
		Recompute: notLocal,
		ReadAt:    time.Date(2026, 11, 13, 13, 0, 0, 0, time.UTC),
	}
	if cfg == nil {
		in.ConfigProblem = "no configuration"
	}
	var out bytes.Buffer
	if err := buildStatusReport(in).render(&out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func assertContains(t *testing.T, text string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("output does not contain %q:\n%s", w, text)
		}
	}
}

func assertNotContains(t *testing.T, text string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(text, w) {
			t.Errorf("output unexpectedly contains %q:\n%s", w, text)
		}
	}
}

func TestStatusRealRecords(t *testing.T) {
	cases := []struct {
		fixture string
		want    []string
		not     []string
	}{
		{"pr46", []string{
			"merged at 2026-11-13T12:03:51Z, merge commit ebc2251; head at merge e2af732",
			"approving issue-recorded implementation review: https://github.com/bdonaldharris/builder-garage/issues/33#issuecomment-6852033450",
			"[local git, checked]    not recomputed: commit abc is not present locally",
			"trigger 1 (equivalence not shown) does not apply",
			"trigger 2 (automated findings) does not apply",
			"no Agent #2 PR review is required for the head",
			"checks on the head: success",
			"human acceptance by the change acceptor (maintainer): NOT EVIDENCED",
			"has an Environment / Deployment Requirements section",
			"none: the pull request is merged",
			"the pull request targets the configured integration branch",
		}, []string{"CONTRADICTION", "REQUIRED"}},
		{"pr48", []string{
			"trigger 1 (equivalence not shown) APPLIES: the head's changeset sha256:5c21b674… differs from the latest approval's sha256:4e68f58c… (issue review)",
			"an Agent #2 PR review is REQUIRED, and NO approving one covers the head's changeset",
			"a review of this head was requested at 2026-11-13T12:20:45Z; no disposition for it is recorded",
			"none: the pull request is merged",
		}, []string{"CONTRADICTION"}},
		{"pr45", []string{
			"CONTRADICTION: the equivalence record claims equivalence to https://github.com/bdonaldharris/builder-garage/issues/35#issuecomment-6843780776",
			"trigger 1 (equivalence not shown) APPLIES",
			"reported 1 finding(s) on earlier commit 8cd5a43",
			"trigger 2 (automated findings) does not apply",
			"Agent #2 PR review of the head: https://github.com/bdonaldharris/builder-garage/pull/45#issuecomment-6843923110",
			"[GitHub, asserted]        its verdict: changes-required",
			"but its changeset is NOT the head's: the approved correction is not what the head contains",
		}, nil},
		{"is33", []string{
			"issue is closed",
			"linked pull request #46 is merged",
			"latest Agent #1 completion record: implementation, sha256:689c9051…",
			"its artifact changeset equals the completion record's (sha256:689c9051…)",
			"[GitHub, asserted]        verdict approved",
			"none: the issue is closed",
		}, []string{"CONTRADICTION", "IGNORED", "legacy"}},
		{"is35", []string{
			"latest Agent #1 completion record: correction, sha256:c0145136…",
			"its artifact changeset equals the completion record's (sha256:c0145136…)",
		}, nil},
		{"is47", []string{
			"linked pull request #48 is merged",
			"verdict approved",
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			text := reportText(t, fixtureBytes(t, tc.fixture), testConfig())
			assertContains(t, text, tc.want...)
			assertNotContains(t, text, tc.not...)
		})
	}
}

func TestStatusEveryLineIsLabeled(t *testing.T) {
	text := reportText(t, fixtureBytes(t, "pr45"), testConfig())
	_, body, _ := strings.Cut(text, "\n\n")
	_, body, _ = strings.Cut(body, "\n\n")
	labels := []string{labelChecked, labelAsserted, labelLocalGit, labelConfig, labelLocal, labelInfo}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "  ") {
			continue // section headings and blank lines
		}
		labeled := false
		for _, l := range labels {
			if strings.HasPrefix(strings.TrimSpace(line), "["+l+"]") {
				labeled = true
			}
		}
		if !labeled {
			t.Errorf("line has no source label: %q", line)
		}
	}
}

func TestStatusOpenNextActions(t *testing.T) {
	open := func(item map[string]any) {
		item["state"] = "OPEN"
		item["merged"] = false
		item["mergedAt"] = nil
		item["mergeCommit"] = nil
	}
	approve := func(item map[string]any) {
		reviews := item["reviews"].(map[string]any)
		reviews["nodes"] = append(reviews["nodes"].([]any), map[string]any{
			"state": "APPROVED", "url": "https://example.invalid/review", "authorAssociation": "OWNER",
			"author": map[string]any{"login": "maintainer", "__typename": "User"},
			"commit": map[string]any{"oid": item["headRefOid"]}, "comments": map[string]any{"totalCount": 0},
		})
	}
	cases := []struct {
		name    string
		fixture string
		edit    func(map[string]any)
		want    string
	}{
		{"review requested", "pr48", open, "Agent #2 PR review of the current head (independent-implementation-review)"},
		{"approved correction not pushed", "pr45", open, "Agent #1 commits and pushes exactly the approved correction"},
		{"awaiting human acceptance", "pr46", open, "the change acceptor's approval, a human gate"},
		{"nothing status can check is unmet", "pr46", func(it map[string]any) { open(it); approve(it) },
			"no gate that status can check is unmet; merging remains a human decision"},
		{"approved issue with no pull request", "is47", func(it map[string]any) {
			it["state"] = "OPEN"
			it["closedAt"] = nil
			it["closedByPullRequestsReferences"] = map[string]any{"nodes": []any{}}
		}, "Agent #1 may commit the approved artifact and open a pull request"},
		{"awaiting review", "is47", func(it map[string]any) {
			it["state"] = "OPEN"
			it["closedByPullRequestsReferences"] = map[string]any{"nodes": []any{}}
			nodes := commentNodes(it)
			it["comments"].(map[string]any)["nodes"] = nodes[:1]
		}, "Agent #2 implementation review (independent-implementation-review)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := reportText(t, editFixture(t, tc.fixture, tc.edit), testConfig())
			_, next, _ := strings.Cut(text, "Next eligible action\n")
			next, _, _ = strings.Cut(next, "\n\n")
			assertContains(t, next, "[information only]", tc.want)
		})
	}
}

func TestStatusIgnoresRecordsFromUnassociatedAuthors(t *testing.T) {
	data := editFixture(t, "pr46", func(item map[string]any) {
		for _, c := range commentNodes(item) {
			c := c.(map[string]any)
			if strings.HasPrefix(c["body"].(string), markerImplementation) {
				c["authorAssociation"] = "NONE"
				c["author"] = map[string]any{"login": "passerby", "__typename": "User"}
			}
		}
	})
	text := reportText(t, data, testConfig())
	assertContains(t, text,
		"IGNORED (author association NONE, not owner, member, or collaborator): equivalence record by passerby",
		"NO equivalence record identifies the current head",
		"trigger 1 (equivalence not shown) APPLIES: the head's identity is not recorded")
}

func TestStatusFlagsEditedRecords(t *testing.T) {
	data := editFixture(t, "is47", func(item map[string]any) {
		commentNodes(item)[1].(map[string]any)["lastEditedAt"] = "2026-11-13T12:30:00Z"
	})
	assertContains(t, reportText(t, data, testConfig()), "edited after posting, last at 2026-11-13T12:30:00Z")
}

func TestStatusClassifiesLegacyAndMalformedRecords(t *testing.T) {
	data := editFixture(t, "is47", func(item map[string]any) {
		nodes := commentNodes(item)
		legacy := map[string]any{}
		malformed := map[string]any{}
		for k, v := range nodes[0].(map[string]any) {
			legacy[k], malformed[k] = v, v
		}
		legacy["body"] = markerImplementation + "\n\nStatus: READY FOR AGENT #2 REVIEW\n"
		legacy["url"] = "https://example.invalid/legacy"
		malformed["body"] = markerImplementation + "\n\n```yaml\ngarage_evidence: 1\nrecord: implementation\nunexpected: key\n```\n"
		malformed["url"] = "https://example.invalid/malformed"
		item["comments"].(map[string]any)["nodes"] = append([]any{legacy, malformed}, nodes...)
	})
	text := reportText(t, data, testConfig())
	assertContains(t, text,
		"has no evidence block (legacy, not evaluated): https://example.invalid/legacy",
		"has a malformed evidence block (not evaluated)",
		"https://example.invalid/malformed",
		"its artifact changeset equals the completion record's")
}

func TestStatusReportsTruncatedComments(t *testing.T) {
	data := editFixture(t, "is47", func(item map[string]any) {
		c := item["comments"].(map[string]any)
		c["totalCount"] = 150
		c["pageInfo"] = map[string]any{"hasPreviousPage": true}
	})
	assertContains(t, reportText(t, data, testConfig()), "only the latest 2 of 150 comments were read")
}

func TestStatusWithoutValidConfiguration(t *testing.T) {
	text := reportText(t, fixtureBytes(t, "pr46"), nil)
	assertContains(t, text,
		"[configuration]         not used: no configuration; configuration-dependent checks were not evaluated",
		"human acceptance by the change acceptor: NOT EVIDENCED")
	assertNotContains(t, text, "targets the configured integration branch")
}

func TestReadOnlyGHRefusesEverythingButAQuery(t *testing.T) {
	query := "query=" + statusQuery
	refused := [][]string{
		{"api", "graphql", "-f", "query=mutation { addComment(input: {}) { clientMutationId } }"},
		{"api", "graphql", "-f", "query=subscription { x }"},
		{"api", "graphql", "-f", "query=query { a } mutation { b }"},
		{"api", "graphql", "-f", query, "-X", "POST"},
		{"api", "graphql", "-f", query, "--method", "DELETE"},
		{"api", "graphql", "--input", "body.json"},
		{"api", "graphql", "-f", query, "-f", query},
		{"api", "graphql", "-F", "owner=o"},
		{"api", "repos/o/r/issues/1/comments", "-f", "body=hi"},
		{"issue", "comment", "1", "--body", "hi"},
		{"pr", "merge", "1"},
		{"api"},
		{"api", "graphql", "-f"},
	}
	for _, args := range refused {
		ran := false
		run := readOnlyGH(func(context.Context, ...string) ([]byte, []byte, error) {
			ran = true
			return nil, nil, nil
		})
		if _, _, err := run(context.Background(), args...); !errors.Is(err, errGHNotReadOnly) {
			t.Errorf("args %q: error = %v, want errGHNotReadOnly", args, err)
		}
		if ran {
			t.Errorf("args %q reached gh", args)
		}
	}

	var sent []string
	_, err := fetchStatusItem(context.Background(), func(_ context.Context, args ...string) ([]byte, []byte, error) {
		sent = args
		return fixtureBytes(t, "pr46"), nil, nil
	}, ghRepository{Host: "github.com", Owner: "o", Name: "r"}, 46)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkReadOnlyGHArgs(sent); err != nil {
		t.Errorf("status sent a request that is not read-only: %q", sent)
	}
}

func TestFetchStatusItemErrors(t *testing.T) {
	repo := ghRepository{Host: "github.com", Owner: "o", Name: "r"}
	cases := []struct {
		name   string
		stdout string
		stderr string
		err    error
		want   string
	}{
		{"not installed", "", "", exec.ErrNotFound, "the gh command is not installed"},
		{"not authenticated", "", "To get started with GitHub CLI, please run:  gh auth login", errors.New("exit status 4"),
			"gh is not authenticated"},
		{"other failure", "", "HTTP 502: Bad Gateway", errors.New("exit status 1"), "gh failed: HTTP 502: Bad Gateway"},
		{"invalid JSON", "<html>", "", nil, "not valid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fetchStatusItem(context.Background(), func(context.Context, ...string) ([]byte, []byte, error) {
				return []byte(tc.stdout), []byte(tc.stderr), tc.err
			}, repo, 1)
			var unavailable ghUnavailable
			if !errors.As(err, &unavailable) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want ghUnavailable containing %q", err, tc.want)
			}
		})
	}

	notFound := `{"data":{"repository":{"issueOrPullRequest":null}},"errors":[{"type":"NOT_FOUND","message":"Could not resolve to an issue or pull request with the number of 9."}]}`
	_, err := fetchStatusItem(context.Background(), func(context.Context, ...string) ([]byte, []byte, error) {
		return []byte(notFound), []byte("gh: Could not resolve"), errors.New("exit status 1")
	}, repo, 9)
	if !errors.Is(err, errStatusNotFound) {
		t.Fatalf("error = %v, want errStatusNotFound", err)
	}
}

func statusTestEnv(t *testing.T, dir string, gh ghRunner) statusEnv {
	t.Helper()
	return statusEnv{
		WorkDir:  dir,
		CacheDir: os.UserCacheDir,
		Git:      runGitNoLazyFetch,
		GH:       gh,
		Now:      func() time.Time { return time.Date(2026, 11, 13, 13, 0, 0, 0, time.UTC) },
	}
}

func TestRunStatusWithoutGitHubReportsNoWorkflowState(t *testing.T) {
	dir := newGitRepo(t, "https://github.com/o/r.git")
	isolateUserCache(t)
	unavailable := func(context.Context, ...string) ([]byte, []byte, error) { return nil, nil, exec.ErrNotFound }

	var out bytes.Buffer
	err := runStatus(context.Background(), 48, statusTestEnv(t, dir, unavailable), &out)
	if !errors.Is(err, errGitHubNotConsulted) {
		t.Fatalf("error = %v, want errGitHubNotConsulted", err)
	}
	text := out.String()
	assertContains(t, text,
		"GITHUB WAS NOT CONSULTED: the gh command is not installed.",
		"No workflow state and no eligible action are reported",
		"not used:", // no configuration in the test repository
		"Local history (private annotation; may be incomplete; never evidence)")
	assertNotContains(t, text, "Gates for", "Next eligible action", "trigger")
}

func TestRunStatusWithoutGitHubRemote(t *testing.T) {
	dir := newGitRepo(t, "")
	isolateUserCache(t)
	called := false
	gh := func(context.Context, ...string) ([]byte, []byte, error) { called = true; return nil, nil, nil }
	var out bytes.Buffer
	err := runStatus(context.Background(), 1, statusTestEnv(t, dir, gh), &out)
	if !errors.Is(err, errGitHubNotConsulted) {
		t.Fatalf("error = %v, want errGitHubNotConsulted", err)
	}
	if called {
		t.Error("gh was run without a GitHub repository")
	}
	assertContains(t, out.String(), "GITHUB WAS NOT CONSULTED: the repository has no origin remote.")
}

func TestRunStatusLocalHistoryIsAnnotationOnly(t *testing.T) {
	fixture := fixtureBytes(t, "pr48") // before newGitRepo changes the working directory
	dir := newGitRepo(t, "https://github.com/o/r.git")
	isolateUserCache(t)
	env := recordEnv{WorkDir: dir, CacheDir: os.UserCacheDir, Git: runGit, Now: time.Now}
	// A local event claiming approval must not satisfy the PR-review gate.
	if _, err := recordEvent(context.Background(), recordOptions{
		PR: 48, Type: "pr-review", Outcome: "approved", URL: "https://github.com/o/r/pull/48#issuecomment-1",
	}, env); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runStatus(context.Background(), 48, statusTestEnv(t, dir, fixtureGH(t, fixture)), &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	assertContains(t, text,
		"an Agent #2 PR review is REQUIRED, and NO approving one covers the head's changeset",
		"[local, not evidence]   ",
		"pr-review approved: https://github.com/o/r/pull/48#issuecomment-1")
	gates, _, _ := strings.Cut(text, "Local history")
	assertNotContains(t, gates, "issuecomment-1\n")
}

func TestRunStatusDegradesOnUnreadableLocalHistory(t *testing.T) {
	fixture := fixtureBytes(t, "pr46") // before newGitRepo changes the working directory
	dir := newGitRepo(t, "https://github.com/o/r.git")
	isolateUserCache(t)
	repo, err := identifyRepository(context.Background(), dir, runGit)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	path := historyPath(cacheDir, repo.Key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "not json\n")

	var out bytes.Buffer
	if err := runStatus(context.Background(), 46, statusTestEnv(t, dir, fixtureGH(t, fixture)), &out); err != nil {
		t.Fatal(err)
	}
	assertContains(t, out.String(), "1 unreadable local entries were skipped", "trigger 1 (equivalence not shown) does not apply")
}

func TestRecomputeCommitChangeset(t *testing.T) {
	dir := newGitRepo(t, "")
	base := gitIn(t, dir, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(dir, "f.txt"), "content\n")
	gitIn(t, dir, "add", "f.txt")
	gitIn(t, dir, "commit", "-q", "-m", "add f")
	head := gitIn(t, dir, "rev-parse", "HEAD")
	blob := gitIn(t, dir, "rev-parse", "HEAD:f.txt")

	// The raw listing garage-artifact-v1 hashes, built independently.
	raw := ":000000 100644 " + strings.Repeat("0", len(blob)) + " " + blob + " A\x00f.txt\x00"
	sum := sha256.Sum256([]byte(raw))
	want := "sha256:" + hex.EncodeToString(sum[:])

	got, reason := recomputeCommitChangeset(context.Background(), runGit, dir, base, head)
	if got != want {
		t.Fatalf("changeset = %q (%s), want %q", got, reason, want)
	}

	missing := strings.Repeat("1", len(head))
	if got, reason := recomputeCommitChangeset(context.Background(), runGit, dir, base, missing); got != "" ||
		!strings.Contains(reason, "not present locally") {
		t.Errorf("missing commit: changeset %q, reason %q", got, reason)
	}
	if got, _ := recomputeCommitChangeset(context.Background(), runGit, dir, "--upload-pack=x", head); got != "" {
		t.Error("a recorded value that is not an object name was passed to git")
	}
}

func TestParseStatusTarget(t *testing.T) {
	for arg, want := range map[string]int{"48": 48, "#33": 33} {
		if got, err := parseStatusTarget(arg); err != nil || got != want {
			t.Errorf("parseStatusTarget(%q) = %d, %v", arg, got, err)
		}
	}
	for _, arg := range []string{"0", "-1", "x", "#", "4.8", "https://github.com/o/r/pull/48"} {
		if _, err := parseStatusTarget(arg); err == nil {
			t.Errorf("parseStatusTarget(%q) succeeded", arg)
		}
	}
}

func TestStatusCommandRequiresOneNumber(t *testing.T) {
	for _, args := range [][]string{{"status"}, {"status", "1", "2"}, {"status", "abc"}} {
		if _, _, err := runCLI(t, "dev", args...); err == nil {
			t.Errorf("garage %v succeeded", args)
		}
	}
}

func TestStatusListsConfiguredTriggersAndConditionsWithoutEvaluatingThem(t *testing.T) {
	cfg := testConfig()
	cfg.Project.PRReviewTriggers = &[]namedCondition{{Name: "schema change", AppliesWhen: "the change edits a schema"}}
	cfg.Project.ReleaseConditions = &[]releaseCond{{Name: "release", AppliesWhen: "a release", RequiredEvidence: "a signed tag"}}
	text := reportText(t, fixtureBytes(t, "pr46"), cfg)
	assertContains(t, text,
		`configured PR-review trigger "schema change": not evaluated by status (when it applies: the change edits a schema)`,
		`release condition "release" requires a signed tag: not evaluated by status`)
}

// newPartialClone makes a treeless partial clone of a local bare repository
// over file://, so no network is used. It returns the clone, the source's
// head commit, and its first commit. In the clone the commits are present and
// their trees are not, so any tree read would fetch them from the promisor
// remote.
func newPartialClone(t *testing.T) (clone, head, first string) {
	t.Helper()
	requireGit(t)
	root := t.TempDir()
	work := filepath.Join(root, "work")
	bare := filepath.Join(root, "src.git")
	gitIn(t, root, "init", "-q", "--bare", bare)
	gitIn(t, bare, "config", "uploadpack.allowfilter", "true")
	gitIn(t, root, "init", "-q", work)
	writeFile(t, filepath.Join(work, "a.txt"), "a\n")
	gitIn(t, work, "add", "a.txt")
	gitIn(t, work, "commit", "-q", "-m", "first")
	first = gitIn(t, work, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(work, "b.txt"), "b\n")
	gitIn(t, work, "add", "b.txt")
	gitIn(t, work, "commit", "-q", "-m", "second")
	head = gitIn(t, work, "rev-parse", "HEAD")
	gitIn(t, work, "push", "-q", bare, "HEAD:refs/heads/main")

	url := filepath.ToSlash(bare)
	if !strings.HasPrefix(url, "/") {
		url = "/" + url // a Windows drive path
	}
	clone = filepath.Join(root, "clone")
	gitIn(t, root, "clone", "-q", "--filter=tree:0", "--no-checkout", "file://"+url, clone)
	return clone, head, first
}

func TestRecomputeDeclinesInAPartialCloneWithoutFetching(t *testing.T) {
	clone, head, first := newPartialClone(t)
	before := gitIn(t, clone, "count-objects", "-v")

	for _, git := range []gitRunner{runGit, runGitNoLazyFetch} {
		got, reason := recomputeCommitChangeset(context.Background(), git, clone, first, head)
		if got != "" || !strings.Contains(reason, "partial clone") {
			t.Errorf("changeset %q, reason %q; want no changeset and a partial-clone reason", got, reason)
		}
	}
	if after := gitIn(t, clone, "count-objects", "-v"); after != before {
		t.Errorf("objects were fetched:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestRunStatusDoesNotRecomputeInAPartialClone(t *testing.T) {
	fixture := editFixture(t, "pr46", func(map[string]any) {})
	clone, _, _ := newPartialClone(t)
	// Status needs a GitHub-shaped origin. Move the promisor to another
	// remote that stays reachable over file://, so an attempted lazy fetch
	// would succeed and show as a change to the object store.
	gitIn(t, clone, "remote", "rename", "origin", "src")
	gitIn(t, clone, "remote", "add", "origin", "https://github.com/o/r.git")
	if got := gitIn(t, clone, "config", "--get", "remote.src.promisor"); got != "true" {
		t.Fatalf("remote.src.promisor = %q after rename", got)
	}
	isolateUserCache(t)
	before := gitIn(t, clone, "count-objects", "-v")

	var out bytes.Buffer
	if err := runStatus(context.Background(), 46, statusTestEnv(t, clone, fixtureGH(t, fixture)), &out); err != nil {
		t.Fatal(err)
	}
	assertContains(t, out.String(), "[local git, checked]    not recomputed: this is a partial clone")
	assertNotContains(t, out.String(), "recomputed from local objects")
	if after := gitIn(t, clone, "count-objects", "-v"); after != before {
		t.Errorf("objects were fetched:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestPartialCloneReason(t *testing.T) {
	exitOne := exec.Command("git", "config", "--get", "garage.status.test.absent")
	exitOne.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	errNoKeys := exitOne.Run()
	var exitErr *exec.ExitError
	if !errors.As(errNoKeys, &exitErr) || exitErr.ExitCode() != 1 {
		t.Skipf("cannot obtain git's exit status 1: %v", errNoKeys)
	}
	cases := []struct {
		name    string
		out     string
		err     error
		partial bool
	}{
		{"ordinary clone", "", errNoKeys, false},
		{"promisor remote", "remote.origin.promisor true\n", nil, true},
		{"older partial clone", "extensions.partialclone origin\n", nil, true},
		{"promisor turned off", "remote.origin.promisor false\n", nil, false},
		{"git cannot run", "", errors.New("exec: git: not found"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			git := func(context.Context, string, ...string) ([]byte, error) { return []byte(tc.out), tc.err }
			if got := partialCloneReason(context.Background(), git, "."); (got != "") != tc.partial {
				t.Errorf("partialCloneReason = %q, want partial=%v", got, tc.partial)
			}
		})
	}
}
