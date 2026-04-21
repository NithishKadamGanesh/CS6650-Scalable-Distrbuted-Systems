package main

// Ingestion Service — HTTP entry point for WarehouseFlow.
// Accepts POST /api/v1/orders, validates payload, assigns UUID,
// and publishes enriched OrderEvent to Kafka "orders" topic.

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/nithishkadam/warehouseflow/ingestion-service/handlers"
	"github.com/nithishkadam/warehouseflow/ingestion-service/kafka"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	brokerAddr := getEnv("KAFKA_BROKER", "localhost:9092")
	topic := getEnv("KAFKA_TOPIC", "orders")
	port := getEnv("PORT", "8081")

	producer, err := kafka.NewProducer(brokerAddr, topic)
	if err != nil {
		log.Fatalf("Failed to create Kafka producer: %v", err)
	}
	defer producer.Close()

	orderHandler := handlers.NewOrderHandler(producer)

	r := mux.NewRouter()
	r.HandleFunc("/health", healthHandler).Methods("GET")
	r.HandleFunc("/api/v1/orders", orderHandler.SubmitOrder).Methods("POST")
	r.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		log.Printf("Ingestion service listening on :%s (kafka=%s topic=%s)", port, brokerAddr, topic)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down ingestion service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy","service":"ingestion"}`))
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
