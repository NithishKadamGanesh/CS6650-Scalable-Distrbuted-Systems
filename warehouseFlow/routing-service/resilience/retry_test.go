package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetrySuccessFirstAttempt(t *testing.T) {
	calls := 0
	err := Do(context.Background(), "op1", RetryConfig{MaxAttempts: 3, BaseDelay: 1 * time.Millisecond},
		func(attempt int) (bool, error) { calls++; return false, nil })
	if err != nil { t.Fatal(err) }
	if calls != 1 { t.Fatalf("expected 1 call, got %d", calls) }
}

func TestRetryEventualSuccess(t *testing.T) {
	calls := 0
	err := Do(context.Background(), "op2", RetryConfig{MaxAttempts: 3, BaseDelay: 1 * time.Millisecond},
		func(attempt int) (bool, error) {
			calls++
			if attempt < 2 { return true, errors.New("transient") }
			return false, nil
		})
	if err != nil { t.Fatal(err) }
	if calls != 3 { t.Fatalf("expected 3 calls, got %d", calls) }
}

func TestRetryNonRetryable(t *testing.T) {
	calls := 0
	err := Do(context.Background(), "op3", RetryConfig{MaxAttempts: 5, BaseDelay: 1 * time.Millisecond},
		func(attempt int) (bool, error) { calls++; return false, errors.New("fatal") })
	if err == nil { t.Fatal("expected error") }
	if calls != 1 { t.Fatalf("expected 1 call, got %d", calls) }
}

func TestRetryExhausted(t *testing.T) {
	calls := 0
	err := Do(context.Background(), "op4", RetryConfig{MaxAttempts: 3, BaseDelay: 1 * time.Millisecond},
		func(attempt int) (bool, error) { calls++; return true, errors.New("always fails") })
	if err == nil { t.Fatal("expected error") }
	if calls != 3 { t.Fatalf("expected 3 calls, got %d", calls) }
}

func TestRetryContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	calls := 0
	err := Do(ctx, "op5", RetryConfig{MaxAttempts: 100, BaseDelay: 50 * time.Millisecond},
		func(attempt int) (bool, error) { calls++; return true, errors.New("fail") })
	if err != context.DeadlineExceeded { t.Fatalf("expected DeadlineExceeded, got %v", err) }
	if calls > 2 { t.Fatalf("expected ≤2 calls before cancel, got %d", calls) }
}

func TestComputeDelayRespectsMax(t *testing.T) {
	cfg := RetryConfig{BaseDelay: 100 * time.Millisecond, MaxDelay: 150 * time.Millisecond, JitterPct: 0}
	if delay := computeDelay(cfg, 5); delay > cfg.MaxDelay {
		t.Fatalf("delay %v exceeded MaxDelay %v", delay, cfg.MaxDelay)
	}
}
