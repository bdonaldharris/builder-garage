//go:build windows

package cli

import (
	"errors"
	"syscall"
)

const errorSharingViolation = syscall.Errno(32)

// MoveFileExW can briefly report either error while another Windows handle is
// releasing the history destination. Retrying under the existing history lock
// is safe and bounded.
func isRetryableHistoryReplacement(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, errorSharingViolation)
}
