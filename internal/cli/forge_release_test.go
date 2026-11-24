package cli

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	releaseTestTag    = "v1.2.0"
	releaseTestPrefix = "projects/group%2Fsub%2Fproject"
	releaseIssueURL   = "https://gitlab.example/group/sub/project/-/issues/9"
	packageBase       = "https://gitlab.example/api/v4/projects/group%2Fsub%2Fproject/packages/generic/app/1.2.0/"
)

var releaseTestRef = strings.Repeat("c", 40)

// fakeGitLab is a small stateful GitLab for release reads and writes. alter may
// replace any response; writes records every non-GET request.
type fakeGitLab struct {
	t          *testing.T
	tags       map[string]glTag
	releases   map[string]glRelease
	links      []glReleaseLink
	packages   []glPackage
	files      map[int][]glPackageFile
	pipelines  []glTagPipeline
	notes      []operationNote
	access     int
	writes     []string
	bodies     []map[string]string
	calls      []string
	alter      func(method, path string, status int, body any) (int, any)
	nextLinkID int
}

func newFakeGitLab(t *testing.T) *fakeGitLab {
	tag := glTag{Name: releaseTestTag, Protected: boolPtr(true)}
	tag.Commit.ID = releaseTestRef
	rel := glRelease{TagName: releaseTestTag, Name: "Release 1.2.0", Description: "Notes"}
	rel.Commit.ID = releaseTestRef
	rel.Links.Self = "https://gitlab.example/group/sub/project/-/releases/v1.2.0"
	return &fakeGitLab{
		t:        t,
		tags:     map[string]glTag{releaseTestTag: tag},
		releases: map[string]glRelease{releaseTestTag: rel},
		links: []glReleaseLink{
			{ID: 1, Name: "app.tar.gz", URL: packageBase + "app.tar.gz", LinkType: "package"},
			{ID: 2, Name: "app.tar.gz.sig", URL: packageBase + "app.tar.gz.sig", LinkType: "other"},
			{ID: 3, Name: "app.intoto.jsonl", URL: packageBase + "app.intoto.jsonl", LinkType: "other"},
		},
		packages: []glPackage{{ID: 40, Name: "app", Version: "1.2.0", PackageType: "generic"}},
		files: map[int][]glPackageFile{40: {
			{ID: 1, PackageID: 40, FileName: "app.tar.gz", FileSHA256: strings.Repeat("1", 64)},
			{ID: 2, PackageID: 40, FileName: "app.tar.gz.sig", FileSHA256: strings.Repeat("2", 64)},
			{ID: 3, PackageID: 40, FileName: "app.intoto.jsonl", FileSHA256: strings.Repeat("3", 64)},
		}},
		pipelines:  []glTagPipeline{{glPipeline: glPipeline{ID: 77, SHA: releaseTestRef, Ref: releaseTestTag, Status: "success", ProjectID: 7}, Tag: boolPtr(true)}},
		access:     30,
		nextLinkID: 10,
		notes:      []operationNote{delegationNote(31, "owner", "release-create", releaseTestTag, releaseTestRef, "delegated", 0)},
	}
}

func delegationNote(id int, author, operation, tag, ref, designation string, minute int) operationNote {
	body := markerDelegation + "\n\n```yaml\ngarage_delegation: 1\nissue: 9\noperation: " + operation + "\ntag: " + tag + "\nref: " + ref + "\ndesignation: " + designation + "\n```\n\nWorkflow Owner: Owner. Executing agent: none.\n"
	return operationNote{glNote: glNote{ID: id, Body: body, Author: glUser{ID: id, Username: author}, CreatedAt: time.Date(2026, 11, 24, 10, minute, 0, 0, time.UTC)}, ProjectID: 7, NoteableID: 90, NoteableIID: 9, NoteableType: "Issue"}
}

func (g *fakeGitLab) respond(method, endpoint string, input []byte) []byte {
	g.calls = append(g.calls, method+" "+endpoint)
	if method != "GET" {
		g.writes = append(g.writes, method+" "+endpoint)
		var body map[string]string
		if err := json.Unmarshal(input, &body); err != nil {
			g.t.Fatalf("write body is not literal JSON: %v", err)
		}
		g.bodies = append(g.bodies, body)
	}
	path, rawQuery, _ := strings.Cut(endpoint, "?")
	query, _ := url.ParseQuery(rawQuery)
	status, body := g.route(method, path, query)
	if g.alter != nil {
		status, body = g.alter(method, endpoint, status, body)
	}
	data, err := json.Marshal(body)
	if err != nil {
		g.t.Fatal(err)
	}
	return gitlabHTTP(status, "", string(data))
}

func (g *fakeGitLab) route(method, path string, query url.Values) (int, any) {
	notFound := map[string]string{"message": "404 Not found"}
	if path == releaseTestPrefix {
		return 200, glProject{ID: 7, Path: "group/sub/project"}
	}
	if path == "user" {
		return 200, glUser{ID: 1, Username: "builder"}
	}
	rest, ok := strings.CutPrefix(path, releaseTestPrefix+"/")
	if !ok {
		g.t.Fatalf("request outside the configured project: %s", path)
	}
	parts := strings.Split(rest, "/")
	unescape := func(s string) string { v, _ := url.PathUnescape(s); return v }
	switch {
	case rest == "members/all/1":
		return 200, glMembership{ID: 1, AccessLevel: &g.access}
	case rest == "issues/9":
		return 200, glResource{ID: 90, IID: 9, ProjectID: 7, WebURL: releaseIssueURL}
	case rest == "issues/9/notes":
		return 200, g.notes
	case len(parts) == 3 && parts[0] == "repository" && parts[1] == "tags":
		if tag, ok := g.tags[unescape(parts[2])]; ok {
			return 200, tag
		}
		return 404, notFound
	case rest == "repository/tags" && method == "POST":
		body := g.bodies[len(g.bodies)-1]
		tag := glTag{Name: body["tag_name"], Message: body["message"], Protected: boolPtr(true)}
		tag.Commit.ID = body["ref"]
		g.tags[tag.Name] = tag
		return 201, tag
	case rest == "releases" && method == "POST":
		body := g.bodies[len(g.bodies)-1]
		tag, ok := g.tags[body["tag_name"]]
		if !ok {
			return 422, map[string]string{"message": "tag missing"}
		}
		rel := glRelease{TagName: body["tag_name"], Name: body["name"], Description: body["description"]}
		rel.Commit.ID = tag.Commit.ID
		g.releases[rel.TagName] = rel
		return 201, rel
	case len(parts) == 2 && parts[0] == "releases":
		rel, ok := g.releases[unescape(parts[1])]
		if !ok {
			return 404, notFound
		}
		if method == "PUT" {
			body := g.bodies[len(g.bodies)-1]
			rel.Name, rel.Description = body["name"], body["description"]
			g.releases[rel.TagName] = rel
		}
		return 200, rel
	case len(parts) == 4 && parts[0] == "releases" && parts[3] == "links":
		if method == "POST" {
			body := g.bodies[len(g.bodies)-1]
			g.nextLinkID++
			link := glReleaseLink{ID: g.nextLinkID, Name: body["name"], URL: body["url"], LinkType: body["link_type"]}
			g.links = append(g.links, link)
			return 201, link
		}
		return 200, g.links
	case len(parts) == 5 && parts[0] == "releases" && parts[3] == "links":
		for _, l := range g.links {
			if strconv.Itoa(l.ID) == parts[4] {
				return 200, l
			}
		}
		return 404, notFound
	case rest == "pipelines":
		var list []glPipeline
		for _, p := range g.pipelines {
			if p.Ref == query.Get("ref") && p.SHA == query.Get("sha") {
				list = append(list, p.glPipeline)
			}
		}
		if list == nil {
			list = []glPipeline{}
		}
		return 200, list
	case len(parts) == 2 && parts[0] == "pipelines":
		for _, p := range g.pipelines {
			if strconv.Itoa(p.ID) == parts[1] {
				return 200, p
			}
		}
		return 404, notFound
	case rest == "packages":
		var list []glPackage
		for _, p := range g.packages {
			if strings.Contains(p.Name, query.Get("package_name")) && p.Version == query.Get("package_version") {
				list = append(list, p)
			}
		}
		if list == nil {
			list = []glPackage{}
		}
		return 200, list
	case len(parts) == 3 && parts[0] == "packages" && parts[2] == "package_files":
		id, _ := strconv.Atoi(parts[1])
		files := g.files[id]
		if files == nil {
			files = []glPackageFile{}
		}
		return 200, files
	}
	g.t.Fatalf("unexpected fake GitLab request %s %s", method, path)
	return 0, nil
}

// readRunner is the GET-only observation runner; it fails the test on a write.
func (g *fakeGitLab) readRunner(ctx context.Context, args ...string) ([]byte, []byte, error) {
	g.t.Helper()
	if _, ok := ctx.Deadline(); !ok {
		g.t.Error("release read has no timeout")
	}
	if len(args) != 7 || args[0] != "api" || args[2] != "gitlab.example" || args[3] != "--method" || args[4] != "GET" || args[5] != "--include" {
		g.t.Fatalf("release inspection made a non-GET or unsafe call: %q", args)
	}
	return g.respond("GET", args[6], nil), nil, nil
}

func (g *fakeGitLab) writeRunner(ctx context.Context, input []byte, args ...string) ([]byte, []byte, error) {
	g.t.Helper()
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if len(args) < 7 || args[0] != "api" || args[2] != "gitlab.example" || args[3] != "--method" || args[5] != "--include" {
		g.t.Fatalf("unsafe invocation: %q", args)
	}
	if args[4] != "GET" && (len(args) != 9 || args[7] != "--input" || args[8] != "-") {
		g.t.Fatal("write body was not sent as literal JSON stdin")
	}
	return g.respond(args[4], args[6], input), nil, nil
}

func gateState(t *testing.T, out releaseInspection, name string) workflowPolicyGate {
	t.Helper()
	for _, g := range out.Gates {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("gate %q missing from %+v", name, out.Gates)
	return workflowPolicyGate{}
}

func TestReleaseInspectionPassesCheckedFacts(t *testing.T) {
	g := newFakeGitLab(t)
	out, err := inspectGitlabRelease(t.Context(), gitlabTestForge(), releaseTestTag, releaseTestRef, g.readRunner)
	if err != nil {
		t.Fatal(err)
	}
	for _, gate := range out.Gates {
		if gate.State != policyPassed || len(gate.Sources) == 0 || !strings.HasPrefix(gate.Source, "https://gitlab.example/api/v4/projects/") {
			t.Fatalf("checked fact did not pass with a source: %+v", gate)
		}
	}
	if len(out.Gates) != 7 || out.Commit != releaseTestRef || out.URL != "https://gitlab.example/group/sub/project/-/releases/v1.2.0" || out.Reads == 0 {
		t.Fatalf("incomplete inspection: %+v", out)
	}
	if len(out.Assets) != 3 || out.Assets[0].SHA256 != strings.Repeat("1", 64) || out.Assets[1].Kind != "signature" || out.Assets[2].Kind != "provenance" {
		t.Fatalf("asset evidence lost: %+v", out.Assets)
	}
	if !strings.Contains(out.Note, "no cryptographic verification") || !strings.Contains(gateState(t, out, "signature evidence").Detail, "no cryptographic verification") {
		t.Fatal("signing outcome overstates what was checked")
	}
	if g.writes != nil {
		t.Fatalf("inspection wrote: %v", g.writes)
	}
	// One package lookup serves every file of the same package version.
	packageLookups := 0
	for _, c := range g.calls {
		if strings.Contains(c, "/packages?") {
			packageLookups++
		}
	}
	if packageLookups != 1 {
		t.Fatalf("package lookups not shared: %d", packageLookups)
	}
}

func TestReleaseInspectionOutcomes(t *testing.T) {
	forbidden := func(match string) func(string, string, int, any) (int, any) {
		return func(method, path string, status int, body any) (int, any) {
			if strings.Contains(path, match) {
				return 403, map[string]string{"message": "403 Forbidden"}
			}
			return status, body
		}
	}
	for _, test := range []struct {
		name   string
		ref    string
		setup  func(*fakeGitLab)
		expect map[string]string
	}{
		{"tag missing", "", func(g *fakeGitLab) { delete(g.tags, releaseTestTag) }, map[string]string{"tag": policyFailed, "protected tag": policyUnavailable, "release": policyUnavailable, "tag pipeline": policyUnavailable, "durable assets": policyUnavailable}},
		{"tag at another commit", strings.Repeat("d", 40), nil, map[string]string{"tag": policyFailed}},
		{"tag unprotected", "", func(g *fakeGitLab) {
			tag := g.tags[releaseTestTag]
			tag.Protected = boolPtr(false)
			g.tags[releaseTestTag] = tag
		}, map[string]string{"protected tag": policyFailed, "tag": policyPassed}},
		{"protection unreported", "", func(g *fakeGitLab) { tag := g.tags[releaseTestTag]; tag.Protected = nil; g.tags[releaseTestTag] = tag }, map[string]string{"protected tag": policyUnavailable}},
		{"tag inaccessible", "", func(g *fakeGitLab) { g.alter = forbidden("/repository/tags/") }, map[string]string{"tag": policyUnavailable, "release": policyUnavailable}},
		{"release missing", "", func(g *fakeGitLab) { delete(g.releases, releaseTestTag) }, map[string]string{"release": policyFailed, "durable assets": policyUnavailable, "signature evidence": policyUnavailable, "provenance evidence": policyUnavailable}},
		{"release on another commit", "", func(g *fakeGitLab) {
			r := g.releases[releaseTestTag]
			r.Commit.ID = strings.Repeat("e", 40)
			g.releases[releaseTestTag] = r
		}, map[string]string{"release": policyFailed, "durable assets": policyUnavailable}},
		{"no tag pipeline", "", func(g *fakeGitLab) { g.pipelines = nil }, map[string]string{"tag pipeline": policyFailed, "release": policyPassed}},
		{"pipeline running", "", func(g *fakeGitLab) { g.pipelines[0].Status = "running" }, map[string]string{"tag pipeline": policyUnavailable}},
		{"pipeline failed", "", func(g *fakeGitLab) { g.pipelines[0].Status = "failed" }, map[string]string{"tag pipeline": policyFailed}},
		{"branch pipeline", "", func(g *fakeGitLab) { g.pipelines[0].Tag = boolPtr(false) }, map[string]string{"tag pipeline": policyUnavailable}},
		{"pipelines inaccessible", "", func(g *fakeGitLab) { g.alter = forbidden("/pipelines?") }, map[string]string{"tag pipeline": policyUnavailable, "release": policyPassed}},
		{"job artifact link", "", func(g *fakeGitLab) {
			g.links[0].URL = "https://gitlab.example/group/sub/project/-/jobs/5/artifacts/raw/app.tar.gz"
		}, map[string]string{"durable assets": policyFailed, "provenance evidence": policyPassed}},
		{"API job artifact link", "", func(g *fakeGitLab) {
			g.links[0].URL = "https://gitlab.example/api/v4/projects/7/jobs/artifacts/v1.2.0/download"
		}, map[string]string{"durable assets": policyFailed}},
		{"external link", "", func(g *fakeGitLab) { g.links[0].URL = "https://downloads.example/app.tar.gz" }, map[string]string{"durable assets": policyUnavailable}},
		{"plain HTTP link", "", func(g *fakeGitLab) { g.links[0].URL = "http://gitlab.example/app.tar.gz" }, map[string]string{"durable assets": policyFailed}},
		{"numeric project package link", "", func(g *fakeGitLab) {
			g.links[0].URL = "https://gitlab.example/api/v4/projects/7/packages/generic/app/1.2.0/app.tar.gz"
		}, map[string]string{"durable assets": policyPassed}},
		{"other project package link", "", func(g *fakeGitLab) {
			g.links[0].URL = "https://gitlab.example/api/v4/projects/8/packages/generic/app/1.2.0/app.tar.gz"
		}, map[string]string{"durable assets": policyUnavailable}},
		{"package file absent", "", func(g *fakeGitLab) { g.files[40] = g.files[40][1:] }, map[string]string{"durable assets": policyFailed}},
		{"digest unreported", "", func(g *fakeGitLab) { g.files[40][0].FileSHA256 = "" }, map[string]string{"durable assets": policyUnavailable}},
		{"ambiguous package file", "", func(g *fakeGitLab) {
			g.files[40] = append(g.files[40], glPackageFile{ID: 9, PackageID: 40, FileName: "app.tar.gz", FileSHA256: strings.Repeat("9", 64)})
		}, map[string]string{"durable assets": policyFailed}},
		{"identical duplicate package file", "", func(g *fakeGitLab) {
			g.files[40] = append(g.files[40], glPackageFile{ID: 9, PackageID: 40, FileName: "app.tar.gz", FileSHA256: strings.Repeat("1", 64)})
		}, map[string]string{"durable assets": policyPassed}},
		{"similarly named package ignored", "", func(g *fakeGitLab) {
			g.packages = []glPackage{{ID: 41, Name: "app-extra", Version: "1.2.0", PackageType: "generic"}}
		}, map[string]string{"durable assets": policyFailed}},
		{"packages inaccessible", "", func(g *fakeGitLab) { g.alter = forbidden("/packages?") }, map[string]string{"durable assets": policyUnavailable, "signature evidence": policyUnavailable}},
		{"signature missing", "", func(g *fakeGitLab) { g.links = append(g.links[:1], g.links[2]) }, map[string]string{"signature evidence": policyFailed, "durable assets": policyPassed}},
		{"signature not durable", "", func(g *fakeGitLab) { g.files[40] = append(g.files[40][:1], g.files[40][2]) }, map[string]string{"signature evidence": policyFailed, "durable assets": policyFailed}},
		{"provenance missing", "", func(g *fakeGitLab) { g.links = g.links[:2] }, map[string]string{"provenance evidence": policyFailed, "signature evidence": policyPassed}},
		{"no links", "", func(g *fakeGitLab) { g.links = []glReleaseLink{} }, map[string]string{"durable assets": policyNotRequired, "signature evidence": policyUnavailable, "provenance evidence": policyFailed}},
		{"links inaccessible", "", func(g *fakeGitLab) { g.alter = forbidden("/assets/links") }, map[string]string{"durable assets": policyUnavailable, "signature evidence": policyUnavailable, "provenance evidence": policyUnavailable}},
		{"links truncated", "", func(g *fakeGitLab) {
			full := make([]glReleaseLink, gitlabPageSize)
			for i := range full {
				full[i] = glReleaseLink{ID: i + 1, Name: "f" + strconv.Itoa(i), URL: packageBase + "f" + strconv.Itoa(i)}
			}
			g.links = full
		}, map[string]string{"durable assets": policyUnavailable}},
		{"malformed release", "", func(g *fakeGitLab) {
			g.alter = func(method, path string, status int, body any) (int, any) {
				if strings.HasSuffix(path, "/releases/v1.2.0") {
					return 200, "not an object"
				}
				return status, body
			}
		}, map[string]string{"release": policyUnavailable, "durable assets": policyUnavailable}},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := newFakeGitLab(t)
			if test.setup != nil {
				test.setup(g)
			}
			out, err := inspectGitlabRelease(t.Context(), gitlabTestForge(), releaseTestTag, test.ref, g.readRunner)
			if err != nil {
				t.Fatal(err)
			}
			for name, state := range test.expect {
				if got := gateState(t, out, name); got.State != state {
					t.Fatalf("%s: got %s (%s), want %s", name, got.State, got.Detail, state)
				}
			}
			for _, gate := range out.Gates {
				if gate.Required && gate.State == policyPassed && strings.Contains(gate.Detail, "unavailable") {
					t.Fatalf("unavailable fact passed: %+v", gate)
				}
			}
			if g.writes != nil {
				t.Fatalf("inspection wrote: %v", g.writes)
			}
		})
	}
}

func TestReleaseInspectionRejectsInvalidInput(t *testing.T) {
	g := newFakeGitLab(t)
	for _, test := range []struct{ tag, ref string }{{"bad tag", ""}, {"v1..2", ""}, {"v1:2", ""}, {"-v1", ""}, {"v1", "abc"}} {
		if _, err := inspectGitlabRelease(t.Context(), gitlabTestForge(), test.tag, test.ref, g.readRunner); err == nil {
			t.Fatalf("accepted %q %q", test.tag, test.ref)
		}
	}
	if g.calls != nil {
		t.Fatal("invalid input reached GitLab")
	}
}

func TestReleaseObservationAllowlist(t *testing.T) {
	c := newGitlabClient(nil, gitlabTestForge())
	sha := strings.Repeat("a", 40)
	for _, endpoint := range []string{
		releaseTestPrefix + "/repository/tags/v1.2.0",
		releaseTestPrefix + "/repository/tags/release%2Fv1",
		releaseTestPrefix + "/releases/v1.2.0",
		releaseTestPrefix + "/releases/v1.2.0/assets/links?per_page=100&page=1",
		releaseTestPrefix + "/pipelines?ref=v1.2.0&sha=" + sha + "&per_page=100&page=1",
		releaseTestPrefix + "/packages?package_name=app&package_type=generic&package_version=1.2.0&per_page=100&page=1",
		releaseTestPrefix + "/packages/40/package_files?per_page=100&page=2",
	} {
		if !c.allowed("GET", endpoint) {
			t.Fatalf("release read refused: %s", endpoint)
		}
	}
	for _, endpoint := range []string{
		releaseTestPrefix + "/repository/tags/v1.2.0?per_page=100",
		releaseTestPrefix + "/repository/tags",
		releaseTestPrefix + "/releases",
		releaseTestPrefix + "/releases/v1.2.0/assets/links/5",
		releaseTestPrefix + "/releases/v1.2.0/evidences",
		releaseTestPrefix + "/pipelines?ref=v1.2.0&per_page=100&page=1",
		releaseTestPrefix + "/pipelines?ref=v1.2.0&sha=abc",
		releaseTestPrefix + "/pipelines?ref=v1.2.0&sha=" + sha + "&status=success",
		releaseTestPrefix + "/packages?package_name=app&package_type=npm&package_version=1.2.0",
		releaseTestPrefix + "/packages?package_name=app&package_type=generic",
		releaseTestPrefix + "/packages/40",
		releaseTestPrefix + "/packages/40/package_files/1",
		releaseTestPrefix + "/repository/tags/v1%3A2",
		releaseTestPrefix + "/merge_requests/5/notes?ref=v1",
		"projects/other%2Fproject/releases/v1.2.0",
	} {
		if c.allowed("GET", endpoint) {
			t.Fatalf("release read outside the allowlist accepted: %s", endpoint)
		}
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		if c.allowed(method, releaseTestPrefix+"/releases/v1.2.0") {
			t.Fatalf("%s allowed through observation", method)
		}
	}
}

func releaseTestConfig(t *testing.T) projectConfig {
	cfg := mustDecodeGitlabConfig(t)
	cfg.Project.Authorities.WorkflowOwner.Members = []string{"github:owner", "gitlab:gitlab.example/owner"}
	return *cfg
}

func releaseOp(operation string) releaseOperation {
	return releaseOperation{Operation: operation, Tag: releaseTestTag, Ref: releaseTestRef, Title: "Release 1.2.0", Notes: "Approved notes\n/close stays literal\n", Actor: "gitlab:gitlab.example/builder", Delegation: releaseIssueURL + "#note_31", Authorized: true}
}

func TestReleaseMutationsVerifyByReadBack(t *testing.T) {
	cfg := releaseTestConfig(t)
	t.Run("tag", func(t *testing.T) {
		g := newFakeGitLab(t)
		delete(g.tags, releaseTestTag)
		delete(g.releases, releaseTestTag)
		g.notes = []operationNote{delegationNote(31, "owner", "tag", releaseTestTag, releaseTestRef, "delegated", 0)}
		o := releaseOp(releaseOpTag)
		o.Message = "Release 1.2.0"
		result, err := executeGitlabRelease(t.Context(), cfg, o, g.writeRunner)
		if err != nil || !result.Verified || result.URL != "https://gitlab.example/group/sub/project/-/tags/v1.2.0" {
			t.Fatalf("tag not verified: %+v %v", result, err)
		}
		if len(g.writes) != 1 || g.writes[0] != "POST "+releaseTestPrefix+"/repository/tags" || g.bodies[0]["ref"] != releaseTestRef || g.bodies[0]["tag_name"] != releaseTestTag || g.bodies[0]["message"] != "Release 1.2.0" {
			t.Fatalf("unexpected tag write: %v %v", g.writes, g.bodies)
		}
	})
	t.Run("create", func(t *testing.T) {
		g := newFakeGitLab(t)
		delete(g.releases, releaseTestTag)
		result, err := executeGitlabRelease(t.Context(), cfg, releaseOp(releaseOpCreate), g.writeRunner)
		if err != nil || !result.Verified || result.URL != "https://gitlab.example/group/sub/project/-/releases/v1.2.0" {
			t.Fatalf("release not verified: %+v %v", result, err)
		}
		body := g.bodies[0]
		if len(g.writes) != 1 || g.writes[0] != "POST "+releaseTestPrefix+"/releases" || body["description"] != "Approved notes\n/close stays literal\n" || body["name"] != "Release 1.2.0" {
			t.Fatalf("unexpected release write: %v %v", g.writes, body)
		}
		if _, ok := body["ref"]; ok {
			t.Fatal("release creation could create a tag implicitly")
		}
	})
	t.Run("update", func(t *testing.T) {
		g := newFakeGitLab(t)
		g.notes = []operationNote{delegationNote(31, "owner", "release-update", releaseTestTag, releaseTestRef, "delegated", 0)}
		result, err := executeGitlabRelease(t.Context(), cfg, releaseOp(releaseOpUpdate), g.writeRunner)
		if err != nil || !result.Verified || len(g.writes) != 1 || g.writes[0] != "PUT "+releaseTestPrefix+"/releases/v1.2.0" {
			t.Fatalf("update not verified: %+v %v %v", result, err, g.writes)
		}
	})
	t.Run("link", func(t *testing.T) {
		g := newFakeGitLab(t)
		g.notes = []operationNote{delegationNote(31, "owner", "link", releaseTestTag, releaseTestRef, "delegated", 0)}
		o := releaseOp(releaseOpLink)
		o.LinkName, o.LinkURL, o.LinkType = "app.zip", packageBase+"app.zip", "package"
		result, err := executeGitlabRelease(t.Context(), cfg, o, g.writeRunner)
		if err != nil || !result.Verified || len(g.writes) != 1 || g.bodies[0]["url"] != packageBase+"app.zip" || g.bodies[0]["link_type"] != "package" {
			t.Fatalf("link not verified: %+v %v %v", result, err, g.bodies)
		}
	})
}

// No delegation, identity or membership failure may reach a write.
func TestReleaseDelegationAuthorityBoundary(t *testing.T) {
	cfg := releaseTestConfig(t)
	for _, test := range []struct {
		name   string
		cfg    func(*projectConfig)
		op     func(*releaseOperation)
		setup  func(*fakeGitLab)
		needle string
	}{
		{"not authorized", nil, func(o *releaseOperation) { o.Authorized = false }, nil, "--authorized"},
		{"no delegation", nil, func(o *releaseOperation) { o.Delegation = "" }, nil, "--delegation"},
		{"GitHub actor", nil, func(o *releaseOperation) { o.Actor = "github:builder" }, nil, "--actor"},
		{"actor mismatch", nil, func(o *releaseOperation) { o.Actor = "gitlab:gitlab.example/other" }, nil, "expected actor"},
		{"reporter access", nil, nil, func(g *fakeGitLab) { g.access = 20 }, "Developer-or-higher"},
		{"delegation is not a note", nil, func(o *releaseOperation) { o.Delegation = releaseIssueURL }, nil, "exact GitLab issue note"},
		{"delegation on an MR", nil, func(o *releaseOperation) {
			o.Delegation = "https://gitlab.example/group/sub/project/-/merge_requests/9#note_31"
		}, nil, "release issue"},
		{"delegation in another project", nil, func(o *releaseOperation) {
			o.Delegation = "https://gitlab.example/other/project/-/issues/9#note_31"
		}, nil, "release issue"},
		{"author is not the Workflow Owner", nil, nil, func(g *fakeGitLab) {
			g.notes = []operationNote{delegationNote(31, "builder", "release-create", releaseTestTag, releaseTestRef, "delegated", 0)}
		}, "current Workflow Owner delegation"},
		{"Workflow Owner has no GitLab identity", func(c *projectConfig) {
			c.Project.Authorities.WorkflowOwner.Members = []string{"owner"}
		}, nil, nil, "current Workflow Owner delegation"},
		{"different operation", nil, nil, func(g *fakeGitLab) {
			g.notes = []operationNote{delegationNote(31, "owner", "release-update", releaseTestTag, releaseTestRef, "delegated", 0)}
		}, "current Workflow Owner delegation"},
		{"different tag", nil, nil, func(g *fakeGitLab) {
			g.notes = []operationNote{delegationNote(31, "owner", "release-create", "v1.2.1", releaseTestRef, "delegated", 0)}
		}, "current Workflow Owner delegation"},
		{"different ref", nil, nil, func(g *fakeGitLab) {
			g.notes = []operationNote{delegationNote(31, "owner", "release-create", releaseTestTag, strings.Repeat("d", 40), "delegated", 0)}
		}, "current Workflow Owner delegation"},
		{"later withdrawal", nil, nil, func(g *fakeGitLab) {
			g.notes = append(g.notes, delegationNote(32, "owner", "release-create", releaseTestTag, releaseTestRef, "withdrawn", 5))
		}, "current Workflow Owner delegation"},
		{"linked note withdraws", nil, nil, func(g *fakeGitLab) {
			g.notes = []operationNote{delegationNote(31, "owner", "release-create", releaseTestTag, releaseTestRef, "withdrawn", 0)}
		}, "withdrew"},
		{"superseded by a later delegation", nil, nil, func(g *fakeGitLab) {
			g.notes = append(g.notes, delegationNote(32, "owner", "release-create", releaseTestTag, releaseTestRef, "delegated", 5))
		}, "current Workflow Owner delegation"},
		{"wrong issue field", nil, nil, func(g *fakeGitLab) {
			g.notes[0].Body = strings.Replace(g.notes[0].Body, "issue: 9", "issue: 8", 1)
		}, "current Workflow Owner delegation"},
		{"malformed block", nil, nil, func(g *fakeGitLab) {
			g.notes[0].Body = strings.Replace(g.notes[0].Body, "garage_delegation: 1", "garage_delegation: 1\nextra: true", 1)
		}, "current Workflow Owner delegation"},
		{"marker not first", nil, nil, func(g *fakeGitLab) { g.notes[0].Body = "Note\n" + g.notes[0].Body }, "current Workflow Owner delegation"},
		{"system note", nil, nil, func(g *fakeGitLab) { g.notes[0].System = true }, "current Workflow Owner delegation"},
		{"notes inaccessible", nil, nil, func(g *fakeGitLab) {
			g.alter = func(method, path string, status int, body any) (int, any) {
				if strings.Contains(path, "/notes") {
					return 403, map[string]string{}
				}
				return status, body
			}
		}, "cannot be verified"},
		{"foreign note in collection", nil, nil, func(g *fakeGitLab) { g.notes[0].NoteableIID = 8 }, "identity mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := newFakeGitLab(t)
			delete(g.releases, releaseTestTag)
			if test.setup != nil {
				test.setup(g)
			}
			c := cfg
			if test.cfg != nil {
				clone := releaseTestConfig(t)
				test.cfg(&clone)
				c = clone
			}
			o := releaseOp(releaseOpCreate)
			if test.op != nil {
				test.op(&o)
			}
			_, err := executeGitlabRelease(t.Context(), c, o, g.writeRunner)
			if err == nil || !strings.Contains(err.Error(), test.needle) {
				t.Fatalf("got %v, want %q", err, test.needle)
			}
			if g.writes != nil {
				t.Fatalf("refused operation wrote: %v", g.writes)
			}
		})
	}
}

func TestReleaseMutationRefusals(t *testing.T) {
	cfg := releaseTestConfig(t)
	for _, test := range []struct {
		name       string
		operation  string
		op         func(*releaseOperation)
		setup      func(*fakeGitLab)
		needle     string
		wantWrites int
	}{
		{"existing tag", releaseOpTag, nil, func(g *fakeGitLab) {
			g.notes = []operationNote{delegationNote(31, "owner", "tag", releaseTestTag, releaseTestRef, "delegated", 0)}
		}, "already exists", 0},
		{"abbreviated ref", releaseOpTag, func(o *releaseOperation) { o.Ref = "cccccc" }, nil, "full commit", 0},
		{"multi-line tag message", releaseOpTag, func(o *releaseOperation) { o.Message = "a\nb" }, nil, "one bounded line", 0},
		{"release without tag", releaseOpCreate, nil, func(g *fakeGitLab) { delete(g.tags, releaseTestTag) }, "must already exist", 0},
		{"tag at another commit", releaseOpCreate, nil, func(g *fakeGitLab) {
			tag := g.tags[releaseTestTag]
			tag.Commit.ID = strings.Repeat("d", 40)
			g.tags[releaseTestTag] = tag
		}, "delegated commit", 0},
		{"release exists", releaseOpCreate, nil, nil, "already exists", 0},
		{"empty notes", releaseOpCreate, func(o *releaseOperation) { o.Notes = " \n" }, nil, "nonempty", 0},
		{"update without release", releaseOpUpdate, nil, func(g *fakeGitLab) {
			delete(g.releases, releaseTestTag)
			g.notes = []operationNote{delegationNote(31, "owner", "release-update", releaseTestTag, releaseTestRef, "delegated", 0)}
		}, "inaccessible", 0},
		{"job artifact link", releaseOpLink, func(o *releaseOperation) {
			o.LinkName, o.LinkURL, o.LinkType = "app", "https://gitlab.example/group/sub/project/-/jobs/4/artifacts/download", "package"
		}, nil, "ephemeral", 0},
		{"link with credentials", releaseOpLink, func(o *releaseOperation) {
			o.LinkName, o.LinkURL, o.LinkType = "app", "https://user:secret@gitlab.example/app", "package"
		}, nil, "without credentials", 0},
		{"unknown link type", releaseOpLink, func(o *releaseOperation) {
			o.LinkName, o.LinkURL, o.LinkType = "app", packageBase+"app", "binary"
		}, nil, "--link-type", 0},
		{"duplicate link", releaseOpLink, func(o *releaseOperation) {
			o.LinkName, o.LinkURL, o.LinkType = "app.tar.gz", packageBase+"other", "package"
		}, func(g *fakeGitLab) {
			g.notes = []operationNote{delegationNote(31, "owner", "link", releaseTestTag, releaseTestRef, "delegated", 0)}
		}, "never replaced", 0},
		{"read-back mismatch", releaseOpCreate, nil, func(g *fakeGitLab) {
			delete(g.releases, releaseTestTag)
			g.alter = func(method, path string, status int, body any) (int, any) {
				if method == "GET" && strings.HasSuffix(path, "/releases/v1.2.0") && status == 200 {
					r := body.(glRelease)
					r.Description = "changed"
					return 200, r
				}
				return status, body
			}
		}, "read-back mismatch", 1},
		{"write failure is not retried", releaseOpCreate, nil, func(g *fakeGitLab) {
			delete(g.releases, releaseTestTag)
			g.alter = func(method, path string, status int, body any) (int, any) {
				if method == "POST" {
					return 500, map[string]string{}
				}
				return status, body
			}
		}, "unavailable", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := newFakeGitLab(t)
			if test.setup != nil {
				test.setup(g)
			}
			o := releaseOp(test.operation)
			if test.op != nil {
				test.op(&o)
			}
			_, err := executeGitlabRelease(t.Context(), cfg, o, g.writeRunner)
			if err == nil || !strings.Contains(err.Error(), test.needle) {
				t.Fatalf("got %v, want %q", err, test.needle)
			}
			if len(g.writes) != test.wantWrites {
				t.Fatalf("writes %v, want %d", g.writes, test.wantWrites)
			}
		})
	}
}

// GitHub projects keep their own tools; release operations never route there.
func TestReleaseOperationsRefuseGitHub(t *testing.T) {
	cfg, err := decodeConfig([]byte(forgeYAML("github", "owner/repo")))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	_, err = executeGitlabRelease(t.Context(), cfg, releaseOp(releaseOpCreate), func(context.Context, []byte, ...string) ([]byte, []byte, error) {
		called = true
		return nil, nil, nil
	})
	if err == nil || called || !strings.Contains(err.Error(), "GitHub retains") {
		t.Fatalf("GitHub release routed through GitLab operations: %v", err)
	}
}
