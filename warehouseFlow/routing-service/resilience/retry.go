package resilience

// Exponential backoff with jitter for transient failures.
// Without jitter, all clients retry at the same instant after a failure,
// creating a thundering herd. Jitter randomizes retry timing.
//
// Schedule with ±50% jitter, base 100ms:
//   Attempt 1: ~100ms  (75-150ms)
//   Attempt 2: ~200ms  (150-300ms)
//   Attempt 3: ~400ms  (300-600ms)

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	retryAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "warehouseflow_retry_attempts_total",
		Help: "Total retry attempts per operation",
	}, []string{"operation"})

	retrySuccesses = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "warehouseflow_retry_successes_total",
		Help: "Retries that eventually succeeded",
	}, []string{"operation"})

	retryExhausted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "warehouseflow_retry_exhausted_total",
		Help: "Retries that exhausted max attempts",
	}, []string{"operation"})
)

// RetryConfig controls retry behavior.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	JitterPct   float64
}

var DefaultRetry = RetryConfig{
	MaxAttempts: 3,
	BaseDelay:   100 * time.Millisecond,
	MaxDelay:    1 * time.Second,
	JitterPct:   0.5,
}

var ErrRetryExhausted = errors.New("retry: max attempts exhausted")

// Retryable is a function that can be retried. Must be idempotent —
// retrying a non-idempotent operation risks duplicate side effects.
type Retryable func(attempt int) (shouldRetry bool, err error)

// Do executes fn with exponential backoff + jitter.
func Do(ctx context.Context, operation string, cfg RetryConfig, fn Retryable) error {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = DefaultRetry.MaxAttempts
	}
	if cfg.BaseDelay == 0 {
		cfg.BaseDelay = DefaultRetry.BaseDelay
	}
	if cfg.MaxDelay == 0 {
		cfg.MaxDelay = DefaultRetry.MaxDelay
	}
	if cfg.JitterPct == 0 {
		cfg.JitterPct = DefaultRetry.JitterPct
	}

	var lastErr error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			retryAttempts.WithLabelValues(operation).Inc()
			delay := computeDelay(cfg, attempt)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		shouldRetry, err := fn(attempt)
		if err == nil {
			if attempt > 0 {
				retrySuccesses.WithLabelValues(operation).Inc()
			}
			return nil
		}
		lastErr = err
		if !shouldRetry {
			return err
		}
	}

	retryExhausted.WithLabelValues(operation).Inc()
	if lastErr != nil {
		return lastErr
	}
	return ErrRetryExhausted
}

// computeDelay returns base * 2^(attempt-1) with jitter, capped at MaxDelay.
func computeDelay(cfg RetryConfig, attempt int) time.Duration {
	mult := 1 << uint(attempt-1)
	base := time.Duration(int64(cfg.BaseDelay) * int64(mult))
	if base > cfg.MaxDelay {
		base = cfg.MaxDelay
	}
	if cfg.JitterPct > 0 {
		jitter := float64(base) * cfg.JitterPct
		offset := (rand.Float64() - 0.5) * 2 * jitter
		base = time.Duration(float64(base) + offset)
	}
	if base < 0 {
		base = cfg.BaseDelay
	}
	return base
}
