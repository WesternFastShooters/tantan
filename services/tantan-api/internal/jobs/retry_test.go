package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/jobs"
)

type temporaryFailure struct{}

func (temporaryFailure) Error() string   { return "temporary" }
func (temporaryFailure) Temporary() bool { return true }

func TestRetryValueUsesBoundedExponentialBackoff(t *testing.T) {
	attempts := 0
	var delays []time.Duration
	value, err := jobs.RetryValue(context.Background(), jobs.RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   250 * time.Millisecond,
		Sleep: func(_ context.Context, duration time.Duration) error {
			delays = append(delays, duration)
			return nil
		},
	}, func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", temporaryFailure{}
		}
		return "ready", nil
	})
	if err != nil || value != "ready" || attempts != 3 {
		t.Fatalf("value=%q attempts=%d err=%v", value, attempts, err)
	}
	if len(delays) != 2 || delays[0] != 250*time.Millisecond || delays[1] != 500*time.Millisecond {
		t.Fatalf("delays=%v", delays)
	}
}

func TestRetryValueDoesNotRetryPermanentFailure(t *testing.T) {
	attempts := 0
	want := errors.New("permanent")
	_, err := jobs.RetryValue(context.Background(), jobs.RetryPolicy{MaxAttempts: 3}, func() (struct{}, error) {
		attempts++
		return struct{}{}, want
	})
	if !errors.Is(err, want) || attempts != 1 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}
