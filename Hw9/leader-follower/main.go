package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.data[key]; ok {
		next := entry.Version + 1
		s.data[key] = KVEntry{Value: entry.Value, Version: next}
		return next
	}
	s.data[key] = KVEntry{Version: 1}
	return 1
}

func (s *Store) UpdateValue(key, value string, version int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = KVEntry{Value: value, Version: version}
}

// ---------------------------------------------------------------------------
// Node configuration
// ---------------------------------------------------------------------------

type NodeConfig struct {
	Role      string   // "leader" or "follower"
	NodeID    string   // e.g. "node1"
	Port      string   // e.g. "8080"
	Peers     []string // follower addresses (for leader) or empty
	LeaderURL string   // leader address (for follower)
	W         int      // write quorum
	R         int      // read quorum
}

var (
	store  *Store
	config NodeConfig
	cfgMu  sync.RWMutex
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

type ConfigRequest struct {
	W int `json:"w"`
	R int `json:"r"`
}

// ---------------------------------------------------------------------------
// Leader: handle SET
// ---------------------------------------------------------------------------

func leaderSet(w http.ResponseWriter, r *http.Request) {
	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "key cannot be empty", http.StatusBadRequest)
		return
	}

	// Assign next version and store locally
	version := store.NextVersion(req.Key)
	store.UpdateValue(req.Key, req.Value, version)

	cfgMu.RLock()
	wVal := config.W
	peers := config.Peers
	cfgMu.RUnlock()

	// W=1 means only leader needs to be updated; replicate async to followers
	if wVal <= 1 {
		go replicateToFollowers(req.Key, req.Value, version, peers, len(peers))
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ReadResponse{
			Key: req.Key, Value: req.Value, Version: version, NodeID: config.NodeID,
		})
		return
	}

	// For W > 1, we need (W-1) follower acks synchronously
	// (leader itself counts as 1 in the quorum)
	neededAcks := wVal - 1
	if neededAcks > len(peers) {
		neededAcks = len(peers)
	}

	// Replicate sequentially with 200ms sleep between each follower
	acksReceived := 0
	for i, peer := range peers {
		if acksReceived >= neededAcks {
			// We have enough acks; replicate remaining async
			go replicateToFollowers(req.Key, req.Value, version, peers[i:], len(peers[i:]))
			break
		}
		ok := sendReplicateRequest(peer, req.Key, req.Value, version)
		if ok {
			acksReceived++
		}
		// Sleep 200ms after each message to a follower
		time.Sleep(200 * time.Millisecond)
	}

	// *** FIX: enforce quorum — refuse to ack the write if we didn't meet W ***
	if acksReceived < neededAcks {
		log.Printf("ERROR: quorum not met: got %d/%d follower acks for key=%s (W=%d)",
			acksReceived, neededAcks, req.Key, wVal)
		http.Error(w, fmt.Sprintf("write quorum not met: needed %d acks, got %d",
			neededAcks, acksReceived), http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ReadResponse{
		Key: req.Key, Value: req.Value, Version: version, NodeID: config.NodeID,
	})
}

// replicateToFollowers sends updates to a slice of peers sequentially.
func replicateToFollowers(key, value string, version int64, peers []string, count int) {
	for i := 0; i < count && i < len(peers); i++ {
		sendReplicateRequest(peers[i], key, value, version)
		time.Sleep(200 * time.Millisecond)
	}
}

// sendReplicateRequest sends a replicate request to a single follower.
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
// Leader: handle GET (read coordination)
// ---------------------------------------------------------------------------

func leaderGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	cfgMu.RLock()
	rVal := config.R
	peers := config.Peers
	cfgMu.RUnlock()

	// Always include leader's own value
	localEntry, localOK := store.Get(key)
	best := ReadResponse{Key: key, Value: localEntry.Value, Version: localEntry.Version, NodeID: config.NodeID}

	if rVal <= 1 {
		// R=1: just return local value
		if !localOK {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(best)
		return
	}

	// R > 1: query (R-1) followers in parallel, return highest version
	neededReads := rVal - 1
	if neededReads > len(peers) {
		neededReads = len(peers)
	}

	type result struct {
		resp ReadResponse
		ok   bool
	}
	ch := make(chan result, neededReads)

	for i := 0; i < neededReads; i++ {
		go func(peer string) {
			resp, ok := sendReadRequest(peer, key)
			ch <- result{resp: resp, ok: ok}
		}(peers[i])
	}

	found := localOK
	for i := 0; i < neededReads; i++ {
		res := <-ch
		if res.ok {
			found = true
			if res.resp.Version > best.Version {
				best = res.resp
			}
		}
	}

	if !found {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(best)
}

// sendReadRequest asks a follower for its value of a key.
func sendReadRequest(peer, key string) (ReadResponse, bool) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/read_request?key=%s", peer, key))
	if err != nil {
		return ReadResponse{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ReadResponse{}, false
	}
	var rr ReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return ReadResponse{}, false
	}
	return rr, true
}

// ---------------------------------------------------------------------------
// Follower: handle /replicate (leader pushes data)
// ---------------------------------------------------------------------------

func followerReplicate(w http.ResponseWriter, r *http.Request) {
	var req ReplicateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Follower sleeps 100ms before processing update
	time.Sleep(100 * time.Millisecond)

	// Only update if incoming version is newer
	existing, ok := store.Get(req.Key)
	if !ok || req.Version > existing.Version {
		store.Set(req.Key, req.Value, req.Version)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Follower: handle /read_request (leader queries for quorum reads)
// ---------------------------------------------------------------------------

func followerReadRequest(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	// Follower sleeps 50ms before responding to read request from leader
	time.Sleep(50 * time.Millisecond)

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
// Shared: /local_read — sneaky test endpoint, returns local value directly
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
// /configure — dynamically change W and R
// ---------------------------------------------------------------------------

func configureHandler(w http.ResponseWriter, r *http.Request) {
	var req ConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	cfgMu.Lock()
	nextW := config.W
	nextR := config.R
	clusterSize := len(config.Peers) + 1
	if req.W > 0 {
		nextW = req.W
	}
	if req.R > 0 {
		nextR = req.R
	}
	if nextW < 1 || nextR < 1 || nextW > clusterSize || nextR > clusterSize {
		cfgMu.Unlock()
		http.Error(w, "invalid quorum values for cluster size", http.StatusBadRequest)
		return
	}
	config.W = nextW
	config.R = nextR
	cfgMu.Unlock()

	log.Printf("Configuration updated: W=%d, R=%d", nextW, nextR)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]int{"w": nextW, "r": nextR})
}

// ---------------------------------------------------------------------------
// /health — health check
// ---------------------------------------------------------------------------

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"node_id": config.NodeID,
		"role":    config.Role,
	})
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	store = NewStore()

	config.Role = getEnv("NODE_ROLE", "leader")
	config.NodeID = getEnv("NODE_ID", "node1")
	config.Port = getEnv("PORT", "8080")
	config.LeaderURL = getEnv("LEADER_URL", "")

	peersStr := getEnv("PEERS", "")
	if peersStr != "" {
		config.Peers = strings.Split(peersStr, ",")
	}

	wVal, _ := strconv.Atoi(getEnv("W_VALUE", "5"))
	rVal, _ := strconv.Atoi(getEnv("R_VALUE", "1"))
	config.W = wVal
	config.R = rVal

	mux := http.NewServeMux()

	// Shared endpoints
	mux.HandleFunc("/local_read", localRead)
	mux.HandleFunc("/configure", configureHandler)
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/replicate", followerReplicate)
	mux.HandleFunc("/read_request", followerReadRequest)

	if config.Role == "leader" {
		mux.HandleFunc("/set", leaderSet)
		mux.HandleFunc("/get", leaderGet)
		log.Printf("Starting LEADER node %s on :%s (W=%d, R=%d, followers=%v)",
			config.NodeID, config.Port, config.W, config.R, config.Peers)
	} else {
		// Followers also expose /set and /get that forward to leader
		mux.HandleFunc("/set", followerForwardSet)
		mux.HandleFunc("/get", followerGet)
		log.Printf("Starting FOLLOWER node %s on :%s (leader=%s)",
			config.NodeID, config.Port, config.LeaderURL)
	}

	log.Fatal(http.ListenAndServe(":"+config.Port, mux))
}

// ---------------------------------------------------------------------------
// Follower: forward /set to leader
// ---------------------------------------------------------------------------

func followerForwardSet(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "writes must be sent to the leader node", http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// Follower: handle /get directly (returns local for R=1 scenarios)
// ---------------------------------------------------------------------------

func followerGet(w http.ResponseWriter, r *http.Request) {
	if config.LeaderURL == "" {
		http.Error(w, "leader address not configured", http.StatusServiceUnavailable)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/get?key=%s", config.LeaderURL, key))
	if err != nil {
		http.Error(w, "failed to forward read to leader", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read leader response", http.StatusBadGateway)
		return
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
