package resilience

import (
	"sync"
	"testing"
	"time"
)

func TestBulkheadCapacity(t *testing.T) {
	b := NewBulkhead("test-bh-cap", 3)
	for i := 0; i < 3; i++ {
		if !b.Acquire(10 * time.Millisecond) { t.Fatalf("acquire %d should succeed", i) }
	}
	if b.Acquire(20 * time.Millisecond) { t.Fatal("4th acquire should be rejected") }
	b.Release()
	if !b.Acquire(10 * time.Millisecond) { t.Fatal("acquire should succeed after release") }
}

func TestBulkheadStats(t *testing.T) {
	b := NewBulkhead("test-bh-stats", 2)
	b.Acquire(10 * time.Millisecond)
	b.Acquire(10 * time.Millisecond)
	b.Acquire(10 * time.Millisecond)

	inflight, max, rejects := b.Stats()
	if inflight != 2 || max != 2 || rejects != 1 {
		t.Fatalf("expected (2,2,1) got (%d,%d,%d)", inflight, max, rejects)
	}
}

func TestBulkheadUnderContention(t *testing.T) {
	b := NewBulkhead("test-bh-cont", 5)
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted, rejected := 0, 0

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b.Acquire(5 * time.Millisecond) {
				mu.Lock(); accepted++; mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				b.Release()
			} else {
				mu.Lock(); rejected++; mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if accepted+rejected != 20 { t.Fatal("accounting mismatch") }
	if accepted == 0 || rejected == 0 { t.Fatal("expected both accepted and rejected under contention") }
}
