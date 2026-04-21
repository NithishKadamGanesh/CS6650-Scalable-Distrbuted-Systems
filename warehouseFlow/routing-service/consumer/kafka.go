package consumer

// Kafka consumer for the routing service. Reads from "orders" topic
// with at-least-once delivery (manual commit after processing).
// Combined with ON CONFLICT DO NOTHING in Postgres, reprocessed messages
// are idempotent.

import (
	"context"
	"log"

	"github.com/nithishkadam/warehouseflow/routing-service/router"
	"github.com/segmentio/kafka-go"
)

// Consumer reads messages and dispatches them to a Router.
type Consumer struct {
	reader *kafka.Reader
	engine router.Router
}

// New creates a new Kafka consumer bound to the given topic + consumer group.
func New(brokerAddr, topic, groupID string, engine router.Router) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{brokerAddr},
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 0, // manual commit — at-least-once delivery
	})
	return &Consumer{reader: reader, engine: engine}
}

// Start consumes messages until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	defer c.reader.Close()

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("Kafka fetch error: %v", err)
			continue
		}

		if err := c.engine.Route(ctx, msg.Value); err != nil {
			log.Printf("Routing error at offset %d: %v", msg.Offset, err)
			// Still commit — persistence to Postgres happens in engine regardless.
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("Commit error: %v", err)
		}
	}
}
