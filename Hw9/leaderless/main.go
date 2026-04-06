package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// KV Entry with logical version
// ---------------------------------------------------------------------------

type KVEntry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

// ---------------------------------------------------------------------------
// In-memory KV Store
// ---------------------------------------------------------------------------

type Store struct {
	mu   sync.RWMutex
	data map[string]KVEntry
}

func NewStore() *Store {
	return &Store{data: make(map[string]KVEntry)}
}

func (s *Store) Get(key string) (KVEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.data[key]
	return entry, ok
}

func (s *Store) Set(key, value string, version int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = KVEntry{Value: value, Version: version}
}

func (s *Store) NextVersion(key string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry, ok := s.data[key]; ok {
		return entry.Version + 1
	}
	return 1
}

// ---------------------------------------------------------------------------
// Node configuration
// ---------------------------------------------------------------------------

type NodeConfig struct {
	NodeID string
	Port   string
	Peers  []string // all other node addresses
}

var (
	store      *Store
	config     NodeConfig
	versionGen int64 // global version counter for this node
	nodeRank   int64
)

// ---------------------------------------------------------------------------
// Request / Response types
// ---------------------------------------------------------------------------

type SetRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ReplicateRequest struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

type ReadResponse struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
	NodeID  string `json:"node_id"`
}

// ---------------------------------------------------------------------------
// SET handler — this node becomes the Write Coordinator
// ---------------------------------------------------------------------------

func handleSet(w http.ResponseWriter, r *http.Request) {
	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "key cannot be empty", http.StatusBadRequest)
		return
	}

	// Encode the local logical counter with the node rank so concurrent coordinators
	// still produce a stable total ordering across the cluster.
	version := nextVersion()

	// Store locally first
	store.Set(req.Key, req.Value, version)

	// As Write Coordinator, propagate to ALL other nodes (W=N)
	// Sequential with 200ms sleep after each message
	allOK := true
	for _, peer := range config.Peers {
		ok := sendReplicateRequest(peer, req.Key, req.Value, version)
		if !ok {
			allOK = false
			log.Printf("WARN: failed to replicate to %s for key=%s", peer, req.Key)
		}
		// Coordinator sleeps 200ms after each message
		time.Sleep(200 * time.Millisecond)
	}

	if !allOK {
		log.Printf("ERROR: not all peers acknowledged write for key=%s", req.Key)
		http.Error(w, "write quorum not satisfied", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ReadResponse{
		Key: req.Key, Value: req.Value, Version: version, NodeID: config.NodeID,
	})
}

// sendReplicateRequest sends a replicate request to a single peer.
func sendReplicateRequest(peer, key, value string, version int64) bool {
	payload, _ := json.Marshal(ReplicateRequest{Key: key, Value: value, Version: version})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("http://%s/replicate", peer),
		"application/json",
		strings.NewReader(string(payload)),
	)
	if err != nil {
		log.Printf("ERROR replicating to %s: %v", peer, err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ---------------------------------------------------------------------------
// /replicate — receive data from a Write Coordinator
// ---------------------------------------------------------------------------

func handleReplicate(w http.ResponseWriter, r *http.Request) {
	var req ReplicateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Receiving node sleeps 100ms before processing
	time.Sleep(100 * time.Millisecond)

	// Only update if version is newer
	existing, ok := store.Get(req.Key)
	if !ok || req.Version > existing.Version {
		store.Set(req.Key, req.Value, req.Version)
	}

	updateVersionFloor(req.Version)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// GET handler — R=1, return local value
// ---------------------------------------------------------------------------

func handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	entry, ok := store.Get(key)
	if !ok {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ReadResponse{
		Key: key, Value: entry.Value, Version: entry.Version, NodeID: config.NodeID,
	})
}

// ---------------------------------------------------------------------------
// /local_read — sneaky test endpoint (same as get for leaderless, but explicit)
// ---------------------------------------------------------------------------

func localRead(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	entry, ok := store.Get(key)
	if !ok {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ReadResponse{
		Key: key, Value: entry.Value, Version: entry.Version, NodeID: config.NodeID,
	})
}

// ---------------------------------------------------------------------------
// /health
// ---------------------------------------------------------------------------

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"node_id": config.NodeID,
		"role":    "leaderless",
	})
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	store = NewStore()

	config.NodeID = getEnv("NODE_ID", "node1")
	config.Port = getEnv("PORT", "8080")
	nodeRank = parseNodeRank(config.NodeID)

	peersStr := getEnv("PEERS", "")
	if peersStr != "" {
		config.Peers = strings.Split(peersStr, ",")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/set", handleSet)
	mux.HandleFunc("/get", handleGet)
	mux.HandleFunc("/local_read", localRead)
	mux.HandleFunc("/replicate", handleReplicate)
	mux.HandleFunc("/health", healthHandler)

	log.Printf("Starting LEADERLESS node %s on :%s (peers=%v)", config.NodeID, config.Port, config.Peers)
	log.Fatal(http.ListenAndServe(":"+config.Port, mux))
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func nextVersion() int64 {
	counter := atomic.AddInt64(&versionGen, 1)
	return counter*10 + nodeRank
}

func updateVersionFloor(version int64) {
	counterFloor := version / 10
	for {
		cur := atomic.LoadInt64(&versionGen)
		if counterFloor <= cur {
			return
		}
		if atomic.CompareAndSwapInt64(&versionGen, cur, counterFloor) {
			return
		}
	}
}

func parseNodeRank(nodeID string) int64 {
	digits := strings.TrimLeftFunc(nodeID, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if digits == "" {
		return 0
	}
	rank, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0
	}
	return rank
}
