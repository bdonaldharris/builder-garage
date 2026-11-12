package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// requireGit skips the test when git is not installed.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newGitRepo creates a repository with one commit and makes it the working
// directory for the test.
func newGitRepo(t *testing.T, origin string) string {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	if origin != "" {
		gitIn(t, dir, "remote", "add", "origin", origin)
	}
	writeFile(t, filepath.Join(dir, "README.md"), "test\n")
	gitIn(t, dir, "add", "README.md")
	gitIn(t, dir, "commit", "-q", "-m", "initial")
	t.Chdir(dir)
	return dir
}

// isolateUserCache points os.UserCacheDir at a fresh temporary directory on
// every platform, so tests never touch the developer's real cache.
func isolateUserCache(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg-cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "AppData", "Local"))
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("os.UserCacheDir() = %v", err)
	}
	return cacheDir
}

func findHistoryFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, historyFileExt) {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func TestRecordCommandStoresEventOutsideRepository(t *testing.T) {
	repo := newGitRepo(t, "https://github.com/Owner/Repo.git")
	cacheDir := isolateUserCache(t)
	before := gitIn(t, repo, "status", "--porcelain", "--ignored")

	stdout, stderr, err := runCLI(t, "dev", "record", "--issue", "39", "--type", "implementation",
		"--outcome", "ready", "--url", "https://github.com/owner/repo/issues/39#issuecomment-1")
	if err != nil {
		t.Fatalf("garage record: %v", err)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, "Recorded implementation (ready) for issue #39") {
		t.Fatalf("stdout = %q", stdout)
	}

	if after := gitIn(t, repo, "status", "--porcelain", "--ignored"); after != before {
		t.Fatalf("the repository changed:\nbefore: %q\nafter:  %q", before, after)
	}
	if files := findHistoryFiles(t, repo); len(files) != 0 {
		t.Fatalf("history written inside the repository: %v", files)
	}
	files := findHistoryFiles(t, cacheDir)
	if len(files) != 1 {
		t.Fatalf("history files in the user cache = %v, want exactly one", files)
	}
	if !strings.Contains(readFile(t, files[0]), `"kind":"issue","number":39,"type":"implementation","outcome":"ready"`) {
		t.Fatalf("stored event = %s", readFile(t, files[0]))
	}
}

func TestRecordCommandWarnsAndSucceedsWhenRecordingFails(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
		args  []string
		want  string
	}{
		{
			name:  "unwritable cache",
			setup: func(t *testing.T) { writeFile(t, filepath.Join(isolateUserCache(t), historyAppDir), "") },
			args:  []string{"--issue", "39", "--type", "review", "--outcome", "approved"},
			want:  "cannot create the history directory",
		},
		{
			name:  "disallowed event type",
			setup: func(t *testing.T) { isolateUserCache(t) },
			args:  []string{"--issue", "39", "--type", "my prompt text", "--outcome", "ready"},
			want:  "event type",
		},
		{
			name:  "url carrying a token",
			setup: func(t *testing.T) { isolateUserCache(t) },
			args:  []string{"--pr", "36", "--type", "finalization", "--outcome", "merged", "--url", "https://github.com/o/r/pull/36?token=x"},
			want:  "credentials or a query string",
		},
		{
			name:  "both issue and pull request",
			setup: func(t *testing.T) { isolateUserCache(t) },
			args:  []string{"--issue", "1", "--pr", "2", "--type", "review", "--outcome", "approved"},
			want:  "not both",
		},
		{
			name:  "no target",
			setup: func(t *testing.T) { isolateUserCache(t) },
			args:  []string{"--type", "review", "--outcome", "approved"},
			want:  "give --issue or --pr",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newGitRepo(t, "git@github.com:owner/repo.git")
			tt.setup(t)
			stdout, stderr, err := runCLI(t, "dev", append([]string{"record"}, tt.args...)...)
			if err != nil {
				t.Fatalf("garage record returned %v, want success", err)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if !strings.Contains(stderr, "garage: warning: event not recorded:") || !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want a warning containing %q", stderr, tt.want)
			}
		})
	}
}

func TestRecordCommandOutsideRepositoryWarns(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	cacheDir := isolateUserCache(t)

	_, stderr, err := runCLI(t, "dev", "record", "--issue", "1", "--type", "discovery", "--outcome", "complete")
	if err != nil {
		t.Fatalf("garage record returned %v, want success", err)
	}
	if !strings.Contains(stderr, errNotRepository.Error()) {
		t.Fatalf("stderr = %q, want %q", stderr, errNotRepository)
	}
	if files := findHistoryFiles(t, cacheDir); len(files) != 0 {
		t.Fatalf("history written outside a repository: %v", files)
	}
}

func TestRecordEventRefusesCacheInsideRepository(t *testing.T) {
	repo := newGitRepo(t, "")
	env := recordEnv{
		WorkDir:  repo,
		CacheDir: func() (string, error) { return filepath.Join(repo, ".cache"), nil },
		Git:      runGit,
		Now:      time.Now,
	}
	_, err := recordEvent(context.Background(),
		recordOptions{Issue: 1, Type: "review", Outcome: "approved"}, env)
	if err == nil || !strings.Contains(err.Error(), "inside the repository") {
		t.Fatalf("recordEvent() = %v, want a refusal to write inside the repository", err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".cache")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a cache directory was created inside the repository")
	}
}

func TestRecordEventUsesInjectedClock(t *testing.T) {
	repo := newGitRepo(t, "")
	cacheDir := t.TempDir()
	fixed := time.Date(2026, 11, 12, 21, 45, 11, 0, time.FixedZone("x", 3600))
	env := recordEnv{
		WorkDir:  repo,
		CacheDir: func() (string, error) { return cacheDir, nil },
		Git:      runGit,
		Now:      func() time.Time { return fixed },
	}
	result, err := recordEvent(context.Background(),
		recordOptions{PR: 36, Type: "finalization", Outcome: "merged"}, env)
	if err != nil {
		t.Fatalf("recordEvent() = %v", err)
	}
	if !strings.Contains(readFile(t, result.Path), `"time":"2026-11-12T20:45:11Z"`) {
		t.Fatalf("stored event does not carry the UTC timestamp: %s", readFile(t, result.Path))
	}
}

func TestNormalizeRemoteURL(t *testing.T) {
	const want = "github.com/owner/repo"
	for _, remote := range []string{
		"https://github.com/owner/repo.git",
		"https://github.com/Owner/Repo",
		"https://github.com/owner/repo/",
		"https://user:secret-token@github.com/owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
		"ssh://git@github.com:22/owner/repo.git",
		"git@github.com:owner/repo.git",
		"github.com:owner/repo",
		"  git@GitHub.com:owner/repo.git\n",
	} {
		if got := normalizeRemoteURL(remote); got != want {
			t.Errorf("normalizeRemoteURL(%q) = %q, want %q", remote, got, want)
		}
	}
	if got := normalizeRemoteURL(""); got != "" {
		t.Errorf("normalizeRemoteURL(\"\") = %q, want empty", got)
	}
	if got := normalizeRemoteURL("https://github.com/owner/repo"); strings.Contains(got, "secret") {
		t.Errorf("credentials leaked into the identity: %q", got)
	}
}

func TestIdentifyRepositoryKeyIsStableAcrossRemoteForms(t *testing.T) {
	fakeGit := func(remote string) gitRunner {
		return func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if slices.Contains(args, "rev-parse") {
				return []byte("/work/repo\n/work/repo/.git\n"), nil
			}
			return []byte(remote + "\n"), nil
		}
	}
	https, err := identifyRepository(context.Background(), "/work/repo", fakeGit("https://github.com/owner/repo.git"))
	if err != nil {
		t.Fatal(err)
	}
	ssh, err := identifyRepository(context.Background(), "/work/repo", fakeGit("git@github.com:owner/repo.git"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := identifyRepository(context.Background(), "/work/repo", fakeGit("git@github.com:owner/other.git"))
	if err != nil {
		t.Fatal(err)
	}
	if https.Key != ssh.Key {
		t.Errorf("HTTPS and SSH forms of one remote gave different keys: %s, %s", https.Key, ssh.Key)
	}
	if https.Key == other.Key {
		t.Errorf("different repositories gave the same key %s", https.Key)
	}
	if !repoKeyPattern.MatchString(https.Key) {
		t.Errorf("key %q is not 32 lower-case hex digits", https.Key)
	}
}

func TestIdentifyRepositorySharedByWorktrees(t *testing.T) {
	for _, origin := range []string{"", "https://github.com/owner/repo.git"} {
		t.Run("origin="+origin, func(t *testing.T) {
			repo := newGitRepo(t, origin)
			worktree := filepath.Join(t.TempDir(), "wt")
			gitIn(t, repo, "worktree", "add", "-q", worktree)

			main, err := identifyRepository(context.Background(), repo, runGit)
			if err != nil {
				t.Fatal(err)
			}
			linked, err := identifyRepository(context.Background(), worktree, runGit)
			if err != nil {
				t.Fatal(err)
			}
			if main.Key != linked.Key {
				t.Fatalf("worktree key %s differs from repository key %s", linked.Key, main.Key)
			}
			if main.TopLevel == linked.TopLevel {
				t.Fatalf("worktree top level should differ from the main working tree")
			}
		})
	}
}

func TestIdentifyRepositoryFallsBackWithoutOrigin(t *testing.T) {
	a := newGitRepo(t, "")
	b := newGitRepo(t, "")
	keyA, err := identifyRepository(context.Background(), a, runGit)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := identifyRepository(context.Background(), b, runGit)
	if err != nil {
		t.Fatal(err)
	}
	if keyA.Key == keyB.Key {
		t.Fatalf("two unrelated repositories without origin share key %s", keyA.Key)
	}
}

func TestRecordHelpDescribesPrivacyAndAuthority(t *testing.T) {
	stdout, _, err := runCLI(t, "dev", "record", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"garage record [flags]", "--issue", "--pr", "--type", "--outcome", "--url", "--artifact",
		"user cache", "never written inside the repository", "not workflow evidence",
		"At most 500 events", "exits successfully",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("record help does not contain %q:\n%s", want, stdout)
		}
	}
}
