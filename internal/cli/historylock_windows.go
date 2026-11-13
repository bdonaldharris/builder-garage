//go:build windows

package cli

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x1
	lockfileExclusiveLock   = 0x2
	errorLockViolation      = syscall.Errno(33)
)

var (
	lockFileEx   = kernel32.NewProc("LockFileEx")
	unlockFileEx = kernel32.NewProc("UnlockFileEx")
)

// tryLockFile takes an exclusive lock on the first byte of file without
// blocking. It reports false when another process holds the lock.
func tryLockFile(file *os.File) (bool, error) {
	var overlapped syscall.Overlapped
	r, _, err := lockFileEx.Call(file.Fd(),
		lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0,
		uintptr(unsafe.Pointer(&overlapped)))
	if r != 0 {
		return true, nil
	}
	if errors.Is(err, errorLockViolation) {
		return false, nil
	}
	return false, err
}

// unlockFile releases the lock. Closing the file also releases it.
func unlockFile(file *os.File) error {
	var overlapped syscall.Overlapped
	r, _, err := unlockFileEx.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if r == 0 {
		return err
	}
	return nil
}
