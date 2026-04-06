package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// ============================================================================
// Domain Types (same as Part II)
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

// ============================================================================
// Lambda Handler
//
// Flow: SNS Topic -> Lambda (this function)
//
// Unlike Part II where SNS -> SQS -> ECS Worker, Lambda is triggered
// directly by SNS. AWS manages:
//   - Scaling (spins up concurrent Lambda instances automatically)
//   - Retries (SNS retries twice on failure, then discards)
//   - Infrastructure (no ECS tasks, no SQS queue to manage)
//
// Each Lambda invocation receives one SNS event which may contain
// multiple records (though SNS typically sends one per invocation).
// ============================================================================

func handler(ctx context.Context, snsEvent events.SNSEvent) error {
	for _, record := range snsEvent.Records {
		snsRecord := record.SNS

		log.Printf("[LAMBDA] Received message ID: %s", snsRecord.MessageID)

		// Parse the order from the SNS message body
		var order Order
		if err := json.Unmarshal([]byte(snsRecord.Message), &order); err != nil {
			log.Printf("[LAMBDA] Failed to parse order: %v", err)
			return fmt.Errorf("failed to parse order: %w", err)
		}

		log.Printf("[LAMBDA] Processing order %s for customer %d",
			order.OrderID, order.CustomerID)

		// Simulate the same 3-second payment processing as Part II
		order.Status = "processing"
		time.Sleep(3 * time.Second)
		order.Status = "completed"

		log.Printf("[LAMBDA] Order %s completed successfully", order.OrderID)
	}

	return nil
}

// ============================================================================
// Main
// ============================================================================

func main() {
	lambda.Start(handler)
}
