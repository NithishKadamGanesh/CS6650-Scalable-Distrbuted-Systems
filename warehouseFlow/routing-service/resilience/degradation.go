package resilience

// Graceful degradation. If a non-critical dependency (audit log) is down,
// route the order anyway and buffer the audit entry for later reconciliation.
// "Serve what you can" — the routing decision succeeds even if audit fails.

import (
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	degradedWrites = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_degraded_writes_total",
		Help: "Writes that fell back to local buffer",
	})

	degradedBufferSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "warehouseflow_degraded_buffer_size",
		Help: "Current local fallback buffer size",
	})

	degradedFlushed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_degraded_flushed_total",
		Help: "Buffered items successfully flushed",
	})
)

// BufferedEntry is a single audit record held while primary is down.
type BufferedEntry struct {
	Timestamp time.Time
	Payload   interface{}
}

// DegradationBuffer holds entries bounded by maxSize (drops oldest on overflow).
type DegradationBuffer struct {
	mu      sync.Mutex
	entries []BufferedEntry
	maxSize int
}

// NewBuffer creates a bounded degradation buffer.
func NewBuffer(maxSize int) *DegradationBuffer {
	return &DegradationBuffer{maxSize: maxSize}
}

// Add stores an entry, dropping oldest if full.
func (b *DegradationBuffer) Add(payload interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.entries) >= b.maxSize {
		b.entries = b.entries[1:]
	}
	b.entries = append(b.entries, BufferedEntry{
		Timestamp: time.Now(),
		Payload:   payload,
	})
	degradedWrites.Inc()
	degradedBufferSize.Set(float64(len(b.entries)))
}

// Drain returns all entries and empties the buffer.
func (b *DegradationBuffer) Drain() []BufferedEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries := b.entries
	b.entries = nil
	degradedBufferSize.Set(0)
	degradedFlushed.Add(float64(len(entries)))
	return entries
}

// Size returns current buffer length.
func (b *DegradationBuffer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
}

// TryPrimary runs primary; on failure, buffers payload.
func TryPrimary(buffer *DegradationBuffer, payload interface{}, primary func() error) bool {
	if err := primary(); err == nil {
		return true
	} else {
		log.Printf("primary write failed, buffering: %v", err)
		buffer.Add(payload)
		return false
	}
}
