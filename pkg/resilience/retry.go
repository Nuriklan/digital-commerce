package resilience

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"
)

var ErrMaxRetriesReached = errors.New("maximum retry attempts reached")

type RetryConfig struct {
	MaxAttempts     int
	InitialInterval time.Duration
	Multiplier      float64
	MaxInterval     time.Duration
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		InitialInterval: 100 * time.Millisecond,
		Multiplier:      2.0,
		MaxInterval:     2 * time.Second,
	}
}

// Retry executes the operation `op` with exponential backoff and jitter
// Immediately interrupts execution upon cancellation of ctx
func Retry(ctx context.Context, cfg RetryConfig, op func(ctx context.Context) error) error {
	interval := cfg.InitialInterval

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := op(ctx)
		if err == nil {
			return nil
		}

		if attempt == cfg.MaxAttempts {
			return fmt.Errorf("%w: last error: %v", ErrMaxRetriesReached, err)
		}

		// Adding jitter: a random deviation of +/- 20% to prevent synchronization storms
		jitter := time.Duration((rand.Float64()*0.4 - 0.2) * float64(interval))
		sleepDuration := interval + jitter

		timer := time.NewTimer(sleepDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		// Exponential growth of the interval
		interval = time.Duration(float64(interval) * cfg.Multiplier)
		if interval > cfg.MaxInterval {
			interval = cfg.MaxInterval
		}
	}

	return ErrMaxRetriesReached
}
