package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// The status report. Every line names its source and basis, so that what
// GitHub shows, what a record merely asserts, what configuration says, and
// what local history annotates are never confused.

const (
	labelChecked  = "GitHub, checked"
	labelAsserted = "GitHub, asserted"
	labelLocalGit = "local git, checked"
	labelConfig   = "configuration"
	labelLocal    = "local, not evidence"
	labelInfo     = "information only"
)

type statusLine struct {
	label string
	text  string
}

type statusSection struct {
	heading string
	lines   []statusLine
}

func (s *statusSection) add(label, format string, args ...any) {
	s.lines = append(s.lines, statusLine{label: label, text: fmt.Sprintf(format, args...)})
}

type statusReport struct {
	header   []string
	sections []*statusSection
}

func (r *statusReport) section(heading string) *statusSection {
	s := &statusSection{heading: heading}
	r.sections = append(r.sections, s)
	return s
}

func (r *statusReport) render(w io.Writer) error {
	var b strings.Builder
	for _, line := range r.header {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, s := range r.sections {
		if len(s.lines) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s\n", s.heading)
		for _, line := range s.lines {
			fmt.Fprintf(&b, "  %-23s %s\n", "["+line.label+"]", line.text)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// statusInputs is everything a report is built from. Local history is
// passed separately and never reaches the evaluation.
type statusInputs struct {
	Item          *ghItem
	Config        *projectConfig // nil when the configuration is not valid
	ConfigProblem string
	// Recompute recomputes a commit-stage changeset from local objects only.
	// It returns the changeset, or "" and the reason it could not.
	Recompute func(integrationRef, commit string) (string, string)
	ReadAt    time.Time
}

// buildStatusReport evaluates the GitHub evidence for one issue or pull
// request. Eligibility comes from GitHub records alone; configuration only
// supplies the values the checks compare against.
func buildStatusReport(in statusInputs) *statusReport {
	it := in.Item
	kind := "issue"
	if it.isPullRequest() {
		kind = "pull request"
	}
	r := &statusReport{header: []string{
		fmt.Sprintf("Status of %s #%d: %s", kind, it.Number, oneLine(it.Title)),
		it.URL,
		"",
		fmt.Sprintf("GitHub records are authoritative. GitHub was read once, at %s.",
			in.ReadAt.UTC().Format(time.RFC3339)),
		"Status writes nothing to GitHub, and assigns, invokes, or waits for no agent.",
		"\"Asserted\" values are recorded claims that no tool can verify, such as a verdict.",
	}}
	if it.isPullRequest() {
		buildPRReport(r, in)
	} else {
		buildIssueReport(r, in)
	}
	addConfigSection(r, in)
	return r
}

func location(it *ghItem) string {
	if it.isPullRequest() {
		return fmt.Sprintf("PR #%d", it.Number)
	}
	return fmt.Sprintf("issue #%d", it.Number)
}

// addRecordsSection lists every workflow record on one issue or pull request.
func addRecordsSection(r *statusReport, heading string, comments ghComments, records []workflowRecord) {
	s := r.section(heading)
	if comments.PageInfo.HasPreviousPage {
		s.add(labelChecked, "only the latest %d of %d comments were read; earlier records are not shown or evaluated",
			len(comments.Nodes), comments.TotalCount)
	}
	if len(records) == 0 {
		s.add(labelChecked, "no workflow records")
		return
	}
	for i := range records {
		rec := &records[i]
		when := rec.Comment.CreatedAt.UTC().Format("2006-01-02 15:04Z")
		prefix := when + " "
		if !rec.Counted {
			prefix += fmt.Sprintf("IGNORED (author association %s, not owner, member, or collaborator): ",
				orNone(rec.Comment.AuthorAssociation))
		}
		switch rec.Kind {
		case recordRequest:
			head := "no head named"
			if rec.RequestHead != "" {
				head = "head " + shortSHA(rec.RequestHead)
			}
			s.add(labelChecked, "%sAgent #2 PR review request by %s, %s: %s", prefix, rec.author(), head, rec.Comment.URL)
		case recordLegacy:
			s.add(labelChecked, "%s%s by %s has no evidence block (legacy, not evaluated): %s",
				prefix, rec.Marker, rec.author(), rec.Comment.URL)
		case recordInvalid:
			s.add(labelChecked, "%s%s by %s has a malformed evidence block (not evaluated): %s: %s",
				prefix, rec.Marker, rec.author(), rec.Problem, rec.Comment.URL)
		case recordEvidence:
			s.add(labelChecked, "%s%s record by %s, %s: %s",
				prefix, rec.Block.Record, rec.author(), describeArtifact(rec.Block.artifact()), rec.Comment.URL)
			if rec.Counted {
				if a := describeAsserted(rec.Block); a != "" {
					s.add(labelAsserted, "  %s", a)
				}
			}
		}
		if rec.Comment.LastEditedAt != nil {
			s.add(labelChecked, "  edited after posting, last at %s", rec.Comment.LastEditedAt.UTC().Format(time.RFC3339))
		}
	}
}

func describeArtifact(a *evidenceArtifact) string {
	text := fmt.Sprintf("%s artifact %s, %d paths", a.Stage, shortChangeset(a.Changeset), *a.Paths)
	if a.Commit != "" {
		text += ", commit " + shortSHA(a.Commit)
	}
	return text
}

func describeAsserted(b *evidenceBlock) string {
	var parts []string
	if v := b.status(); v != "" {
		parts = append(parts, "status "+v)
	}
	if v := b.verdict(); v != "" {
		parts = append(parts, "verdict "+v)
	}
	if b.Asserted != nil {
		if b.Asserted.ExecutionContext != "" {
			parts = append(parts, "execution context "+b.Asserted.ExecutionContext)
		}
		if b.Asserted.Inspected != "" {
			parts = append(parts, "inspected "+b.Asserted.Inspected)
		}
	}
	return strings.Join(parts, "; ")
}

// latestEvidence returns the latest counted, well-formed record of any of the
// given types.
func latestEvidence(records []workflowRecord, types ...string) *workflowRecord {
	var latest *workflowRecord
	for i := range records {
		rec := &records[i]
		for _, t := range types {
			if rec.isEvidence(t) && (latest == nil || !rec.Comment.CreatedAt.Before(latest.Comment.CreatedAt)) {
				latest = rec
			}
		}
	}
	return latest
}

// evaluateImplementationReview reports the issue-side sequence: the latest
// Agent #1 completion record and the Agent #2 review that answers it. It
// returns the next eligible action, as information.
func evaluateImplementationReview(s *statusSection, records []workflowRecord) string {
	completion := latestEvidence(records, "implementation", "correction")
	if completion == nil {
		s.add(labelChecked, "no counted Agent #1 completion record with an evidence block")
		return "Agent #1 implementation (application-implementation-workflow)"
	}
	cb := completion.Block
	s.add(labelChecked, "latest Agent #1 completion record: %s, %s: %s",
		cb.Record, shortChangeset(cb.artifact().Changeset), completion.Comment.URL)
	s.add(labelAsserted, "its status: %s", cb.status())
	if cb.status() == "material-blocker" {
		return "resolve the material blocker recorded at " + completion.Comment.URL
	}

	review := latestEvidence(records, "review")
	if review == nil || review.Comment.CreatedAt.Before(completion.Comment.CreatedAt) {
		s.add(labelChecked, "no Agent #2 review of that completion record is recorded")
		return "Agent #2 implementation review (independent-implementation-review)"
	}
	rb := review.Block
	s.add(labelChecked, "latest Agent #2 implementation review: %s", review.Comment.URL)
	if rb.RespondsTo != completion.Comment.URL {
		s.add(labelChecked, "  it responds to %s, not to the latest completion record", rb.RespondsTo)
	}
	if rb.artifact().Changeset == cb.artifact().Changeset {
		s.add(labelChecked, "  its artifact changeset equals the completion record's (%s)", shortChangeset(cb.artifact().Changeset))
	} else {
		s.add(labelChecked, "  CONTRADICTION: its artifact changeset %s differs from the completion record's %s",
			shortChangeset(rb.artifact().Changeset), shortChangeset(cb.artifact().Changeset))
	}
	s.add(labelAsserted, "  verdict %s", rb.verdict())
	s.add(labelInfo, "  a working-tree artifact can be recomputed only by a holder of that working tree; status compares the recorded values")
	if rb.verdict() == "approved" {
		return "Agent #1 may commit the approved artifact and open a pull request (integration-pr-review)"
	}
	return "Agent #1 correction (application-implementation-workflow)"
}

func buildIssueReport(r *statusReport, in statusInputs) {
	it := in.Item
	s := r.section("State")
	closed := ""
	if it.ClosedAt != nil {
		closed = ", closed at " + it.ClosedAt.UTC().Format(time.RFC3339)
	}
	s.add(labelChecked, "issue is %s%s", strings.ToLower(it.State), closed)
	var linked []ghLinkedPR
	if it.ClosedByPullRequestsReferences != nil {
		linked = it.ClosedByPullRequestsReferences.Nodes
	}
	for _, pr := range linked {
		s.add(labelChecked, "linked pull request #%d is %s: %s", pr.Number, strings.ToLower(pr.State), pr.URL)
	}

	records := parseWorkflowRecords(it.Comments.Nodes, location(it))
	addRecordsSection(r, "Workflow records on "+location(it), it.Comments, records)

	g := r.section("Implementation review (authoritative record on the issue)")
	next := evaluateImplementationReview(g, records)

	n := r.section("Next eligible action")
	switch {
	case it.State != "OPEN":
		n.add(labelInfo, "none: the issue is %s", strings.ToLower(it.State))
	case len(linked) > 0:
		n.add(labelInfo, "the pull-request sequence governs from here; run \"garage status %d\"", linked[len(linked)-1].Number)
	default:
		n.add(labelInfo, "%s", next)
	}
}

// approvalRecord is an approving Agent #2 disposition.
type approvalRecord struct {
	rec  *workflowRecord
	what string // "issue review" or "PR review"
}

func (a approvalRecord) changeset() string { return a.rec.Block.artifact().Changeset }

func buildPRReport(r *statusReport, in statusInputs) {
	it := in.Item
	head := it.HeadRefOid

	s := r.section("State")
	switch {
	case it.Merged:
		merge := ""
		if it.MergeCommit != nil {
			merge = ", merge commit " + shortSHA(it.MergeCommit.Oid)
		}
		s.add(labelChecked, "merged at %s%s; head at merge %s", fmtTime(it.MergedAt), merge, shortSHA(head))
	default:
		s.add(labelChecked, "pull request is %s; current head %s", strings.ToLower(it.State), shortSHA(head))
	}
	s.add(labelChecked, "base branch %s", it.BaseRefName)

	// The issue-side records: the authoritative implementation review.
	var issueRecords []workflowRecord
	var issues []int
	if it.ClosingIssuesReferences != nil {
		for _, issue := range it.ClosingIssuesReferences.Nodes {
			issues = append(issues, issue.Number)
			recs := parseWorkflowRecords(issue.Comments.Nodes, fmt.Sprintf("issue #%d", issue.Number))
			addRecordsSection(r, fmt.Sprintf("Workflow records on governing issue #%d", issue.Number), issue.Comments, recs)
			issueRecords = append(issueRecords, recs...)
		}
	}
	prRecords := parseWorkflowRecords(it.Comments.Nodes, location(it))
	addRecordsSection(r, "Workflow records on "+location(it), it.Comments, prRecords)

	g := r.section("Gates for the current head " + shortSHA(head))
	if len(issues) == 0 {
		g.add(labelChecked, "the pull request closes no issue, so no issue-side records were read")
	}
	for _, rec := range prRecords {
		if rec.isEvidence("equivalence") && !slices.Contains(issues, rec.Block.Issue) {
			g.add(labelChecked, "an equivalence record names issue #%d, which this pull request does not close; its records were not read: %s",
				rec.Block.Issue, rec.Comment.URL)
			break
		}
	}

	approvals := collectApprovals(g, issueRecords, prRecords)
	if issueReview := latestApproval(approvals, "issue review"); issueReview != nil {
		g.add(labelChecked, "approving issue-recorded implementation review: %s", issueReview.rec.Comment.URL)
	} else {
		g.add(labelChecked, "NO approving issue-recorded implementation review was found")
	}

	headChangeset := evaluateHeadIdentity(g, in, prRecords, head)
	latest := latestApproval(approvals, "")
	trigger1 := evaluateEquivalence(g, prRecords, approvals, latest, headChangeset, head)
	trigger2 := evaluateAutomatedFindings(g, it, head)
	if in.Config != nil && in.Config.Project.PRReviewTriggers != nil {
		for _, t := range *in.Config.Project.PRReviewTriggers {
			g.add(labelConfig, "configured PR-review trigger %q: not evaluated by status (when it applies: %s)", t.Name, oneLine(t.AppliesWhen))
		}
	}
	reviewMet, reviewNext := evaluatePRReview(g, prRecords, approvals, head, headChangeset, trigger1 || trigger2)
	ciOK := evaluateChecks(g, it, head)
	accepted := evaluateHumanAcceptance(g, in, it, head)
	envOK := strings.Contains(it.Body, "Environment / Deployment Requirements")
	if envOK {
		g.add(labelChecked, "the pull request description has an Environment / Deployment Requirements section")
	} else {
		g.add(labelChecked, "the pull request description has NO Environment / Deployment Requirements section")
	}
	if in.Config != nil && in.Config.Project.ReleaseConditions != nil {
		for _, c := range *in.Config.Project.ReleaseConditions {
			g.add(labelConfig, "release condition %q requires %s: not evaluated by status", c.Name, oneLine(c.RequiredEvidence))
		}
	}

	n := r.section("Next eligible action")
	if it.State != "OPEN" {
		n.add(labelInfo, "none: the pull request is %s; any unmet item above is reported as a fact, not a judgment", strings.ToLower(it.State))
		return
	}
	switch {
	case headChangeset == "":
		n.add(labelInfo, "Agent #1 records the equivalence record for the current head (integration-pr-review)")
	case !reviewMet:
		n.add(labelInfo, "%s", reviewNext)
	case !ciOK:
		n.add(labelInfo, "required checks must pass on the current head")
	case !accepted:
		n.add(labelInfo, "the change acceptor's approval, a human gate")
	case !envOK:
		n.add(labelInfo, "Agent #1 surfaces the Environment / Deployment Requirements")
	default:
		n.add(labelInfo, "no gate that status can check is unmet; merging remains a human decision, and asserted and unevaluated items above still apply")
	}
}

// collectApprovals gathers the counted approving dispositions. A PR-review
// approval counts only when its supplements link names an approving issue
// review that was read.
func collectApprovals(g *statusSection, issueRecords, prRecords []workflowRecord) []approvalRecord {
	var approvals []approvalRecord
	issueApproved := map[string]bool{}
	for i := range issueRecords {
		rec := &issueRecords[i]
		if rec.isEvidence("review") && rec.Block.verdict() == "approved" {
			approvals = append(approvals, approvalRecord{rec: rec, what: "issue review"})
			issueApproved[rec.Comment.URL] = true
		}
	}
	for i := range prRecords {
		rec := &prRecords[i]
		if !rec.isEvidence("pr-review") || rec.Block.verdict() != "approved" {
			continue
		}
		if !issueApproved[rec.Block.Supplements] {
			g.add(labelChecked, "an approving PR review does not supplement an approving issue review that was read, so it is not counted: %s",
				rec.Comment.URL)
			continue
		}
		approvals = append(approvals, approvalRecord{rec: rec, what: "PR review"})
	}
	return approvals
}

// latestApproval returns the latest approval of the given kind, or of any
// kind when what is "".
func latestApproval(approvals []approvalRecord, what string) *approvalRecord {
	var latest *approvalRecord
	for i := range approvals {
		a := &approvals[i]
		if what != "" && a.what != what {
			continue
		}
		if latest == nil || !a.rec.Comment.CreatedAt.Before(latest.rec.Comment.CreatedAt) {
			latest = a
		}
	}
	return latest
}

// headEquivalenceRecord returns the latest counted equivalence record for
// the head.
func headEquivalenceRecord(prRecords []workflowRecord, head string) *workflowRecord {
	var latest *workflowRecord
	for i := range prRecords {
		rec := &prRecords[i]
		if rec.isEvidence("equivalence") && rec.Block.artifact().Commit == head {
			latest = rec
		}
	}
	return latest
}

// evaluateHeadIdentity reports the head's recorded garage-artifact-v1
// identity and, when the objects are local, recomputes it. It returns the
// changeset to use, or "" when none is recorded.
func evaluateHeadIdentity(g *statusSection, in statusInputs, prRecords []workflowRecord, head string) string {
	eq := headEquivalenceRecord(prRecords, head)
	if eq == nil {
		g.add(labelChecked, "NO equivalence record identifies the current head, so its artifact identity is unknown")
		return ""
	}
	a := eq.Block.artifact()
	g.add(labelChecked, "equivalence record for the head: changeset %s: %s", shortChangeset(a.Changeset), eq.Comment.URL)
	if in.Recompute == nil {
		return a.Changeset
	}
	recomputed, reason := in.Recompute(a.IntegrationRef, head)
	switch {
	case recomputed == "":
		g.add(labelLocalGit, "not recomputed: %s; the recorded value is compared as recorded", reason)
	case recomputed == a.Changeset:
		g.add(labelLocalGit, "recomputed from local objects: matches the recorded changeset")
	default:
		g.add(labelLocalGit, "CONTRADICTION: recomputed from local objects as %s, but the record says %s; the recomputed value is used",
			shortChangeset(recomputed), shortChangeset(a.Changeset))
		return recomputed
	}
	return a.Changeset
}

// evaluateEquivalence reports minimum PR-review trigger 1 and returns
// whether it applies.
func evaluateEquivalence(g *statusSection, prRecords []workflowRecord, approvals []approvalRecord,
	latest *approvalRecord, headChangeset, head string) bool {
	if eq := headEquivalenceRecord(prRecords, head); eq != nil && eq.Block.EquivalentTo != "none" {
		var linked *approvalRecord
		for i := range approvals {
			if approvals[i].rec.Comment.URL == eq.Block.EquivalentTo {
				linked = &approvals[i]
			}
		}
		switch {
		case linked == nil:
			g.add(labelChecked, "the equivalence record links %s, which is not a counted approval that was read", eq.Block.EquivalentTo)
		case linked.changeset() != headChangeset:
			g.add(labelChecked, "CONTRADICTION: the equivalence record claims equivalence to %s, whose changeset %s differs from the head's",
				eq.Block.EquivalentTo, shortChangeset(linked.changeset()))
		}
	}
	switch {
	case headChangeset == "":
		g.add(labelChecked, "trigger 1 (equivalence not shown) APPLIES: the head's identity is not recorded")
		return true
	case latest == nil:
		g.add(labelChecked, "trigger 1 (equivalence not shown) APPLIES: no counted approval was found")
		return true
	case latest.changeset() == headChangeset:
		g.add(labelChecked, "trigger 1 (equivalence not shown) does not apply: the head's changeset equals the latest approval (%s): %s",
			latest.what, latest.rec.Comment.URL)
		return false
	default:
		g.add(labelChecked, "trigger 1 (equivalence not shown) APPLIES: the head's changeset %s differs from the latest approval's %s (%s)",
			shortChangeset(headChangeset), shortChangeset(latest.changeset()), latest.what)
		return true
	}
}

// evaluateAutomatedFindings reports minimum PR-review trigger 2. Automated
// findings are detected as reviews by bot accounts that left review
// comments.
func evaluateAutomatedFindings(g *statusSection, it *ghItem, head string) bool {
	onHead := 0
	if it.Reviews != nil {
		for _, rv := range it.Reviews.Nodes {
			if rv.Author == nil || rv.Author.Typename != "Bot" || rv.Comments.TotalCount == 0 || rv.Commit == nil {
				continue
			}
			if rv.Commit.Oid == head {
				onHead += rv.Comments.TotalCount
				g.add(labelChecked, "automated review by %s reported %d finding(s) on the head: %s",
					rv.Author.Login, rv.Comments.TotalCount, rv.URL)
			} else {
				g.add(labelChecked, "automated review by %s reported %d finding(s) on earlier commit %s: %s",
					rv.Author.Login, rv.Comments.TotalCount, shortSHA(rv.Commit.Oid), rv.URL)
			}
		}
	}
	if onHead > 0 {
		g.add(labelChecked, "trigger 2 (automated findings) APPLIES")
		return true
	}
	g.add(labelChecked, "trigger 2 (automated findings) does not apply: no bot review with findings on the head (other reporting mechanisms are not detected)")
	return false
}

// evaluatePRReview reports whether an Agent #2 PR review is required and
// recorded for the head. It returns whether the requirement is met, and the
// next action when it is not.
func evaluatePRReview(g *statusSection, prRecords []workflowRecord, approvals []approvalRecord,
	head, headChangeset string, required bool) (bool, string) {
	if !required {
		g.add(labelChecked, "no Agent #2 PR review is required for the head")
		return true, ""
	}
	if headChangeset != "" {
		for i := len(approvals) - 1; i >= 0; i-- {
			if a := approvals[i]; a.what == "PR review" && a.changeset() == headChangeset {
				g.add(labelChecked, "an Agent #2 PR review is required, and an approving one covers the head's changeset: %s", a.rec.Comment.URL)
				return true, ""
			}
		}
	}
	g.add(labelChecked, "an Agent #2 PR review is REQUIRED, and NO approving one covers the head's changeset")

	// The latest PR review of the head itself.
	var headReview *workflowRecord
	for i := range prRecords {
		rec := &prRecords[i]
		if rec.isEvidence("pr-review") && headChangeset != "" && rec.Block.artifact().Changeset == headChangeset {
			headReview = rec
		}
	}
	if headReview != nil {
		g.add(labelChecked, "  Agent #2 PR review of the head: %s", headReview.Comment.URL)
		g.add(labelAsserted, "  its verdict: %s", headReview.Block.verdict())
		correction := latestEvidence(prRecords, "correction")
		if correction == nil || correction.Comment.CreatedAt.Before(headReview.Comment.CreatedAt) {
			return false, "Agent #1 correction of the PR review's blockers (application-implementation-workflow)"
		}
		cs := correction.Block.artifact().Changeset
		g.add(labelChecked, "  a later Agent #1 correction record (changeset %s): %s", shortChangeset(cs), correction.Comment.URL)
		for i := len(approvals) - 1; i >= 0; i-- {
			if a := approvals[i]; a.what == "PR review" && a.changeset() == cs {
				g.add(labelChecked, "  that correction was approved (%s), but its changeset is NOT the head's: the approved correction is not what the head contains",
					a.rec.Comment.URL)
				return false, "Agent #1 commits and pushes exactly the approved correction, and records its equivalence (integration-pr-review)"
			}
		}
		return false, "Agent #2 re-review of the correction (independent-implementation-review)"
	}
	for i := len(prRecords) - 1; i >= 0; i-- {
		rec := &prRecords[i]
		if rec.Kind == recordRequest && rec.Counted && rec.RequestHead == head {
			g.add(labelChecked, "  a review of this head was requested at %s; no disposition for it is recorded: %s",
				rec.Comment.CreatedAt.UTC().Format(time.RFC3339), rec.Comment.URL)
			return false, "Agent #2 PR review of the current head (independent-implementation-review)"
		}
	}
	g.add(labelChecked, "  no Agent #2 PR review request names this head")
	return false, "Agent #1 posts the Agent #2 PR Review Request for the current head (integration-pr-review)"
}

func evaluateChecks(g *statusSection, it *ghItem, head string) bool {
	if it.Commits == nil || len(it.Commits.Nodes) == 0 || it.Commits.Nodes[0].Commit.Oid != head {
		g.add(labelChecked, "the check state of the head was not available")
		return false
	}
	rollup := it.Commits.Nodes[0].Commit.StatusCheckRollup
	if rollup == nil {
		g.add(labelChecked, "no checks are reported for the head")
		return false
	}
	g.add(labelChecked, "checks on the head: %s", strings.ToLower(rollup.State))
	return rollup.State == "SUCCESS"
}

// evaluateHumanAcceptance reports the change acceptor's approval. Only an
// approving GitHub review is evidence of it (#40 decision, item 4).
func evaluateHumanAcceptance(g *statusSection, in statusInputs, it *ghItem, head string) bool {
	acceptor := "the change acceptor"
	if in.Config != nil {
		acceptor = fmt.Sprintf("the change acceptor (%s)", in.Config.Project.Authorities.ChangeAcceptor.Role)
	}
	found := false
	if it.Reviews != nil {
		for _, rv := range it.Reviews.Nodes {
			if rv.State != "APPROVED" || rv.Author == nil || rv.Author.Typename == "Bot" ||
				!contains(countedAssociations, rv.AuthorAssociation) {
				continue
			}
			found = true
			on := "the head"
			if rv.Commit != nil && rv.Commit.Oid != head {
				on = "earlier commit " + shortSHA(rv.Commit.Oid)
			}
			g.add(labelChecked, "approving GitHub review by %s on %s: %s", rv.Author.Login, on, rv.URL)
		}
	}
	if !found {
		g.add(labelChecked, "human acceptance by %s: NOT EVIDENCED (no approving GitHub review; this reports evidence, not compliance)", acceptor)
	}
	return found
}

func addConfigSection(r *statusReport, in statusInputs) {
	s := r.section("Project configuration")
	if in.Config == nil {
		s.add(labelConfig, "not used: %s; configuration-dependent checks were not evaluated", in.ConfigProblem)
		return
	}
	branch := defaultIntegrationBranch + " (default)"
	if b := in.Config.Project.IntegrationBranch; b != nil {
		branch = *b
	}
	s.add(labelConfig, "valid; integration branch (E1): %s", branch)
	if in.Item.isPullRequest() {
		want := strings.TrimSuffix(branch, " (default)")
		if in.Item.BaseRefName == want {
			s.add(labelChecked, "the pull request targets the configured integration branch")
		} else {
			s.add(labelChecked, "the pull request targets %s, NOT the configured integration branch %s", in.Item.BaseRefName, want)
		}
	}
}

// addLocalSection lists local history for the target. It is annotation only:
// it is never consulted for any gate or eligibility above.
func addLocalSection(r *statusReport, kind string, number int, events []historyEvent, discarded int, problem string) {
	s := r.section("Local history (private annotation; may be incomplete; never evidence)")
	if problem != "" {
		s.add(labelLocal, "unavailable: %s", problem)
		return
	}
	shown := 0
	for _, e := range events {
		if e.Kind != kind || e.Number != number {
			continue
		}
		shown++
		link := ""
		if e.URL != "" {
			link = ": " + e.URL
		}
		s.add(labelLocal, "%s %s %s%s", e.Time, e.Type, e.Outcome, link)
	}
	if shown == 0 {
		s.add(labelLocal, "no local events for this %s", map[string]string{"issue": "issue", "pr": "pull request"}[kind])
	}
	if discarded > 0 {
		s.add(labelLocal, "%d unreadable local entries were skipped", discarded)
	}
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func shortChangeset(s string) string {
	if v, ok := strings.CutPrefix(s, "sha256:"); ok && len(v) > 8 {
		return "sha256:" + v[:8] + "…"
	}
	return s
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return "an unknown time"
	}
	return t.UTC().Format(time.RFC3339)
}

func orNone(s string) string {
	if s == "" {
		return "NONE"
	}
	return s
}
