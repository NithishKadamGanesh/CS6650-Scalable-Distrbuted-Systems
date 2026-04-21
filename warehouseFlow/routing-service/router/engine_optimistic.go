package router

// Optimistic concurrency strategy: read inventory, decide, then attempt
// atomic CAS decrement. On conflict (inventory changed between read and
// decrement), retry up to N times with exponential backoff + jitter.
//
// Higher throughput than pessimistic at low contention; degrades under
// extreme scarcity. Used in Experiment 3.

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/nithishkadam/warehouseflow/routing-service/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	optimisticRetries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_optimistic_retries_total",
		Help: "Total CAS retry attempts",
	})
	optimisticRetryDepth = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "warehouseflow_optimistic_retry_depth",
		Help:    "Retries per order before success or rejection",
		Buckets: []float64{0, 1, 2, 3, 4, 5},
	})
	optimisticConflicts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_optimistic_conflicts_total",
		Help: "Total CAS conflicts detected",
	})
)

const optimisticMaxRetries = 3

// OptimisticEngine wraps Engine with CAS retry logic.
type OptimisticEngine struct {
	*Engine
}

// NewOptimisticEngine creates an optimistic-concurrency routing engine.
func NewOptimisticEngine(warehouses *store.WarehouseRegistry, pg store.DecisionStore) *OptimisticEngine {
	return &OptimisticEngine{Engine: NewEngine(warehouses, pg)}
}

// Route delegates to the shared pipeline with optimistic selector.
func (e *OptimisticEngine) Route(ctx context.Context, rawMessage []byte) error {
	return e.Engine.routeWithStrategy(ctx, rawMessage, e.findWarehouseAndPickerOptimistic)
}

func (e *OptimisticEngine) findWarehouseAndPickerOptimistic(
	ctx context.Context,
	order OrderEvent,
) (string, string, error) {
	warehouses := e.warehouses.GetPrioritized(order.Region)

	for _, w := range warehouses {
		if err := w.Ping(ctx); err != nil {
			log.Printf("[optimistic] Warehouse %s unreachable: %v", w.ID, err)
			continue
		}

		for retries := 0; retries <= optimisticMaxRetries; retries++ {
			if retries > 0 {
				optimisticRetries.Inc()
				// Exponential backoff with ±50% jitter.
				base := time.Duration(retries*retries) * 2 * time.Millisecond
				jitter := time.Duration(rand.Int63n(int64(base) + 1))
				time.Sleep(base + jitter - base/2)
			}

			available, err := w.GetInventory(ctx, order.SKU)
			if err != nil || available < int64(order.Quantity) {
				break
			}

			pickerID, err := w.GetAvailablePicker(ctx)
			if err != nil {
				break
			}

			// Atomic CAS decrement via Lua script.
			if _, err := w.DecrementInventory(ctx, order.SKU, order.Quantity); err != nil {
				_ = w.ReleasePicker(ctx, pickerID)
				optimisticConflicts.Inc()
				log.Printf("[optimistic] CAS conflict on %s SKU %s (retry %d)",
					w.ID, order.SKU, retries+1)
				continue
			}

			optimisticRetryDepth.Observe(float64(retries))

			go func(warehouse *store.Warehouse, picker string) {
				time.Sleep(500 * time.Millisecond)
				_ = warehouse.ReleasePicker(context.Background(), picker)
			}(w, pickerID)

			return w.ID, pickerID, nil
		}
	}

	return "", "", fmt.Errorf("no warehouse available (optimistic) for SKU %s qty %d after retries",
		order.SKU, order.Quantity)
}
