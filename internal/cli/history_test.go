package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRepoKey = "0123456789abcdef0123456789abcdef"

func validEvent() historyEvent {
	return historyEvent{
		Version: historySchemaVersion,
		Time:    "2026-11-12T21:45:11Z",
		Repo:    testRepoKey,
		Kind:    "issue",
		Number:  39,
		Type:    "implementation",
		Outcome: "ready",
		URL:     "https://github.com/owner/repo/issues/39#issuecomment-1",
	}
}

func historyLines(t *testing.T, path string) []string {
	t.Helper()
	content := strings.TrimRight(readFile(t, path), "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func TestHistoryEventValidation(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*historyEvent)
	}{
		{"unknown event type", func(e *historyEvent) { e.Type = "deploy" }},
		{"free-text event type", func(e *historyEvent) { e.Type = "please review my prompt" }},
		{"unknown outcome", func(e *historyEvent) { e.Outcome = "shipped" }},
		{"unknown target kind", func(e *historyEvent) { e.Kind = "discussion" }},
		{"zero number", func(e *historyEvent) { e.Number = 0 }},
		{"negative number", func(e *historyEvent) { e.Number = -4 }},
		{"http url", func(e *historyEvent) { e.URL = "http://github.com/owner/repo/issues/39" }},
		{"relative url", func(e *historyEvent) { e.URL = "/owner/repo/issues/39" }},
		{"url with credentials", func(e *historyEvent) { e.URL = "https://user:token@github.com/owner/repo" }},
		{"url with query", func(e *historyEvent) { e.URL = "https://github.com/owner/repo?token=abc" }},
		{"url with whitespace", func(e *historyEvent) { e.URL = "https://github.com/a b" }},
		{"overlong url", func(e *historyEvent) { e.URL = "https://github.com/" + strings.Repeat("a", maxURLLength) }},
		{"artifact with a diff", func(e *historyEvent) { e.Artifact = "diff --git a/x b/x\n+secret" }},
		{"artifact with spaces", func(e *historyEvent) { e.Artifact = "two words" }},
		{"overlong artifact", func(e *historyEvent) { e.Artifact = strings.Repeat("a", maxArtifactLen+1) }},
		{"malformed repository key", func(e *historyEvent) { e.Repo = "not-a-key" }},
		{"malformed time", func(e *historyEvent) { e.Time = "yesterday" }},
		{"wrong version", func(e *historyEvent) { e.Version = 2 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent()
			tt.modify(&event)
			err := event.validate()
			if !errors.Is(err, errHistoryInput) {
				t.Fatalf("validate() = %v, want an error wrapping errHistoryInput", err)
			}
		})
	}

	t.Run("valid event with optional fields", func(t *testing.T) {
		event := validEvent()
		event.Artifact = "sha256:7794ea3eac68adcd36b1a14b6eb47c6878c582f929546d832a6193148e308e16"
		if err := event.validate(); err != nil {
			t.Fatalf("validate() = %v, want nil", err)
		}
		event.URL, event.Artifact = "", ""
		if err := event.validate(); err != nil {
			t.Fatalf("validate() without optional fields = %v, want nil", err)
		}
	})
}

func TestAppendHistoryRejectsInvalidEventWithoutWriting(t *testing.T) {
	cacheDir := t.TempDir()
	event := validEvent()
	event.Type = "anything"

	if _, err := appendHistoryEvent(cacheDir, event); !errors.Is(err, errHistoryInput) {
		t.Fatalf("appendHistoryEvent() = %v, want errHistoryInput", err)
	}
	if names := entryNames(t, cacheDir); len(names) != 0 {
		t.Fatalf("cache directory contains %v after a rejected event, want nothing", names)
	}
}

func TestAppendHistoryStoresOnlyPermittedFields(t *testing.T) {
	cacheDir := t.TempDir()
	result, err := appendHistoryEvent(cacheDir, validEvent())
	if err != nil {
		t.Fatalf("appendHistoryEvent() = %v", err)
	}

	lines := historyLines(t, result.Path)
	if len(lines) != 1 {
		t.Fatalf("history has %d lines, want 1", len(lines))
	}
	want := `{"v":1,"time":"2026-11-12T21:45:11Z","repo":"` + testRepoKey +
		`","kind":"issue","number":39,"type":"implementation","outcome":"ready",` +
		`"url":"https://github.com/owner/repo/issues/39#issuecomment-1"}`
	if lines[0] != want {
		t.Fatalf("stored line =\n%s\nwant\n%s", lines[0], want)
	}

	if !strings.HasPrefix(result.Path, filepath.Join(cacheDir, historyAppDir, historySubDir)) {
		t.Fatalf("history path %s is not under the cache directory", result.Path)
	}
}

func TestAppendHistoryEnforcesEventLimit(t *testing.T) {
	cacheDir := t.TempDir()
	extra := 7
	var result historyAppendResult
	for i := 1; i <= historyEventLimit+extra; i++ {
		event := validEvent()
		event.Number = i
		var err error
		result, err = appendHistoryEvent(cacheDir, event)
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	if result.Kept != historyEventLimit || result.Trimmed != 1 {
		t.Fatalf("last append kept %d and trimmed %d, want %d and 1", result.Kept, result.Trimmed, historyEventLimit)
	}
	lines := historyLines(t, result.Path)
	if len(lines) != historyEventLimit {
		t.Fatalf("history has %d events, want %d", len(lines), historyEventLimit)
	}
	if !strings.Contains(lines[0], fmt.Sprintf(`"number":%d,`, extra+1)) {
		t.Fatalf("oldest kept event is %s, want number %d", lines[0], extra+1)
	}
	if !strings.Contains(lines[len(lines)-1], fmt.Sprintf(`"number":%d,`, historyEventLimit+extra)) {
		t.Fatalf("newest event is %s, want number %d", lines[len(lines)-1], historyEventLimit+extra)
	}
}

func TestAppendHistoryDiscardsCorruptEntries(t *testing.T) {
	cacheDir := t.TempDir()
	path := historyPath(cacheDir, testRepoKey)

	first, err := appendHistoryEvent(cacheDir, validEvent())
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	valid := historyLines(t, first.Path)[0]

	withBranch := strings.Replace(valid, `"kind"`, `"branch":"feat/secret","kind"`, 1)
	corrupt := strings.Join([]string{
		"not json at all",
		valid,
		`{"v":1,"time":"2026-11-12T21:45:11Z"`,
		withBranch,
		`{"v":1,"time":"2026-11-12T21:45:11Z","repo":"` + testRepoKey + `","kind":"issue","number":1,"type":"prompt","outcome":"ready"}`,
		valid + " trailing",
	}, "\n") + "\n"
	writeFile(t, path, corrupt)

	event := validEvent()
	event.Number = 40
	result, err := appendHistoryEvent(cacheDir, event)
	if err != nil {
		t.Fatalf("append over corrupt history: %v", err)
	}
	if result.Discarded != 5 || result.Kept != 2 {
		t.Fatalf("discarded %d and kept %d, want 5 and 2", result.Discarded, result.Kept)
	}

	lines := historyLines(t, path)
	if len(lines) != 2 || lines[0] != valid || !strings.Contains(lines[1], `"number":40,`) {
		t.Fatalf("history after recovery =\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(readFile(t, path), "branch") {
		t.Fatalf("a disallowed field survived recovery")
	}
}

func TestAppendHistoryDiscardsOversizedFile(t *testing.T) {
	cacheDir := t.TempDir()
	path := historyPath(cacheDir, testRepoKey)
	writeFile(t, path, strings.Repeat("x", historyMaxFileBytes+10))

	result, err := appendHistoryEvent(cacheDir, validEvent())
	if err != nil {
		t.Fatalf("append over oversized history: %v", err)
	}
	if result.Kept != 1 || result.Discarded == 0 {
		t.Fatalf("kept %d and discarded %d, want 1 and more than 0", result.Kept, result.Discarded)
	}
	if lines := historyLines(t, path); len(lines) != 1 {
		t.Fatalf("history has %d lines, want 1", len(lines))
	}
}

func TestAppendHistoryRefusesSymlinkedFile(t *testing.T) {
	cacheDir := t.TempDir()
	path := historyPath(cacheDir, testRepoKey)
	target := filepath.Join(t.TempDir(), "elsewhere")
	writeFile(t, target, "")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, target, path)

	if _, err := appendHistoryEvent(cacheDir, validEvent()); err == nil {
		t.Fatalf("appendHistoryEvent() through a symbolic link succeeded, want an error")
	}
	if got := readFile(t, target); got != "" {
		t.Fatalf("symbolic link target was written: %q", got)
	}
}

func TestAppendHistoryFailsWhenCacheIsUnwritable(t *testing.T) {
	cacheDir := t.TempDir()
	// A regular file where the application directory belongs makes the
	// history directory impossible to create on every platform.
	writeFile(t, filepath.Join(cacheDir, historyAppDir), "")

	if _, err := appendHistoryEvent(cacheDir, validEvent()); err == nil {
		t.Fatalf("appendHistoryEvent() succeeded with an unwritable cache, want an error")
	}
}

func TestHistoryFilePermissions(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("POSIX permissions do not apply on Windows")
	}
	cacheDir := t.TempDir()
	result, err := appendHistoryEvent(cacheDir, validEvent())
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{
		result.Path:               0o600,
		filepath.Dir(result.Path): 0o700,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s has permissions %o, want %o", path, got, want)
		}
	}
}
