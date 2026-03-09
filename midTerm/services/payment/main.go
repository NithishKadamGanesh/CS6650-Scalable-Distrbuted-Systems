package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type ServiceState struct {
	mu        sync.RWMutex
	healthy   bool
	name      string
	baseDelay time.Duration
}

func (s *ServiceState) IsHealthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.healthy
}

func (s *ServiceState) SetHealthy(h bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthy = h
	if h {
		log.Printf("✅ [%s] Service RECOVERED", s.name)
	} else {
		log.Printf("💥 [%s] Service CRASHED!", s.name)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	name := getEnv("SERVICE_NAME", "Payment")
	delayMs, _ := strconv.Atoi(getEnv("BASE_DELAY_MS", "60"))
	port := getEnv("PORT", "8080")

	svc := &ServiceState{
		healthy:   true,
		name:      name,
		baseDelay: time.Duration(delayMs) * time.Millisecond,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if svc.IsHealthy() {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": name})
		} else {
			w.WriteHeader(503)
			json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "service": name})
		}
	})

	mux.HandleFunc("/process", func(w http.ResponseWriter, r *http.Request) {
		if !svc.IsHealthy() {
			if rand.Float64() < 0.5 {
				time.Sleep(8 * time.Second)
			}
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   fmt.Sprintf("%s service unavailable", name),
				"service": name,
			})
			return
		}

		jitter := time.Duration(rand.Intn(20)) * time.Millisecond
		time.Sleep(svc.baseDelay + jitter)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"service": name, "status": "ok"})
	})

	mux.HandleFunc("/admin/crash", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		svc.SetHealthy(false)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "crashed", "service": name})
	})

	mux.HandleFunc("/admin/recover", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		svc.SetHealthy(true)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "recovered", "service": name})
	})

	log.Printf("🍕 [%s] Starting on :%s (base delay: %dms)", name, port, delayMs)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
