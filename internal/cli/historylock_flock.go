//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package cli

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile takes an exclusive flock on file without blocking. It reports
// false when another process holds the lock.
func tryLockFile(file *os.File) (bool, error) {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		default:
			return false, err
		}
	}
}

// unlockFile releases the lock. Closing the file also releases it.
func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
