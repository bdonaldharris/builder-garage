package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// forgeConfig is an opt-in identity contract, never an authentication store.
type forgeConfig struct {
	Provider                 string `yaml:"provider"`
	Endpoint                 string `yaml:"endpoint,omitempty"`
	Project                  string `yaml:"project"`
	APIBaseURL               string `yaml:"api_base_url,omitempty"`
	AllowCrossProjectTargets bool   `yaml:"allow_cross_project_targets,omitempty"`
}

var forgeHostLabel = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

var forgeSegment = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func validForgeProject(project, provider string) bool {
	parts := strings.Split(project, "/")
	if len(parts) < 2 || (provider == "github" && len(parts) != 2) || strings.HasSuffix(project, ".git") {
		return false
	}
	for _, part := range parts {
		if !forgeSegment.MatchString(part) || part == "." || part == ".." || strings.HasSuffix(part, ".") {
			return false
		}
	}
	return true
}

func effectiveForge(f forgeConfig) forgeConfig {
	if f.Endpoint == "" {
		f.Endpoint = "https://" + f.Provider + ".com"
	}
	if f.Provider == "gitlab" && f.APIBaseURL == "" {
		f.APIBaseURL = f.Endpoint + "/api/v4"
	}
	return f
}

// trustedHTTPS validates syntax only; certificate trust is owned by the CLI
// transport. Never include the supplied URL in errors (it could hold a secret).
func trustedHTTPS(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || strings.HasSuffix(u.Host, ":") || strings.ContainsAny(raw, "\\ ?#\t\r\n") {
		return nil, errors.New("must be an absolute HTTPS URL without credentials, query, fragment, or escaped path")
	}
	if net.ParseIP(u.Hostname()) == nil {
		for _, label := range strings.Split(u.Hostname(), ".") {
			if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") || !forgeHostLabel.MatchString(label) {
				return nil, errors.New("invalid HTTPS hostname")
			}
		}
	}
	if port := u.Port(); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid HTTPS port")
		}
	}
	return u, nil
}

func validateForge(f forgeConfig) error {
	if f.Provider != "github" && f.Provider != "gitlab" {
		return errors.New("provider must be github or gitlab")
	}
	f = effectiveForge(f)
	u, err := trustedHTTPS(f.Endpoint)
	if err != nil || u.Path != "" {
		return errors.New("endpoint must be an HTTPS origin without a path, credentials, query, or fragment")
	}
	if (f.Provider == "github" && strings.EqualFold(u.Hostname(), "gitlab.com")) || (f.Provider == "gitlab" && strings.EqualFold(u.Hostname(), "github.com")) {
		return errors.New("provider does not match the public forge endpoint")
	}
	if !validForgeProject(f.Project, f.Provider) {
		return errors.New("project must be a canonical owner/project path (GitLab subgroups are allowed), without .git")
	}
	if f.Provider == "github" && f.APIBaseURL != "" {
		return errors.New("api_base_url is only supported for GitLab")
	}
	if f.Provider == "gitlab" {
		api, err := trustedHTTPS(f.APIBaseURL)
		if err != nil || !strings.EqualFold(canonicalHTTPSHost(api.Host), canonicalHTTPSHost(u.Host)) || api.Path != "/api/v4" {
			return errors.New("api_base_url must be the endpoint's HTTPS /api/v4 URL")
		}
	}
	return nil
}

// explicitOrigin uses a separate parser from private-history normalization:
// canonical paths and HTTPS ports must not be silently folded away.
func explicitOrigin(raw string) (host, project string, err error) {
	raw = strings.TrimSpace(raw)
	if isSCPLikeRemote(raw) {
		host, project, _ = strings.Cut(raw, ":")
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
	} else {
		u, e := url.Parse(raw)
		if e != nil || (u.Scheme != "https" && u.Scheme != "ssh") || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
			return "", "", errors.New("origin must be an HTTPS, SSH, or scp-like project remote")
		}
		host, project = u.Host, strings.TrimPrefix(u.Path, "/")
		// An SSH port selects a transport, not the web endpoint's port.
		if u.Scheme == "ssh" {
			host = u.Hostname()
		}
	}
	project = strings.TrimSuffix(strings.TrimSuffix(project, "/"), ".git")
	if host == "" || !validForgeProject(project, "gitlab") {
		return "", "", errors.New("origin does not identify a canonical project")
	}
	return host, project, nil
}

func checkForgeOrigin(ctx context.Context, dir string, git gitRunner, cfg projectConfig) error {
	if proj(&cfg).Forge == nil {
		return nil
	}
	f := effectiveForge(*cfg.Project.Forge)
	if err := validateForge(f); err != nil {
		return fmt.Errorf("project.forge: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	raw, err := git(ctx, dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return errors.New("project.forge requires a matching origin remote")
	}
	host, project, err := explicitOrigin(string(raw))
	u, _ := url.Parse(f.Endpoint)
	match := project == f.Project
	if f.Provider == "github" {
		match = strings.EqualFold(project, f.Project)
	}
	endpointHost := canonicalHTTPSHost(u.Host)
	if isSCPLikeRemote(strings.TrimSpace(string(raw))) || strings.HasPrefix(strings.TrimSpace(string(raw)), "ssh://") {
		endpointHost = u.Hostname()
	}
	if err != nil || !strings.EqualFold(canonicalHTTPSHost(host), endpointHost) || !match {
		return errors.New("origin does not match project.forge endpoint/project; configuration was not overridden")
	}
	return nil
}

func canonicalHTTPSHost(host string) string {
	if _, p, err := net.SplitHostPort(host); err == nil && p == "443" {
		return strings.TrimSuffix(host, ":443")
	}
	return host
}
