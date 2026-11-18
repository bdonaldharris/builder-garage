package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testOrigin = "https://github.com/owner/repo.git"

// historyFixture is a test repository with an isolated user cache.
type historyFixture struct {
	dir      string
	cacheDir string
	key      string
}

func newHistoryFixture(t *testing.T, origin string) historyFixture {
	t.Helper()
	cacheDir := isolateUserCache(t)
	dir := newGitRepo(t, origin)
	repo, err := identifyRepository(context.Background(), dir, runGit)
	if err != nil {
		t.Fatal(err)
	}
	return historyFixture{dir: dir, cacheDir: cacheDir, key: repo.Key}
}

func (f historyFixture) historyDir() string {
	return filepath.Join(f.cacheDir, historyAppDir, historySubDir)
}

// event returns a valid stored event line for key.
func event(key, time, kind string, number int, url string) string {
	e := historyEvent{Version: historySchemaVersion, Time: time, Repo: key, Kind: kind,
		Number: number, Type: "review", Outcome: "approved", URL: url}
	line, err := json.Marshal(e)
	if err != nil {
		panic(err)
	}
	return string(line)
}

func writeHistoryFile(t *testing.T, dir, key string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, key+historyFileExt)
	writeFile(t, path, strings.Join(lines, "\n")+"\n")
	return path
}

// snapshotCache records every entry under root, with its size, mode,
// modification time, and content digest.
func snapshotCache(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil
		}
		digest := ""
		if info.Mode().IsRegular() {
			data, _ := os.ReadFile(path)
			sum := sha256.Sum256(data)
			digest = hex.EncodeToString(sum[:])
		}
		snap[path] = fmt.Sprintf("%v %d %d %s", info.Mode(), info.Size(), info.ModTime().UnixNano(), digest)
		return nil
	})
	return snap
}

func requireUnchanged(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Errorf("the cache changed: %d entries before, %d after", len(before), len(after))
	}
	for path, b := range before {
		if a, ok := after[path]; !ok || a != b {
			t.Errorf("%s changed: %q, then %q", path, b, a)
		}
	}
}

func TestHistoryListShowsTheRepositorysEvents(t *testing.T) {
	f := newHistoryFixture(t, testOrigin)
	writeHistoryFile(t, f.historyDir(), f.key,
		event(f.key, "2026-11-13T10:00:00Z", "issue", 7, "https://github.com/owner/repo/issues/7#issuecomment-1"),
		event(f.key, "2026-11-13T11:00:00Z", "pr", 8, ""),
		event(f.key, "2026-11-14T09:00:00Z", "issue", 9, "https://github.com/owner/repo/issues/9#issuecomment-2"))

	out, _, err := runCLI(t, "dev", "history", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Local history for this repository (" + historyLabel + ")",
		"File: " + filepath.Join(f.historyDir(), f.key+historyFileExt),
		"2026-11-13T10:00:00Z  issue #7  review  approved  https://github.com/owner/repo/issues/7#issuecomment-1\n" +
			"2026-11-13T11:00:00Z  PR #8  review  approved\n" +
			"2026-11-14T09:00:00Z  issue #9  review  approved  https://github.com/owner/repo/issues/9#issuecomment-2\n",
		"3 events.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestHistoryListFilters(t *testing.T) {
	f := newHistoryFixture(t, testOrigin)
	writeHistoryFile(t, f.historyDir(), f.key,
		event(f.key, "2026-11-13T10:00:00Z", "issue", 7, ""),
		event(f.key, "2026-11-13T11:00:00Z", "pr", 7, ""),
		event(f.key, "2026-11-13T12:00:00Z", "issue", 8, ""))

	for _, tc := range []struct {
		args       []string
		want, lack []string
	}{
		{[]string{"--issue", "7"}, []string{"issue #7", "1 event."}, []string{"PR #7", "issue #8"}},
		{[]string{"--pr", "7"}, []string{"PR #7", "1 event."}, []string{"issue #7", "issue #8"}},
		{[]string{"--issue", "99"}, []string{"No local events for issue #99."}, []string{"issue #7"}},
		{[]string{"--pr", "99"}, []string{"No local events for PR #99."}, nil},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, _, err := runCLI(t, "dev", append([]string{"history", "list"}, tc.args...)...)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output lacks %q:\n%s", w, out)
				}
			}
			for _, l := range tc.lack {
				if strings.Contains(out, l) {
					t.Errorf("output contains %q:\n%s", l, out)
				}
			}
		})
	}
}

func TestHistoryListRejectsBadArguments(t *testing.T) {
	newHistoryFixture(t, testOrigin)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--issue", "1", "--pr", "2"}, "either --issue or --pr, not both"},
		{[]string{"--issue", "-3"}, "positive integer"},
		{[]string{"--format", "yaml"}, `unknown --format "yaml"`},
		{[]string{"extra"}, "unknown command"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			_, _, err := runCLI(t, "dev", append([]string{"history", "list"}, tc.args...)...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestHistoryListWithoutHistory(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		newHistoryFixture(t, testOrigin)
		out, _, err := runCLI(t, "dev", "history", "list")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "No local events for this repository yet.") {
			t.Errorf("output:\n%s", out)
		}
	})
	t.Run("empty", func(t *testing.T) {
		f := newHistoryFixture(t, testOrigin)
		writeFile(t, filepath.Join(f.historyDir(), f.key+historyFileExt), "")
		out, _, err := runCLI(t, "dev", "history", "list")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "No local events for this repository yet.") {
			t.Errorf("output:\n%s", out)
		}
	})
}

func TestHistoryListSkipsUnreadableEntries(t *testing.T) {
	f := newHistoryFixture(t, testOrigin)
	writeHistoryFile(t, f.historyDir(), f.key,
		event(f.key, "2026-11-13T10:00:00Z", "issue", 7, ""),
		`{"v":1,"time":"not a time"}`,
		`{"v":1,"time":"2026-11-13T10:00:00Z","repo":"`+f.key+`","kind":"issue","number":1,"type":"review","outcome":"approved","branch":"secret"}`,
		`{"v":1,"time":"2026-11-13T1`) // a partially written line

	out, _, err := runCLI(t, "dev", "history", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1 event.") || !strings.Contains(out, "3 unreadable entries skipped.") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "secret") || strings.Contains(out, "not a time") {
		t.Errorf("an unreadable entry was shown:\n%s", out)
	}

	jsonl, stderr, err := runCLI(t, "dev", "history", "list", "--format", "jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(jsonl, "\n") != 1 || !strings.Contains(stderr, "skipped 3 unreadable history entries") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", jsonl, stderr)
	}
}

func TestHistoryListRefusesASymbolicLink(t *testing.T) {
	f := newHistoryFixture(t, testOrigin)
	target := filepath.Join(t.TempDir(), "elsewhere.jsonl")
	writeFile(t, target, event(f.key, "2026-11-13T10:00:00Z", "issue", 7, "")+"\n")
	if err := os.MkdirAll(f.historyDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(f.historyDir(), f.key+historyFileExt)); err != nil {
		t.Skipf("cannot create a symbolic link: %v", err)
	}
	if _, _, err := runCLI(t, "dev", "history", "list"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("error = %v, want a refusal", err)
	}
}

func TestHistoryListJSONLMatchesTheStoredEvents(t *testing.T) {
	f := newHistoryFixture(t, testOrigin)
	lines := []string{
		event(f.key, "2026-11-13T10:00:00Z", "issue", 7, "https://github.com/owner/repo/issues/7"),
		event(f.key, "2026-11-13T11:00:00Z", "pr", 8, ""),
	}
	writeHistoryFile(t, f.historyDir(), f.key, lines...)

	out, _, err := runCLI(t, "dev", "history", "list", "--format", "jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Join(lines, "\n") + "\n"; out != want {
		t.Errorf("jsonl output:\n%s\nwant:\n%s", out, want)
	}
}

func TestHistoryListNeedsARepository(t *testing.T) {
	isolateUserCache(t)
	t.Chdir(t.TempDir())
	if _, _, err := runCLI(t, "dev", "history", "list"); err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("history list: error = %v", err)
	}
	if _, _, err := runCLI(t, "dev", "history", "path"); err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("history path: error = %v", err)
	}
}

func TestHistoryPath(t *testing.T) {
	f := newHistoryFixture(t, testOrigin)
	path := filepath.Join(f.historyDir(), f.key+historyFileExt)

	out, _, err := runCLI(t, "dev", "history", "path")
	if err != nil {
		t.Fatal(err)
	}
	if want := path + "\nKey: " + f.key + "\nNo events yet: the file does not exist.\n"; out != want {
		t.Errorf("without a file:\n%s\nwant:\n%s", out, want)
	}
	jsonl, _, err := runCLI(t, "dev", "history", "path", "--format", "jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf(`{"path":%q,"key":%q,"exists":false,"events":0}`+"\n", path, f.key); jsonl != want {
		t.Errorf("jsonl without a file:\n%s\nwant:\n%s", jsonl, want)
	}

	writeHistoryFile(t, f.historyDir(), f.key,
		event(f.key, "2026-11-13T10:00:00Z", "issue", 7, ""),
		event(f.key, "2026-11-13T11:00:00Z", "issue", 8, ""))
	out, _, err = runCLI(t, "dev", "history", "path")
	if err != nil {
		t.Fatal(err)
	}
	if want := path + "\nKey: " + f.key + "\nEvents: 2\n"; out != want {
		t.Errorf("with a file:\n%s\nwant:\n%s", out, want)
	}
	jsonl, _, err = runCLI(t, "dev", "history", "path", "--format", "jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf(`{"path":%q,"key":%q,"exists":true,"events":2}`+"\n", path, f.key); jsonl != want {
		t.Errorf("jsonl with a file:\n%s\nwant:\n%s", jsonl, want)
	}
}

// allLocalFixture writes three history files besides lock and temporary
// files: the current repository's, another all linking to this repository
// (as after a rename), and one with mixed and missing links.
func allLocalFixture(t *testing.T) (historyFixture, string, string) {
	t.Helper()
	f := newHistoryFixture(t, testOrigin)
	renamed := strings.Repeat("a", 32)
	other := strings.Repeat("b", 32)
	if renamed > f.key || other < f.key {
		// Keep a predictable order: renamed, current, other.
		renamed, other = strings.Repeat("0", 32), strings.Repeat("f", 32)
	}
	writeHistoryFile(t, f.historyDir(), f.key,
		event(f.key, "2026-11-17T10:00:00Z", "issue", 5, "https://github.com/owner/repo/issues/5#issuecomment-9"))
	writeHistoryFile(t, f.historyDir(), renamed,
		event(renamed, "2026-11-13T10:00:00Z", "issue", 5, "https://github.com/Owner/Repo/issues/5#issuecomment-1"),
		event(renamed, "2026-11-14T10:00:00Z", "pr", 6, "https://github.com/owner/repo/pull/6"),
		event(renamed, "2026-11-15T10:00:00Z", "issue", 7, ""))
	writeHistoryFile(t, f.historyDir(), other,
		event(other, "2026-11-12T10:00:00Z", "issue", 5, "https://github.com/owner/repo/issues/5"),
		event(other, "2026-11-12T11:00:00Z", "issue", 1, "https://github.com/someone/else/issues/1"))
	writeFile(t, filepath.Join(f.historyDir(), f.key+historyFileExt+".lock"), "")
	writeFile(t, filepath.Join(f.historyDir(), ".history-123.tmp"), "junk\n")
	writeFile(t, filepath.Join(f.historyDir(), "notes.jsonl"), event(other, "2026-11-12T10:00:00Z", "issue", 99, "")+"\n")
	return f, renamed, other
}

func TestHistoryListAllLocal(t *testing.T) {
	f, renamed, other := allLocalFixture(t)

	out, _, err := runCLI(t, "dev", "history", "list", "--all-local")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Local history files (" + historyLabel + ")",
		"Directory: " + f.historyDir(),
		"\n" + f.key + "  (current; events link to this repository)\n",
		"\n" + renamed + "  (events link to this repository)\n" +
			"  File: " + filepath.Join(f.historyDir(), renamed+historyFileExt) + "\n" +
			"  Events: 3, 2026-11-13T10:00:00Z to 2026-11-15T10:00:00Z\n" +
			"  Links: github.com/owner/repo (2)\n" +
			"  Without a link: 1\n" +
			"  2026-11-13T10:00:00Z  issue #5  review  approved  https://github.com/Owner/Repo/issues/5#issuecomment-1\n",
		"\n" + other + "\n" +
			"  File: " + filepath.Join(f.historyDir(), other+historyFileExt) + "\n" +
			"  Events: 2, 2026-11-12T10:00:00Z to 2026-11-12T11:00:00Z\n" +
			"  Links: github.com/owner/repo (1), github.com/someone/else (1)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, lack := range []string{".lock", ".tmp", "notes.jsonl", "issue #99"} {
		if strings.Contains(out, lack) {
			t.Errorf("output contains %q, which is not a history file:\n%s", lack, out)
		}
	}
	// Each file's events stay under its own summary, in key order: a header
	// section, then one section per file holding exactly its own events.
	sections := strings.Split(out, "\n\n")
	if len(sections) != 4 {
		t.Fatalf("want a header and three file sections, got %d:\n%s", len(sections), out)
	}
	for key, want := range map[string]int{renamed: 3, f.key: 1, other: 2} {
		for _, s := range sections[1:] {
			if strings.HasPrefix(s, key) {
				if got := strings.Count(s, "\n  2026-"); got != want {
					t.Errorf("file %s shows %d events under it, want %d:\n%s", key, got, want, s)
				}
			}
		}
	}
	if strings.Index(out, renamed) > strings.Index(out, f.key) || strings.Index(out, f.key) > strings.Index(out, other) {
		t.Errorf("files are not in key order:\n%s", out)
	}
}

func TestHistoryListAllLocalFilters(t *testing.T) {
	_, renamed, _ := allLocalFixture(t)
	out, _, err := runCLI(t, "dev", "history", "list", "--all-local", "--issue", "5")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "  issue #5  "); n != 3 {
		t.Errorf("issue #5 events = %d, want one from each of three files:\n%s", n, out)
	}
	for _, lack := range []string{"PR #6", "issue #7", "issue #1 "} {
		if strings.Contains(out, lack) {
			t.Errorf("filtered output contains %q:\n%s", lack, out)
		}
	}
	// Summaries still describe the whole file.
	if !strings.Contains(out, "\n"+renamed+"  (events link to this repository)\n") || !strings.Contains(out, "Events: 3,") {
		t.Errorf("the summary changed under a filter:\n%s", out)
	}
}

func TestHistoryListAllLocalJSONL(t *testing.T) {
	f, renamed, other := allLocalFixture(t)
	out, _, err := runCLI(t, "dev", "history", "list", "--all-local", "--format", "jsonl")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	var order []string
	var summaries = map[string]historyFileSummary{}
	currentFile := ""
	for _, line := range lines {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Fatalf("not JSON: %q", line)
		}
		if raw, ok := probe["file"]; ok {
			var s historyFileSummary
			if err := json.Unmarshal(raw, &s); err != nil {
				t.Fatal(err)
			}
			summaries[s.Key] = s
			order = append(order, s.Key)
			currentFile = s.Key
			continue
		}
		e, ok := decodeHistoryLine([]byte(line))
		if !ok {
			t.Fatalf("an event line is not a stored event: %q", line)
		}
		if e.Repo != currentFile {
			t.Errorf("event of %s appears under file %s: histories were interleaved", e.Repo, currentFile)
		}
	}
	if strings.Join(order, ",") != strings.Join([]string{renamed, f.key, other}, ",") {
		t.Errorf("file order = %v", order)
	}
	if s := summaries[f.key]; !s.Current || !s.ThisRepository || s.Events != 1 {
		t.Errorf("current summary = %+v", s)
	}
	if s := summaries[renamed]; s.Current || !s.ThisRepository || s.Events != 3 || s.Unlinked != 1 ||
		s.Links["github.com/owner/repo"] != 2 || s.First != "2026-11-13T10:00:00Z" || s.Last != "2026-11-15T10:00:00Z" {
		t.Errorf("renamed summary = %+v", s)
	}
	if s := summaries[other]; s.ThisRepository || len(s.Links) != 2 {
		t.Errorf("mixed summary = %+v", s)
	}
}

func TestHistoryListAllLocalMarks(t *testing.T) {
	t.Run("no links is not marked", func(t *testing.T) {
		f := newHistoryFixture(t, testOrigin)
		key := strings.Repeat("c", 32)
		writeHistoryFile(t, f.historyDir(), key, event(key, "2026-11-13T10:00:00Z", "issue", 1, ""))
		out, _, err := runCLI(t, "dev", "history", "list", "--all-local")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "\n"+key+"\n") {
			t.Errorf("a file with no links was marked:\n%s", out)
		}
	})
	t.Run("a checkout without origin marks nothing", func(t *testing.T) {
		f := newHistoryFixture(t, "")
		key := strings.Repeat("c", 32)
		writeHistoryFile(t, f.historyDir(), key, event(key, "2026-11-13T10:00:00Z", "issue", 1, "https://github.com/owner/repo/issues/1"))
		out, _, err := runCLI(t, "dev", "history", "list", "--all-local")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "events link to this repository") {
			t.Errorf("marked without an origin to compare with:\n%s", out)
		}
	})
	t.Run("outside a repository", func(t *testing.T) {
		cacheDir := isolateUserCache(t)
		t.Chdir(t.TempDir())
		dir := filepath.Join(cacheDir, historyAppDir, historySubDir)
		key := strings.Repeat("c", 32)
		writeHistoryFile(t, dir, key, event(key, "2026-11-13T10:00:00Z", "issue", 1, "https://github.com/owner/repo/issues/1"))
		out, _, err := runCLI(t, "dev", "history", "list", "--all-local")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "\n"+key+"\n") || strings.Contains(out, "current") {
			t.Errorf("outside a repository:\n%s", out)
		}
	})
	t.Run("no history directory", func(t *testing.T) {
		newHistoryFixture(t, testOrigin)
		out, _, err := runCLI(t, "dev", "history", "list", "--all-local")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "No local history files.") {
			t.Errorf("output:\n%s", out)
		}
	})
}

func TestHistoryCommandsAreReadOnly(t *testing.T) {
	f, _, _ := allLocalFixture(t)
	// Also a repository with no history yet: reading must not create it.
	fresh := newGitRepo(t, "https://github.com/owner/fresh.git")
	t.Chdir(f.dir)

	before := snapshotCache(t, f.cacheDir)
	for _, args := range [][]string{
		{"history", "list"},
		{"history", "list", "--issue", "5"},
		{"history", "list", "--format", "jsonl"},
		{"history", "list", "--all-local"},
		{"history", "list", "--all-local", "--format", "jsonl"},
		{"history", "path"},
		{"history", "path", "--format", "jsonl"},
	} {
		if _, _, err := runCLI(t, "dev", args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	t.Chdir(fresh)
	for _, args := range [][]string{{"history", "list"}, {"history", "path"}} {
		if _, _, err := runCLI(t, "dev", args...); err != nil {
			t.Fatalf("%v in a repository without history: %v", args, err)
		}
	}
	requireUnchanged(t, before, snapshotCache(t, f.cacheDir))
}

func TestLinkedRepository(t *testing.T) {
	for _, tc := range []struct {
		url, want string
		ok        bool
	}{
		{"https://github.com/Owner/Repo/issues/5#issuecomment-1", "github.com/owner/repo", true},
		{"https://github.com:443/owner/repo/pull/6", "github.com/owner/repo", true},
		{"https://github.com/owner", "", false},
		{"https://github.com/", "", false},
		{"", "", false},
		{"not a url", "", false},
	} {
		got, ok := linkedRepository(tc.url)
		if got != tc.want || ok != tc.ok {
			t.Errorf("linkedRepository(%q) = %q, %v; want %q, %v", tc.url, got, ok, tc.want, tc.ok)
		}
	}
}

func TestIdentifyRepositoryReportsTheNormalizedRemote(t *testing.T) {
	dir := newGitRepo(t, "git@GitHub.com:Owner/Repo.git")
	repo, err := identifyRepository(context.Background(), dir, runGit)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Remote != "github.com/owner/repo" {
		t.Errorf("Remote = %q", repo.Remote)
	}
	local := newGitRepo(t, "")
	repo, err = identifyRepository(context.Background(), local, runGit)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Remote != "" {
		t.Errorf("Remote without origin = %q, want empty", repo.Remote)
	}
}
