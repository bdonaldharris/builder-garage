package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Environment for TestHistoryHelperProcess, which runs as a separate process.
const (
	historyHelperMode  = "GARAGE_TEST_HISTORY_HELPER"
	historyHelperDir   = "GARAGE_TEST_HISTORY_DIR"
	historyHelperCache = "GARAGE_TEST_HISTORY_CACHE"
	historyHelperStart = "GARAGE_TEST_HISTORY_START"
	historyHelperBase  = "GARAGE_TEST_HISTORY_BASE"
	historyHelperCount = "GARAGE_TEST_HISTORY_COUNT"
	historyHelperPath  = "GARAGE_TEST_HISTORY_PATH"
)

// TestHistoryHelperProcess is not a test of its own. The tests below start the
// test binary again with historyHelperMode set, so that it runs here as a
// separate process:
//
//   - "record" waits for the start file, then records COUNT events numbered
//     from BASE, from the repository at DIR, into the cache at CACHE;
//   - "hold" takes the lock for the history at PATH, reports "locked", and
//     keeps it until the process is killed.
func TestHistoryHelperProcess(t *testing.T) {
	mode := os.Getenv(historyHelperMode)
	if mode == "" {
		t.Skip("runs only as a helper process")
	}
	switch mode {
	case "record":
		os.Exit(helperRecord())
	case "hold":
		unlock, err := lockHistory(os.Getenv(historyHelperPath), 10*time.Second)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer unlock()
		fmt.Println("locked")
		time.Sleep(time.Minute)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
		os.Exit(2)
	}
}

func helperRecord() int {
	start := os.Getenv(historyHelperStart)
	for deadline := time.Now().Add(30 * time.Second); ; {
		if _, err := os.Stat(start); err == nil {
			break
		}
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "the start file never appeared")
			return 1
		}
		time.Sleep(time.Millisecond)
	}
	base, _ := strconv.Atoi(os.Getenv(historyHelperBase))
	count, _ := strconv.Atoi(os.Getenv(historyHelperCount))
	cache := os.Getenv(historyHelperCache)
	env := recordEnv{
		WorkDir:  os.Getenv(historyHelperDir),
		CacheDir: func() (string, error) { return cache, nil },
		Git:      runGit,
		Now:      time.Now,
	}
	for i := 0; i < count; i++ {
		opts := recordOptions{Issue: base + i, Type: "implementation", Outcome: "ready"}
		if _, err := recordEvent(context.Background(), opts, env); err != nil {
			fmt.Fprintf(os.Stderr, "event %d: %v\n", base+i, err)
			return 1
		}
	}
	return 0
}

// helperCommand returns a command that runs TestHistoryHelperProcess in a
// separate process with the given environment.
func helperCommand(t *testing.T, env ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHistoryHelperProcess$")
	cmd.Env = append(os.Environ(), append([]string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull,
	}, env...)...)
	return cmd
}

// Separate processes recording at the same time to one repository's history,
// from its main working tree, a linked worktree, and a separate clone, keep
// every event.
func TestHistoryConcurrentProcessesKeepEveryEvent(t *testing.T) {
	requireGit(t)
	const origin = "https://github.com/owner/repo.git"
	mainDir := newGitRepo(t, origin)
	worktree := filepath.Join(t.TempDir(), "wt")
	gitIn(t, mainDir, "worktree", "add", "-q", worktree)
	clone := filepath.Join(t.TempDir(), "clone")
	gitIn(t, mainDir, "clone", "-q", mainDir, clone)
	gitIn(t, clone, "remote", "set-url", "origin", origin)

	var key string
	for _, dir := range []string{mainDir, worktree, clone} {
		repo, err := identifyRepository(context.Background(), dir, runGit)
		if err != nil {
			t.Fatal(err)
		}
		if key == "" {
			key = repo.Key
		} else if repo.Key != key {
			t.Fatalf("%s has repository key %s, want the shared key %s", dir, repo.Key, key)
		}
	}

	cache := t.TempDir()
	start := filepath.Join(t.TempDir(), "start")
	dirs := []string{mainDir, mainDir, worktree, worktree, clone, clone}
	const perProcess = 25
	type child struct {
		cmd    *exec.Cmd
		stderr strings.Builder
	}
	children := make([]*child, len(dirs))
	for i, dir := range dirs {
		c := &child{cmd: helperCommand(t,
			historyHelperMode+"=record",
			historyHelperDir+"="+dir,
			historyHelperCache+"="+cache,
			historyHelperStart+"="+start,
			historyHelperBase+"="+strconv.Itoa((i+1)*1000),
			historyHelperCount+"="+strconv.Itoa(perProcess),
		)}
		c.cmd.Stderr = &c.stderr
		if err := c.cmd.Start(); err != nil {
			t.Fatal(err)
		}
		children[i] = c
	}
	writeFile(t, start, "")
	for i, c := range children {
		if err := c.cmd.Wait(); err != nil {
			t.Errorf("recording process %d (%s): %v\n%s", i, dirs[i], err, c.stderr.String())
		}
	}
	if t.Failed() {
		return
	}

	events, discarded, err := readHistory(historyPath(cache, key))
	if err != nil {
		t.Fatal(err)
	}
	if discarded != 0 {
		t.Errorf("discarded %d history lines, want 0", discarded)
	}
	want := len(dirs) * perProcess
	if len(events) != want {
		t.Fatalf("history holds %d events, want all %d recorded events", len(events), want)
	}
	seen := map[int]bool{}
	for _, e := range events {
		if seen[e.Number] {
			t.Errorf("event %d is stored twice", e.Number)
		}
		seen[e.Number] = true
	}
	for i := range dirs {
		for j := 0; j < perProcess; j++ {
			if n := (i+1)*1000 + j; !seen[n] {
				t.Errorf("event %d was lost", n)
			}
		}
	}
}

// startLockHolder starts a separate process that holds the lock for the
// history at path, and returns once it reports holding it.
func startLockHolder(t *testing.T, path string) *exec.Cmd {
	t.Helper()
	cmd := helperCommand(t, historyHelperMode+"=hold", historyHelperPath+"="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("lock holder did not report the lock: %q, %v\n%s", line, err, stderr.String())
	}
	return cmd
}

// A lock held by another process makes recording wait at most the bounded
// time, then give up without writing. Once the holder dies, the operating
// system releases the lock.
func TestHistoryLockIsBoundedAndReleasedWhenItsHolderDies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history"+historyFileExt)
	holder := startLockHolder(t, path)

	began := time.Now()
	if _, err := lockHistory(path, 200*time.Millisecond); !errors.Is(err, errHistoryLocked) {
		t.Fatalf("lockHistory while held = %v, want errHistoryLocked", err)
	}
	if waited := time.Since(began); waited > 5*time.Second {
		t.Errorf("lockHistory waited %s, want about 200ms", waited)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("the history file was created while the lock was held")
	}

	if err := holder.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = holder.Wait()
	unlock, err := lockHistory(path, 5*time.Second)
	if err != nil {
		t.Fatalf("lockHistory after the holder died = %v, want the lock", err)
	}
	unlock()
}

// While another process holds the history lock for longer than the bounded
// wait, garage record warns and exits successfully, and the history is left
// exactly as it was.
func TestRecordCommandWarnsAndSucceedsWhenHistoryStaysLocked(t *testing.T) {
	cacheDir := isolateUserCache(t)
	repoDir := newGitRepo(t, "git@github.com:owner/repo.git")
	repo, err := identifyRepository(context.Background(), repoDir, runGit)
	if err != nil {
		t.Fatal(err)
	}
	path := historyPath(cacheDir, repo.Key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "")
	before := readFile(t, path)

	saved := historyLockTimeout
	historyLockTimeout = 200 * time.Millisecond
	t.Cleanup(func() { historyLockTimeout = saved })
	startLockHolder(t, path)

	stdout, stderr, err := runCLI(t, "dev", "record", "--issue", "44", "--type", "implementation", "--outcome", "ready")
	if err != nil {
		t.Fatalf("garage record returned %v, want success", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "garage: warning: event not recorded:") ||
		!strings.Contains(stderr, errHistoryLocked.Error()) {
		t.Errorf("stderr = %q, want a warning that the history stayed locked", stderr)
	}
	if after := readFile(t, path); after != before {
		t.Errorf("history changed while locked: %q", after)
	}
}

func TestHistoryLockRefusesANonRegularLockFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links needs extra privileges on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "history"+historyFileExt)
	if err := os.Symlink(filepath.Join(dir, "elsewhere"), path+historyLockExt); err != nil {
		t.Fatal(err)
	}
	if _, err := lockHistory(path, time.Second); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("lockHistory with a symlinked lock file = %v, want a refusal", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "elsewhere")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the symbolic link was followed")
	}
}
