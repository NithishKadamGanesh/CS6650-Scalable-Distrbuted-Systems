package resilience

// Bounded-context helpers enforcing aggressive timeouts on external calls.
// Without fail-fast, a slow dependency can exhaust goroutines causing
// cascading failure. Budgets calibrated from Experiment 1 P99 observations.

import (
	"context"
	"time"
)

const (
	RedisTimeout    = 200 * time.Millisecond
	PostgresTimeout = 500 * time.Millisecond
	KafkaTimeout    = 1 * time.Second
	SQSTimeout      = 2 * time.Second
)

// WithRedis wraps a context with Redis timeout.
func WithRedis(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, RedisTimeout)
}

// WithPostgres wraps a context with Postgres timeout.
func WithPostgres(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, PostgresTimeout)
}

// WithKafka wraps a context with Kafka timeout.
func WithKafka(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, KafkaTimeout)
}

// WithSQS wraps a context with SQS timeout.
func WithSQS(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, SQSTimeout)
}

// WithCustom wraps a context with a user-specified timeout.
func WithCustom(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}
