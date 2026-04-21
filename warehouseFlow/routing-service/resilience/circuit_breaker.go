package resilience

// Per-warehouse circuit breaker protecting the routing engine from
// slow or failing warehouse state stores.
//
// State machine:
//   CLOSED    → (N failures)        → OPEN
//   OPEN      → (cooldown elapsed)  → HALF-OPEN
//   HALF-OPEN → (M successes)       → CLOSED
//   HALF-OPEN → (any failure)       → OPEN

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	return [...]string{"CLOSED", "OPEN", "HALF-OPEN"}[s]
}

var (
	cbStateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "warehouseflow_cb_state",
		Help: "Circuit breaker state (0=CLOSED, 1=OPEN, 2=HALF-OPEN)",
	}, []string{"warehouse_id"})

	cbTripsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "warehouseflow_cb_trips_total",
		Help: "Total times each breaker has tripped to OPEN",
	}, []string{"warehouse_id"})

	cbRejectsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "warehouseflow_cb_rejects_total",
		Help: "Requests rejected by open breakers",
	}, []string{"warehouse_id"})
)

// Config controls breaker behavior.
type Config struct {
	FailureThreshold int
	Cooldown         time.Duration
	RecoveryTarget   int
	OnStateChange    func(name string, from, to State)
}

var DefaultConfig = Config{
	FailureThreshold: 5,
	Cooldown:         10 * time.Second,
	RecoveryTarget:   2,
}

// CircuitBreaker protects one downstream dependency.
type CircuitBreaker struct {
	name           string
	cfg            Config
	mu             sync.RWMutex
	state          State
	failures       int
	successesInRow int
	lastFailure    time.Time
}

// New creates a breaker with the given config (falls back to defaults).
func New(name string, cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold == 0 {
		cfg = DefaultConfig
	}
	cbStateGauge.WithLabelValues(name).Set(0)
	return &CircuitBreaker{name: name, cfg: cfg, state: StateClosed}
}

// Allow returns true if a request should be permitted.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailure) > cb.cfg.Cooldown {
			cb.transition(StateHalfOpen)
			return true
		}
		cbRejectsTotal.WithLabelValues(cb.name).Inc()
		return false
	case StateHalfOpen:
		return true
	}
	return false
}

// RecordSuccess signals a successful downstream call.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	if cb.state == StateHalfOpen {
		cb.successesInRow++
		if cb.successesInRow >= cb.cfg.RecoveryTarget {
			cb.transition(StateClosed)
			cb.successesInRow = 0
		}
	}
}

// RecordFailure signals a failed or timed-out downstream call.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()
	cb.successesInRow = 0

	if cb.state == StateHalfOpen {
		cb.transition(StateOpen)
	} else if cb.state == StateClosed && cb.failures >= cb.cfg.FailureThreshold {
		cb.transition(StateOpen)
	}
}

// State returns the current state (thread-safe).
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset forces CLOSED. Useful for testing.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transition(StateClosed)
	cb.failures = 0
	cb.successesInRow = 0
}

func (cb *CircuitBreaker) transition(to State) {
	from := cb.state
	if from == to {
		return
	}
	cb.state = to
	cbStateGauge.WithLabelValues(cb.name).Set(float64(to))
	if to == StateOpen {
		cbTripsTotal.WithLabelValues(cb.name).Inc()
	}
	if cb.cfg.OnStateChange != nil {
		go cb.cfg.OnStateChange(cb.name, from, to)
	}
}
