package cli

import "time"

const (
	historyReplaceAttempts = 50
	historyReplaceDelay    = 10 * time.Millisecond
)

// commitHistoryReplacement retries only platform-classified transient
// replacement failures. The history lock remains held across every attempt,
// so retries cannot interleave two record operations.
func commitHistoryReplacement(source, destination string) error {
	return retryHistoryReplacement(func() error {
		return commitConfigReplacement(source, destination)
	}, isRetryableHistoryReplacement, historyReplaceAttempts, time.Sleep)
}

func retryHistoryReplacement(replace func() error, retryable func(error) bool, attempts int, sleep func(time.Duration)) error {
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		err = replace()
		if err == nil || !retryable(err) || attempt == attempts-1 {
			return err
		}
		sleep(historyReplaceDelay)
	}
	return err
}
