package cli

import "fmt"

// GitLab evidence is first bound to its observed issue identity. A numeric
// block issue is never enough to relocate a record to another project/issue.
func gitlabIssueRecords(comments workflowComments, project string, number int) []workflowRecord {
	records := parseWorkflowRecords(comments.Nodes, gitlabRecordLocation(project, workflowIssueKind, number))
	for i := range records {
		if records[i].Block != nil && records[i].Block.Issue != number {
			records[i].Counted = false
			records[i].Comment.Eligibility = "evidence issue does not match this governing issue"
		}
	}
	return records
}

type gitlabIssueReview struct {
	Issue              workflowIssue
	Records            []workflowRecord
	Completion, Review *workflowRecord
	Problem            string
}

// Only the current issue-side completion and its current disposition can
// establish authority. An older approval cannot hide a newer correction,
// rejection, unreadable record or mismatched response/artifact.
func currentGitlabIssueReview(issue workflowIssue) gitlabIssueReview {
	state := gitlabIssueReview{Issue: issue, Records: gitlabIssueRecords(issue.Comments, issue.Project, issue.Number)}
	if !gitlabRecordsComplete(issue.Comments, state.Records) {
		state.Problem = "workflow notes or author membership are incomplete"
		return state
	}
	state.Completion = latestGitlabIssueRole(state.Records, false)
	state.Review = latestGitlabIssueRole(state.Records, true)
	switch {
	case state.Completion == nil:
		state.Problem = "no current counted completion record"
	case state.Completion.Kind != recordEvidence:
		state.Problem = "current completion record is malformed or lacks an evidence block"
	case state.Completion.Block.status() == "material-blocker":
		state.Problem = "current completion records a material blocker"
	case state.Review == nil:
		state.Problem = "no current counted implementation review"
	case state.Review.Kind != recordEvidence:
		state.Problem = "current implementation review is malformed or lacks an evidence block"
	case state.Review.Comment.CreatedAt.Before(state.Completion.Comment.CreatedAt):
		state.Problem = "current completion has not received a subsequent implementation review"
	case state.Review.Block.RespondsTo != state.Completion.Comment.URL:
		state.Problem = "current implementation review does not respond to the current completion"
	case state.Review.Block.artifact().Changeset != state.Completion.Block.artifact().Changeset:
		state.Problem = "current implementation review does not cover the current completion artifact"
	}
	return state
}
func (s gitlabIssueReview) approved() bool {
	return s.Problem == "" && s.Review != nil && s.Review.Block.verdict() == "approved"
}
func latestGitlabIssueRole(records []workflowRecord, review bool) *workflowRecord {
	var latest *workflowRecord
	for i := range records {
		record := &records[i]
		if !record.Counted {
			continue
		}
		if review {
			if record.Marker != markerReview || record.Kind == recordRequest {
				continue
			}
			if record.Kind == recordEvidence && record.Block.Record != "review" {
				continue
			}
		} else {
			if record.Marker != markerImplementation && record.Marker != markerCorrection {
				continue
			}
			if record.Kind == recordEvidence && record.Block.Record != "implementation" && record.Block.Record != "correction" {
				continue
			}
		}
		if latest == nil || !record.Comment.CreatedAt.Before(latest.Comment.CreatedAt) {
			latest = record
		}
	}
	return latest
}

func gitlabGoverningReviews(observation *workflowObservation) ([]gitlabIssueReview, bool) {
	complete := observation.GoverningComplete && len(observation.GoverningTargets) > 0
	loaded := map[workflowTargetKey]workflowIssue{}
	for _, issue := range observation.Governing {
		loaded[workflowTargetKey{issue.Project, workflowIssueKind, issue.Number}] = issue
	}
	states := make([]gitlabIssueReview, 0, len(observation.GoverningTargets))
	for _, target := range observation.GoverningTargets {
		issue, ok := loaded[workflowTargetKey{target.Project, target.Kind, target.Number}]
		if !ok {
			complete = false
			continue
		}
		state := currentGitlabIssueReview(issue)
		if !gitlabRecordsComplete(issue.Comments, state.Records) {
			complete = false
		}
		states = append(states, state)
	}
	return states, complete
}

func gitlabMRRecords(item *workflowItem) []workflowRecord {
	records := parseWorkflowRecords(item.Comments.Nodes, gitlabRecordLocation(item.RepositoryIdentity, item.Kind, item.Number))
	issues := map[int]bool{}
	for _, target := range item.Observation.GoverningTargets {
		if target.Kind == workflowIssueKind {
			issues[target.Number] = true
		}
	}
	for i := range records {
		if records[i].Block != nil && !issues[records[i].Block.Issue] {
			records[i].Counted = false
			records[i].Comment.Eligibility = "evidence issue does not match any required governing issue"
		}
	}
	return records
}

// A supplemental review must answer a record on this MR and supplement a
// still-applicable approval of the current authoritative issue artifact.
func gitlabSupplementProblem(review *workflowRecord, records []workflowRecord, state gitlabIssueReview, headChangeset string) string {
	var parent *workflowRecord
	for i := range records {
		record := &records[i]
		if record.Comment.URL == review.Block.RespondsTo && record.Counted && record.Kind == recordEvidence && record.Block.Issue == state.Issue.Number &&
			(record.Block.Record == "equivalence" || record.Block.Record == "correction" || record.Block.Record == "implementation") {
			parent = record
		}
	}
	if parent == nil || parent.Block.artifact().Changeset != headChangeset || parent.Block.status() == "material-blocker" || review.Comment.CreatedAt.Before(parent.Comment.CreatedAt) {
		return "current supplemental review does not answer a matching MR completion/equivalence record"
	}
	for i := range state.Records {
		approval := &state.Records[i]
		if approval.Comment.URL != review.Block.Supplements || !approval.isEvidence("review") || approval.Block.verdict() != "approved" ||
			approval.Block.artifact().Changeset != state.Completion.Block.artifact().Changeset || review.Comment.CreatedAt.Before(approval.Comment.CreatedAt) {
			continue
		}
		// Approval follows artifact equivalence, not URL recency. An older
		// supplement remains usable when the current authoritative approval
		// confirms the same artifact; it cannot override a current rejection.
		for j := range state.Records {
			completion := &state.Records[j]
			if completion.Comment.URL == approval.Block.RespondsTo &&
				(completion.isEvidence("implementation") || completion.isEvidence("correction")) &&
				completion.Block.status() != "material-blocker" &&
				completion.Block.artifact().Changeset == approval.Block.artifact().Changeset &&
				!approval.Comment.CreatedAt.Before(completion.Comment.CreatedAt) {
				return ""
			}
		}
	}

	return "current supplemental review does not supplement an applicable approval of the current governing artifact"
}

// Select a current-artifact supplemental disposition, including a newer
// rejection/unreadable disposition. Historical approvals are never selected
// by filtering away the negative state first.
func latestGitlabMRReview(records []workflowRecord, state gitlabIssueReview, states []gitlabIssueReview, headChangeset, head string) *workflowRecord {
	var latest *workflowRecord
	for i := range records {
		record := &records[i]
		if !record.Counted || record.Marker != markerReview || record.Kind == recordRequest {
			continue
		}
		if record.Kind == recordEvidence {
			if record.Block.Record != "pr-review" || record.Block.Issue != state.Issue.Number {
				continue
			}
			artifact := record.Block.artifact()
			if artifact.Changeset != headChangeset && artifact.Commit != head {
				continue
			}
			// Same-IID governing issues in different projects remain separate. A
			// supplement known to belong to another issue cannot cover this one.
			otherIssue := false
			for _, other := range states {
				if other.Issue.Number != state.Issue.Number {
					continue
				}
				if other.Issue.Project == state.Issue.Project {
					continue
				}
				for _, r := range other.Records {
					if r.Comment.URL == record.Block.Supplements {
						otherIssue = true
					}
				}
			}
			if otherIssue {
				continue
			}
		}
		if latest == nil || !record.Comment.CreatedAt.Before(latest.Comment.CreatedAt) {
			latest = record
		}
	}
	return latest
}

func evaluateGitlabMRReviews(g *statusSection, in statusInputs, records []workflowRecord, states []gitlabIssueReview, complete bool) {
	if !complete || !gitlabRecordsComplete(in.Item.Comments, records) {
		g.add(labelChecked, "review evaluation: unavailable because the required governing set, workflow notes or author membership are incomplete; no review gate passes")
		return
	}
	ready := true
	for _, state := range states {
		if state.Problem != "" {
			g.add(labelChecked, "current governing review for %s: %s; approval is not established", gitlabRecordLocation(state.Issue.Project, workflowIssueKind, state.Issue.Number), state.Problem)
			ready = false
		} else if !state.approved() {
			g.add(labelChecked, "current governing review for %s: %s; historical approvals do not override this disposition", gitlabRecordLocation(state.Issue.Project, workflowIssueKind, state.Issue.Number), state.Review.Block.verdict())
			ready = false
		}
	}
	if !ready {
		g.add(labelChecked, "review equivalence is not established for the current authoritative governing set; no review gate passes")
		return
	}
	headChangeset := ""
	if in.Item.HeadRefOid != "" && in.Item.BaseRefOid != "" {
		headChangeset = evaluateHeadIdentity(g, in, records, in.Item.HeadRefOid)
	}
	if headChangeset == "" {
		g.add(labelChecked, "trigger 1 (equivalence not shown) APPLIES: current source/target artifact identity is unavailable")
		return
	}
	covered := true
	for _, state := range states {
		label := gitlabRecordLocation(state.Issue.Project, workflowIssueKind, state.Issue.Number)
		review := latestGitlabMRReview(records, state, states, headChangeset, in.Item.HeadRefOid)
		if review == nil {
			if state.Review.Block.artifact().Changeset != headChangeset {
				g.add(labelChecked, "current artifact lacks a matching approval for governing %s; supplemental MR review is required", label)
				covered = false
			}
			continue
		}
		problem := ""
		switch {
		case review.Kind != recordEvidence:
			problem = "current supplemental disposition is malformed or lacks an evidence block"
		case review.Block.verdict() != "approved":
			problem = "current supplemental disposition is " + review.Block.verdict()
		case review.Block.artifact().Changeset != headChangeset:
			problem = "current supplemental disposition does not identify the current artifact"
		default:
			problem = gitlabSupplementProblem(review, records, state, headChangeset)
		}
		if problem != "" {
			g.add(labelChecked, "MR review for governing %s: %s; no current approval is established", label, problem)
			covered = false
		}
	}
	if !covered {
		g.add(labelChecked, "review equivalence is not established for every required current disposition; no review gate passes")
		return
	}
	g.add(labelChecked, "trigger 1 (equivalence not shown) does not apply: the head's changeset equals the latest approval applicable to every governing issue")
	for _, state := range states {
		g.add(labelChecked, "current authoritative issue approval for %s: %s", fmt.Sprintf("%s#%d", state.Issue.Project, state.Issue.Number), state.Review.Comment.URL)
	}
	g.add(labelChecked, "additional MR review requirements: unavailable until automated-finding and configured-trigger evidence is established; no exemption is inferred")
}
