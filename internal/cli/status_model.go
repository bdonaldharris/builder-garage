package cli

import (
	"context"
	"time"
)

// workflowTarget identifies an artifact independently of a forge's transport.
// An empty kind is the legacy GitHub issue-or-change number lookup.
type workflowTarget struct {
	Kind    string
	Number  int
	Project string
	Host    string
}

const (
	workflowIssueKind  = "issue"
	workflowChangeKind = "change"
)

// statusProvider is consumed by the shared CLI/web operation. It returns
// normalized facts, never GraphQL/REST payloads or rendered text.
type statusProvider interface {
	Read(context.Context, workflowTarget, *projectConfig) (*workflowItem, int, error)
}

type workflowActor struct {
	Login string
	Bot   bool
}
type workflowComment struct {
	URL, Body         string
	CreatedAt         time.Time
	LastEditedAt      *time.Time
	AuthorAssociation string
	Author            *workflowActor
}
type workflowComments struct {
	TotalCount int
	Truncated  bool
	Nodes      []workflowComment
}
type workflowLinkedChange struct {
	Number     int
	URL, State string
	MergedAt   *time.Time
}
type workflowCommit struct{ Oid string }
type workflowReview struct {
	State                  string
	SubmittedAt            *time.Time
	URL, AuthorAssociation string
	Author                 *workflowActor
	Commit                 *workflowCommit
	CommentCount           int
}
type workflowIssue struct {
	Number     int
	URL, State string
	Comments   workflowComments
	References *workflowReferences
}
type workflowIssues struct {
	TotalCount int
	Nodes      []workflowIssue
}
type workflowChange struct {
	Number         int
	URL, State     string
	Merged         bool
	MergedAt       *time.Time
	Body           string
	BaseRefName    string
	HeadRefName    string
	BaseRepository string
	HeadRepository string
}
type workflowRepository struct{ NameWithOwner string }

func (r *workflowRepository) name() string {
	if r == nil {
		return ""
	}
	return r.NameWithOwner
}

type workflowReference struct {
	IsCrossRepository bool
	Source            *workflowReferenceSource
}
type workflowReferenceSource struct {
	Kind                           string
	Number                         int
	URL, State                     string
	Merged                         bool
	BaseRefName, HeadRefName       string
	BaseRepository, HeadRepository *workflowRepository
	MergedAt                       *time.Time
	Body                           string
}
type workflowReferences struct {
	TotalCount int
	Nodes      []workflowReference
}
type workflowChecks struct {
	Commit, State string
	Available     bool
}
type workflowItem struct {
	Kind                           string
	Number                         int
	Title, URL, State              string
	ClosedAt                       *time.Time
	Comments                       workflowComments
	ClosedByChanges                []workflowLinkedChange
	References                     *workflowReferences
	Body                           string
	Merged                         bool
	MergedAt                       *time.Time
	BaseRefName                    string
	BaseRefOid                     string
	HeadRefName                    string
	HeadRefOid                     string
	BaseRepository, HeadRepository *workflowRepository
	MergeCommit                    *workflowCommit
	Reviews                        []workflowReview
	Checks                         *workflowChecks
	ClosingIssues                  *workflowIssues
	DefaultBranch, RepositoryHost  string
	BodyIssues                     []workflowIssue
	BodyIssueNotes                 []string
	RepositoryIdentity             string
	StackRegistryFetched           bool
	StackRegistryComments          []workflowComment
	StatusPRs                      map[int]workflowChange
}

func (it *workflowItem) isPullRequest() bool { return it.Kind == workflowChangeKind }
