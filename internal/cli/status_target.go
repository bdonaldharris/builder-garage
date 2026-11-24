package cli

import (
	"errors"
	"strings"
)

// parseWorkflowTarget adds explicit kinds and canonical GitHub URLs while
// retaining the legacy single-number contract. GitLab grammar belongs to its
// observation adapter; configuring GitLab must never select this adapter.
func parseWorkflowTarget(args []string) (workflowTarget, error) {
	var t workflowTarget
	if len(args) == 2 {
		switch args[0] {
		case "issue":
			t.Kind = workflowIssueKind
		case "pr":
			t.Kind = workflowChangeKind
		default:
			return t, errors.New("use issue NUMBER or pr NUMBER for an explicit GitHub target")
		}
		if strings.HasPrefix(args[1], "#") {
			return t, errors.New("an explicit target requires a positive number without #")
		}
		n, err := parseStatusTarget(args[1])
		t.Number = n
		return t, err
	}
	if len(args) != 1 {
		return t, errors.New("provide a number, issue NUMBER, pr NUMBER, or canonical HTTPS issue/PR URL")
	}
	if !strings.Contains(args[0], ":") && !strings.Contains(args[0], "/") {
		n, err := parseStatusTarget(args[0])
		t.Number = n
		return t, err
	}
	u, err := trustedHTTPS(args[0])
	if err != nil {
		return t, errors.New("target must be a canonical HTTPS issue/PR URL without credentials, query, or fragment")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 4 || !validForgeProject(parts[0]+"/"+parts[1], "github") {
		return t, errors.New("target URL must identify one canonical GitHub issue or pull request")
	}
	switch parts[2] {
	case "issues":
		t.Kind = workflowIssueKind
	case "pull":
		t.Kind = workflowChangeKind
	default:
		return t, errors.New("target URL must use /issues/NUMBER or /pull/NUMBER")
	}
	if parts[3] == "" || strings.Trim(parts[3], "0123456789") != "" || strings.HasPrefix(parts[3], "0") {
		return t, errors.New("target URL requires a canonical positive number")
	}
	t.Number, err = parseStatusTarget(parts[3])
	t.Project, t.Host = parts[0]+"/"+parts[1], u.Host
	return t, err
}
