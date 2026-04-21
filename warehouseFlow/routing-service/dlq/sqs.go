package dlq

// SQS Dead-Letter Queue integration for the routing service.
// Orders that exhaust all routing attempts land here rather than being
// silently dropped. Queue depth is a key metric in Experiment 2.

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	dlqSends = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_dlq_sends_total",
		Help: "Orders sent to dead-letter queue",
	})
	dlqSendErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_dlq_send_errors_total",
		Help: "DLQ send failures (double failure — logged locally)",
	})
)

// Client publishes failed orders to SQS.
type Client struct {
	sqs      *sqs.Client
	queueURL string
}

// FailedOrder is the payload sent to SQS.
type FailedOrder struct {
	OrderID    string `json:"order_id"`
	CustomerID string `json:"customer_id"`
	SKU        string `json:"sku"`
	Quantity   int    `json:"quantity"`
	Region     string `json:"region"`
	FailReason string `json:"fail_reason"`
	FailedAt   string `json:"failed_at"`
}

// NewClient creates a DLQ client bound to the given queue URL.
func NewClient(sqsClient *sqs.Client, queueURL string) *Client {
	return &Client{sqs: sqsClient, queueURL: queueURL}
}

// Send pushes a failed order to SQS. Best-effort — logs but doesn't error.
func (c *Client) Send(ctx context.Context, order FailedOrder) {
	if c.queueURL == "" {
		log.Printf("DLQ URL not configured, order %s not parked", order.OrderID)
		return
	}

	body, err := json.Marshal(order)
	if err != nil {
		log.Printf("DLQ marshal error for %s: %v", order.OrderID, err)
		dlqSendErrors.Inc()
		return
	}

	_, err = c.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(c.queueURL),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		log.Printf("DLQ send error for %s: %v", order.OrderID, err)
		dlqSendErrors.Inc()
		return
	}

	dlqSends.Inc()
	log.Printf("Order %s parked in DLQ: %s", order.OrderID, order.FailReason)
}
