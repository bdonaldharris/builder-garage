package cli

import (
	"errors"
	"testing"
	"time"
)

func TestRetryHistoryReplacementRetriesOnlyClassifiedErrors(t *testing.T) {
	want := errors.New("transient replacement failure")
	attempts := 0
	sleeps := 0
	err := retryHistoryReplacement(func() error {
		attempts++
		if attempts < 3 {
			return want
		}
		return nil
	}, func(err error) bool { return errors.Is(err, want) }, 4, func(time.Duration) { sleeps++ })
	if err != nil || attempts != 3 || sleeps != 2 {
		t.Fatalf("retry result = err %v, attempts %d, sleeps %d; want nil, 3, 2", err, attempts, sleeps)
	}

	attempts = 0
	err = retryHistoryReplacement(func() error {
		attempts++
		return want
	}, func(error) bool { return false }, 4, func(time.Duration) { t.Fatal("non-retryable error slept") })
	if !errors.Is(err, want) || attempts != 1 {
		t.Fatalf("non-retryable result = err %v, attempts %d; want original error after one attempt", err, attempts)
	}
}
