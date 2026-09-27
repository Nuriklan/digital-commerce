package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nuriklan/digital-commerce/pkg/resilience"
)

func TestRetry_SuccessOnRetry(t *testing.T) {
	attempts := 0
	cfg := resilience.RetryConfig{
		MaxAttempts:     3,
		InitialInterval: 10 * time.Millisecond,
		Multiplier:      2.0,
		MaxInterval:     100 * time.Millisecond,
	}

	err := resilience.Retry(context.Background(), cfg, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected retry to succeed on 3rd attempt, got error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	cfg := resilience.RetryConfig{
		MaxAttempts:     5,
		InitialInterval: 50 * time.Millisecond,
		Multiplier:      2.0,
		MaxInterval:     200 * time.Millisecond,
	}

	err := resilience.Retry(ctx, cfg, func(ctx context.Context) error {
		return errors.New("service unavailable")
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestCircuitBreaker_TripsOpenAndRecovers(t *testing.T) {
	cb := resilience.NewCircuitBreaker(2, 50*time.Millisecond)

	failOp := func(ctx context.Context) error { return errors.New("error") }
	successOp := func(ctx context.Context) error { return nil }

	_ = cb.Execute(context.Background(), failOp)
	if cb.State() != resilience.StateClosed {
		t.Errorf("expected CLOSED, got %s", cb.State())
	}

	_ = cb.Execute(context.Background(), failOp)
	if cb.State() != resilience.StateOpen {
		t.Errorf("expected OPEN, got %s", cb.State())
	}

	err := cb.Execute(context.Background(), successOp)
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}

	time.Sleep(60 * time.Millisecond)

	err = cb.Execute(context.Background(), successOp)
	if err != nil {
		t.Fatalf("expected successful probe, got %v", err)
	}

	if cb.State() != resilience.StateClosed {
		t.Errorf("expected circuit breaker to recover to CLOSED, got %s", cb.State())
	}
}
