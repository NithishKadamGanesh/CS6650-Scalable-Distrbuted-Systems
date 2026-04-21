package store

// Distributed lock primitives for pessimistic concurrency strategy.
// Uses Redis SETNX (atomic SET if Not eXists) with a TTL to prevent
// deadlock if the holder crashes.

import (
	"context"
	"time"
)

// AcquireLock tries to acquire a lock. Returns (true, nil) if acquired.
func (w *Warehouse) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return w.client.SetNX(ctx, key, "1", ttl).Result()
}

// ReleaseLock drops the lock. No-op if already expired.
func (w *Warehouse) ReleaseLock(ctx context.Context, key string) error {
	return w.client.Del(ctx, key).Err()
}

// RefreshLock extends the TTL of a held lock.
func (w *Warehouse) RefreshLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return w.client.Expire(ctx, key, ttl).Result()
}
