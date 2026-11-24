package cli

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func parseGitlabTarget(args []string, f forgeConfig) (workflowTarget, error) {
	f = effectiveForge(f)
	host, _ := url.Parse(f.Endpoint)
	t := workflowTarget{Host: host.Host, Project: f.Project}
	var raw string
	if len(args) == 2 {
		switch args[0] {
		case "issue":
			t.Kind = workflowIssueKind
		case "mr":
			t.Kind = workflowChangeKind
		default:
			return t, errors.New("GitLab requires issue IID or mr IID")
		}
		raw = args[1]
	} else if len(args) == 1 {
		u, e := trustedHTTPS(args[0])
		if e != nil {
			return t, errors.New("GitLab requires issue IID, mr IID, or a canonical HTTPS issue/MR URL; bare numbers and #IID are ambiguous")
		}
		split := strings.LastIndex(u.Path, "/-/")
		if split < 0 {
			return t, errors.New("GitLab URL must use /-/issues/IID or /-/merge_requests/IID")
		}
		project := strings.TrimPrefix(u.Path[:split], "/")
		parts := strings.Split(u.Path[split+3:], "/")
		if !validForgeProject(project, "gitlab") || len(parts) != 2 {
			return t, errors.New("GitLab URL must identify one canonical project and issue/MR")
		}
		switch parts[0] {
		case "issues":
			t.Kind = workflowIssueKind
		case "merge_requests":
			t.Kind = workflowChangeKind
		default:
			return t, errors.New("GitLab URL must use issues or merge_requests")
		}
		t.Project, t.Host, raw = project, u.Host, parts[1]
	} else {
		return t, errors.New("provide issue IID, mr IID, or one GitLab HTTPS URL")
	}
	if !positiveID(raw) {
		return t, errors.New("GitLab target requires a canonical positive IID")
	}
	t.Number, _ = strconv.Atoi(raw)
	if t.Number > maxTargetNumber {
		return t, errors.New("GitLab target IID exceeds the supported range")
	}
	return t, validateGitlabTarget(t, f)
}
func validateGitlabTarget(t workflowTarget, f forgeConfig) error {
	f = effectiveForge(f)
	host, _ := url.Parse(f.Endpoint)
	if t.Kind != workflowIssueKind && t.Kind != workflowChangeKind || t.Number < 1 || t.Number > maxTargetNumber {
		return errors.New("GitLab requires a typed issue or MR IID")
	}
	if t.Host != "" && !strings.EqualFold(canonicalHTTPSHost(t.Host), canonicalHTTPSHost(host.Host)) {
		return errors.New("GitLab target host does not match configured forge")
	}
	if t.Project != "" && (!validForgeProject(t.Project, "gitlab") || (t.Project != f.Project && !f.AllowCrossProjectTargets)) {
		return errors.New("GitLab target project does not match configuration; cross-project targets are not enabled")
	}
	return nil
}

var gitlabReferencePattern = regexp.MustCompile(`(?:^|[\s(\[])(?:([A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+))?([#!])([1-9][0-9]*)\b`)

// Markdown references are typed and project-qualified before any fetch or
// deduplication. A disallowed qualified reference is never retried as local.
func gitlabReferences(body string, f forgeConfig) ([]workflowTarget, []string) {
	f = effectiveForge(f)
	host, _ := url.Parse(f.Endpoint)
	var targets []workflowTarget
	var problems []string
	seen := map[workflowTargetKey]bool{}
	for _, m := range gitlabReferencePattern.FindAllStringSubmatch(htmlComment.ReplaceAllString(body, ""), -1) {
		project := m[1]
		if project == "" {
			project = f.Project
		}
		n, e := strconv.Atoi(m[3])
		if e != nil || n > maxTargetNumber {
			continue
		}
		kind := workflowIssueKind
		if m[2] == "!" {
			kind = workflowChangeKind
		}
		target := workflowTarget{Host: host.Host, Project: project, Kind: kind, Number: n}
		if err := validateGitlabTarget(target, f); err != nil {
			problems = append(problems, "unsupported reference: qualified project is invalid or cross-project targets are disabled")
			continue
		}
		key := workflowTargetKey{project, kind, n}
		if !seen[key] {
			seen[key] = true
			targets = append(targets, target)
		}
	}
	return targets, problems
}

type workflowTargetKey struct {
	Project, Kind string
	Number        int
}
