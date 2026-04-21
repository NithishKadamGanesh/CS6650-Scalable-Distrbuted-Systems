package main

// Routing Service — Kafka consumer that makes order routing decisions.
// Reads from "orders" topic, applies routing logic (optionally with
// optimistic/pessimistic concurrency), writes audit log to Postgres,
// parks unroutable orders in SQS dead-letter queue.

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nithishkadam/warehouseflow/routing-service/consumer"
	"github.com/nithishkadam/warehouseflow/routing-service/router"
	"github.com/nithishkadam/warehouseflow/routing-service/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg := loadConfig()

	// ── Postgres ──
	pg, err := store.NewPostgres(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer pg.Close()
	if err := pg.Migrate(); err != nil {
		log.Fatalf("DB migration failed: %v", err)
	}

	// ── Redis warehouse nodes ──
	warehouses := store.NewWarehouseRegistry([]store.WarehouseConfig{
		{ID: "warehouse-a", Region: "US-EAST", RedisAddr: cfg.RedisAddrA},
		{ID: "warehouse-b", Region: "US-CENTRAL", RedisAddr: cfg.RedisAddrB},
		{ID: "warehouse-c", Region: "US-WEST", RedisAddr: cfg.RedisAddrC},
	})
	defer warehouses.CloseAll()

	if err := warehouses.SeedInventory(context.Background()); err != nil {
		log.Printf("Warning: inventory seed failed: %v", err)
	}

	// ── Routing engine (with optional resilience patterns) ──
	engine := router.NewEngine(warehouses, pg)

	// Choose strategy based on env var; default is the base engine
	var strategyEngine router.Router = engine
	switch cfg.Strategy {
	case "optimistic":
		strategyEngine = router.NewOptimisticEngine(warehouses, pg)
		log.Println("Using OPTIMISTIC concurrency strategy (CAS retry)")
	case "pessimistic":
		strategyEngine = router.NewPessimisticEngine(warehouses, pg)
		log.Println("Using PESSIMISTIC concurrency strategy (SETNX lock)")
	default:
		log.Println("Using BASE routing engine (atomic Lua decrement)")
	}

	// ── Kafka consumer ──
	c := consumer.New(cfg.KafkaBroker, cfg.KafkaTopic, cfg.KafkaGroupID, strategyEngine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		log.Println("Starting Kafka consumer...")
		if err := c.Start(ctx); err != nil {
			log.Printf("Consumer stopped: %v", err)
		}
	}()

	// ── Metrics + health endpoint ──
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"healthy","service":"routing"}`))
	})

	srv := &http.Server{Addr: ":" + cfg.Port}
	go func() {
		log.Printf("Routing service metrics on :%s", cfg.Port)
		srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down routing service...")
	cancel()
	time.Sleep(2 * time.Second)
	srv.Shutdown(context.Background())
}

type config struct {
	KafkaBroker  string
	KafkaTopic   string
	KafkaGroupID string
	RedisAddrA   string
	RedisAddrB   string
	RedisAddrC   string
	PostgresDSN  string
	Port         string
	Strategy     string
}

func loadConfig() config {
	return config{
		KafkaBroker:  getEnv("KAFKA_BROKER", "localhost:9092"),
		KafkaTopic:   getEnv("KAFKA_TOPIC", "orders"),
		KafkaGroupID: getEnv("KAFKA_GROUP_ID", "routing-service"),
		RedisAddrA:   getEnv("REDIS_ADDR_A", "localhost:6379"),
		RedisAddrB:   getEnv("REDIS_ADDR_B", "localhost:6380"),
		RedisAddrC:   getEnv("REDIS_ADDR_C", "localhost:6381"),
		PostgresDSN:  getEnv("POSTGRES_DSN", "postgres://warehouseflow:warehouseflow@localhost:5432/warehouseflow?sslmode=disable"),
		Port:         getEnv("PORT", "8082"),
		Strategy:     getEnv("CONCURRENCY_STRATEGY", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
