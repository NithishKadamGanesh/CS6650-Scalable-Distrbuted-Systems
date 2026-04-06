package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

type Config struct {
	WriteNodes  []string // node(s) to send writes to
	ReadNodes   []string // node(s) to send reads to
	WritePct    int      // write percentage (1, 10, 50, 90)
	NumRequests int      // total number of requests
	Concurrency int      // number of concurrent workers
	NumKeys     int      // number of distinct keys (small = more temporal locality)
	OutputFile  string   // CSV output file prefix
}

// ---------------------------------------------------------------------------
// Response type
// ---------------------------------------------------------------------------

type ReadResponse struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
	NodeID  string `json:"node_id"`
}

// ---------------------------------------------------------------------------
// Result tracking
// ---------------------------------------------------------------------------

type RequestResult struct {
	Type       string // "read" or "write"
	Key        string
	Latency    time.Duration
	StatusCode int
	Version    int64
	Stale      bool
	Timestamp  time.Time
}

// Track the latest version written for each key (client-side)
type VersionTracker struct {
	mu       sync.RWMutex
	versions map[string]int64
	issued   map[string]int64
	// Track write timestamps for measuring read-write interval
	writeTimes map[string]time.Time
}

func NewVersionTracker() *VersionTracker {
	return &VersionTracker{
		versions:   make(map[string]int64),
		issued:     make(map[string]int64),
		writeTimes: make(map[string]time.Time),
	}
}

func (vt *VersionTracker) RecordIssuedWrite(key string, clientSeq int64) {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	vt.issued[key] = clientSeq
	vt.writeTimes[key] = time.Now()
}

func (vt *VersionTracker) UpdateWrite(key string, version int64) {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	vt.versions[key] = version
}

func (vt *VersionTracker) IsStale(key string, value string) bool {
	vt.mu.RLock()
	defer vt.mu.RUnlock()
	observedSeq, ok := parseClientSeq(value)
	if !ok {
		return false
	}
	if latest, ok := vt.issued[key]; ok {
		return observedSeq < latest
	}
	return false
}

func (vt *VersionTracker) GetWriteTime(key string) (time.Time, bool) {
	vt.mu.RLock()
	defer vt.mu.RUnlock()
	t, ok := vt.writeTimes[key]
	return t, ok
}

// ---------------------------------------------------------------------------
// Load generator
// ---------------------------------------------------------------------------

func main() {
	cfg := parseConfig()

	log.Printf("Load Test Config: writes=%d%%, reads=%d%%, requests=%d, concurrency=%d, keys=%d",
		cfg.WritePct, 100-cfg.WritePct, cfg.NumRequests, cfg.Concurrency, cfg.NumKeys)
	log.Printf("Write nodes: %v", cfg.WriteNodes)
	log.Printf("Read nodes: %v", cfg.ReadNodes)

	tracker := NewVersionTracker()
	var results []RequestResult
	var resultsMu sync.Mutex
	var staleCount int64
	var clientSeq int64

	// Work queue
	work := make(chan int, cfg.NumRequests)
	for i := 0; i < cfg.NumRequests; i++ {
		work <- i
	}
	close(work)

	var wg sync.WaitGroup

	startTime := time.Now()

	for w := 0; w < cfg.Concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			client := &http.Client{Timeout: 10 * time.Second}

			for range work {
				// Decide read or write
				isWrite := rng.Intn(100) < cfg.WritePct

				// Pick a key from the small key space
				keyIdx := rng.Intn(cfg.NumKeys)
				key := fmt.Sprintf("key_%d", keyIdx)

				var result RequestResult
				result.Key = key
				result.Timestamp = time.Now()

				if isWrite {
					result.Type = "write"
					issuedSeq := atomic.AddInt64(&clientSeq, 1)
					value := fmt.Sprintf("seq_%d_val_%d_%d", issuedSeq, keyIdx, time.Now().UnixNano())
					tracker.RecordIssuedWrite(key, issuedSeq)

					// Send write to a write node
					node := cfg.WriteNodes[rng.Intn(len(cfg.WriteNodes))]
					start := time.Now()
					resp, statusCode, err := doSet(client, node, key, value)
					result.Latency = time.Since(start)
					result.StatusCode = statusCode

					if err != nil {
					} else {
						result.Version = resp.Version
						tracker.UpdateWrite(key, resp.Version)
					}
				} else {
					result.Type = "read"

					// Send read to a random read node
					node := cfg.ReadNodes[rng.Intn(len(cfg.ReadNodes))]
					start := time.Now()
					resp, statusCode, err := doGet(client, node, key)
					result.Latency = time.Since(start)
					result.StatusCode = statusCode

					if err == nil && statusCode == 200 {
						result.Version = resp.Version
						if tracker.IsStale(key, resp.Value) {
							result.Stale = true
							atomic.AddInt64(&staleCount, 1)
						}
					}
				}

				resultsMu.Lock()
				results = append(results, result)
				resultsMu.Unlock()
			}
		}(w)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	// Summary
	var readCount, writeCount int
	var readLatSum, writeLatSum time.Duration
	for _, r := range results {
		if r.Type == "read" {
			readCount++
			readLatSum += r.Latency
		} else {
			writeCount++
			writeLatSum += r.Latency
		}
	}

	log.Printf("===== RESULTS =====")
	log.Printf("Total time: %v", elapsed)
	log.Printf("Requests: %d total (%d writes, %d reads)", len(results), writeCount, readCount)
	if writeCount > 0 {
		log.Printf("Avg write latency: %v", writeLatSum/time.Duration(writeCount))
	}
	if readCount > 0 {
		log.Printf("Avg read latency: %v", readLatSum/time.Duration(readCount))
	}
	log.Printf("Stale reads: %d", staleCount)
	log.Printf("Throughput: %.1f req/s", float64(len(results))/elapsed.Seconds())

	// Write results to CSV
	writeCSV(cfg.OutputFile, results, tracker)
	log.Printf("Results written to %s", cfg.OutputFile)
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func doSet(client *http.Client, node, key, value string) (ReadResponse, int, error) {
	payload := fmt.Sprintf(`{"key":"%s","value":"%s"}`, key, value)
	resp, err := client.Post(
		fmt.Sprintf("http://%s/set", node),
		"application/json",
		strings.NewReader(payload),
	)
	if err != nil {
		return ReadResponse{}, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return ReadResponse{}, resp.StatusCode, errors.New(resp.Status)
	}

	var rr ReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return ReadResponse{}, resp.StatusCode, err
	}
	return rr, resp.StatusCode, nil
}

func doGet(client *http.Client, node, key string) (ReadResponse, int, error) {
	resp, err := client.Get(fmt.Sprintf("http://%s/get?key=%s", node, key))
	if err != nil {
		return ReadResponse{}, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ReadResponse{}, resp.StatusCode, nil
	}

	var rr ReadResponse
	json.NewDecoder(resp.Body).Decode(&rr)
	return rr, resp.StatusCode, nil
}

// ---------------------------------------------------------------------------
// CSV output
// ---------------------------------------------------------------------------

func writeCSV(filename string, results []RequestResult, tracker *VersionTracker) {
	f, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Header
	w.Write([]string{"type", "key", "latency_ms", "status_code", "version", "stale", "timestamp_unix_ns", "rw_interval_ms"})

	for _, r := range results {
		rwInterval := ""
		if r.Type == "read" {
			if wt, ok := tracker.GetWriteTime(r.Key); ok {
				interval := r.Timestamp.Sub(wt)
				if interval > 0 {
					rwInterval = fmt.Sprintf("%.2f", float64(interval.Microseconds())/1000.0)
				}
			}
		}

		w.Write([]string{
			r.Type,
			r.Key,
			fmt.Sprintf("%.2f", float64(r.Latency.Microseconds())/1000.0),
			strconv.Itoa(r.StatusCode),
			strconv.FormatInt(r.Version, 10),
			strconv.FormatBool(r.Stale),
			strconv.FormatInt(r.Timestamp.UnixNano(), 10),
			rwInterval,
		})
	}
}

// ---------------------------------------------------------------------------
// Config parsing from env / args
// ---------------------------------------------------------------------------

func parseConfig() Config {
	cfg := Config{
		WritePct:    getEnvInt("WRITE_PCT", 10),
		NumRequests: getEnvInt("NUM_REQUESTS", 1000),
		Concurrency: getEnvInt("CONCURRENCY", 10),
		NumKeys:     getEnvInt("NUM_KEYS", 10),
		OutputFile:  getEnv("OUTPUT_FILE", "results.csv"),
	}

	writeNodesStr := getEnv("WRITE_NODES", "localhost:8080")
	cfg.WriteNodes = strings.Split(writeNodesStr, ",")

	readNodesStr := getEnv("READ_NODES", "localhost:8080,localhost:8081,localhost:8082,localhost:8083,localhost:8084")
	cfg.ReadNodes = strings.Split(readNodesStr, ",")

	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func parseClientSeq(value string) (int64, bool) {
	if !strings.HasPrefix(value, "seq_") {
		return 0, false
	}
	rest := strings.TrimPrefix(value, "seq_")
	idx := strings.Index(rest, "_")
	if idx <= 0 {
		return 0, false
	}
	seq, err := strconv.ParseInt(rest[:idx], 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}
