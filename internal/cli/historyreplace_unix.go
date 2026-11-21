//go:build !windows

package cli

func isRetryableHistoryReplacement(error) bool { return false }
