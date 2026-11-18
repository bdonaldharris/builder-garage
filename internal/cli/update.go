package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// The release-update check (#125 phase 3).
//
// "garage update check" is the only command that contacts GitHub directly,
// and only when it is run (#125 decisions 1, 2, and 7). It makes one
// unauthenticated HTTPS request for the repository's latest-release redirect,
// does not follow the redirect, and reads the latest tag from the validated
// Location. It downloads, installs, replaces, and records nothing, and it
// never inspects how garage was installed (decisions 3, 4, and 8).

const (
	// updateRepository is the repository whose releases are checked.
	updateRepository = "bdonaldharris/builder-garage"
	// updateRepositoryURL is the base of every URL the check uses or prints.
	updateRepositoryURL = "https://github.com/" + updateRepository
	// updateCheckTimeout bounds the one request a check makes.
	updateCheckTimeout = 15 * time.Second
	// updateUserAgent identifies the request without a version or any host
	// or user detail.
	updateUserAgent = "garage"
)

// releaseTagPattern is the only form a release tag takes, the same rule
// install.sh applies (ADR 0002, ADR 0003).
var releaseTagPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// sourceVersionPattern matches a source build's version derived by
// "git describe" after or at a version tag, such as v0.2.2-3-gabc1234 or
// v0.2.2-dirty (ADR 0002, decision 3).
var sourceVersionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-.+$`)

// updateEnv holds what a check depends on outside the program.
type updateEnv struct {
	// client sends the request. It must not follow redirects.
	client *http.Client
	// baseURL is the repository URL the latest-release redirect is requested
	// from, and the only one it may redirect within.
	baseURL string
	// goos selects which update paths are listed.
	goos string
}

func defaultUpdateEnv() updateEnv {
	return updateEnv{
		client:  newUpdateClient(),
		baseURL: updateRepositoryURL,
		goos:    runtime.GOOS,
	}
}

// newUpdateClient returns a client that never follows a redirect, so the
// redirect itself is the answer. It uses the default transport, so proxy
// settings from the environment apply.
func newUpdateClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func newUpdateCommand(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check whether a newer garage release is available",
		Long: `Check whether a newer garage release is available.

Garage never checks for updates on its own, in the background, or as part of
another command. It contacts GitHub only when you run "garage update check".`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newUpdateCheckCommand(version))
	return cmd
}

func newUpdateCheckCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Report the latest garage release, and how to update to it",
		Long: `Report this garage's version and the latest published release. When the
release is newer, list the documented ways to update to it.

The check makes one unauthenticated HTTPS request to GitHub, for the
repository's latest-release page, and reads the release's tag from GitHub's
redirect. It needs no token and no gh command. It downloads, installs, and
replaces nothing, and it does not install or update skills.

It exits 0 whenever the check completes, whether or not a newer release is
available, and with an error when it cannot complete, for example when GitHub
cannot be reached or answers unexpectedly.`,
		Example: `  garage update check`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdateCheck(cmd.Context(), defaultUpdateEnv(), version, cmd.OutOrStdout())
		},
	}
}

// Failures of the latest-release lookup.
var (
	errUpdateNotReached = errors.New("GitHub was not reached")
	errUpdateRefused    = errors.New("GitHub refused the request")
	errUpdateNoRelease  = errors.New("no published release was found")
	errUpdateUnexpected = errors.New("GitHub answered unexpectedly")
)

// runUpdateCheck looks up the latest release, compares it with version, and
// writes the result to out.
func runUpdateCheck(ctx context.Context, env updateEnv, version string, out io.Writer) error {
	latest, err := lookupLatestRelease(ctx, env)
	if err != nil {
		return fmt.Errorf("could not check for a newer release: %w", err)
	}
	_, err = io.WriteString(out, updateReport(version, latest, env.goos))
	return err
}

// lookupLatestRelease returns the latest release's tag, read from the
// redirect GitHub answers for <base>/releases/latest.
func lookupLatestRelease(ctx context.Context, env updateEnv) (string, error) {
	base, err := url.Parse(env.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL %q: %w", env.baseURL, err)
	}

	ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.baseURL+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", updateUserAgent)

	resp, err := env.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("%w: no answer within %s", errUpdateNotReached, updateCheckTimeout)
		}
		return "", fmt.Errorf("%w: %v", errUpdateNotReached, err)
	}
	// The body is never used; read a little so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return latestTagFromLocation(base, resp.Header.Get("Location"))
	case http.StatusNotFound:
		return "", errUpdateNoRelease
	case http.StatusForbidden, http.StatusTooManyRequests:
		return "", fmt.Errorf("%w (HTTP %d); try again later", errUpdateRefused, resp.StatusCode)
	default:
		return "", fmt.Errorf("%w (HTTP %d, not a redirect to a release)", errUpdateUnexpected, resp.StatusCode)
	}
}

// latestTagFromLocation validates a latest-release redirect. It accepts only
// an absolute <base>/releases/tag/vX.Y.Z on the base's scheme and host, with
// no query, fragment, or credentials, and returns the tag.
func latestTagFromLocation(base *url.URL, location string) (string, error) {
	if location == "" {
		return "", fmt.Errorf("%w: a redirect with no location", errUpdateUnexpected)
	}
	loc, err := url.Parse(location)
	if err != nil || !loc.IsAbs() || loc.Scheme != base.Scheme || loc.Host != base.Host ||
		loc.User != nil || loc.RawQuery != "" || loc.ForceQuery || loc.Fragment != "" {
		return "", fmt.Errorf("%w: a redirect to %q", errUpdateUnexpected, location)
	}
	path := loc.EscapedPath()
	if path == base.EscapedPath()+"/releases" {
		// GitHub sends the releases list when there is no latest release.
		return "", errUpdateNoRelease
	}
	tag, ok := strings.CutPrefix(path, base.EscapedPath()+"/releases/tag/")
	if !ok || !releaseTagPattern.MatchString(tag) {
		return "", fmt.Errorf("%w: a redirect to %q, which is not a vMAJOR.MINOR.PATCH release", errUpdateUnexpected, location)
	}
	if _, ok := parseReleaseVersion(tag); !ok {
		return "", fmt.Errorf("%w: release tag %q is out of range", errUpdateUnexpected, tag)
	}
	return tag, nil
}

// releaseVersion is a release's major, minor, and patch numbers.
type releaseVersion [3]uint64

func (v releaseVersion) compare(w releaseVersion) int {
	for i := range v {
		switch {
		case v[i] < w[i]:
			return -1
		case v[i] > w[i]:
			return 1
		}
	}
	return 0
}

func (v releaseVersion) String() string {
	return fmt.Sprintf("v%d.%d.%d", v[0], v[1], v[2])
}

// parseReleaseVersion parses a strict vMAJOR.MINOR.PATCH. A component too
// large to represent is not a version.
func parseReleaseVersion(s string) (releaseVersion, bool) {
	m := releaseTagPattern.FindStringSubmatch(s)
	if m == nil {
		return releaseVersion{}, false
	}
	return parseVersionParts(m[1:])
}

func parseVersionParts(parts []string) (releaseVersion, bool) {
	var v releaseVersion
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return releaseVersion{}, false
		}
		v[i] = n
	}
	return v, true
}

// versionKind classifies the running garage's version.
type versionKind int

const (
	versionIncomparable versionKind = iota // dev, a bare commit, an override, ...
	versionRelease                         // exactly vX.Y.Z
	versionSource                          // vX.Y.Z-<suffix>, a source build
)

// classifyVersion returns the kind of version and, for a release or source
// build, the release version it is or is based on.
func classifyVersion(version string) (versionKind, releaseVersion) {
	if v, ok := parseReleaseVersion(version); ok {
		return versionRelease, v
	}
	if m := sourceVersionPattern.FindStringSubmatch(version); m != nil {
		if v, ok := parseVersionParts(m[1:4]); ok {
			return versionSource, v
		}
	}
	return versionIncomparable, releaseVersion{}
}

// updateReport is the check's output for the running version and the latest
// release tag, which has already been validated.
func updateReport(version, latestTag, goos string) string {
	latest, _ := parseReleaseVersion(latestTag)
	var b strings.Builder
	fmt.Fprintf(&b, "garage version %s\n", version)
	fmt.Fprintf(&b, "Latest release: %s (%s/releases/tag/%s)\n\n", latestTag, updateRepositoryURL, latestTag)

	kind, current := classifyVersion(version)
	newer := false
	switch kind {
	case versionRelease:
		switch latest.compare(current) {
		case 1:
			newer = true
			fmt.Fprintf(&b, "A newer release is available: %s.\n", latestTag)
		case 0:
			b.WriteString("garage is up to date.\n")
		default:
			b.WriteString("This version is newer than the latest release.\n")
		}
	case versionSource:
		fmt.Fprintf(&b, "This garage was built from source, based on %s.\n", current)
		if latest.compare(current) > 0 {
			newer = true
			fmt.Fprintf(&b, "A newer release is available: %s.\n", latestTag)
		} else {
			fmt.Fprintf(&b, "No release is newer than %s.\n", current)
		}
	default:
		fmt.Fprintf(&b, "This garage's version, %s, is not a release version, so it cannot be compared with the latest release.\n", version)
	}

	if newer {
		b.WriteString("\n")
		b.WriteString(updateGuidance(latestTag, goos))
		b.WriteString("\nAfter updating, installed skills are reported Stale until you run\n" +
			"\"garage skills install\" in each project that uses them.\n")
	}
	return b.String()
}

// updateGuidance lists every documented way to update to tag on goos. It
// never guesses which one installed this garage (#125 decision 8).
func updateGuidance(tag, goos string) string {
	docs := fmt.Sprintf("%s/blob/%s/docs/release-verification.md", updateRepositoryURL, tag)
	installSh := fmt.Sprintf(
		"  With install.sh, which verifies the release first (needs cosign v3 or later):\n"+
			"    curl -fsSL https://raw.githubusercontent.com/%s/%s/install.sh | sh -s -- --version %s\n\n"+
			"  From a checkout, building from source:\n"+
			"    git fetch --tags && git checkout %s && ./install.sh\n",
		updateRepository, tag, tag, tag)

	var b strings.Builder
	b.WriteString("Update with the method you installed garage with:\n\n")
	switch goos {
	case "darwin":
		b.WriteString(installSh)
	case "linux":
		b.WriteString(installSh)
		b.WriteString("\n  With Homebrew:\n" +
			"    brew upgrade --cask bdonaldharris/tap/garage\n")
		fmt.Fprintf(&b, "\n  With a .deb or .rpm package: download and verify the %s package, then\n"+
			"  install it with apt or dnf, as described in:\n"+
			"    %s#linux-packages\n", tag, docs)
	case "windows":
		fmt.Fprintf(&b, "  Download and verify the %s archive, as described in:\n"+
			"    %s#windows-powershell\n", tag, docs)
	default:
		fmt.Fprintf(&b, "  Download and verify the %s release, as described in:\n"+
			"    %s\n", tag, docs)
	}
	return b.String()
}
