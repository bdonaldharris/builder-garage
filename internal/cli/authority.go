package cli

import (
	"errors"
	"net/url"
	"strings"
)

// authorityMember is a platform identity, never a role or proof of humanity.
type authorityMember struct{ Provider, Host, User string }

func parseAuthorityMember(raw string) (authorityMember, error) {
	provider, rest, qualified := strings.Cut(raw, ":")
	if !qualified {
		return authorityMember{Provider: "github", User: raw}, nil
	}
	if provider == "github" && validAuthorityUser(rest) {
		return authorityMember{Provider: provider, User: rest}, nil
	}
	if provider == "gitlab" {
		host, user, ok := strings.Cut(rest, "/")
		u, err := trustedHTTPS("https://" + host)
		if ok && err == nil && u.Path == "" && validAuthorityUser(user) {
			return authorityMember{provider, strings.ToLower(canonicalHTTPSHost(u.Host)), user}, nil
		}
	}
	return authorityMember{}, errors.New("want an existing GitHub member, github:USER, or gitlab:HOST/USER")
}
func validAuthorityUser(s string) bool {
	if s == "" || strings.ContainsAny(s, "/:@?#\\ \t\r\n") {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return s != "." && s != ".."
}
func authorityMatches(a *authority, provider, host, user string) bool {
	if a == nil {
		return false
	}
	for _, raw := range a.Members {
		m, err := parseAuthorityMember(raw)
		if err != nil || m.Provider != provider {
			continue
		}
		if provider == "github" && strings.EqualFold(m.User, user) {
			return true
		}
		if provider == "gitlab" && strings.EqualFold(m.Host, canonicalHTTPSHost(host)) && m.User == user {
			return true
		}
	}
	return false
}
func forgeHost(f forgeConfig) string { u, _ := url.Parse(effectiveForge(f).Endpoint); return u.Host }
