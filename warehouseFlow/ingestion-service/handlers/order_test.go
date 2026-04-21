package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockProducer records the payload sent to Publish so tests can verify it.
type mockProducer struct {
	key     string
	payload []byte
	err     error
	called  bool
}

func (m *mockProducer) Publish(_ context.Context, key string, value []byte) error {
	m.called = true
	m.key = key
	m.payload = value
	return m.err
}

// Valid requests should return 202 and publish to Kafka.
func TestSubmitOrderQueuesValidRequest(t *testing.T) {
	producer := &mockProducer{}
	handler := NewOrderHandler(producer)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(`{
		"customer_id": "cust-123",
		"sku": "SKU-ALPHA",
		"quantity": 2,
		"region": "US-EAST"
	}`))
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	handler.SubmitOrder(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, res.Code)
	}
	if !producer.called {
		t.Fatal("expected producer to be called")
	}
	if producer.key == "" {
		t.Fatal("expected generated order ID to be used as Kafka key")
	}

	var event OrderEvent
	if err := json.Unmarshal(producer.payload, &event); err != nil {
		t.Fatalf("failed to unmarshal published event: %v", err)
	}
	if event.CustomerID != "cust-123" || event.SKU != "SKU-ALPHA" || event.Quantity != 2 || event.Region != "US-EAST" {
		t.Fatalf("unexpected event payload: %+v", event)
	}
	if event.OrderID == "" {
		t.Fatal("expected event to include an order ID")
	}
}

// Invalid payloads should return 400 without touching Kafka.
func TestSubmitOrderRejectsInvalidPayload(t *testing.T) {
	producer := &mockProducer{}
	handler := NewOrderHandler(producer)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(`{"customer_id":"","sku":"SKU-ALPHA","quantity":0}`))
	res := httptest.NewRecorder()

	handler.SubmitOrder(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
	if producer.called {
		t.Fatal("producer should not be called for invalid requests")
	}
}

// Kafka failures should bubble up as 500.
func TestSubmitOrderReturnsServerErrorWhenPublishFails(t *testing.T) {
	producer := &mockProducer{err: errors.New("kafka unavailable")}
	handler := NewOrderHandler(producer)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(`{
		"customer_id": "cust-123",
		"sku": "SKU-ALPHA",
		"quantity": 1
	}`))
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	handler.SubmitOrder(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, res.Code)
	}
	if !producer.called {
		t.Fatal("expected producer to be called")
	}
}
