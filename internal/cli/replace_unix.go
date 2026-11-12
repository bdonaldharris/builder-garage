//go:build unix

package cli

import "os"

// commitConfigReplacement atomically replaces the destination directory entry
// when source and destination are on the same filesystem. Temporary
// configurations are created in the destination directory to guarantee that.
func commitConfigReplacement(source, destination string) error {
	return os.Rename(source, destination)
}
