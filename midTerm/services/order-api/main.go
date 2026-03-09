package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// PERCENTILE TRACKER — same analysis approach from HW6 Locust reports
// =============================================================================

type PercentileTracker struct {
	mu      sync.Mutex
	window  []float64
	maxSize int
}

func NewPercentileTracker(size int) *PercentileTracker {
	return &PercentileTracker{window: make([]float64, 0, size), maxSize: size}
}

func (pt *PercentileTracker) Record(v float64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if len(pt.window) >= pt.maxSize {
		pt.window = pt.window[1:]
	}
	pt.window = append(pt.window, v)
}

func (pt *PercentileTracker) Reset() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.window = pt.window[:0]
}

func (pt *PercentileTracker) Snapshot() (median, p95, p99, mean float64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if len(pt.window) == 0 {
		return
	}
	s := make([]float64, len(pt.window))
	copy(s, pt.window)
	sort.Float64s(s)
	n := len(s)
	var sum float64
	for _, v := range s {
		sum += v
	}
	mean = sum / float64(n)
	median = s[n/2]
	p95 = s[int(math.Ceil(float64(n)*0.95))-1]
	p99 = s[int(math.Ceil(float64(n)*0.99))-1]
	return
}

// =============================================================================
// PATTERN 1: FAIL FAST — aggressive timeout, no retries
// =============================================================================

type FailFastClient struct {
	timeout time.Duration
}

func NewFailFastClient(timeout time.Duration) *FailFastClient {
	return &FailFastClient{timeout: timeout}
}

func (f *FailFastClient) Call(url string) (*http.Response, error) {
	client := &http.Client{Timeout: f.timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fail-fast: unreachable within %v: %w", f.timeout, err)
	}
	if resp.StatusCode >= 500 {
		resp.Body.Close()
		return nil, fmt.Errorf("fail-fast: got %d", resp.StatusCode)
	}
	return resp, nil
}

// =============================================================================
// PATTERN 2: CIRCUIT BREAKER — CLOSED → OPEN → HALF-OPEN → CLOSED
// Uses sync.RWMutex (HW3 reasoning: State() is read-heavy, transitions rare)
// =============================================================================

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitState) String() string {
	return [...]string{"CLOSED", "OPEN", "HALF-OPEN"}[s]
}

type CircuitBreaker struct {
	mu             sync.RWMutex
	state          CircuitState
	failures       int
	threshold      int
	cooldown       time.Duration
	lastFailure    time.Time
	successesInRow int
	recoveryTarget int
	onStateChange  func(from, to CircuitState)
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{threshold: threshold, cooldown: cooldown, recoveryTarget: 2}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailure) > cb.cooldown {
			cb.transition(StateHalfOpen)
			return true
		}
		return false
	case StateHalfOpen:
		return true
	}
	return false
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	if cb.state == StateHalfOpen {
		cb.successesInRow++
		if cb.successesInRow >= cb.recoveryTarget {
			cb.transition(StateClosed)
			cb.successesInRow = 0
		}
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	cb.successesInRow = 0
	if cb.state == StateHalfOpen {
		cb.transition(StateOpen)
	} else if cb.failures >= cb.threshold {
		cb.transition(StateOpen)
	}
}

func (cb *CircuitBreaker) transition(to CircuitState) {
	from := cb.state
	cb.state = to
	if cb.onStateChange != nil {
		go cb.onStateChange(from, to)
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
	cb.successesInRow = 0
}

// =============================================================================
// PATTERN 3: BULKHEAD — buffered channel as counting semaphore (HW3 pattern)
// =============================================================================

type Bulkhead struct {
	name    string
	sem     chan struct{}
	maxSize int
	rejects atomic.Int64
}

func NewBulkhead(name string, max int) *Bulkhead {
	return &Bulkhead{name: name, sem: make(chan struct{}, max), maxSize: max}
}

func (b *Bulkhead) Acquire(timeout time.Duration) bool {
	select {
	case b.sem <- struct{}{}:
		return true
	case <-time.After(timeout):
		b.rejects.Add(1)
		return false
	}
}

func (b *Bulkhead) Release()      { <-b.sem }
func (b *Bulkhead) ResetRejects() { b.rejects.Store(0) }

func (b *Bulkhead) Stats() (int, int, int64) {
	return len(b.sem), b.maxSize, b.rejects.Load()
}

// =============================================================================
// PATTERN 4: RETRY WITH EXPONENTIAL BACKOFF + JITTER
// =============================================================================
// Without jitter, all clients retry at the exact same instant after a failure,
// creating a "thundering herd" that re-crashes the recovering service.
// This is the same contention pattern from HW3: goroutines all hitting a
// mutex simultaneously. Jitter randomizes retry timing to spread load.
//
// Backoff schedule (with ±50% jitter):
//   Attempt 1: ~100ms   (75-150ms)
//   Attempt 2: ~200ms   (150-300ms)
//   Attempt 3: ~400ms   (300-600ms)
//   Max delay capped at 1s

type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

var DefaultRetryConfig = RetryConfig{
	MaxAttempts: 3,
	BaseDelay:   100 * time.Millisecond,
	MaxDelay:    1 * time.Second,
}

// =============================================================================
// METRICS
// =============================================================================

type Metrics struct {
	mu           sync.Mutex
	Total        int64
	Successes    int64
	Failures     int64
	Timeouts     int64
	CircuitTrips int64
	BulkRejects  int64
	Retries      int64
	DegradedOK   int64 // orders accepted in degraded mode
	percentiles  *PercentileTracker
}

func NewMetrics() *Metrics {
	return &Metrics{percentiles: NewPercentileTracker(1000)}
}

func (m *Metrics) Record(success bool, latency time.Duration, timeout bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Total++
	m.percentiles.Record(float64(latency.Milliseconds()))
	if success {
		m.Successes++
	} else {
		m.Failures++
		if timeout {
			m.Timeouts++
		}
	}
}

func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Total = 0
	m.Successes = 0
	m.Failures = 0
	m.Timeouts = 0
	m.CircuitTrips = 0
	m.BulkRejects = 0
	m.Retries = 0
	m.DegradedOK = 0
	m.percentiles.Reset()
}

// =============================================================================
// SERVICE ENDPOINT + ORDER API
// =============================================================================

type ServiceEndpoint struct {
	Name string
	URL  string
}

type OrderAPI struct {
	endpoints []ServiceEndpoint
	circuit   map[string]*CircuitBreaker
	bulkhead  map[string]*Bulkhead
	ffClient  *FailFastClient
	metrics   *Metrics
	mode      string
	modeMu    sync.RWMutex
}

func NewOrderAPI(endpoints []ServiceEndpoint, mode string) *OrderAPI {
	api := &OrderAPI{
		endpoints: endpoints,
		circuit:   make(map[string]*CircuitBreaker),
		bulkhead:  make(map[string]*Bulkhead),
		ffClient:  NewFailFastClient(500 * time.Millisecond),
		metrics:   NewMetrics(),
		mode:      mode,
	}
	for _, ep := range endpoints {
		api.circuit[ep.Name] = NewCircuitBreaker(5, 5*time.Second)
		api.bulkhead[ep.Name] = NewBulkhead(ep.Name, 5)
	}
	for name, cb := range api.circuit {
		svcName := name
		cb.onStateChange = func(from, to CircuitState) {
			log.Printf("🔌 [%s] Circuit: %s → %s", svcName, from, to)
			if to == StateOpen {
				api.metrics.mu.Lock()
				api.metrics.CircuitTrips++
				api.metrics.mu.Unlock()
			}
		}
	}
	return api
}

func (api *OrderAPI) getMode() string {
	api.modeMu.RLock()
	defer api.modeMu.RUnlock()
	return api.mode
}

func (api *OrderAPI) setMode(m string) {
	api.modeMu.Lock()
	defer api.modeMu.Unlock()
	api.mode = m
	log.Printf("🔄 Mode → %s", m)
}

// modeHas checks if a specific pattern is active.
// "all" enables every pattern. Individual modes enable only themselves.
func (api *OrderAPI) modeHas(pattern string) bool {
	mode := api.getMode()
	return mode == pattern || mode == "all"
}

// callService makes a single HTTP call to a downstream service,
// applying whichever resilience patterns are currently active.
func (api *OrderAPI) callService(ep ServiceEndpoint) error {
	mode := api.getMode()
	url := ep.URL + "/process"

	// BULKHEAD
	if mode == "bulkhead" || mode == "all" {
		bh := api.bulkhead[ep.Name]
		if !bh.Acquire(100 * time.Millisecond) {
			api.metrics.mu.Lock()
			api.metrics.BulkRejects++
			api.metrics.mu.Unlock()
			return fmt.Errorf("bulkhead: %s at capacity", ep.Name)
		}
		defer bh.Release()
	}

	// CIRCUIT BREAKER
	if mode == "circuit" || mode == "all" {
		if !api.circuit[ep.Name].Allow() {
			return fmt.Errorf("circuit open: %s rejected", ep.Name)
		}
	}

	// MAKE THE CALL
	// Circuit breaker ALWAYS uses fail-fast client — without a timeout cap,
	// hanging services block goroutines for 30s and the circuit can't count
	// failures fast enough to trip. This mirrors real-world practice: you
	// never run a circuit breaker without a timeout.
	var resp *http.Response
	var err error
	if mode == "failfast" || mode == "circuit" || mode == "all" {
		resp, err = api.ffClient.Call(url)
	} else {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err = client.Get(url)
	}

	// RECORD FOR CIRCUIT BREAKER
	if mode == "circuit" || mode == "all" {
		cb := api.circuit[ep.Name]
		if err != nil {
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}
	}

	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%s returned %d", ep.Name, resp.StatusCode)
	}
	return nil
}

// callServiceWithRetry wraps callService with exponential backoff + jitter.
// This prevents the "thundering herd" problem where all clients retry at
// the exact same instant, re-crashing the recovering service.
func (api *OrderAPI) callServiceWithRetry(ep ServiceEndpoint, cfg RetryConfig) error {
	var lastErr error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		lastErr = api.callService(ep)
		if lastErr == nil {
			return nil
		}

		// Don't retry if circuit is open — it will fail instantly anyway
		if api.modeHas("circuit") {
			if api.circuit[ep.Name].State() == StateOpen {
				return lastErr
			}
		}

		if attempt < cfg.MaxAttempts-1 {
			// Exponential backoff: 100ms → 200ms → 400ms → ...
			delay := cfg.BaseDelay * time.Duration(1<<uint(attempt))
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
			// Jitter: randomize within [delay/2, delay*3/2]
			// This spreads retries across time, preventing thundering herd
			jitter := time.Duration(rand.Int63n(int64(delay)))
			actualDelay := delay/2 + jitter

			api.metrics.mu.Lock()
			api.metrics.Retries++
			api.metrics.mu.Unlock()

			log.Printf("🔁 [%s] Retry %d/%d in %v (backoff=%v, jitter=%v)",
				ep.Name, attempt+1, cfg.MaxAttempts-1, actualDelay, delay, jitter)

			time.Sleep(actualDelay)
		}
	}
	return fmt.Errorf("retry exhausted after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// =============================================================================
// HTTP HANDLERS
// =============================================================================

func (api *OrderAPI) HandleOrder(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	mode := api.getMode()
	useRetry := mode == "retry" || mode == "all"
	useDegraded := mode == "degraded" || mode == "all"

	degraded := []string{} // services that failed but were skipped

	for _, ep := range api.endpoints {
		var err error

		// Choose call strategy based on active patterns
		if useRetry {
			err = api.callServiceWithRetry(ep, DefaultRetryConfig)
		} else {
			err = api.callService(ep)
		}

		if err != nil {
			if useDegraded {
				// GRACEFUL DEGRADATION: accept the order anyway,
				// mark this service as degraded. In production you'd
				// queue the failed step for async retry.
				degraded = append(degraded, ep.Name)
				log.Printf("⚠️ [%s] Degraded: %s (order continues)", ep.Name, err)
				continue // skip to next service in chain
			}

			// No degradation: fail the whole order
			lat := time.Since(start)
			api.metrics.Record(false, lat, lat > 400*time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":      fmt.Sprintf("%s: %s", ep.Name, err),
				"latency_ms": lat.Milliseconds(),
				"mode":       mode,
			})
			return
		}
	}

	lat := time.Since(start)

	if len(degraded) > 0 {
		// Order accepted with partial failures
		api.metrics.Record(true, lat, false)
		api.metrics.mu.Lock()
		api.metrics.DegradedOK++
		api.metrics.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "🍕 Order accepted (degraded mode)",
			"degraded":   degraded,
			"note":       fmt.Sprintf("%s will be retried asynchronously", degraded),
			"latency_ms": lat.Milliseconds(),
			"mode":       mode,
		})
		return
	}

	// Full success — all services responded
	api.metrics.Record(true, lat, false)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "🍕 Order placed!",
		"latency_ms": lat.Milliseconds(),
		"mode":       mode,
	})
}

func (api *OrderAPI) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	api.metrics.mu.Lock()
	defer api.metrics.mu.Unlock()

	median, p95, p99, mean := api.metrics.percentiles.Snapshot()
	sr := 0.0
	if api.metrics.Total > 0 {
		sr = float64(api.metrics.Successes) / float64(api.metrics.Total) * 100
	}

	cbStates := map[string]string{}
	for name, cb := range api.circuit {
		cbStates[name] = cb.State().String()
	}

	bhStats := map[string]interface{}{}
	for name, bh := range api.bulkhead {
		used, max, rejects := bh.Stats()
		bhStats[name] = map[string]int64{"used": int64(used), "max": int64(max), "rejects": rejects}
	}

	// Probe downstream health
	svcHealth := map[string]bool{}
	for _, ep := range api.endpoints {
		client := &http.Client{Timeout: 1 * time.Second}
		resp, err := client.Get(ep.URL + "/health")
		svcHealth[ep.Name] = err == nil && resp != nil && resp.StatusCode == 200
		if resp != nil {
			resp.Body.Close()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mode":              api.getMode(),
		"total_requests":    api.metrics.Total,
		"successes":         api.metrics.Successes,
		"failures":          api.metrics.Failures,
		"timeouts":          api.metrics.Timeouts,
		"circuit_trips":     api.metrics.CircuitTrips,
		"bulkhead_rejects":  api.metrics.BulkRejects,
		"retries":           api.metrics.Retries,
		"degraded_ok":       api.metrics.DegradedOK,
		"success_rate_pct":  math.Round(sr*100) / 100,
		"latency_mean_ms":   math.Round(mean*100) / 100,
		"latency_median_ms": math.Round(median*100) / 100,
		"latency_p95_ms":    math.Round(p95*100) / 100,
		"latency_p99_ms":    math.Round(p99*100) / 100,
		"circuit_breakers":  cbStates,
		"bulkheads":         bhStats,
		"services":          svcHealth,
	})
}

func (api *OrderAPI) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": "order-api"})
}

func (api *OrderAPI) HandleMode(w http.ResponseWriter, r *http.Request) {
	m := r.URL.Query().Get("mode")
	valid := map[string]bool{
		"none": true, "failfast": true, "circuit": true,
		"bulkhead": true, "retry": true, "degraded": true, "all": true,
	}
	if !valid[m] {
		http.Error(w, `{"error":"invalid mode, use: none|failfast|circuit|bulkhead|retry|degraded|all"}`, 400)
		return
	}
	api.setMode(m)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"mode": m})
}

func (api *OrderAPI) HandleReset(w http.ResponseWriter, r *http.Request) {
	api.metrics.Reset()
	for _, cb := range api.circuit {
		cb.Reset()
	}
	for _, bh := range api.bulkhead {
		bh.ResetRejects()
	}
	for _, ep := range api.endpoints {
		client := &http.Client{Timeout: 2 * time.Second}
		client.Post(ep.URL+"/admin/recover", "application/json", nil)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
}

func (api *OrderAPI) HandleCrash(w http.ResponseWriter, r *http.Request) {
	svc := r.URL.Query().Get("service")
	for _, ep := range api.endpoints {
		if ep.Name == svc {
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Post(ep.URL+"/admin/crash", "application/json", nil)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"cannot reach %s: %s"}`, svc, err), 502)
				return
			}
			resp.Body.Close()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"crashed": svc})
			return
		}
	}
	http.Error(w, `{"error":"unknown service, use: inventory|payment|kitchen"}`, 400)
}

func (api *OrderAPI) HandleRecover(w http.ResponseWriter, r *http.Request) {
	svc := r.URL.Query().Get("service")
	for _, ep := range api.endpoints {
		if ep.Name == svc {
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Post(ep.URL+"/admin/recover", "application/json", nil)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"cannot reach %s: %s"}`, svc, err), 502)
				return
			}
			resp.Body.Close()
			if cb, ok := api.circuit[svc]; ok {
				cb.Reset()
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"recovered": svc})
			return
		}
	}
	http.Error(w, `{"error":"unknown service, use: inventory|payment|kitchen"}`, 400)
}

// =============================================================================
// MIDDLEWARE + MAIN
// =============================================================================

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	port := getEnv("PORT", "8080")
	mode := getEnv("MODE", "none")

	endpoints := []ServiceEndpoint{
		{Name: "inventory", URL: getEnv("INVENTORY_URL", "http://localhost:9001")},
		{Name: "payment", URL: getEnv("PAYMENT_URL", "http://localhost:9002")},
		{Name: "kitchen", URL: getEnv("KITCHEN_URL", "http://localhost:9003")},
	}

	api := NewOrderAPI(endpoints, mode)

	mux := http.NewServeMux()
	mux.HandleFunc("/order", api.HandleOrder)
	mux.HandleFunc("/metrics", api.HandleMetrics)
	mux.HandleFunc("/health", api.HandleHealth)
	mux.HandleFunc("/mode", api.HandleMode)
	mux.HandleFunc("/reset", api.HandleReset)
	mux.HandleFunc("/crash", api.HandleCrash)
	mux.HandleFunc("/recover", api.HandleRecover)

	log.Printf(" Order API starting on :%s (mode: %s)", port, mode)
	log.Printf("   Patterns: fail-fast | circuit-breaker | bulkhead | retry+backoff | degraded | all")
	for _, ep := range endpoints {
		log.Printf("   → %s: %s", ep.Name, ep.URL)
	}

	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}