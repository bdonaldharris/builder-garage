//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package cli

import "os"

// tryLockFile reports that no cross-process lock is available, so recording
// is refused rather than risk losing another process's event.
func tryLockFile(*os.File) (bool, error) {
	return false, errHistoryLockUnsupported
}

func unlockFile(*os.File) error { return nil }
