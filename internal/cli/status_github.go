package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitHub access for "garage status".
//
// GitHub is read through the gh command (#19 decision): Garage never stores
// or handles a token, and each invocation makes exactly one read, a single
// GraphQL query. The runner refuses anything that is not a read-only query,
// so status can never write GitHub state.

// statusGHTimeout bounds the one GitHub read an invocation makes.
const statusGHTimeout = 30 * time.Second

// statusQuery is the only request status sends to GitHub. It reads one issue
// or pull request, its comments, and, for a pull request, its reviews, the
// check state of its head, and the issues it closes, with their comments.
const statusQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issueOrPullRequest(number: $number) {
      __typename
      ... on Issue {
        number title url state closedAt
        comments(last: 100) { totalCount pageInfo { hasPreviousPage } nodes { ...WorkflowComment } }
        closedByPullRequestsReferences(first: 10, includeClosedPrs: true) { nodes { number url state mergedAt } }
      }
      ... on PullRequest {
        number title url state body merged mergedAt baseRefName headRefOid
        mergeCommit { oid }
        comments(last: 100) { totalCount pageInfo { hasPreviousPage } nodes { ...WorkflowComment } }
        reviews(last: 100) { nodes { state submittedAt url authorAssociation author { login __typename } commit { oid } comments { totalCount } } }
        commits(last: 1) { nodes { commit { oid statusCheckRollup { state } } } }
        closingIssuesReferences(first: 5) { nodes { number url state comments(last: 100) { totalCount pageInfo { hasPreviousPage } nodes { ...WorkflowComment } } } }
      }
    }
  }
}
fragment WorkflowComment on IssueComment { url body createdAt lastEditedAt authorAssociation author { login __typename } }`

// ghRunner runs gh with args and returns its standard output and standard
// error.
type ghRunner func(ctx context.Context, args ...string) (stdout, stderr []byte, err error)

func runGH(ctx context.Context, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// errGHNotReadOnly reports a GitHub request that status refuses to send.
var errGHNotReadOnly = errors.New("refusing a GitHub request that is not a read-only GraphQL query")

var graphQLWritePattern = regexp.MustCompile(`\b(mutation|subscription)\b`)

// readOnlyGH wraps run so that only a read-only "gh api graphql" query can
// be sent. Any other gh invocation is refused before gh is run.
func readOnlyGH(run ghRunner) ghRunner {
	return func(ctx context.Context, args ...string) ([]byte, []byte, error) {
		if err := checkReadOnlyGHArgs(args); err != nil {
			return nil, nil, err
		}
		return run(ctx, args...)
	}
}

// checkReadOnlyGHArgs accepts only "gh api graphql" with field flags, an
// optional --hostname, and exactly one query that is a GraphQL query
// operation. A method override, an input file, a mutation, or a subscription
// is refused.
func checkReadOnlyGHArgs(args []string) error {
	if len(args) < 2 || args[0] != "api" || args[1] != "graphql" {
		return errGHNotReadOnly
	}
	queries := 0
	for i := 2; i < len(args); i++ {
		switch flag := args[i]; flag {
		case "-f", "-F", "--raw-field", "--field", "--hostname":
			if i+1 >= len(args) {
				return errGHNotReadOnly
			}
			i++
			if flag == "--hostname" {
				continue
			}
			if value, ok := strings.CutPrefix(args[i], "query="); ok {
				query := strings.TrimSpace(value)
				if !strings.HasPrefix(query, "query") || graphQLWritePattern.MatchString(query) {
					return errGHNotReadOnly
				}
				queries++
			}
		default:
			return errGHNotReadOnly
		}
	}
	if queries != 1 {
		return errGHNotReadOnly
	}
	return nil
}

// ghRepository names a GitHub repository.
type ghRepository struct {
	Host  string
	Owner string
	Name  string
}

// githubRepository derives the GitHub repository from the "origin" remote of
// the repository in dir, locally and without network access.
func githubRepository(ctx context.Context, dir string, git gitRunner) (ghRepository, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	out, err := git(ctx, dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return ghRepository{}, errors.New("the repository has no origin remote")
	}
	parts := strings.Split(normalizeRemoteURL(string(out)), "/")
	if len(parts) != 3 || parts[0] == "path" || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return ghRepository{}, errors.New("the origin remote does not name a GitHub repository as host/owner/name")
	}
	return ghRepository{Host: parts[0], Owner: parts[1], Name: parts[2]}, nil
}

// errGitHubNotConsulted reports that status could not read GitHub. The view
// it printed then reports no workflow state and no eligibility.
var errGitHubNotConsulted = errors.New("GitHub was not consulted")

// ghUnavailable describes why GitHub could not be read.
type ghUnavailable struct{ reason string }

func (e ghUnavailable) Error() string { return e.reason }

// errStatusNotFound reports that the number names no issue or pull request.
var errStatusNotFound = errors.New("no issue or pull request has that number")

// GitHub GraphQL response types. Only the fields status reads are decoded.

type ghActor struct {
	Login    string `json:"login"`
	Typename string `json:"__typename"`
}

type ghComment struct {
	URL               string     `json:"url"`
	Body              string     `json:"body"`
	CreatedAt         time.Time  `json:"createdAt"`
	LastEditedAt      *time.Time `json:"lastEditedAt"`
	AuthorAssociation string     `json:"authorAssociation"`
	Author            *ghActor   `json:"author"`
}

type ghComments struct {
	TotalCount int `json:"totalCount"`
	PageInfo   struct {
		HasPreviousPage bool `json:"hasPreviousPage"`
	} `json:"pageInfo"`
	Nodes []ghComment `json:"nodes"`
}

type ghLinkedPR struct {
	Number   int        `json:"number"`
	URL      string     `json:"url"`
	State    string     `json:"state"`
	MergedAt *time.Time `json:"mergedAt"`
}

type ghReview struct {
	State             string     `json:"state"`
	SubmittedAt       *time.Time `json:"submittedAt"`
	URL               string     `json:"url"`
	AuthorAssociation string     `json:"authorAssociation"`
	Author            *ghActor   `json:"author"`
	Commit            *struct {
		Oid string `json:"oid"`
	} `json:"commit"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
}

type ghClosingIssue struct {
	Number   int        `json:"number"`
	URL      string     `json:"url"`
	State    string     `json:"state"`
	Comments ghComments `json:"comments"`
}

type ghItem struct {
	Typename string     `json:"__typename"`
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	URL      string     `json:"url"`
	State    string     `json:"state"`
	ClosedAt *time.Time `json:"closedAt"`
	Comments ghComments `json:"comments"`

	// Issue only.
	ClosedByPullRequestsReferences *struct {
		Nodes []ghLinkedPR `json:"nodes"`
	} `json:"closedByPullRequestsReferences"`

	// Pull request only.
	Body        string     `json:"body"`
	Merged      bool       `json:"merged"`
	MergedAt    *time.Time `json:"mergedAt"`
	BaseRefName string     `json:"baseRefName"`
	HeadRefOid  string     `json:"headRefOid"`
	MergeCommit *struct {
		Oid string `json:"oid"`
	} `json:"mergeCommit"`
	Reviews *struct {
		Nodes []ghReview `json:"nodes"`
	} `json:"reviews"`
	Commits *struct {
		Nodes []struct {
			Commit struct {
				Oid               string `json:"oid"`
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	ClosingIssuesReferences *struct {
		Nodes []ghClosingIssue `json:"nodes"`
	} `json:"closingIssuesReferences"`
}

func (it *ghItem) isPullRequest() bool { return it.Typename == "PullRequest" }

type ghResponse struct {
	Data *struct {
		Repository *struct {
			Item *ghItem `json:"issueOrPullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"errors"`
}

// fetchStatusItem makes the invocation's one GitHub read. A missing or
// unauthenticated gh, or any other failure to read, is a ghUnavailable error.
func fetchStatusItem(ctx context.Context, gh ghRunner, repo ghRepository, number int) (*ghItem, error) {
	ctx, cancel := context.WithTimeout(ctx, statusGHTimeout)
	defer cancel()

	stdout, stderr, err := readOnlyGH(gh)(ctx, "api", "graphql",
		"--hostname", repo.Host,
		"-f", "query="+statusQuery,
		"-F", "owner="+repo.Owner,
		"-F", "name="+repo.Name,
		"-F", "number="+strconv.Itoa(number))

	var resp ghResponse
	decodeErr := json.Unmarshal(stdout, &resp)
	if decodeErr == nil && notFoundResponse(resp) {
		return nil, errStatusNotFound
	}
	if err != nil {
		return nil, classifyGHError(ctx, err, stderr)
	}
	if decodeErr != nil {
		return nil, ghUnavailable{"gh returned a response that is not valid JSON"}
	}
	if len(resp.Errors) > 0 {
		return nil, ghUnavailable{"GitHub reported an error: " + oneLine(resp.Errors[0].Message)}
	}
	if resp.Data == nil || resp.Data.Repository == nil {
		return nil, ghUnavailable{"GitHub returned no repository data"}
	}
	if resp.Data.Repository.Item == nil {
		return nil, errStatusNotFound
	}
	return resp.Data.Repository.Item, nil
}

func notFoundResponse(resp ghResponse) bool {
	for _, e := range resp.Errors {
		if e.Type == "NOT_FOUND" && strings.Contains(e.Message, "issue or pull request") {
			return true
		}
	}
	return false
}

func classifyGHError(ctx context.Context, err error, stderr []byte) error {
	switch {
	case errors.Is(err, errGHNotReadOnly):
		return err
	case errors.Is(err, exec.ErrNotFound):
		return ghUnavailable{"the gh command is not installed"}
	case ctx.Err() != nil:
		return ghUnavailable{"the GitHub read timed out"}
	}
	text := strings.ToLower(string(stderr))
	if strings.Contains(text, "gh auth login") || strings.Contains(text, "not logged in") ||
		strings.Contains(text, "authentication") || strings.Contains(text, "http 401") {
		return ghUnavailable{"gh is not authenticated; run \"gh auth login\""}
	}
	if msg := oneLine(string(stderr)); msg != "" {
		return ghUnavailable{"gh failed: " + msg}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return ghUnavailable{fmt.Sprintf("gh failed with exit status %d", exitErr.ExitCode())}
	}
	return ghUnavailable{"cannot run gh: " + err.Error()}
}

// oneLine returns the first non-empty line of s, trimmed and bounded.
func oneLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if len(line) > 200 {
				line = line[:200] + "…"
			}
			return line
		}
	}
	return ""
}
