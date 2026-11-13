package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// historyLockTimeout bounds how long recording waits for another garage
// process recording to the same history. Holders keep the lock only for one
// read and one small write, so a longer wait means a stuck process. It is a
// variable only so tests can shorten it.
var historyLockTimeout = 5 * time.Second

const (
	// historyLockPoll is the interval between attempts to take the lock.
	historyLockPoll = 10 * time.Millisecond
	// historyLockExt names the lock file kept beside a history file.
	historyLockExt = ".lock"
)

// errHistoryLocked reports that the history stayed locked by another process
// for the whole bounded wait. Nothing was read or written.
var errHistoryLocked = errors.New("another garage process is still recording to this history")

// errHistoryLockUnsupported reports that this platform has no cross-process
// file lock that garage can use. Recording is refused rather than risk losing
// another process's event.
var errHistoryLockUnsupported = errors.New("local history locking is not supported on this platform")

// lockHistory takes the cross-process lock for the history file at path, so
// that one read-modify-replace runs at a time across processes, clones, and
// worktrees that share the history. It waits at most timeout. The returned
// function releases the lock.
//
// The lock is an operating-system lock on a separate file that is never
// removed. The history file itself is replaced by rename, so it cannot carry
// the lock. The operating system releases the lock when its holder exits, so
// a crashed process never leaves the history locked.
func lockHistory(path string, timeout time.Duration) (func(), error) {
	lockPath := path + historyLockExt
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing to use %s: it is not a regular file", lockPath)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("cannot inspect the history lock: %w", err)
	}
	file, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot open the history lock: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		acquired, err := tryLockFile(file)
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("cannot lock the history: %w", err)
		}
		if acquired {
			return func() {
				_ = unlockFile(file)
				file.Close()
			}, nil
		}
		if !time.Now().Before(deadline) {
			file.Close()
			return nil, fmt.Errorf("%w (waited %s); the event was not recorded", errHistoryLocked, timeout)
		}
		time.Sleep(historyLockPoll)
	}
}
