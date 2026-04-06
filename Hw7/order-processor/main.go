package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// ============================================================================
// Domain Types (same as order-receiver)
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
	Status     string    `json:"status"`
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

// SNS wraps the message body in an envelope when delivering to SQS
type SNSEnvelope struct {
	Message string `json:"Message"`
}

// ============================================================================
// Metrics
// ============================================================================

var (
	messagesReceived  int64
	messagesProcessed int64
	messagesFailed    int64
	currentWorkers    int64
)

// ============================================================================
// Payment Processor Simulation (same buffered channel pattern)
// ============================================================================
//
// The worker count controls how many payments process concurrently.
// This is the variable you tune in Phase 5:
//   1 worker  → 0.33 orders/sec
//   5 workers → 1.67 orders/sec
//   20 workers → 6.67 orders/sec
//   100 workers → 33.33 orders/sec
//
// Each worker holds a slot in the buffered channel for 3 seconds.
//

var paymentSem chan struct{}

func initPaymentSemaphore(workers int) {
	paymentSem = make(chan struct{}, workers)
	log.Printf("Payment semaphore initialized with %d worker slots", workers)
}

func simulatePaymentProcessing(order *Order) error {
	order.Status = "processing"

	// Acquire a worker slot — blocks if all workers are busy
	paymentSem <- struct{}{}
	defer func() { <-paymentSem }()

	// Simulate the 3-second payment verification
	time.Sleep(3 * time.Second)

	order.Status = "completed"
	return nil
}

// ============================================================================
// SQS Poller
// ============================================================================

var sqsClient *sqs.Client
var queueURL string

func initSQS() {
	queueURL = os.Getenv("SQS_QUEUE_URL")
	if queueURL == "" {
		log.Fatal("SQS_QUEUE_URL environment variable is required")
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

	sqsClient = sqs.NewFromConfig(cfg)
	log.Printf("SQS client initialized — queue: %s, region: %s", queueURL, region)
}

// pollSQS continuously pulls messages from the queue.
//
// Pattern from the assignment:
//   1. ReceiveMessage (waits up to 20s for messages via long polling, returns up to 10)
//   2. For each message, spawn a goroutine for processing
//   3. Repeat forever
//
// Long polling (WaitTimeSeconds=20) is critical:
//   - Without it: SQS returns immediately even if empty → burns API calls
//   - With it: SQS holds the connection open up to 20s waiting for messages
//     → reduces empty responses, lowers cost, faster message delivery
//
func pollSQS(ctx context.Context) {
	log.Println("Starting SQS poller...")

	waitTime := int32(20) // Long polling: wait up to 20 seconds for messages
	maxMsgs := int32(10)  // Receive up to 10 messages per poll

	for {
		select {
		case <-ctx.Done():
			log.Println("SQS poller shutting down")
			return
		default:
		}

		// Step 1: ReceiveMessage with long polling
		result, err := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &queueURL,
			MaxNumberOfMessages: maxMsgs,
			WaitTimeSeconds:     waitTime,
			// VisibilityTimeout defaults to 30s (set on the queue itself)
			// If processing takes >30s, the message becomes visible again
			// and another worker might pick it up → design for idempotency!
		})
		if err != nil {
			log.Printf("Error receiving messages: %v", err)
			time.Sleep(5 * time.Second) // Back off on error
			continue
		}

		if len(result.Messages) == 0 {
			continue // Long poll returned empty, loop back
		}

		log.Printf("Received %d messages from SQS", len(result.Messages))

		// Step 2: Spawn a goroutine per message
		for _, msg := range result.Messages {
			atomic.AddInt64(&messagesReceived, 1)
			go processMessage(ctx, msg)
		}
		// Step 3: Loop back to poll again
	}
}

// processMessage handles a single SQS message:
// 1. Parse the SNS envelope
// 2. Extract and deserialize the Order
// 3. Run payment processing (3s delay, limited by semaphore)
// 4. Delete the message from SQS on success
func processMessage(ctx context.Context, msg sqstypes.Message) {
	atomic.AddInt64(&currentWorkers, 1)
	defer atomic.AddInt64(&currentWorkers, -1)

	// SNS wraps the order JSON in an envelope: {"Message": "<escaped JSON>"}
	var envelope SNSEnvelope
	if err := json.Unmarshal([]byte(*msg.Body), &envelope); err != nil {
		log.Printf("Failed to parse SNS envelope: %v", err)
		atomic.AddInt64(&messagesFailed, 1)
		return
	}

	// Extract the actual order from the SNS Message field
	var order Order
	if err := json.Unmarshal([]byte(envelope.Message), &order); err != nil {
		log.Printf("Failed to parse order from SNS message: %v", err)
		atomic.AddInt64(&messagesFailed, 1)
		return
	}

	log.Printf("[PROCESSOR] Processing order %s for customer %d", order.OrderID, order.CustomerID)

	// Run payment processing — this blocks on the semaphore + 3s delay
	if err := simulatePaymentProcessing(&order); err != nil {
		log.Printf("[PROCESSOR] Payment failed for order %s: %v", order.OrderID, err)
		atomic.AddInt64(&messagesFailed, 1)
		// Don't delete the message — it will become visible again after
		// the visibility timeout (30s) for retry
		return
	}

	// Payment succeeded — delete the message from SQS
	_, err := sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      &queueURL,
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.Printf("[PROCESSOR] Failed to delete message for order %s: %v", order.OrderID, err)
		// Message will be reprocessed after visibility timeout
		// Payment processing should be idempotent!
	}

	atomic.AddInt64(&messagesProcessed, 1)
	log.Printf("[PROCESSOR] Order %s completed successfully", order.OrderID)
}

// ============================================================================
// Health & Metrics HTTP Server (runs alongside the poller)
// ============================================================================

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "order-processor",
	})
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages_received":  atomic.LoadInt64(&messagesReceived),
		"messages_processed": atomic.LoadInt64(&messagesProcessed),
		"messages_failed":    atomic.LoadInt64(&messagesFailed),
		"active_workers":     atomic.LoadInt64(&currentWorkers),
		"max_workers":        cap(paymentSem),
	})
}

// ============================================================================
// Main
// ============================================================================

func main() {
	// Configure number of worker goroutines from env var
	// Phase 3: WORKER_COUNT=1
	// Phase 5: WORKER_COUNT=5, 20, 100
	workerCount := 1
	if wc := os.Getenv("WORKER_COUNT"); wc != "" {
		if n, err := strconv.Atoi(wc); err == nil && n > 0 {
			workerCount = n
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	fmt.Printf("=== Order Processor ===\n")
	fmt.Printf("Worker goroutines: %d\n", workerCount)
	fmt.Printf("Max throughput: %.2f orders/sec\n", float64(workerCount)/3.0)
	fmt.Printf("========================\n")

	// Initialize
	initSQS()
	initPaymentSemaphore(workerCount)

	// Start the SQS poller in the background
	ctx := context.Background()
	go pollSQS(ctx)

	// Start HTTP server for health checks and metrics
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)

	log.Printf("Order Processor HTTP server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
