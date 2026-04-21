package router

// Pessimistic concurrency strategy: acquire a distributed lock before
// reading inventory and decrementing. Guarantees zero oversells at the
// cost of serializing access to the inventory slot. Used in Experiment 3.

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nithishkadam/warehouseflow/routing-service/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	pessimisticLockAcquired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_pessimistic_lock_acquired_total",
		Help: "Total distributed locks successfully acquired",
	})
	pessimisticLockContended = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warehouseflow_pessimistic_lock_contended_total",
		Help: "Total lock acquisition attempts that had to wait",
	})
	pessimisticLockLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "warehouseflow_pessimistic_lock_wait_seconds",
		Help:    "Time spent waiting to acquire inventory lock",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5},
	})
)

const (
	lockTTL        = 2 * time.Second
	lockRetryDelay = 5 * time.Millisecond
	lockMaxRetries = 50
)

// PessimisticEngine wraps Engine with distributed-lock logic.
type PessimisticEngine struct {
	*Engine
}

// NewPessimisticEngine creates a pessimistic-lock routing engine.
func NewPessimisticEngine(warehouses *store.WarehouseRegistry, pg store.DecisionStore) *PessimisticEngine {
	return &PessimisticEngine{Engine: NewEngine(warehouses, pg)}
}

// Route delegates to the shared pipeline with pessimistic selector.
func (e *PessimisticEngine) Route(ctx context.Context, rawMessage []byte) error {
	return e.Engine.routeWithStrategy(ctx, rawMessage, e.findWarehouseAndPickerPessimistic)
}

func (e *PessimisticEngine) findWarehouseAndPickerPessimistic(
	ctx context.Context,
	order OrderEvent,
) (string, string, error) {
	warehouses := e.warehouses.GetPrioritized(order.Region)

	for _, w := range warehouses {
		if err := w.Ping(ctx); err != nil {
			continue
		}

		lockKey := fmt.Sprintf("lock:inventory:%s:%s", w.ID, order.SKU)
		lockStart := time.Now()

		// Spin-wait to acquire the distributed lock.
		acquired := false
		for i := 0; i < lockMaxRetries; i++ {
			ok, err := w.AcquireLock(ctx, lockKey, lockTTL)
			if err != nil {
				log.Printf("[pessimistic] Lock error on %s: %v", w.ID, err)
				break
			}
			if ok {
				acquired = true
				pessimisticLockAcquired.Inc()
				break
			}
			pessimisticLockContended.Inc()
			time.Sleep(lockRetryDelay)
		}

		if !acquired {
			log.Printf("[pessimistic] Could not acquire lock on %s for SKU %s", w.ID, order.SKU)
			continue
		}

		pessimisticLockLatency.Observe(time.Since(lockStart).Seconds())

		available, err := w.GetInventory(ctx, order.SKU)
		if err != nil || available < int64(order.Quantity) {
			_ = w.ReleaseLock(ctx, lockKey)
			continue
		}

		pickerID, err := w.GetAvailablePicker(ctx)
		if err != nil {
			_ = w.ReleaseLock(ctx, lockKey)
			continue
		}

		if _, err := w.DecrementInventory(ctx, order.SKU, order.Quantity); err != nil {
			_ = w.ReleasePicker(ctx, pickerID)
			_ = w.ReleaseLock(ctx, lockKey)
			continue
		}

		_ = w.ReleaseLock(ctx, lockKey)

		go func(warehouse *store.Warehouse, picker string) {
			time.Sleep(500 * time.Millisecond)
			_ = warehouse.ReleasePicker(context.Background(), picker)
		}(w, pickerID)

		return w.ID, pickerID, nil
	}

	return "", "", fmt.Errorf("no warehouse available (pessimistic) for SKU %s qty %d",
		order.SKU, order.Quantity)
}
