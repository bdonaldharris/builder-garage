package cli

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeReleases serves a repository at <server>/o/r whose /releases/latest
// answers with the given status and Location, and counts every request.
type fakeReleases struct {
	server   *httptest.Server
	requests atomic.Int32
	paths    chan string
}

func newFakeReleases(t *testing.T, status int, location func(base string) string) *fakeReleases {
	t.Helper()
	f := &fakeReleases{paths: make(chan string, 16)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		f.paths <- r.Method + " " + r.URL.Path + " " + r.Header.Get("User-Agent")
		if location != nil {
			w.Header().Set("Location", location(f.base()))
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeReleases) base() string { return f.server.URL + "/o/r" }

func (f *fakeReleases) env(goos string) updateEnv {
	return updateEnv{client: newUpdateClient(), baseURL: f.base(), goos: goos}
}

func redirectTo(suffix string) func(string) string {
	return func(base string) string { return base + suffix }
}

func checkUpdate(t *testing.T, env updateEnv, version string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runUpdateCheck(context.Background(), env, version, &out)
	return out.String(), err
}

func TestUpdateCheckSendsOneRequestAndNeverFollowsTheRedirect(t *testing.T) {
	f := newFakeReleases(t, http.StatusFound, redirectTo("/releases/tag/v0.2.2"))
	if _, err := checkUpdate(t, f.env("linux"), "v0.2.2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := f.requests.Load(); n != 1 {
		t.Fatalf("requests = %d, want exactly 1 (the redirect must not be followed)", n)
	}
	if got, want := <-f.paths, "GET /o/r/releases/latest garage"; got != want {
		t.Errorf("request = %q, want %q", got, want)
	}
}

func TestUpdateCheckOutcomes(t *testing.T) {
	const latest = "v0.2.2"
	for _, tc := range []struct {
		name, version string
		want          []string
		newer         bool
	}{
		{"older release", "v0.2.1", []string{"A newer release is available: v0.2.2."}, true},
		{"numeric, not textual, ordering", "v0.10.0", []string{"This version is newer than the latest release."}, false},
		{"same release", "v0.2.2", []string{"garage is up to date."}, false},
		{"newer release", "v0.3.0", []string{"This version is newer than the latest release."}, false},
		{"source build after an older tag", "v0.2.1-4-gabc1234",
			[]string{"built from source, based on v0.2.1.", "A newer release is available: v0.2.2."}, true},
		{"source build after the latest tag", "v0.2.2-3-gabc1234",
			[]string{"built from source, based on v0.2.2.", "No release is newer than v0.2.2."}, false},
		{"dirty source build at the latest tag", "v0.2.2-dirty",
			[]string{"built from source, based on v0.2.2.", "No release is newer than v0.2.2."}, false},
		{"development build", "dev", []string{"version, dev, is not a release version"}, false},
		{"abbreviated commit", "abc1234", []string{"version, abc1234, is not a release version"}, false},
		{"override without v", "1.2.3", []string{"version, 1.2.3, is not a release version"}, false},
		{"leading zero", "v0.02.1", []string{"is not a release version"}, false},
		{"component too large", "v99999999999999999999.0.0", []string{"is not a release version"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeReleases(t, http.StatusFound, redirectTo("/releases/tag/"+latest))
			out, err := checkUpdate(t, f.env("linux"), tc.version)
			if err != nil {
				t.Fatalf("a completed check must not fail: %v", err)
			}
			for _, want := range append([]string{
				"garage version " + tc.version + "\n",
				"Latest release: v0.2.2 (https://github.com/bdonaldharris/builder-garage/releases/tag/v0.2.2)",
			}, tc.want...) {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			for _, part := range []string{"Update with the method you installed garage with:", `"garage skills install" in each project`} {
				if strings.Contains(out, part) != tc.newer {
					t.Errorf("output contains %q = %v, want %v:\n%s", part, !tc.newer, tc.newer, out)
				}
			}
		})
	}
}

func TestUpdateGuidanceListsEveryChannelForTheOS(t *testing.T) {
	installSh := "curl -fsSL https://raw.githubusercontent.com/bdonaldharris/builder-garage/v0.2.2/install.sh | sh -s -- --version v0.2.2"
	checkout := "git fetch --tags && git checkout v0.2.2 && ./install.sh"
	brew := "brew upgrade --cask bdonaldharris/tap/garage"
	packages := "https://github.com/bdonaldharris/builder-garage/blob/v0.2.2/docs/release-verification.md#linux-packages"
	windows := "https://github.com/bdonaldharris/builder-garage/blob/v0.2.2/docs/release-verification.md#windows-powershell"
	npm := "npm install -g @buildergarage/garage@0.2.2"
	all := []string{installSh, checkout, brew, packages, windows, npm}

	for _, tc := range []struct {
		goos string
		want []string
	}{
		{"linux", []string{installSh, checkout, npm, brew, packages}},
		{"darwin", []string{installSh, checkout, npm}},
		{"windows", []string{windows}},
		{"freebsd", []string{"https://github.com/bdonaldharris/builder-garage/blob/v0.2.2/docs/release-verification.md\n"}},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			got := updateGuidance("v0.2.2", tc.goos)
			for _, s := range all {
				want := false
				for _, w := range tc.want {
					want = want || w == s
				}
				if strings.Contains(got, s) != want {
					t.Errorf("guidance contains %q = %v, want %v:\n%s", s, !want, want, got)
				}
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("guidance lacks %q:\n%s", w, got)
				}
			}
		})
	}
}

func TestUpdateCheckRefusesUnexpectedAnswers(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		location func(string) string
		want     error
	}{
		{"another host", http.StatusFound, func(string) string { return "https://example.com/o/r/releases/tag/v9.9.9" }, errUpdateUnexpected},
		{"another scheme", http.StatusFound, func(b string) string { return strings.Replace(b, "http://", "https://", 1) + "/releases/tag/v9.9.9" }, errUpdateUnexpected},
		{"another repository", http.StatusFound, func(b string) string { return strings.TrimSuffix(b, "/o/r") + "/o/other/releases/tag/v9.9.9" }, errUpdateUnexpected},
		{"a relative location", http.StatusFound, func(string) string { return "/o/r/releases/tag/v9.9.9" }, errUpdateUnexpected},
		{"a pre-release tag", http.StatusFound, redirectTo("/releases/tag/v0.3.0-rc.1"), errUpdateUnexpected},
		{"a tag without v", http.StatusFound, redirectTo("/releases/tag/0.3.0"), errUpdateUnexpected},
		{"an extra path segment", http.StatusFound, redirectTo("/releases/tag/v0.3.0/x"), errUpdateUnexpected},
		{"a query", http.StatusFound, redirectTo("/releases/tag/v0.3.0?x=1"), errUpdateUnexpected},
		{"a fragment", http.StatusFound, redirectTo("/releases/tag/v0.3.0#x"), errUpdateUnexpected},
		{"credentials", http.StatusFound, func(b string) string { return strings.Replace(b, "http://", "http://u:p@", 1) + "/releases/tag/v0.3.0" }, errUpdateUnexpected},
		{"a tag out of range", http.StatusFound, redirectTo("/releases/tag/v99999999999999999999.0.0"), errUpdateUnexpected},
		{"no location", http.StatusFound, nil, errUpdateUnexpected},
		{"not a redirect", http.StatusOK, nil, errUpdateUnexpected},
		{"a server error", http.StatusInternalServerError, nil, errUpdateUnexpected},
		{"no release", http.StatusNotFound, nil, errUpdateNoRelease},
		{"the releases list", http.StatusFound, redirectTo("/releases"), errUpdateNoRelease},
		{"forbidden", http.StatusForbidden, nil, errUpdateRefused},
		{"rate limited", http.StatusTooManyRequests, nil, errUpdateRefused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeReleases(t, tc.status, tc.location)
			out, err := checkUpdate(t, f.env("linux"), "v0.2.1")
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), "could not check for a newer release: ") {
				t.Errorf("error %q does not say the check could not complete", err)
			}
			if out != "" {
				t.Errorf("a failed check wrote output:\n%s", out)
			}
			if n := f.requests.Load(); n != 1 {
				t.Errorf("requests = %d, want 1 (no retries, no redirect followed)", n)
			}
		})
	}
}

func TestUpdateCheckReportsAnUnreachableGitHub(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String() + "/o/r"
	ln.Close() // nothing listens there now

	_, err = checkUpdate(t, updateEnv{client: newUpdateClient(), baseURL: base, goos: "linux"}, "v0.2.1")
	if !errors.Is(err, errUpdateNotReached) {
		t.Fatalf("error = %v, want %v", err, errUpdateNotReached)
	}
}

func TestUpdateCheckHonorsCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	var out bytes.Buffer
	err := runUpdateCheck(ctx, updateEnv{client: newUpdateClient(), baseURL: server.URL + "/o/r", goos: "linux"}, "v0.2.1", &out)
	if !errors.Is(err, errUpdateNotReached) {
		t.Fatalf("error = %v, want %v", err, errUpdateNotReached)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("the check took %s after its context ended", elapsed)
	}
}

func TestUpdateCommandHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "dev", "update")
	if err != nil {
		t.Fatalf("garage update: %v", err)
	}
	for _, want := range []string{"check ", "It contacts GitHub only when you run \"garage update check\"."} {
		if !strings.Contains(stdout, want) {
			t.Errorf("garage update output lacks %q:\n%s", want, stdout)
		}
	}
	if _, _, err := runCLI(t, "dev", "update", "check", "extra"); err == nil {
		t.Error("garage update check accepted an argument")
	}
}
