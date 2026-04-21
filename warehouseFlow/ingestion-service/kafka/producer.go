package kafka

// Kafka producer wrapper over segmentio/kafka-go.
// Uses hash-based partitioning on order_id so related messages go
// to the same partition (enables ordered processing per order).

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// Producer wraps a kafka-go writer.
type Producer struct {
	writer *kafka.Writer
	topic  string
}

// NewProducer creates a Kafka producer and ensures the topic exists
// with 3 partitions (tuned for 4-task ECS routing service).
func NewProducer(brokerAddr, topic string) (*Producer, error) {
	if err := ensureTopic(brokerAddr, topic, 3); err != nil {
		log.Printf("Warning: could not pre-create topic %s: %v", topic, err)
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokerAddr),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		BatchSize:    100,
		BatchTimeout: 5 * time.Millisecond,
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}

	return &Producer{writer: writer, topic: topic}, nil
}

// Publish sends a message to Kafka with the given key and value.
func (p *Producer) Publish(ctx context.Context, key string, value []byte) error {
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: value,
		Time:  time.Now(),
	})
}

// Close shuts down the Kafka writer.
func (p *Producer) Close() {
	if err := p.writer.Close(); err != nil {
		log.Printf("Error closing Kafka writer: %v", err)
	}
}

// ensureTopic creates the topic with the given partition count if it doesn't exist.
func ensureTopic(brokerAddr, topic string, partitions int) error {
	conn, err := kafka.Dial("tcp", brokerAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}

	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	return controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitions,
		ReplicationFactor: 1,
	})
}
