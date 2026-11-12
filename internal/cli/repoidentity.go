package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gitTimeout bounds each local git invocation used to identify a repository.
const gitTimeout = 5 * time.Second

// repoIdentity identifies the repository a command runs in. It is derived
// locally, without network access.
type repoIdentity struct {
	// Key is a stable, opaque key for the repository: the first 32 hex digits
	// of the SHA-256 of its identity string. The identity string itself, which
	// may contain a remote URL, is never stored.
	Key string
	// TopLevel is the absolute path of the working tree's top-level directory.
	TopLevel string
}

// errNotRepository reports that the directory is not inside a git repository.
var errNotRepository = errors.New("not inside a git repository")

// gitRunner runs git with args in dir and returns its standard output.
type gitRunner func(ctx context.Context, dir string, args ...string) ([]byte, error)

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.Bytes(), err
}

// identifyRepository derives the repository identity for dir.
//
// The identity is the normalized URL of the "origin" remote, so every clone
// and worktree of the same repository shares one history. Without an origin
// remote, it falls back to the absolute path of the repository's common git
// directory, which worktrees of one clone share. Credentials in a remote URL
// never contribute to the identity.
func identifyRepository(ctx context.Context, dir string, git gitRunner) (repoIdentity, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	out, err := git(ctx, dir, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return repoIdentity{}, errNotRepository
		}
		return repoIdentity{}, fmt.Errorf("cannot run git: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return repoIdentity{}, errNotRepository
	}
	topLevel := filepath.Clean(strings.TrimSpace(lines[0]))
	commonDir := filepath.Clean(strings.TrimSpace(lines[1]))

	identity := "local:" + filepath.ToSlash(commonDir)
	// "git config --get" exits 1 when the key is absent; that is the fallback
	// case, not a failure.
	if remote, err := git(ctx, dir, "config", "--get", "remote.origin.url"); err == nil {
		if normalized := normalizeRemoteURL(string(remote)); normalized != "" {
			identity = "remote:" + normalized
		}
	}

	sum := sha256.Sum256([]byte(identity))
	return repoIdentity{Key: hex.EncodeToString(sum[:16]), TopLevel: topLevel}, nil
}

// normalizeRemoteURL reduces a git remote URL to "host/path" in lower case,
// without user information, port, scheme, trailing slash, or ".git" suffix, so
// that the HTTPS, SSH, and scp-like forms of one remote produce the same
// value. It returns "" when nothing usable remains.
func normalizeRemoteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var host, path string
	if isSCPLikeRemote(raw) {
		hostPart, pathPart, _ := strings.Cut(raw, ":")
		if at := strings.LastIndex(hostPart, "@"); at >= 0 {
			hostPart = hostPart[at+1:]
		}
		host, path = hostPart, pathPart
	} else if u, err := url.Parse(raw); err == nil && u.Scheme != "" {
		host, path = u.Hostname(), u.Path
		if host == "" {
			host = u.Scheme
		}
	} else {
		host, path = "path", filepath.ToSlash(raw)
	}

	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	if host == "" && path == "" {
		return ""
	}
	return strings.ToLower(host + "/" + path)
}

// isSCPLikeRemote reports whether raw uses git's scp-like syntax,
// "[user@]host:path", which has a colon before any slash and no scheme.
func isSCPLikeRemote(raw string) bool {
	if strings.Contains(raw, "://") {
		return false
	}
	colon := strings.Index(raw, ":")
	if colon <= 0 {
		return false
	}
	slash := strings.Index(raw, "/")
	if slash >= 0 && slash < colon {
		return false
	}
	// A single letter followed by a colon is a Windows drive, not a host.
	return colon != 1
}
