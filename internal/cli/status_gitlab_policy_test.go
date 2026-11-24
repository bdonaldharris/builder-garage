package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func policyFixture(t *testing.T, edit func(string, int, []byte) ([]byte, bool)) *gitlabFixtureRunner {
	t.Helper()
	f := &gitlabFixtureRunner{t: t}
	seen := map[string]int{}
	f.override = func(endpoint string, _ int) ([]byte, bool) {
		path := strings.SplitN(endpoint, "?", 2)[0]
		seen[path]++
		prefix := "projects/group%2Fsub%2Fproject"
		var data []byte
		switch path {
		case prefix:
			var p map[string]any
			json.Unmarshal(gitlabFixture(t, "project"), &p)
			p["allow_merge_on_skipped_pipeline"] = false
			p["merge_method"] = "merge"
			data, _ = json.Marshal(p)
		case prefix + "/merge_requests/5":
			var mr map[string]any
			json.Unmarshal(gitlabFixture(t, "mr5"), &mr)
			mr["draft"] = false
			mr["author"] = map[string]any{"id": 1, "username": "builder"}
			mr["head_pipeline"] = map[string]any{"id": 30, "project_id": 7, "sha": strings.Repeat("a", 40), "status": "success"}
			data, _ = json.Marshal(mr)
		case prefix + "/pipelines/30":
			var pipeline map[string]any
			json.Unmarshal(gitlabFixture(t, "pipeline"), &pipeline)
			pipeline["project_id"] = 7
			pipeline["source"] = "merge_request_event"
			data, _ = json.Marshal(pipeline)
		case prefix + "/merge_requests/5/pipelines":
			data = []byte(`[{"id":30,"project_id":7,"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]`)
		case prefix + "/pipelines/30/jobs":
			data = gitlabFixture(t, "policy-jobs")
		case prefix + "/protected_branches":
			data = gitlabFixture(t, "policy-branches")
		case prefix + "/protected_tags":
			data = []byte(`[{"name":"v*","create_access_levels":[{"access_level":40}]}]`)
		case prefix + "/merge_requests/5/versions":
			data = gitlabFixture(t, "policy-versions")
		case prefix + "/merge_requests/5/approval_state":
			data = gitlabFixture(t, "policy-rules")
		case prefix + "/approvals":
			data = []byte(`{"reset_approvals_on_push":true,"merge_requests_author_approval":false,"merge_requests_disable_committers_approval":false}`)
		case prefix + "/members/all/2":
			data = []byte(`{"id":2,"access_level":40}`)
		case prefix + "/merge_requests/5/approvals":
			data = []byte(`{"approved":true,"has_approval_rules":true,"approvals_required":1,"approvals_left":0,"approved_by":[{"user":{"id":2,"username":"maintainer"},"approved_at":"2026-11-24T12:30:00Z"}]}`)
		case prefix + "/repository/branches/stack%2Fbase":
			data = gitlabFixture(t, "branch")
		}
		if edit != nil {
			if response, ok := edit(endpoint, seen[path], data); ok {
				return response, true
			}
		}
		if data != nil {
			return gitlabHTTP(200, "", string(data)), true
		}
		return nil, false
	}
	return f
}
func readPolicyFixture(t *testing.T, f *gitlabFixtureRunner) (*workflowItem, string) {
	t.Helper()
	cfg := mustDecodeGitlabConfig(t)
	cfg.Project.Authorities.ChangeAcceptor.Members = []string{"gitlab:gitlab.example/maintainer"}
	item, reads, err := (gitlabStatusAdapter{Runner: f.run, Forge: gitlabTestForge(), Now: func() time.Time { return time.Date(2026, 11, 24, 13, 0, 0, 0, time.UTC) }}).Read(t.Context(), workflowTarget{Kind: workflowChangeKind, Number: 5}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if reads > gitlabReadBudget {
		t.Fatal("unbounded reads")
	}
	var out bytes.Buffer
	if err := buildGitlabStatusReport(statusInputs{Item: item, Config: cfg, ReadAt: time.Now()}).render(&out); err != nil {
		t.Fatal(err)
	}
	return item, out.String()
}
func gate(t *testing.T, item *workflowItem, name string) workflowPolicyGate {
	t.Helper()
	for _, g := range item.Observation.PolicyGates {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("missing gate %s", name)
	return workflowPolicyGate{}
}
func changeJSON(t *testing.T, data []byte, edit func(map[string]any)) []byte {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	edit(v)
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestGitlabCheckedPolicyPositiveAndSources(t *testing.T) {
	item, report := readPolicyFixture(t, policyFixture(t, nil))
	for _, name := range []string{"pipeline enforcement", "current pipeline", "protected target", "discussion resolution", "native approvals", "human acceptance", "native merge status"} {
		g := gate(t, item, name)
		if g.State != policyPassed || !g.Required || !strings.HasPrefix(g.Source, "https://gitlab.example/api/v4/projects/group%2Fsub%2Fproject") {
			t.Fatalf("%s: %+v", name, g)
		}
	}
	if gate(t, item, "protected tags").Required {
		t.Fatal("tag inventory became an MR gate")
	}
	if !strings.Contains(report, "human acceptance: passed") || !strings.Contains(report, "source: https://gitlab.example/api/v4/") || strings.Contains(report, "GitHub, checked") {
		t.Fatalf("wrong policy provenance: %s", report)
	}
	// The fixture has one Issue completion/review and MR equivalence/review.
	// Native approvals did not create any additional Agent #2 records.
	if got := parseWorkflowRecords(item.Comments.Nodes, ""); len(got) != 2 {
		t.Fatalf("native approval became workflow review: %+v", got)
	}
}
func TestGitlabRequiredAndOptionalCapabilities(t *testing.T) {
	for _, scenario := range []string{"tags unavailable", "optional discussions unavailable", "required discussions unavailable", "rules unavailable required", "rules unavailable optional", "pipeline unavailable", "policy missing"} {
		t.Run(scenario, func(t *testing.T) {
			f := policyFixture(t, func(endpoint string, _ int, data []byte) ([]byte, bool) {
				path := strings.SplitN(endpoint, "?", 2)[0]
				if scenario == "tags unavailable" && strings.HasSuffix(path, "/protected_tags") || strings.HasPrefix(scenario, "rules unavailable") && strings.HasSuffix(path, "/approval_state") || scenario == "pipeline unavailable" && strings.HasSuffix(path, "/pipelines/30") {
					return gitlabHTTP(403, "", `{}`), true
				}
				if strings.Contains(scenario, "discussions unavailable") && strings.HasSuffix(path, "/discussions") {
					return gitlabHTTP(403, "", `{}`), true
				}
				if scenario == "optional discussions unavailable" && path == "projects/group%2Fsub%2Fproject" {
					return gitlabHTTP(200, "", string(changeJSON(t, data, func(v map[string]any) { v["only_allow_merge_if_all_discussions_are_resolved"] = false }))), true
				}
				if scenario == "rules unavailable optional" && strings.HasSuffix(path, "/merge_requests/5/approvals") {
					return gitlabHTTP(200, "", string(changeJSON(t, data, func(v map[string]any) { v["has_approval_rules"] = false }))), true
				}
				if scenario == "policy missing" && path == "projects/group%2Fsub%2Fproject" {
					return gitlabHTTP(200, "", string(changeJSON(t, data, func(v map[string]any) { delete(v, "only_allow_merge_if_pipeline_succeeds") }))), true
				}
				return nil, false
			})
			item, _ := readPolicyFixture(t, f)
			if scenario != "pipeline unavailable" && gate(t, item, "current pipeline").State != policyPassed {
				t.Fatal("unrelated capability blocked the pipeline gate")
			}
			switch scenario {
			case "tags unavailable":
				g := gate(t, item, "protected tags")
				if g.Required || g.State != policyUnavailable {
					t.Fatal(g)
				}
			case "optional discussions unavailable":
				g := gate(t, item, "discussion resolution")
				if g.Required || g.State != policyNotRequired {
					t.Fatal(g)
				}
			case "required discussions unavailable":
				g := gate(t, item, "discussion resolution")
				if !g.Required || g.State != policyUnavailable {
					t.Fatal(g)
				}
			case "rules unavailable required":
				g := gate(t, item, "native approvals")
				if !g.Required || g.State != policyUnavailable {
					t.Fatal(g)
				}
			case "rules unavailable optional":
				if gate(t, item, "native approvals").Required || gate(t, item, "native approvals").State != policyNotRequired || gate(t, item, "human acceptance").State != policyPassed {
					t.Fatal("optional tier capability blocked qualified current acceptance")
				}
			case "pipeline unavailable":
				if gate(t, item, "current pipeline").State != policyUnavailable {
					t.Fatal("missing pipeline passed")
				}
			case "policy missing":
				if gate(t, item, "pipeline enforcement").State != policyUnavailable {
					t.Fatal("missing policy defaulted to false/pass")
				}
			}
		})
	}
}
func TestGitlabPipelineStatesAndOwnership(t *testing.T) {
	for _, scenario := range []string{"failed", "running", "manual", "canceled", "skipped", "foreign project", "wrong SHA", "unrelated pipeline", "required job failed", "required job unknown", "job wrong owner", "policy bypass"} {
		t.Run(scenario, func(t *testing.T) {
			f := policyFixture(t, func(ep string, _ int, data []byte) ([]byte, bool) {
				path := strings.SplitN(ep, "?", 2)[0]
				if strings.HasSuffix(path, "/pipelines/30") {
					out := changeJSON(t, data, func(v map[string]any) {
						switch scenario {
						case "foreign project":
							v["project_id"] = 99
						case "wrong SHA":
							v["sha"] = strings.Repeat("e", 40)
						default:
							if contains([]string{"failed", "running", "manual", "canceled", "skipped"}, scenario) {
								v["status"] = scenario
							}
						}
					})
					return gitlabHTTP(200, "", string(out)), true
				}
				if scenario == "unrelated pipeline" && strings.HasSuffix(path, "/merge_requests/5/pipelines") {
					return gitlabHTTP(200, "", `[]`), true
				}
				if strings.HasPrefix(scenario, "required job") || scenario == "job wrong owner" {
					if strings.HasSuffix(path, "/jobs") {
						var jobs []map[string]any
						json.Unmarshal(data, &jobs)
						switch scenario {
						case "required job failed":
							jobs[0]["status"] = "failed"
						case "required job unknown":
							delete(jobs[0], "allow_failure")
						case "job wrong owner":
							jobs[0]["pipeline"].(map[string]any)["id"] = 99
						}
						b, _ := json.Marshal(jobs)
						return gitlabHTTP(200, "", string(b)), true
					}
				}
				if scenario == "policy bypass" && path == "projects/group%2Fsub%2Fproject" {
					return gitlabHTTP(200, "", string(changeJSON(t, data, func(v map[string]any) { v["allow_merge_on_skipped_pipeline"] = true }))), true
				}
				return nil, false
			})
			item, _ := readPolicyFixture(t, f)
			name := "current pipeline"
			if scenario == "policy bypass" {
				name = "pipeline enforcement"
			}
			g := gate(t, item, name)
			if g.State == policyPassed || g.State == policyNotRequired {
				t.Fatalf("unsafe %s passed: %+v", scenario, g)
			}
		})
	}
}

func TestGitlabApprovalIdentityAndFreshness(t *testing.T) {
	for _, scenario := range []string{"wrong acceptor", "stale approval", "missing timestamp", "future approval", "author disallowed", "membership unknown", "rule unmet", "syncing", "wrong diff head", "wrong diff target", "wrong diff MR", "changed approvals", "changed settings", "changed versions"} {
		t.Run(scenario, func(t *testing.T) {
			f := policyFixture(t, func(ep string, read int, data []byte) ([]byte, bool) {
				path := strings.SplitN(ep, "?", 2)[0]
				if strings.HasSuffix(path, "/merge_requests/5/approvals") {
					out := changeJSON(t, data, func(v map[string]any) {
						by := v["approved_by"].([]any)[0].(map[string]any)
						switch scenario {
						case "wrong acceptor":
							by["user"].(map[string]any)["username"] = "other"
						case "stale approval":
							by["approved_at"] = "2026-11-24T11:00:00Z"
						case "missing timestamp":
							delete(by, "approved_at")
						case "future approval":
							by["approved_at"] = "2026-11-24T14:00:00Z"
						case "rule unmet":
							v["approved"] = false
						case "changed approvals":
							if read >= 3 {
								v["approved_by"] = []any{}
							}
						}
					})
					return gitlabHTTP(200, "", string(out)), true
				}
				if strings.HasSuffix(path, "/versions") && strings.Contains(scenario, "diff") || scenario == "changed versions" && strings.HasSuffix(path, "/versions") && read > 1 {
					var vs []map[string]any
					json.Unmarshal(data, &vs)
					switch scenario {
					case "wrong diff head":
						vs[0]["head_commit_sha"] = strings.Repeat("e", 40)
					case "wrong diff target":
						vs[0]["start_commit_sha"] = strings.Repeat("e", 40)
					case "wrong diff MR":
						vs[0]["merge_request_id"] = 99
					case "changed versions":
						vs[0]["id"] = 5
					}
					out, _ := json.Marshal(vs)
					return gitlabHTTP(200, "", string(out)), true
				}
				if scenario == "membership unknown" && strings.HasSuffix(path, "/members/all/2") {
					return gitlabHTTP(403, "", `{}`), true
				}
				if scenario == "author disallowed" && strings.HasSuffix(path, "/merge_requests/5") {
					return gitlabHTTP(200, "", string(changeJSON(t, data, func(v map[string]any) { v["author"] = map[string]any{"id": 2, "username": "maintainer"} }))), true
				}
				if scenario == "syncing" && strings.HasSuffix(path, "/merge_requests/5") {
					return gitlabHTTP(200, "", string(changeJSON(t, data, func(v map[string]any) { v["detailed_merge_status"] = "approvals_syncing" }))), true
				}
				if scenario == "rule unmet" && strings.HasSuffix(path, "/approval_state") {
					return gitlabHTTP(200, "", strings.Replace(string(data), `"approved":true`, `"approved":false`, 1)), true
				}
				if scenario == "changed settings" && path == "projects/group%2Fsub%2Fproject/approvals" && read > 1 {
					return gitlabHTTP(200, "", string(changeJSON(t, data, func(v map[string]any) { v["merge_requests_author_approval"] = true }))), true
				}
				return nil, false
			})
			item, _ := readPolicyFixture(t, f)
			g := gate(t, item, "human acceptance")
			if scenario == "rule unmet" {
				if g.State != policyPassed || gate(t, item, "native approvals").State != policyFailed {
					t.Fatal("human and native-rule gates were conflated")
				}
			} else if g.State == policyPassed {
				t.Fatalf("stale/unqualified acceptance passed: %+v", g)
			}
			if gate(t, item, "current pipeline").State != policyPassed {
				t.Fatal("approval capability blocked unrelated pipeline")
			}
		})
	}
}
func TestGitlabProtectionAndSnapshotChanges(t *testing.T) {
	for _, scenario := range []string{"overlap grants push", "force push", "missing control", "no match", "no merge actor", "policy denied", "target moved", "source moved", "retargeted", "draft changed", "native status changed"} {
		t.Run(scenario, func(t *testing.T) {
			f := policyFixture(t, func(ep string, read int, data []byte) ([]byte, bool) {
				path := strings.SplitN(ep, "?", 2)[0]
				if strings.HasSuffix(path, "/protected_branches") {
					if scenario == "policy denied" {
						return gitlabHTTP(403, "", `{}`), true
					}
					var refs []map[string]any
					json.Unmarshal(data, &refs)
					switch scenario {
					case "overlap grants push":
						refs = append(refs, map[string]any{"name": "*", "allow_force_push": false, "push_access_levels": []any{map[string]any{"access_level": 40}}, "merge_access_levels": []any{map[string]any{"access_level": 40}}})
					case "force push":
						refs[0]["allow_force_push"] = true
					case "missing control":
						delete(refs[0], "push_access_levels")
					case "no match":
						refs[0]["name"] = "other/*"
					case "no merge actor":
						refs[0]["merge_access_levels"] = []any{}
					}
					out, _ := json.Marshal(refs)
					return gitlabHTTP(200, "", string(out)), true
				}
				if scenario == "target moved" && strings.Contains(path, "/repository/branches/") && read > 1 {
					return gitlabHTTP(200, "", strings.ReplaceAll(string(data), strings.Repeat("b", 40), strings.Repeat("c", 40))), true
				}
				if strings.HasSuffix(path, "/merge_requests/5") && read > 1 {
					out := changeJSON(t, data, func(v map[string]any) {
						switch scenario {
						case "source moved":
							v["sha"] = strings.Repeat("e", 40)
						case "retargeted":
							v["target_branch"] = "another/base"
						case "draft changed":
							v["draft"] = true
						case "native status changed":
							v["detailed_merge_status"] = "not_approved"
						}
					})
					return gitlabHTTP(200, "", string(out)), true
				}
				return nil, false
			})
			item, _ := readPolicyFixture(t, f)
			if contains([]string{"target moved", "source moved", "retargeted", "draft changed", "native status changed"}, scenario) {
				for _, name := range []string{"current pipeline", "native approvals", "human acceptance"} {
					if gate(t, item, name).State != policyUnavailable {
						t.Fatalf("changed snapshot retained %s pass", name)
					}
				}
			} else {
				g := gate(t, item, "protected target")
				if g.State == policyPassed {
					t.Fatalf("weak protection passed: %+v", g)
				}
			}
		})
	}
}
func TestGitlabForkAndBranchPipelineBinding(t *testing.T) {
	for _, scenario := range []string{"fork MR pipeline", "bound branch pipeline", "wrong branch pipeline", "synthetic SHA"} {
		t.Run(scenario, func(t *testing.T) {
			f := policyFixture(t, func(ep string, _ int, data []byte) ([]byte, bool) {
				path := strings.SplitN(ep, "?", 2)[0]
				if scenario == "fork MR pipeline" {
					switch path {
					case "projects/8":
						return gitlabHTTP(200, "", `{"id":8,"path_with_namespace":"fork/project"}`), true
					case "projects/group%2Fsub%2Fproject/merge_requests/5":
						return gitlabHTTP(200, "", string(changeJSON(t, data, func(v map[string]any) {
							v["source_project_id"] = 8
							v["head_pipeline"].(map[string]any)["project_id"] = 8
						}))), true
					case "projects/group%2Fsub%2Fproject/merge_requests/5/pipelines":
						return gitlabHTTP(200, "", `[{"id":30,"project_id":8,"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]`), true
					case "projects/fork%2Fproject/pipelines/30":
						return gitlabHTTP(200, "", `{"id":30,"project_id":8,"source":"merge_request_event","ref":"feature/topic","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","status":"success"}`), true
					case "projects/fork%2Fproject/pipelines/30/jobs":
						return gitlabHTTP(200, "", strings.ReplaceAll(string(gitlabFixture(t, "policy-jobs")), `"project_id":7`, `"project_id":8`)), true
					}
				}
				if path == "projects/group%2Fsub%2Fproject/pipelines/30" {
					out := changeJSON(t, data, func(v map[string]any) {
						switch scenario {
						case "bound branch pipeline":
							v["source"] = "push"
						case "wrong branch pipeline":
							v["source"] = "push"
							v["ref"] = "unrelated"
						case "synthetic SHA":
							v["sha"] = strings.Repeat("e", 40)
						}
					})
					return gitlabHTTP(200, "", string(out)), true
				}
				return nil, false
			})
			item, _ := readPolicyFixture(t, f)
			g := gate(t, item, "current pipeline")
			if scenario == "fork MR pipeline" || scenario == "bound branch pipeline" {
				if g.State != policyPassed {
					t.Fatalf("valid ownership refused: %+v", g)
				}
			} else if g.State == policyPassed {
				t.Fatal("unbound pipeline passed")
			}
		})
	}
}
func TestGitlabPolicyObservationAllowlist(t *testing.T) {
	c := newGitlabClient(nil, gitlabTestForge())
	prefix := "projects/group%2Fsub%2Fproject/"
	for _, path := range []string{"protected_branches?per_page=100&page=1", "protected_tags?per_page=100&page=1", "approvals", "merge_requests/5/approval_state", "merge_requests/5/pipelines?per_page=100&page=1", "merge_requests/5/versions?per_page=100&page=1", "pipelines/30/jobs?per_page=100&page=1"} {
		if !c.allowed("GET", prefix+path) || c.allowed("POST", prefix+path) || c.allowed("PUT", prefix+path) {
			t.Fatalf("policy resource boundary: %s", path)
		}
	}
	for _, path := range []string{"protected_branches/main", "merge_requests/5/approve", "merge_requests/5/merge", "merge_requests/5/versions?token=secret", "pipelines/30/jobs?include_retried=true", "protected_tags?per_page=100&page=1&search=*"} {
		if c.allowed("GET", prefix+path) {
			t.Fatalf("unsupported policy resource accepted: %s", path)
		}
	}
}

func TestGitlabNativeAcceptanceRequiresQualifiedConfig(t *testing.T) {
	for _, members := range [][]string{nil, {"maintainer"}, {"github:maintainer"}, {"gitlab:other.example/maintainer"}} {
		cfg := mustDecodeGitlabConfig(t)
		cfg.Project.Authorities.ChangeAcceptor.Members = members
		f := policyFixture(t, nil)
		item, _, err := (gitlabStatusAdapter{Runner: f.run, Forge: gitlabTestForge(), Now: func() time.Time { return time.Date(2026, 11, 24, 13, 0, 0, 0, time.UTC) }}).Read(t.Context(), workflowTarget{Kind: workflowChangeKind, Number: 5}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if gate(t, item, "human acceptance").State != policyUnavailable || gate(t, item, "current pipeline").State != policyPassed {
			t.Fatalf("unqualified config granted authority or blocked unrelated CI: %v", members)
		}
	}
}

func TestGitlabDiscussionRecordsRequireCompleteFeeds(t *testing.T) {
	for _, source := range []string{"issues/6/discussions", "merge_requests/5/discussions"} {
		for _, required := range []bool{false, true} {
			for _, failure := range []string{"forbidden", "truncated", "null", "invalid"} {
				name := source + "/optional/" + failure
				if required {
					name = source + "/required/" + failure
				}
				t.Run(name, func(t *testing.T) {
					f := policyFixture(t, func(endpoint string, _ int, data []byte) ([]byte, bool) {
						path := strings.SplitN(endpoint, "?", 2)[0]
						if path == "projects/group%2Fsub%2Fproject" {
							return gitlabHTTP(200, "", string(changeJSON(t, data, func(v map[string]any) {
								v["only_allow_merge_if_all_discussions_are_resolved"] = required
							}))), true
						}
						if strings.HasSuffix(path, source) {
							switch failure {
							case "forbidden":
								return gitlabHTTP(403, "", `{}`), true
							case "truncated":
								return gitlabHTTP(200, "X-Next-Page: 99\r\n", `[]`), true
							case "null":
								return gitlabHTTP(200, "", `null`), true
							case "invalid":
								return gitlabHTTP(200, "", `{`), true
							}
						}
						return nil, false
					})
					item, report := readPolicyFixture(t, f)
					assertGitlabReviewNotPassed(t, report)
					g := gate(t, item, "discussion resolution")
					expected := policyNotRequired
					if required {
						expected = policyPassed
						if source == "merge_requests/5/discussions" {
							expected = policyUnavailable
						}
					}
					if g.Required != required || g.State != expected {
						t.Fatal(g)
					}
					for _, name := range []string{"current pipeline", "human acceptance"} {
						if g := gate(t, item, name); g.State != policyPassed {
							t.Fatalf("unrelated native gate lost its evidence: %+v", g)
						}
					}
				})
			}
		}
	}
}
