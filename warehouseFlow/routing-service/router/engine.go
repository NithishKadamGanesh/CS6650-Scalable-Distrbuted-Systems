package router

// Routing engine — the heart of WarehouseFlow.
// For each order: ping warehouse → check inventory → claim picker →
// atomically decrement inventory → write audit log. First warehouse
// that satisfies all checks wins. If none, order is rejected.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nithishkadam/warehouseflow/routing-service/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ─── Prometheus metrics ──────────────────────────────────────────────────────
var (
	ordersRouted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_orders_routed_total",
		Help: "Total orders successfully routed",
	})
	ordersRejected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_orders_rejected_total",
		Help: "Total orders rejected (no inventory or pickers)",
	})
	routingErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_routing_errors_total",
		Help: "Total routing errors (unexpected failures)",
	})
	routingLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "warehouseflow_routing_latency_seconds",
		Help:    "End-to-end routing decision latency",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
	})
	warehouseRoutedOrders = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "warehouseflow_warehouse_routed_orders_total",
		Help: "Orders routed per warehouse",
	}, []string{"warehouse_id"})
)

// Router is the interface the Kafka consumer depends on. Both the base
// Engine and the strategy-specific engines (Optimistic, Pessimistic) implement it.
type Router interface {
	Route(ctx context.Context, rawMessage []byte) error
}

// OrderEvent mirrors the ingestion service payload.
type OrderEvent struct {
	OrderID    string    `json:"order_id"`
	CustomerID string    `json:"customer_id"`
	SKU        string    `json:"sku"`
	Quantity   int       `json:"quantity"`
	Region     string    `json:"region"`
	ReceivedAt time.Time `json:"received_at"`
}

// Engine is the base routing engine.
type Engine struct {
	warehouses *store.WarehouseRegistry
	pg         store.DecisionStore
}

// NewEngine creates a new routing engine.
func NewEngine(warehouses *store.WarehouseRegistry, pg store.DecisionStore) *Engine {
	return &Engine{warehouses: warehouses, pg: pg}
}

// Route processes a Kafka message, makes a routing decision, and persists
// the result to the audit log.
func (e *Engine) Route(ctx context.Context, rawMessage []byte) error {
	return e.routeWithStrategy(ctx, rawMessage, e.findWarehouseAndPicker)
}

// findWarehouseAndPicker is the default selection logic: iterate warehouses
// in region-preference order, pick the first that has inventory + available picker.
func (e *Engine) findWarehouseAndPicker(ctx context.Context, order OrderEvent) (string, string, error) {
	warehouses := e.warehouses.GetPrioritized(order.Region)

	for _, w := range warehouses {
		if err := w.Ping(ctx); err != nil {
			log.Printf("Warehouse %s unreachable, skipping: %v", w.ID, err)
			continue
		}

		available, err := w.GetInventory(ctx, order.SKU)
		if err != nil {
			log.Printf("Warehouse %s inventory check failed: %v", w.ID, err)
			continue
		}
		if available < int64(order.Quantity) {
			continue
		}

		pickerID, err := w.GetAvailablePicker(ctx)
		if err != nil {
			continue
		}

		// Atomic Lua decrement — prevents oversells even without client-side locking.
		if _, err := w.DecrementInventory(ctx, order.SKU, order.Quantity); err != nil {
			_ = w.ReleasePicker(ctx, pickerID)
			log.Printf("Warehouse %s decrement failed for SKU %s: %v", w.ID, order.SKU, err)
			continue
		}

		// Release picker after simulated pick duration (asynchronously).
		go func(warehouse *store.Warehouse, picker string) {
			time.Sleep(500 * time.Millisecond)
			_ = warehouse.ReleasePicker(context.Background(), picker)
		}(w, pickerID)

		return w.ID, pickerID, nil
	}

	return "", "", fmt.Errorf("no warehouse available with inventory for SKU %s qty %d",
		order.SKU, order.Quantity)
}

// warehouseSelectorFn allows strategy engines to inject their own selection logic
// while reusing all the surrounding boilerplate (unmarshalling, timing, audit log).
type warehouseSelectorFn func(ctx context.Context, order OrderEvent) (warehouseID, pickerID string, err error)

// routeWithStrategy is the shared routing pipeline used by Engine,
// OptimisticEngine, and PessimisticEngine.
func (e *Engine) routeWithStrategy(
	ctx context.Context,
	rawMessage []byte,
	selector warehouseSelectorFn,
) error {
	start := time.Now()

	var order OrderEvent
	if err := json.Unmarshal(rawMessage, &order); err != nil {
		routingErrors.Inc()
		return fmt.Errorf("unmarshal error: %w", err)
	}

	decision := store.RoutingDecision{
		OrderID:    order.OrderID,
		CustomerID: order.CustomerID,
		SKU:        order.SKU,
		Quantity:   order.Quantity,
		RoutedAt:   time.Now().UTC(),
	}

	warehouseID, pickerID, err := selector(ctx, order)
	latencyMs := time.Since(start).Milliseconds()
	decision.LatencyMs = latencyMs

	if err != nil {
		decision.Status = "rejected"
		decision.FailReason = err.Error()
		ordersRejected.Inc()
		log.Printf("Order %s rejected: %v", order.OrderID, err)
	} else {
		decision.WarehouseID = warehouseID
		decision.PickerID = pickerID
		decision.Status = "routed"
		ordersRouted.Inc()
		warehouseRoutedOrders.WithLabelValues(warehouseID).Inc()
		routingLatency.Observe(time.Since(start).Seconds())
		log.Printf("Order %s → warehouse=%s picker=%s latency=%dms",
			order.OrderID, warehouseID, pickerID, latencyMs)
	}

	if dbErr := e.pg.InsertDecision(ctx, decision); dbErr != nil {
		log.Printf("Failed to persist routing decision for order %s: %v", order.OrderID, dbErr)
	}

	return nil
}
