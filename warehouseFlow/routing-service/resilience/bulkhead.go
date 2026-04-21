package resilience

// Per-warehouse bulkhead using a buffered channel as a counting semaphore.
// Prevents one slow warehouse from starving goroutines that would route to
// healthy warehouses. HW3 pattern — counting semaphore via buffered channel.

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	bulkheadInflight = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "warehouseflow_bulkhead_inflight",
		Help: "Current in-flight requests per bulkhead",
	}, []string{"warehouse_id"})

	bulkheadRejects = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "warehouseflow_bulkhead_rejects_total",
		Help: "Requests rejected because bulkhead was full",
	}, []string{"warehouse_id"})
)

// Bulkhead is a counting semaphore limiting concurrent access.
type Bulkhead struct {
	name     string
	sem      chan struct{}
	maxSize  int
	rejects  atomic.Int64
	inflight atomic.Int64
}

// NewBulkhead creates a bulkhead with the given max concurrency.
func NewBulkhead(name string, max int) *Bulkhead {
	return &Bulkhead{
		name:    name,
		sem:     make(chan struct{}, max),
		maxSize: max,
	}
}

// Acquire tries to get a slot. Returns true if acquired within timeout.
func (b *Bulkhead) Acquire(timeout time.Duration) bool {
	select {
	case b.sem <- struct{}{}:
		b.inflight.Add(1)
		bulkheadInflight.WithLabelValues(b.name).Set(float64(b.inflight.Load()))
		return true
	case <-time.After(timeout):
		b.rejects.Add(1)
		bulkheadRejects.WithLabelValues(b.name).Inc()
		return false
	}
}

// Release returns a slot. MUST follow a successful Acquire.
func (b *Bulkhead) Release() {
	<-b.sem
	b.inflight.Add(-1)
	bulkheadInflight.WithLabelValues(b.name).Set(float64(b.inflight.Load()))
}

// Stats returns current usage info.
func (b *Bulkhead) Stats() (inflight, max int, rejects int64) {
	return int(b.inflight.Load()), b.maxSize, b.rejects.Load()
}

// ResetRejects zeroes the counter.
func (b *Bulkhead) ResetRejects() {
	b.rejects.Store(0)
}
