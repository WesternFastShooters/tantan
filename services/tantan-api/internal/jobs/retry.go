package jobs

import (
	"context"
	"errors"
	"time"
)

type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Sleep       func(context.Context, time.Duration) error
}

type temporary interface {
	Temporary() bool
}

func RetryValue[T any](ctx context.Context, policy RetryPolicy, operation func() (T, error)) (T, error) {
	var empty T
	if operation == nil {
		return empty, errors.New("retry operation is required")
	}
	if policy.MaxAttempts < 1 {
		policy.MaxAttempts = 1
	}
	if policy.BaseDelay <= 0 {
		policy.BaseDelay = 250 * time.Millisecond
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = 5 * time.Second
	}
	if policy.Sleep == nil {
		policy.Sleep = sleepContext
	}
	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		value, err := operation()
		if err == nil {
			return value, nil
		}
		var retryable temporary
		if !errors.As(err, &retryable) || !retryable.Temporary() || attempt == policy.MaxAttempts-1 {
			return empty, err
		}
		delay := policy.BaseDelay * time.Duration(1<<attempt)
		if delay > policy.MaxDelay {
			delay = policy.MaxDelay
		}
		if err := policy.Sleep(ctx, delay); err != nil {
			return empty, err
		}
	}
	return empty, errors.New("retry operation exhausted")
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
