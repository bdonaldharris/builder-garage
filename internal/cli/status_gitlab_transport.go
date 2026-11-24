package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// glabRunner is the external authentication boundary. Observation constructs
// every argument itself; neither callers nor pagination can supply CLI flags.
type glabRunner func(context.Context, ...string) ([]byte, []byte, error)

const glabResponseLimit = 8 << 20
const gitlabReadBudget = 64
const gitlabPageLimit = 3
const gitlabPageSize = 100

var errGLabNotReadOnly = errors.New("refusing GitLab observation outside its GET resource allowlist")

type boundedGLabOutput struct{ bytes.Buffer }

func (b *boundedGLabOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > glabResponseLimit {
		return 0, errors.New("GitLab response exceeds observation size limit")
	}
	return b.Buffer.Write(p)
}
func runGLab(ctx context.Context, args ...string) ([]byte, []byte, error) {
	return runGLabInput(ctx, nil, args...)
}
func runGLabInput(ctx context.Context, input []byte, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "glab", args...)
	cmd.Stdin = bytes.NewReader(input)
	var out, stderr boundedGLabOutput
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	return out.Bytes(), stderr.Bytes(), err
}

type gitlabReadError struct {
	Status  int
	Problem string
}

func (e *gitlabReadError) Error() string { return e.Problem }
func gitlabHTTPProblem(status int) string {
	switch status {
	case 401, 403, 404:
		return "inaccessible: authentication, visibility, or resource absence cannot be established"
	case 405, 501:
		return "unsupported: this deployment does not expose the observation capability"
	case 429:
		return "unavailable: GitLab request limit reached"
	default:
		return "unavailable: GitLab observation request failed"
	}
}

type gitlabClient struct {
	run      glabRunner
	forge    forgeConfig
	reads    int
	projects map[string]bool
}

func newGitlabClient(run glabRunner, f forgeConfig) *gitlabClient {
	return &gitlabClient{run: run, forge: effectiveForge(f), projects: map[string]bool{f.Project: true}}
}
func (c *gitlabClient) resource(project string, parts ...string) string {
	return "projects/" + url.PathEscape(project) + "/" + strings.Join(parts, "/")
}
func positiveID(s string) bool {
	n, e := strconv.Atoi(s)
	return e == nil && n > 0 && strconv.Itoa(n) == s
}

// allowed accepts only the deliberately implemented REST-v4 observations.
// Escaped project/branch components stay single API identifiers. No URL,
// dot-segment, placeholder, unknown query, request body, or write is accepted.
func (c *gitlabClient) allowed(method, endpoint string) bool {
	if method != "GET" || strings.ContainsAny(endpoint, "\\#:\r\n") {
		return false
	}
	u, e := url.Parse(endpoint)
	if e != nil || u.IsAbs() || u.Host != "" || strings.HasPrefix(u.Path, "/") {
		return false
	}
	raw := strings.Split(strings.SplitN(endpoint, "?", 2)[0], "/")
	if len(raw) < 2 || raw[0] != "projects" {
		return false
	}
	project, e := url.PathUnescape(raw[1])
	if e != nil || (!validForgeProject(project, "gitlab") && !positiveID(project)) || !c.projects[project] {
		return false
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return false
	}
	for k, v := range q {
		if (k != "page" && k != "per_page") || len(v) != 1 || !positiveID(v[0]) {
			return false
		}
		if k == "per_page" && v[0] != strconv.Itoa(gitlabPageSize) {
			return false
		}
	}
	if len(raw) == 2 {
		return len(q) == 0
	}
	rest := raw[2:]
	switch rest[0] {
	case "issues", "merge_requests":
		if len(rest) < 2 || !positiveID(rest[1]) {
			return false
		}
		if len(rest) == 2 {
			return len(q) == 0
		}
		if len(rest) != 3 {
			return false
		}
		switch rest[2] {
		case "notes", "discussions":
			return true
		case "closed_by":
			return rest[0] == "issues"
		case "closes_issues":
			return rest[0] == "merge_requests"
		case "approvals", "approval_state":
			return rest[0] == "merge_requests" && len(q) == 0
		case "versions", "pipelines":
			return rest[0] == "merge_requests"
		}
	case "protected_branches", "protected_tags":
		return len(rest) == 1
	case "approvals":
		return len(rest) == 1 && len(q) == 0
	case "members":
		return len(rest) == 3 && rest[1] == "all" && positiveID(rest[2]) && len(q) == 0
	case "repository":
		if len(rest) != 3 || rest[1] != "branches" || len(q) != 0 {
			return false
		}
		branch, e := url.PathUnescape(rest[2])
		return e == nil && branch != "" && !strings.Contains(branch, "..") && !strings.ContainsAny(branch, "\r\n\\")
	case "pipelines":
		return (len(rest) == 2 && positiveID(rest[1]) && len(q) == 0) || (len(rest) == 3 && positiveID(rest[1]) && rest[2] == "jobs")
	}
	return false
}
func (c *gitlabClient) get(ctx context.Context, method, endpoint string, out any) (http.Header, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.New("unavailable: GitLab observation canceled or timed out")
	}
	if !c.allowed(method, endpoint) {
		return nil, errGLabNotReadOnly
	}
	if c.reads >= gitlabReadBudget {
		return nil, errors.New("truncated: GitLab observation read budget exhausted")
	}
	if c.run == nil {
		return nil, errors.New("unavailable: glab runner is not configured")
	}
	c.reads++
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	host, _ := url.Parse(c.forge.Endpoint)
	data, _, runErr := c.run(ctx, "api", "--hostname", host.Host, "--method", "GET", "--include", endpoint)
	if ctx.Err() != nil {
		return nil, errors.New("unavailable: GitLab observation canceled or timed out")
	}
	if len(data) > glabResponseLimit {
		return nil, errors.New("truncated: GitLab response exceeds observation size limit")
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), &http.Request{Method: "GET"})
	if err != nil {
		return nil, errors.New("unavailable: glab could not supply an authenticated HTTP response (install/authenticate glab for the configured host)")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return response.Header, &gitlabReadError{Status: response.StatusCode, Problem: gitlabHTTPProblem(response.StatusCode)}
	}
	if runErr != nil {
		return nil, errors.New("unavailable: glab observation failed; no response facts accepted")
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, glabResponseLimit+1))
	if err := decoder.Decode(out); err != nil {
		return response.Header, errors.New("unavailable: GitLab returned an invalid observation response")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return response.Header, errors.New("unavailable: GitLab returned trailing response data")
	}
	return response.Header, nil
}

// list constructs each continuation instead of handing arbitrary server links
// back to glab. A supplied Link is validated before any next request is made.
func gitlabList[T any](ctx context.Context, c *gitlabClient, endpoint string) ([]T, bool, error) {
	var result []T
	for page := 1; page <= gitlabPageLimit; page++ {
		var items []T
		query := fmt.Sprintf("%s?per_page=%d&page=%d", endpoint, gitlabPageSize, page)
		headers, err := c.get(ctx, "GET", query, &items)
		if err != nil {
			return result, true, err
		}
		if items == nil {
			return result, true, errors.New("unavailable: GitLab returned null instead of a collection")
		}
		result = append(result, items...)
		next := headers.Get("X-Next-Page")
		linkNext := ""
		for _, link := range strings.Split(headers.Get("Link"), ",") {
			fields := strings.Split(link, ";")
			if len(fields) < 2 || !strings.Contains(strings.Join(fields[1:], ";"), `rel="next"`) {
				continue
			}
			u, e := url.Parse(strings.Trim(strings.TrimSpace(fields[0]), "<>"))
			base, _ := url.Parse(c.forge.APIBaseURL)
			if e != nil || u.Scheme != "https" || u.User != nil || !strings.EqualFold(canonicalHTTPSHost(u.Host), canonicalHTTPSHost(base.Host)) || u.Fragment != "" || u.EscapedPath() != base.EscapedPath()+"/"+endpoint {
				return result, true, errors.New("truncated: refused an untrusted GitLab pagination continuation")
			}
			values, e := url.ParseQuery(u.RawQuery)
			if e != nil || len(values) != 2 || values.Get("per_page") != strconv.Itoa(gitlabPageSize) || !c.allowed("GET", endpoint+"?"+u.RawQuery) {
				return result, true, errors.New("truncated: refused unsupported GitLab pagination parameters")
			}
			linkNext = values.Get("page")
		}
		if next != "" && linkNext != "" && next != linkNext {
			return result, true, errors.New("truncated: contradictory GitLab pagination headers")
		}
		if next == "" {
			next = linkNext
		}
		if totalPages := headers.Get("X-Total-Pages"); totalPages != "" {
			total, e := strconv.Atoi(totalPages)
			if e == nil && total == 0 && page == 1 && len(items) == 0 && next == "" {
				return result, false, nil
			}
			if e != nil || total < page {
				return result, true, errors.New("truncated: inconsistent GitLab pagination totals")
			}
			if total == page {
				if next != "" {
					return result, true, errors.New("truncated: next page contradicts GitLab pagination totals")
				}
				return result, false, nil
			}
			if next == "" && total > page {
				next = strconv.Itoa(page + 1)
			}
		}
		if next == "" {
			// A short page is complete. A full page without reliable totals gets
			// one further bounded read rather than being silently declared complete.
			if len(items) < gitlabPageSize {
				return result, false, nil
			}
			next = strconv.Itoa(page + 1)
		}
		if next != strconv.Itoa(page+1) {
			return result, true, errors.New("truncated: refused nonsequential GitLab pagination")
		}
	}
	return result, true, errors.New("truncated: GitLab observation page limit reached")
}
