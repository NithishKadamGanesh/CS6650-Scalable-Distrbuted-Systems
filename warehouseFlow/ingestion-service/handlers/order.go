package handlers

// Order submission handler. Validates payload, assigns UUID,
// serializes to JSON, and publishes to Kafka via injected producer.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Publisher is the interface the handler depends on. Enables testing
// with mock producers without needing a real Kafka broker.
type Publisher interface {
	Publish(ctx context.Context, key string, value []byte) error
}

var (
	ordersReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_orders_received_total",
		Help: "Total number of orders received by the ingestion service",
	})
	ordersPublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_orders_published_total",
		Help: "Total number of orders successfully published to Kafka",
	})
	orderPublishErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_order_publish_errors_total",
		Help: "Total number of Kafka publish failures",
	})
	ingestionLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "warehouseflow_ingestion_latency_seconds",
		Help:    "Latency from HTTP receive to Kafka publish",
		Buckets: prometheus.DefBuckets,
	})
)

// OrderRequest is the incoming payload from a client.
type OrderRequest struct {
	CustomerID string `json:"customer_id"`
	SKU        string `json:"sku"`
	Quantity   int    `json:"quantity"`
	Region     string `json:"region"`
}

// OrderEvent is the enriched event published to Kafka.
type OrderEvent struct {
	OrderID    string    `json:"order_id"`
	CustomerID string    `json:"customer_id"`
	SKU        string    `json:"sku"`
	Quantity   int       `json:"quantity"`
	Region     string    `json:"region"`
	ReceivedAt time.Time `json:"received_at"`
}

// OrderHandler handles HTTP order submission.
type OrderHandler struct {
	producer Publisher
}

// NewOrderHandler constructs a new handler with the given publisher.
func NewOrderHandler(producer Publisher) *OrderHandler {
	return &OrderHandler{producer: producer}
}

// SubmitOrder handles POST /api/v1/orders.
func (h *OrderHandler) SubmitOrder(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ordersReceived.Inc()

	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.CustomerID == "" || req.SKU == "" || req.Quantity <= 0 {
		http.Error(w, `{"error":"customer_id, sku, and quantity > 0 are required"}`, http.StatusBadRequest)
		return
	}

	event := OrderEvent{
		OrderID:    uuid.New().String(),
		CustomerID: req.CustomerID,
		SKU:        req.SKU,
		Quantity:   req.Quantity,
		Region:     req.Region,
		ReceivedAt: time.Now().UTC(),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal order event: %v", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if err := h.producer.Publish(r.Context(), event.OrderID, payload); err != nil {
		log.Printf("Failed to publish order %s to Kafka: %v", event.OrderID, err)
		orderPublishErrors.Inc()
		http.Error(w, `{"error":"failed to queue order"}`, http.StatusInternalServerError)
		return
	}

	ordersPublished.Inc()
	ingestionLatency.Observe(time.Since(start).Seconds())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"order_id": event.OrderID,
		"status":   "queued",
	})
}
