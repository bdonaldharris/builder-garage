package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func gitlabReviewReport(t *testing.T, kind string, number int, run *gitlabFixtureRunner) (*workflowItem, string) {
	t.Helper()
	item, _, err := (gitlabStatusAdapter{Runner: run.run, Forge: gitlabTestForge()}).Read(t.Context(), workflowTarget{Kind: kind, Number: number}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := buildGitlabStatusReport(statusInputs{Item: item, Config: mustDecodeGitlabConfig(t), ReadAt: time.Now()}).render(&out); err != nil {
		t.Fatal(err)
	}
	return item, out.String()
}
func gitlabReviewNotes(t *testing.T, name string, edit func([]glNote) []glNote) []byte {
	t.Helper()
	var notes []glNote
	if err := json.Unmarshal(gitlabFixture(t, name), &notes); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(edit(notes))
	if err != nil {
		t.Fatal(err)
	}
	return gitlabHTTP(200, "", string(data))
}
func gitlabRejectNote(note glNote) glNote {
	note.ID += 30
	note.CreatedAt = note.CreatedAt.Add(30 * time.Minute)
	note.UpdatedAt = nil
	note.Body = strings.ReplaceAll(note.Body, "verdict: approved", "verdict: changes-required")
	note.Body = strings.ReplaceAll(note.Body, "acceptance_criteria_met: true", "acceptance_criteria_met: false")
	return note
}
func assertGitlabReviewNotPassed(t *testing.T, report string) {
	t.Helper()
	for _, bad := range []string{"Agent #1 may commit the approved artifact", "Agent #1 may open a pull request", "changeset equals the latest approval", "an approving one covers the head's changeset"} {
		if strings.Contains(report, bad) {
			t.Fatalf("inapplicable or incomplete review evidence passed (%s):\n%s", bad, report)
		}
	}
}
func TestGitlabIssueEvidenceBindsSelectedIssue(t *testing.T) {
	for _, scope := range []string{"completion", "review", "both"} {
		for _, kind := range []string{workflowIssueKind, workflowChangeKind} {
			t.Run(scope+"/"+kind, func(t *testing.T) {
				run := &gitlabFixtureRunner{t: t}
				run.override = func(endpoint string, _ int) ([]byte, bool) {
					if strings.Contains(endpoint, "/issues/6/notes?") {
						return gitlabReviewNotes(t, "issue-notes", func(notes []glNote) []glNote {
							for i := range notes {
								if scope == "both" || scope == "completion" && i == 0 || scope == "review" && i == 1 {
									notes[i].Body = strings.ReplaceAll(notes[i].Body, "issue: 6", "issue: 7")
								}
							}
							return notes
						}), true
					}
					return nil, false
				}
				number := 6
				if kind == workflowChangeKind {
					number = 5
				}
				_, report := gitlabReviewReport(t, kind, number, run)
				assertGitlabReviewNotPassed(t, report)
				assertContains(t, report, "evidence issue does not match")
			})
		}
	}
}
func TestGitlabCurrentAuthoritativeDispositionControlsBothViews(t *testing.T) {
	for _, scenario := range []string{"new rejection", "new completion", "wrong response link", "wrong artifact", "new malformed review"} {
		for _, kind := range []string{workflowIssueKind, workflowChangeKind} {
			t.Run(scenario+"/"+kind, func(t *testing.T) {
				run := &gitlabFixtureRunner{t: t}
				run.override = func(endpoint string, _ int) ([]byte, bool) {
					if strings.Contains(endpoint, "/issues/6/notes?") {
						return gitlabReviewNotes(t, "issue-notes", func(notes []glNote) []glNote {
							switch scenario {
							case "new rejection":
								notes = append(notes, gitlabRejectNote(notes[1]))
							case "new completion":
								n := notes[0]
								n.ID = 40
								n.CreatedAt = n.CreatedAt.Add(time.Hour)
								n.UpdatedAt = nil
								n.Body = strings.ReplaceAll(n.Body, strings.Repeat("d", 64), strings.Repeat("e", 64))
								notes = append(notes, n)
							case "wrong response link":
								notes[1].Body = strings.ReplaceAll(notes[1].Body, "#note_11", "#note_10")
							case "wrong artifact":
								notes[1].Body = strings.ReplaceAll(notes[1].Body, strings.Repeat("d", 64), strings.Repeat("e", 64))
							case "new malformed review":
								n := notes[1]
								n.ID = 40
								n.CreatedAt = n.CreatedAt.Add(time.Hour)
								n.UpdatedAt = nil
								n.Body = markerReview + "\n\n```yaml\ngarage_evidence: broken\n```"
								notes = append(notes, n)
							}
							return notes
						}), true
					}
					return nil, false
				}
				number := 6
				if kind == workflowChangeKind {
					number = 5
				}
				_, report := gitlabReviewReport(t, kind, number, run)
				assertGitlabReviewNotPassed(t, report)
			})
		}
	}
}
func TestGitlabEveryGoverningIssueRemainsRequired(t *testing.T) {
	for _, scenario := range []string{"inaccessible", "no records", "different artifact", "wrong cross-issue supplement", "both approved"} {
		t.Run(scenario, func(t *testing.T) {
			run := &gitlabFixtureRunner{t: t}
			run.override = func(endpoint string, _ int) ([]byte, bool) {
				if strings.Contains(endpoint, "/merge_requests/5/notes?") && scenario == "wrong cross-issue supplement" {
					return gitlabReviewNotes(t, "mr-notes", func(notes []glNote) []glNote {
						notes[1].Body = strings.ReplaceAll(notes[1].Body, "/issues/6#note_12", "/issues/7#note_12")
						return notes
					}), true
				}
				if endpoint == "projects/group%2Fsub%2Fproject/merge_requests/5" {
					var mr map[string]any
					_ = json.Unmarshal(gitlabFixture(t, "mr5"), &mr)
					mr["description"] = strings.ReplaceAll(mr["description"].(string), "#6", "#6 #7")
					data, _ := json.Marshal(mr)
					return gitlabHTTP(200, "", string(data)), true
				}
				if endpoint == "projects/group%2Fsub%2Fproject/issues/7" {
					if scenario == "inaccessible" {
						return gitlabHTTP(403, "", "{}"), true
					}
					data := strings.ReplaceAll(string(gitlabFixture(t, "issue6")), `"iid": 6`, `"iid": 7`)
					data = strings.ReplaceAll(data, `"id": 106`, `"id": 107`)
					data = strings.ReplaceAll(data, "/issues/6", "/issues/7")
					return gitlabHTTP(200, "", data), true
				}
				if strings.Contains(endpoint, "/issues/7/notes?") && scenario != "no records" {
					return gitlabReviewNotes(t, "issue-notes", func(notes []glNote) []glNote {
						for i := range notes {
							notes[i].Body = strings.ReplaceAll(notes[i].Body, "issue: 6", "issue: 7")
							notes[i].Body = strings.ReplaceAll(notes[i].Body, "/issues/6", "/issues/7")
							if scenario == "different artifact" {
								notes[i].Body = strings.ReplaceAll(notes[i].Body, strings.Repeat("d", 64), strings.Repeat("e", 64))
							}
						}
						return notes
					}), true
				}
				return nil, false
			}
			_, report := gitlabReviewReport(t, workflowChangeKind, 5, run)
			if scenario == "both approved" {
				assertContains(t, report, "changeset equals the latest approval")
			} else {
				assertGitlabReviewNotPassed(t, report)
			}
		})
	}
}
func TestGitlabSupplementalReviewMustBeCurrentAndRelated(t *testing.T) {
	for _, scenario := range []string{"new rejection", "wrong supplement", "wrong response", "wrong issue", "new malformed review"} {
		t.Run(scenario, func(t *testing.T) {
			run := &gitlabFixtureRunner{t: t}
			run.override = func(endpoint string, _ int) ([]byte, bool) {
				if strings.Contains(endpoint, "/merge_requests/5/notes?") {
					return gitlabReviewNotes(t, "mr-notes", func(notes []glNote) []glNote {
						switch scenario {
						case "new rejection":
							notes = append(notes, gitlabRejectNote(notes[1]))
						case "wrong supplement":
							notes[1].Body = strings.ReplaceAll(notes[1].Body, "/issues/6#note_12", "/issues/6#note_99")
						case "wrong response":
							notes[1].Body = strings.ReplaceAll(notes[1].Body, "#note_21", "#note_20")
						case "wrong issue":
							for i := range notes {
								notes[i].Body = strings.ReplaceAll(notes[i].Body, "issue: 6", "issue: 7")
							}
						case "new malformed review":
							n := notes[1]
							n.ID = 40
							n.CreatedAt = n.CreatedAt.Add(time.Hour)
							n.UpdatedAt = nil
							n.Body = markerReview + "\n\n```yaml\ngarage_evidence: broken\n```"
							notes = append(notes, n)
						}
						return notes
					}), true
				}
				return nil, false
			}
			_, report := gitlabReviewReport(t, workflowChangeKind, 5, run)
			assertGitlabReviewNotPassed(t, report)
		})
	}
}
func TestGitlabCurrentReviewPositiveControls(t *testing.T) {
	for _, kind := range []string{workflowIssueKind, workflowChangeKind} {
		t.Run(kind, func(t *testing.T) {
			run := &gitlabFixtureRunner{t: t}
			number := 6
			if kind == workflowChangeKind {
				number = 5
			}
			_, report := gitlabReviewReport(t, kind, number, run)
			if kind == workflowIssueKind {
				assertContains(t, report, "Agent #1 may commit the approved artifact")
			} else {
				assertContains(t, report, "changeset equals the latest approval")
			}
		})
	}
}

func TestGitlabReviewChronologyMustBeObserved(t *testing.T) {
	for _, kind := range []string{workflowIssueKind, workflowChangeKind} {
		t.Run(kind, func(t *testing.T) {
			run := &gitlabFixtureRunner{t: t}
			run.override = func(endpoint string, _ int) ([]byte, bool) {
				if strings.Contains(endpoint, "/issues/6/notes?") {
					return gitlabReviewNotes(t, "issue-notes", func(notes []glNote) []glNote { notes[1].CreatedAt = time.Time{}; return notes }), true
				}
				return nil, false
			}
			number := 6
			if kind == workflowChangeKind {
				number = 5
			}
			_, report := gitlabReviewReport(t, kind, number, run)
			assertGitlabReviewNotPassed(t, report)
		})
	}
}
func TestGitlabFreshCurrentApprovalRestoresReviewState(t *testing.T) {
	for _, scenario := range []string{"issue reapproval same artifact", "MR reapproval"} {
		t.Run(scenario, func(t *testing.T) {
			run := &gitlabFixtureRunner{t: t}
			run.override = func(endpoint string, _ int) ([]byte, bool) {
				if strings.Contains(endpoint, "/issues/6/notes?") && scenario == "issue reapproval same artifact" {
					return gitlabReviewNotes(t, "issue-notes", func(notes []glNote) []glNote {
						rejected := gitlabRejectNote(notes[1])
						completion := notes[0]
						completion.ID = 40
						completion.CreatedAt = completion.CreatedAt.Add(time.Hour)
						completion.UpdatedAt = nil
						approved := notes[1]
						approved.ID = 41
						approved.CreatedAt = approved.CreatedAt.Add(time.Hour)
						approved.UpdatedAt = nil
						approved.Body = strings.ReplaceAll(approved.Body, "#note_11", "#note_40")
						return append(notes, rejected, completion, approved)
					}), true
				}
				if strings.Contains(endpoint, "/merge_requests/5/notes?") && scenario == "MR reapproval" {
					return gitlabReviewNotes(t, "mr-notes", func(notes []glNote) []glNote {
						rejected := gitlabRejectNote(notes[1])
						approved := notes[1]
						approved.ID = 60
						approved.CreatedAt = approved.CreatedAt.Add(time.Hour)
						approved.UpdatedAt = nil
						return append(notes, rejected, approved)
					}), true
				}
				return nil, false
			}
			_, report := gitlabReviewReport(t, workflowChangeKind, 5, run)
			assertContains(t, report, "changeset equals the latest approval")
		})
	}
}

func TestGitlabGoverningSameIIDKeepsProjectAuthority(t *testing.T) {
	for _, scenario := range []string{"both approved", "foreign rejected", "foreign artifact differs"} {
		t.Run(scenario, func(t *testing.T) {
			run := &gitlabFixtureRunner{t: t}
			run.override = func(endpoint string, _ int) ([]byte, bool) {
				if endpoint == "projects/group%2Fsub%2Fproject/merge_requests/5" {
					var mr map[string]any
					_ = json.Unmarshal(gitlabFixture(t, "mr5"), &mr)
					mr["description"] = strings.ReplaceAll(mr["description"].(string), "Refs #6", "Refs #6 other/project#6")
					data, _ := json.Marshal(mr)
					return gitlabHTTP(200, "", string(data)), true
				}
				if endpoint == "projects/other%2Fproject" {
					return gitlabHTTP(200, "", `{"id":8,"path_with_namespace":"other/project"}`), true
				}
				if endpoint == "projects/other%2Fproject/issues/6" {
					var issue map[string]any
					_ = json.Unmarshal(gitlabFixture(t, "issue6"), &issue)
					issue["project_id"] = 8
					issue["web_url"] = "https://gitlab.example/other/project/-/issues/6"
					data, _ := json.Marshal(issue)
					return gitlabHTTP(200, "", string(data)), true
				}
				if strings.HasPrefix(endpoint, "projects/other%2Fproject/issues/6/notes?") {
					return gitlabReviewNotes(t, "issue-notes", func(notes []glNote) []glNote {
						for i := range notes {
							notes[i].Body = strings.ReplaceAll(notes[i].Body, "/group/sub/project/-/issues/6", "/other/project/-/issues/6")
							if scenario == "foreign artifact differs" {
								notes[i].Body = strings.ReplaceAll(notes[i].Body, strings.Repeat("d", 64), strings.Repeat("e", 64))
							}
						}
						if scenario == "foreign rejected" {
							notes = append(notes, gitlabRejectNote(notes[1]))
						}
						return notes
					}), true
				}
				if endpoint == "projects/other%2Fproject/members/all/1" {
					return gitlabHTTP(200, "", string(gitlabFixture(t, "membership"))), true
				}
				return nil, false
			}
			f := gitlabTestForge()
			f.AllowCrossProjectTargets = true
			item, _, err := (gitlabStatusAdapter{Runner: run.run, Forge: f}).Read(t.Context(), workflowTarget{Kind: workflowChangeKind, Number: 5}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(item.Observation.GoverningTargets) != 2 || len(item.Observation.Governing) != 2 {
				t.Fatal("same-IID governing identities collapsed")
			}
			var out bytes.Buffer
			_ = buildGitlabStatusReport(statusInputs{Item: item, Config: mustDecodeGitlabConfig(t), ReadAt: time.Now()}).render(&out)
			if scenario == "both approved" {
				assertContains(t, out.String(), "changeset equals the latest approval", "other/project#6")
			} else {
				assertGitlabReviewNotPassed(t, out.String())
			}
		})
	}
}
