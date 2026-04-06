package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/google/uuid"
)

// ============================================================================
// Domain Types
// ============================================================================

type Item struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"` // pending, processing, completed
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

type OrderRequest struct {
	CustomerID int    `json:"customer_id"`
	Items      []Item `json:"items"`
}

type SyncResponse struct {
	Order   Order  `json:"order"`
	Message string `json:"message"`
}

type AsyncResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// ============================================================================
// Metrics (simple atomic counters for observability)
// ============================================================================

var (
	syncTotal      int64
	syncSuccess    int64
	syncFailed     int64
	asyncTotal     int64
	asyncPublished int64
	asyncFailed    int64
)

// ============================================================================
// Payment Processor Simulation (Buffered Channel as Semaphore)
// ============================================================================
//
// KEY CONCEPT (from Effective Go):
//   A buffered channel can be used as a semaphore. The capacity of the channel
//   buffer limits the number of simultaneous calls that can be processed.
//
// Why not just time.Sleep(3s)?
//   When a goroutine sleeps, Go's runtime simply parks it — the OS thread is
//   NOT blocked. The Go scheduler can run thousands of sleeping goroutines on a
//   handful of threads. This means time.Sleep alone doesn't simulate a real
//   bottleneck: the HTTP server would happily accept unlimited concurrent
//   requests, each sleeping independently.
//
// The buffered channel approach:
//   - Channel capacity = max concurrent payments the processor can handle
//   - Sending to a full channel BLOCKS the goroutine until a slot opens
//   - This creates genuine backpressure: if the payment processor is at
//     capacity, new requests must wait — just like a real slow service
//
// With capacity=1 and 3s per payment: throughput = 1/3 ≈ 0.33 orders/sec
//

var paymentSem = make(chan struct{}, 1) // capacity=1 → one payment at a time

// simulatePaymentProcessing acquires the semaphore, simulates the 3-second
// payment verification, then releases the slot. Any goroutine that calls
// this while the channel is full will block on the send — creating the
// realistic bottleneck the assignment asks for.
func simulatePaymentProcessing(order *Order) error {
	order.Status = "processing"

	// Acquire a slot — blocks if the channel buffer is full
	paymentSem <- struct{}{}
	defer func() { <-paymentSem }() // Release the slot when done

	// Simulate the 3-second payment verification delay
	time.Sleep(3 * time.Second)

	order.Status = "completed"
	return nil
}

// ============================================================================
// SNS Client (for async path)
// ============================================================================

var (
	snsClient *sns.Client
	topicARN  string
)

func initSNS() {
	topicARN = os.Getenv("SNS_TOPIC_ARN")
	if topicARN == "" {
		log.Println("WARNING: SNS_TOPIC_ARN not set — async endpoint will fail")
		return
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	snsClient = sns.NewFromConfig(cfg)
	log.Printf("SNS client initialized — topic: %s, region: %s", topicARN, region)
}

// ============================================================================
// HTTP Handlers
// ============================================================================

// GET /health — ALB health check
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "order-receiver",
	})
}

// GET /metrics — simple observability endpoint
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{
		"sync_total":      atomic.LoadInt64(&syncTotal),
		"sync_success":    atomic.LoadInt64(&syncSuccess),
		"sync_failed":     atomic.LoadInt64(&syncFailed),
		"async_total":     atomic.LoadInt64(&asyncTotal),
		"async_published": atomic.LoadInt64(&asyncPublished),
		"async_failed":    atomic.LoadInt64(&asyncFailed),
	})
}

// POST /orders/sync — Synchronous order processing
//
// Flow: Client → API → Payment (3s blocked) → 200 OK
//
// Under load, the buffered channel semaphore causes requests to queue up.
// With capacity=1: only 1 payment processes at a time.
// At 20 concurrent users, most requests wait 3s * position-in-queue.
// Many will timeout or the server will run out of handler goroutines.
func syncOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	atomic.AddInt64(&syncTotal, 1)

	// Parse request
	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		atomic.AddInt64(&syncFailed, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "invalid_request",
			Message: "Could not parse order request",
		})
		return
	}

	// Create the order
	order := Order{
		OrderID:    uuid.New().String(),
		CustomerID: req.CustomerID,
		Status:     "pending",
		Items:      req.Items,
		CreatedAt:  time.Now(),
	}

	log.Printf("[SYNC] Processing order %s for customer %d", order.OrderID, order.CustomerID)

	// *** THIS IS THE BOTTLENECK ***
	// The call below blocks until a semaphore slot is available AND the 3s
	// payment processing completes. Under flash-sale load (20+ concurrent
	// users), most requests will be stuck here waiting for the single slot.
	if err := simulatePaymentProcessing(&order); err != nil {
		atomic.AddInt64(&syncFailed, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "payment_failed",
			Message: "Payment verification failed",
		})
		return
	}

	atomic.AddInt64(&syncSuccess, 1)
	log.Printf("[SYNC] Order %s completed", order.OrderID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SyncResponse{
		Order:   order,
		Message: "Order processed successfully",
	})
}

// POST /orders/async — Asynchronous order processing
//
// Flow: Client → API → SNS Publish → 202 Accepted  (< 100ms)
//                          ↓
//              SQS Queue → Background Workers → Payment (3s)
//
// The client gets a near-instant acknowledgment. Actual payment happens
// asynchronously via the order-processor service polling SQS.
func asyncOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	atomic.AddInt64(&asyncTotal, 1)

	// Parse request
	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		atomic.AddInt64(&asyncFailed, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "invalid_request",
			Message: "Could not parse order request",
		})
		return
	}

	// Create the order with "pending" status
	order := Order{
		OrderID:    uuid.New().String(),
		CustomerID: req.CustomerID,
		Status:     "pending",
		Items:      req.Items,
		CreatedAt:  time.Now(),
	}

	// Publish to SNS (which fans out to SQS)
	orderJSON, err := json.Marshal(order)
	if err != nil {
		atomic.AddInt64(&asyncFailed, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "serialization_error",
			Message: "Failed to serialize order",
		})
		return
	}

	msgBody := string(orderJSON)
	_, err = snsClient.Publish(context.Background(), &sns.PublishInput{
		TopicArn: &topicARN,
		Message:  &msgBody,
	})
	if err != nil {
		atomic.AddInt64(&asyncFailed, 1)
		log.Printf("[ASYNC] Failed to publish order %s: %v", order.OrderID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "publish_failed",
			Message: "Failed to queue order for processing",
		})
		return
	}

	atomic.AddInt64(&asyncPublished, 1)
	log.Printf("[ASYNC] Order %s accepted and queued", order.OrderID)

	// Return 202 Accepted — order is queued, not yet processed
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(AsyncResponse{
		OrderID: order.OrderID,
		Status:  "accepted",
		Message: "Order accepted and queued for processing",
	})
}

// ============================================================================
// Main
// ============================================================================

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Initialize SNS for async path
	initSNS()

	// Register routes
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)
	http.HandleFunc("/orders/sync", syncOrderHandler)
	http.HandleFunc("/orders/async", asyncOrderHandler)

	log.Printf("Order Receiver starting on port %s", port)
	log.Printf("  POST /orders/sync   → synchronous (3s payment delay)")
	log.Printf("  POST /orders/async  → asynchronous (SNS → SQS)")
	log.Printf("  GET  /health        → health check")
	log.Printf("  GET  /metrics       → request counters")

	fmt.Println("Server running...")
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
